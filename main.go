package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"cs2-demo-analyzer/internal/parser"
	"cs2-demo-analyzer/internal/server"
	"cs2-demo-analyzer/internal/storage"
)

func main() {
	// ── Configuration ─────────────────────────────────────────────────────────
	addr := envOr("ADDR", ":8080")

	// Worker count: default to number of logical CPUs.
	// Each worker processes one demo at a time, fully concurrently.
	workerCount := runtime.NumCPU()
	log.Printf("[main] using %d parse workers", workerCount)

	// ── Dependency wiring ─────────────────────────────────────────────────────

	// 1. In-memory job store.
	store := storage.NewStore()

	// 2. WebSocket hub — created first so the pipeline can hold a reference.
	wsHub := server.NewWebSocketHub()

	// 3. Parse pipeline — starts workerCount goroutines immediately.
	//    Workers sit idle in a channel select until jobs arrive.
	pipeline := parser.NewPipeline(workerCount, store, wsHub)

	// 4. HTTP handlers (depend on store + pipeline).
	handlers := server.NewHandlers(store, pipeline)

	// 5. HTTP server (depend on handlers + ws hub).
	srv := server.New(addr, handlers, wsHub)

	// ── Start ─────────────────────────────────────────────────────────────────

	// Run the HTTP server in a goroutine so we can handle shutdown signals.
	go func() {
		if err := srv.Start(); err != nil {
			log.Printf("[main] server stopped: %v", err)
		}
	}()

	// ── Graceful shutdown ─────────────────────────────────────────────────────

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("[main] shutting down...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("[main] shutdown error: %v", err)
	}

	log.Println("[main] done")
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
