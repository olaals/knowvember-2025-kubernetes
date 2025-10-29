package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	rdb           *RedisClient
	k8s           *K8sClient
	maxUploadSize int64 = 10 << 20
)

type Post struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	CreatedAt int64  `json:"created_at"`
}

func main() {
	redisAddr := getenv("REDIS_ADDR", "redis:6379")
	port := getenv("PORT", "8050")
	if mb := os.Getenv("MAX_UPLOAD_MB"); mb != "" {
		if n, err := strconv.ParseInt(mb, 10, 64); err == nil && n > 0 {
			maxUploadSize = n << 20
		}
	}

	rdb = NewRedisClient(redisAddr)
	defer rdb.Close()

	if kc, err := NewInClusterK8sClient(); err != nil {
		log.Printf("[k8s] in-cluster client not available: %v", err)
	} else {
		k8s = kc
		log.Printf("[k8s] in-cluster client ready (ns=%s)", k8s.namespace)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", logRequests(withCORS(healthHandler)))
	mux.HandleFunc("/posts", logRequests(withCORS(postsHandler)))
	mux.HandleFunc("/posts/", logRequests(withCORS(postByIDHandler)))
	mux.HandleFunc("/images/", logRequests(withCORS(imagesHandler)))
	mux.HandleFunc("/jobs/effect", logRequests(withCORS(createEffectJobHandler)))
	mux.HandleFunc("/jobs/", logRequests(withCORS(jobStatusHandler)))

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("[startup] API listening on :%s (redis=%s, maxUploadMiB=%d, reconnect=on)", port, redisAddr, maxUploadSize>>20)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("[server] fatal error: %v", err)
	}
}

func healthHandler(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type createPostReq struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

func postsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		handleListPosts(w, r)
	case http.MethodPost:
		handleCreatePost(w, r)
	default:
		http.NotFound(w, r)
	}
}

func handleCreatePost(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req createPostReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpError(w, http.StatusBadRequest, "invalid json")
		return
	}

	req.Title = strings.TrimSpace(req.Title)
	req.Body = strings.TrimSpace(req.Body)
	if req.Title == "" || req.Body == "" {
		httpError(w, http.StatusBadRequest, "title and body required")
		return
	}

	id, err := generateUniqueID(ctx, 12)
	if err != nil {
		httpError(w, http.StatusInternalServerError, "id generation failed")
		return
	}
	ts := time.Now().Unix()

	key := "post:" + id
	if err := rdb.HSet(ctx, key, map[string]any{
		"title":      req.Title,
		"body":       req.Body,
		"created_at": strconv.FormatInt(ts, 10),
	}); err != nil {
		httpError(w, http.StatusInternalServerError, "hset failed")
		return
	}

	_ = rdb.ZAdd(ctx, "posts:all", float64(ts), id)

	log.Printf("[post] created id=%s", id)
	writeJSON(w, http.StatusCreated, Post{ID: id, Title: req.Title, Body: req.Body, CreatedAt: ts})
}

func handleListPosts(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	limit := 20
	if s := r.URL.Query().Get("limit"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			limit = n
		}
	}
	ids, err := rdb.ZRevRange(ctx, "posts:all", 0, int64(limit-1))
	if err != nil {
		httpError(w, http.StatusInternalServerError, "zrevrange failed")
		return
	}
	out := make([]Post, 0, len(ids))
	for _, idStr := range ids {
		m, err := rdb.HGetAll(ctx, "post:"+idStr)
		if err != nil || len(m) == 0 {
			continue
		}
		ts, _ := strconv.ParseInt(m["created_at"], 10, 64)
		out = append(out, Post{ID: idStr, Title: m["title"], Body: m["body"], CreatedAt: ts})
	}
	log.Printf("[post] listed %d items", len(out))
	writeJSON(w, http.StatusOK, out)
}

var idRe = regexp.MustCompile(`^[A-Za-z0-9]{12}$`)

func postByIDHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	ctx := r.Context()
	id := strings.TrimPrefix(r.URL.Path, "/posts/")
	if !idRe.MatchString(id) {
		httpError(w, http.StatusBadRequest, "invalid id")
		return
	}
	m, err := rdb.HGetAll(ctx, "post:"+id)
	if err != nil || len(m) == 0 {
		httpError(w, http.StatusNotFound, "post not found")
		return
	}
	ts, _ := strconv.ParseInt(m["created_at"], 10, 64)
	log.Printf("[post] get id=%s", id)
	writeJSON(w, http.StatusOK, Post{ID: id, Title: m["title"], Body: m["body"], CreatedAt: ts})
}

func imagesHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		if err := handleUploadImage(w, r); err != nil {
			log.Printf("[image] upload error: %v", err)
		}
	case http.MethodGet:
		handleGetImage(w, r)
	default:
		http.NotFound(w, r)
	}
}

