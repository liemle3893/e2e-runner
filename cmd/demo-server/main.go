// Command demo-server is the system under test for Tryve's own end-to-end suite.
//
// It exposes a small REST API backed by PostgreSQL, Redis, MongoDB, and Azure
// Event Hubs — one endpoint group per adapter — so that `tests/e2e/adapters/`
// can exercise every adapter against a real service rather than a mock.
//
// It is a test fixture, not part of the Tryve product. It is built only when
// explicitly named (`make demo-server`); the release build covers ./cmd/tryve
// alone. It reuses the drivers Tryve already depends on, so it adds no modules.
//
// Bring up its dependencies with `docker compose up -d`, then run it and point
// the suite at it:
//
//	make demo-server && ./bin/demo-server
//	tryve run --env demo
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// defaultPort is the port the server listens on when PORT is unset. It matches
// the `demo` environment's baseUrl in e2e.config.yaml.
const defaultPort = "3000"

// shutdownGrace bounds how long in-flight requests have to finish once a
// termination signal arrives.
const shutdownGrace = 10 * time.Second

func main() {
	log.SetFlags(0)

	services := newServices()

	// Every service connects lazily and independently: the server starts even
	// when some backends are unavailable, so a suite exercising only the HTTP
	// and Redis paths is not blocked by a missing Event Hubs emulator. /health
	// reports what is actually reachable.
	initCtx, cancelInit := context.WithTimeout(context.Background(), 15*time.Second)
	services.init(initCtx)
	cancelInit()

	srv := &http.Server{
		Addr:              ":" + port(),
		Handler:           withRequestLog(newRouter(services)),
		ReadHeaderTimeout: 10 * time.Second,
	}

	// Serve in the background so the main goroutine can wait for a signal.
	serveErr := make(chan error, 1)
	go func() {
		log.Printf("demo-server listening on http://localhost:%s", port())
		logRoutes()
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	select {
	case err := <-serveErr:
		log.Fatalf("server failed: %v", err)
	case <-ctx.Done():
		log.Println("\nshutting down…")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownGrace)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("http shutdown: %v", err)
	}
	services.close(shutdownCtx)
	log.Println("all connections closed")
}

// port returns the listen port, honouring the PORT environment variable.
func port() string {
	if p := os.Getenv("PORT"); p != "" {
		return p
	}
	return defaultPort
}

// newRouter wires every route. Patterns use the method-aware matching that
// net/http gained in Go 1.22, so no third-party router is needed.
func newRouter(s *services) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("GET /health/{service}", s.handleServiceHealth)

	mux.HandleFunc("POST /users", s.handleCreateUser)
	mux.HandleFunc("GET /users", s.handleListUsers)
	mux.HandleFunc("GET /users/{id}", s.handleGetUser)
	mux.HandleFunc("PUT /users/{id}", s.handleUpdateUser)
	mux.HandleFunc("DELETE /users/{id}", s.handleDeleteUser)

	mux.HandleFunc("GET /cache/{key}", s.handleGetCache)
	mux.HandleFunc("PUT /cache/{key}", s.handleSetCache)
	mux.HandleFunc("DELETE /cache/{key}", s.handleDeleteCache)
	mux.HandleFunc("HEAD /cache/{key}", s.handleHeadCache)

	mux.HandleFunc("POST /documents", s.handleCreateDocument)
	mux.HandleFunc("GET /documents", s.handleListDocuments)
	mux.HandleFunc("DELETE /documents", s.handleDeleteDocuments)
	mux.HandleFunc("GET /documents/{id}", s.handleGetDocument)
	mux.HandleFunc("DELETE /documents/{id}", s.handleDeleteDocument)

	mux.HandleFunc("POST /events", s.handlePublishEvent)
	mux.HandleFunc("GET /events/consume", s.handleConsumeEvents)

	mux.HandleFunc("GET /{$}", handleRoot)

	return mux
}

// withRequestLog logs one line per request, matching the previous server's
// output so existing debugging habits still work.
func withRequestLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s %s", time.Now().UTC().Format(time.RFC3339), r.Method, r.URL.Path)
		next.ServeHTTP(w, r)
	})
}

// handleRoot serves the endpoint index.
func handleRoot(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"name":    "Demo Server",
		"version": "1.0.0",
		"endpoints": map[string]any{
			"health":    "/health",
			"users":     "/users",
			"cache":     "/cache/:key",
			"documents": "/documents",
			"events":    "/events",
		},
	})
}

// logRoutes prints the available endpoints at startup.
func logRoutes() {
	for _, line := range []string{
		"  GET    /health           - Health check",
		"  GET    /health/{service} - Per-service health",
		"  POST   /users            - Create user",
		"  GET    /users            - List users",
		"  GET    /users/{id}       - Get user",
		"  PUT    /users/{id}       - Update user",
		"  DELETE /users/{id}       - Delete user",
		"  GET    /cache/{key}      - Get cache value",
		"  PUT    /cache/{key}      - Set cache value",
		"  DELETE /cache/{key}      - Delete cache value",
		"  POST   /documents        - Create document",
		"  GET    /documents        - List documents",
		"  GET    /documents/{id}   - Get document",
		"  DELETE /documents/{id}   - Delete document",
		"  POST   /events           - Publish event",
		"  GET    /events/consume   - Consume events (test)",
	} {
		fmt.Println(line)
	}
}
