package main

import (
	"embed"
	"io/fs"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"
)

//go:embed index.html assets
var staticFS embed.FS

func main() {
	addr := ":" + getenv("PORT", "8045")
	upstream := getenv("UPSTREAM_API", "")

	u, err := url.Parse(upstream)
	if err != nil {
		log.Fatalf("invalid UPSTREAM_API: %v", err)
	}

	apiProxy := httputil.NewSingleHostReverseProxy(u)
	apiProxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, e error) {
		log.Printf("[proxy error] %s %s: %v", r.Method, r.URL.Path, e)
		http.Error(w, "upstream unavailable", http.StatusBadGateway)
	}
	origDirector := apiProxy.Director
	apiProxy.Director = func(r *http.Request) {
		origDirector(r)
		orig := r.URL.Path
		r.URL.Path = strings.TrimPrefix(r.URL.Path, "/api")
		r.Header.Set("X-Forwarded-Host", r.Host)
		r.Header.Set("X-Forwarded-Proto", "http")
		log.Printf("[proxy] %s -> %s%s", orig, u.String(), r.URL.Path)
	}

	mux := http.NewServeMux()

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		log.Print("[healthz] OK")
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	})

	mux.Handle("/api/", apiProxy)

	assets, err := fs.Sub(staticFS, ".")
	if err != nil {
		log.Fatalf("failed to create sub-FS: %v", err)
	}
	fileServer := http.FileServer(http.FS(assets))
	mux.Handle("/assets/", logRequests(fileServer))

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[http] %s %s", r.Method, r.URL.Path)
		if r.URL.Path != "/" && !strings.HasPrefix(r.URL.Path, "/assets/") {
			r.URL.Path = "/"
		}
		http.ServeFileFS(w, r, staticFS, "index.html")
	})

	s := &http.Server{
		Addr:              addr,
		Handler:           withStdHeaders(logRequests(mux)),
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Printf("[startup] frontend-bff listening on %s -> proxying /api to %s", addr, u.String())
	if err := s.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("[server] error: %v", err)
	}
}
