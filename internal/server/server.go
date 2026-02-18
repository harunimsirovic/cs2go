package server

import (
	"context"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// Server wraps the HTTP server and all its dependencies.
type Server struct {
	httpServer *http.Server
	router     *chi.Mux
}

// New assembles the server with all routes wired up.
func New(addr string, handlers *Handlers, wsHub *WebSocketHub) *Server {
	r := chi.NewRouter()

	// ── Middleware ────────────────────────────────────────────────────────────
	r.Use(middleware.Logger)                    // structured request logging
	r.Use(middleware.Recoverer)                 // catch panics, return 500
	r.Use(middleware.RealIP)                    // honour X-Forwarded-For
	r.Use(middleware.Timeout(60 * time.Second)) // cancel context after 60s

	// CORS: allow all origins in dev (tighten for production)
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	})

	// ── Routes ────────────────────────────────────────────────────────────────

	// API routes
	r.Post("/upload", handlers.HandleUpload)
	r.Get("/jobs", handlers.HandleListJobs)
	r.Get("/jobs/{jobID}", handlers.HandleStatus)
	r.Get("/jobs/{jobID}/result", handlers.HandleResult)

	// WebSocket — clients connect here to stream progress in real time.
	// URL: ws://localhost:8080/ws?job_id=<uuid>
	r.Get("/ws", wsHub.HandleWebSocket)

	// Serve Angular build output with SPA fallback.
	r.Get("/*", spaFileServer(resolveFrontendDir()))

	return &Server{
		router: r,
		httpServer: &http.Server{
			Addr:         addr,
			Handler:      r,
			ReadTimeout:  10 * time.Second,
			WriteTimeout: 120 * time.Second, // long for large demo uploads
			IdleTimeout:  120 * time.Second,
		},
	}
}

func resolveFrontendDir() string {
	candidates := []string{
		filepath.Join("frontend", "dist", "cs2go", "browser"),
		filepath.Join("frontend", "dist", "cs2go"),
		filepath.Join("frontend"),
	}

	for _, dir := range candidates {
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			return dir
		}
	}

	return filepath.Join("frontend")
}

func spaFileServer(root string) http.HandlerFunc {
	fileServer := http.FileServer(http.Dir(root))
	return func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}

		target := filepath.Join(root, path)
		if info, err := os.Stat(target); err == nil && !info.IsDir() {
			fileServer.ServeHTTP(w, r)
			return
		}

		r.URL.Path = "/index.html"
		fileServer.ServeHTTP(w, r)
	}
}

// Start begins listening. Blocks until the server exits.
func (s *Server) Start() error {
	log.Printf("[server] listening on %s", s.httpServer.Addr)
	return s.httpServer.ListenAndServe()
}

// Shutdown gracefully drains connections.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}
