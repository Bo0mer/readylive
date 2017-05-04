// Package readylive wraps the standard library HTTP server with readiness and
// liveness endpoints.
package readylive

// TODO(ivan): Provide usage example.

import (
	"context"
	"net/http"
	"sync"
	"time"
)

type readinessHandler struct {
	mu    sync.Mutex
	ready bool
}

func (h *readinessHandler) SetReady(ready bool) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.ready = ready
}

func (h *readinessHandler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.ready {
		w.WriteHeader(http.StatusOK)
		return
	}

	w.WriteHeader(http.StatusServiceUnavailable)
}

// ServerOption configures a server instance.
type ServerOption func(s *Server)

// WithReadyPath sets the readiness check path.
func WithReadyPath(path string) ServerOption {
	return func(s *Server) {
		s.readyPath = path
	}
}

// WithReadyHandler sets the http.Handler for the readiness check.
func WithReadyHandler(h http.Handler) ServerOption {
	return func(s *Server) {
		s.ready = h
	}
}

// WithAlivePath sets the liveness check path.
func WithAlivePath(path string) ServerOption {
	return func(s *Server) {
		s.alivePath = path
	}
}

// WithAliveHandler sets the http.Handler for the liveness check.
func WithAliveHandler(h http.Handler) ServerOption {
	return func(s *Server) {
		s.alive = h
	}
}

// WaitBeforeShutdown sets the duration in which the server will report it is
// not ready before shutting down.
func WaitBeforeShutdown(d time.Duration) ServerOption {
	return func(s *Server) {
		s.shutdownWait = d
	}
}

// ShutdownTimeout sets the duration to wait for all ongoing requests to finish
// before shutting down. The shutdown timeout starts to count after the wait
// before shutdown duration passes.
func ShutdownTimeout(d time.Duration) ServerOption {
	return func(s *Server) {
		s.shutdownTimeout = d
	}
}

// Server is a HTTP server with additional readiness and liveness check
// endpoints.
type Server struct {
	srv       *http.Server
	readyPath string
	ready     http.Handler
	alivePath string
	alive     http.Handler

	// shutdownWait is the duration to keep srv running after shutdown is
	// called and readiness is reported false.
	shutdownWait time.Duration
	// shutdownTimeout is the duration to wait for ongoing requests to finish.
	shutdownTimeout time.Duration

	// errChan gets errors returned by srv.ListenAndServe.
	errChan chan error
}

// WrapServer attaches readiness and liveness handlers to srv.
// If no options are provided, the server uses default implementations for the
// readiness and liveness handlers, serving requests on '/ready' and '/health'
// respectively.
//
// The default duration for disabling the readiness check before shutting down
// is 15 seconds. Then another 5 seconds are given to all ongoing requests to
// finish before closing the server forcefully.
//
// Once wrapped, srv should not be modified directly.
func WrapServer(srv *http.Server, options ...ServerOption) *Server {
	s := &Server{
		srv:             srv,
		shutdownWait:    15 * time.Second,
		shutdownTimeout: 5 * time.Second,
		errChan:         make(chan error, 1),
	}

	for _, opt := range options {
		opt(s)
	}

	return s
}

// ListenAndServe starts the server in its own goroutine.
func (s *Server) ListenAndServe() {
	// Attach ready and alive handlers.
	if s.ready == nil {
		s.ready = &readinessHandler{ready: true}
	}
	if s.readyPath == "" {
		s.readyPath = "/ready"
	}
	if s.alive == nil {
		s.alive = &readinessHandler{ready: true}
	}
	if s.alivePath == "" {
		s.alivePath = "/health"
	}

	mux := http.NewServeMux()
	mux.Handle(s.readyPath, s.ready)
	mux.Handle(s.alivePath, s.alive)
	mux.Handle("/", s.srv.Handler)
	s.srv.Handler = mux

	go func() {
		s.errChan <- s.srv.ListenAndServe()
	}()
}

// Shutdown shutdowns the server gracefully.
// It returns any error returned by the underlying http.Server.
func (s *Server) Shutdown(ctx context.Context) error {
	// TODO(ivan): Document this specific behavior.
	if r, ok := s.ready.(readiness); ok {
		r.SetReady(false)
	}

	wait := time.After(s.shutdownWait)
	select {
	case err := <-s.errChan:
		// The server did not start.
		return err
	case <-wait:
		break
	case <-ctx.Done():
		break
	}

	ctx, cancel := context.WithTimeout(ctx, s.shutdownTimeout)
	defer cancel()
	if err := s.srv.Shutdown(ctx); err != nil {
		if err == context.DeadlineExceeded {
			return s.srv.Close()
		}
		return err
	}
	return nil
}

type readiness interface {
	SetReady(bool)
}