func handleUploadImage(w http.ResponseWriter, r *http.Request) error {
	ctx := r.Context()
	id := strings.TrimPrefix(r.URL.Path, "/images/")
	if !idRe.MatchString(id) {
		httpError(w, http.StatusBadRequest, "invalid post id")
		return nil
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize+1<<20)
	ct := r.Header.Get("Content-Type")
	mediaType, _, _ := mime.ParseMediaType(ct)

	if strings.HasPrefix(mediaType, "multipart/") {
		if err := r.ParseMultipartForm(maxUploadSize); err != nil {
			httpError(w, http.StatusRequestEntityTooLarge, "file too large")
			return err
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			httpError(w, http.StatusBadRequest, "missing file")
			return err
		}
		defer file.Close()
		data, err := io.ReadAll(file)
		if err != nil {
			httpError(w, http.StatusBadRequest, "read failed")
			return err
		}
		ctype := header.Header.Get("Content-Type")
		if ctype == "" {
			ctype = sniffContentType(header.Filename, data)
		}
		if err := saveImage(ctx, id, data, ctype); err != nil {
			httpError(w, http.StatusInternalServerError, "save failed")
			return err
		}
		log.Printf("[image] uploaded id=%s bytes=%d ctype=%s", id, len(data), ctype)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "bytes": len(data)})
		return nil
	}

	data, err := io.ReadAll(io.LimitReader(r.Body, maxUploadSize+1))
	if err != nil {
		httpError(w, http.StatusBadRequest, "read failed")
		return err
	}
	if int64(len(data)) > maxUploadSize {
		httpError(w, http.StatusRequestEntityTooLarge, "file too large")
		return fmt.Errorf("file too large")
	}
	ctype := r.Header.Get("Content-Type")
	if ctype == "" {
		ctype = "application/octet-stream"
	}
	if err := saveImage(ctx, id, data, ctype); err != nil {
		httpError(w, http.StatusInternalServerError, "save failed")
		return err
	}
	log.Printf("[image] uploaded raw id=%s bytes=%d ctype=%s", id, len(data), ctype)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "bytes": len(data)})
	return nil
}

func handleGetImage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := strings.TrimPrefix(r.URL.Path, "/images/")
	if !idRe.MatchString(id) {
		httpError(w, http.StatusBadRequest, "invalid id")
		return
	}
	data, ctype, err := loadImage(ctx, id)
	if err != nil {
		httpError(w, http.StatusNotFound, "image not found")
		return
	}
	log.Printf("[image] serve id=%s bytes=%d", id, len(data))
	w.Header().Set("Content-Type", ctype)
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

type effectJobReq struct {
	PostID string `json:"post_id"`
	Effect string `json:"effect"`
}

type effectJobResp struct {
	JobName string `json:"job_name"`
}

func createEffectJobHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.NotFound(w, r)
		return
	}
	if k8s == nil {
		httpError(w, http.StatusServiceUnavailable, "kubernetes not available in this environment")
		return
	}

	var req effectJobReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpError(w, http.StatusBadRequest, "invalid json")
		return
	}
	req.PostID = strings.TrimSpace(req.PostID)
	req.Effect = strings.ToLower(strings.TrimSpace(req.Effect))
	if !idRe.MatchString(req.PostID) {
		httpError(w, http.StatusBadRequest, "invalid post_id")
		return
	}
	if req.Effect != "grayscale" && req.Effect != "invert" {
		httpError(w, http.StatusBadRequest, "effect must be grayscale or invert")
		return
	}

	jobImage := getenv("JOB_IMAGE", "image-job:0.1")
	redisAddr := getenv("REDIS_ADDR", "redis:6379")

	jobName, err := k8s.CreateImageEffectJob(r.Context(), jobImage, redisAddr, req.PostID, req.Effect)
	if err != nil {
		log.Printf("[k8s] create job error: %v", err)
		httpError(w, http.StatusInternalServerError, "failed to create job")
		return
	}
	writeJSON(w, http.StatusAccepted, effectJobResp{JobName: jobName})
}

func jobStatusHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.NotFound(w, r)
		return
	}
	if k8s == nil {
		httpError(w, http.StatusServiceUnavailable, "kubernetes not available in this environment")
		return
	}
	p := strings.TrimPrefix(r.URL.Path, "/jobs/")
	parts := strings.SplitN(p, "/", 2)
	if len(parts) != 2 || parts[1] != "status" || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	name := parts[0]
	st, reason, err := k8s.JobStatus(r.Context(), name)
	if err != nil {
		httpError(w, http.StatusInternalServerError, "status error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status": st,
		"reason": reason,
		"name":   name,
	})
}

func saveImage(ctx context.Context, id string, data []byte, ctype string) error {
	if err := rdb.Set(ctx, "image:"+id, data, 0); err != nil {
		return err
	}
	if ctype == "" {
		ctype = "application/octet-stream"
	}
	return rdb.Set(ctx, "image:ctype:"+id, []byte(ctype), 0)
}

func loadImage(ctx context.Context, id string) ([]byte, string, error) {
	data, err := rdb.GetBytes(ctx, "image:"+id)
	if err != nil {
		return nil, "", err
	}
	ctype, err := rdb.GetString(ctx, "image:ctype:"+id)
	if err != nil || ctype == "" {
		ctype = "application/octet-stream"
	}
	return data, ctype, nil
}

func generateUniqueID(ctx context.Context, n int) (string, error) {
	for range 5 {
		id, err := randomID(n)
		if err != nil {
			return "", err
		}
		exists, err := rdb.Exists(ctx, "post:"+id)
		if err != nil {
			return "", err
		}
		if exists == 0 {
			return id, nil
		}
	}
	return "", fmt.Errorf("could not generate unique id")
}
