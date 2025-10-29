package main

import (
	"crypto/rand"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path"
	"strings"
	"time"
)

// getenv returns env var k or def if unset, and logs the decision.
func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		log.Printf("[env] %s=%s", k, v)
		return v
	}
	log.Printf("[env] %s not set, default=%s", k, def)
	return def
}

// withCORS adds permissive CORS headers for simple APIs.
func withCORS(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next(w, r)
	}
}

// statusRecorder captures status codes for request logging.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (sr *statusRecorder) WriteHeader(code int) {
	sr.status = code
	sr.ResponseWriter.WriteHeader(code)
}

// logRequests wraps handlers to log method, path, status, and duration.
func logRequests(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next(rec, r)
		log.Printf("[req] %s %s from %s -> %d (%v)", r.Method, r.URL.Path, r.RemoteAddr, rec.status, time.Since(start))
	}
}

// httpError writes a JSON error payload with the given status code.
func httpError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

// writeJSON serializes v to JSON with the desired status code.
func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// sniffContentType infers a reasonable content-type from filename or bytes.
func sniffContentType(filename string, data []byte) string {
	ext := strings.ToLower(path.Ext(filename))
	switch ext {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".svg":
		return "image/svg+xml"
	default:
		return http.DetectContentType(data)
	}
}

// base62 alphabet for randomID.
const base62 = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// randomID returns a crypto-random base62 string of length n.
func randomID(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	out := make([]byte, n)
	for i := range buf {
		out[i] = base62[int(buf[i])%len(base62)]
	}
	return string(out), nil
}