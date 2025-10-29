package main

import (
	"log"
	"net/http"
	"os"
	"time"
)

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		duration := time.Since(start)
		log.Printf("[req] %s %s from %s -> %d (%v)", r.Method, r.URL.Path, r.RemoteAddr, rec.status, duration)
	})
}

func getenv(k, v string) string {
	if s := os.Getenv(k); s != "" {
		log.Printf("[env] %s=%s", k, s)
		return s
	}
	log.Printf("[env] %s not set, using default %s", k, v)
	return v
}

func withStdHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		next.ServeHTTP(w, r)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (sr *statusRecorder) WriteHeader(code int) {
	sr.status = code
	sr.ResponseWriter.WriteHeader(code)
}
