package rest

import (
	"bufio"
	"errors"
	"net"
	"net/http"
	"sync/atomic"
)

// onceHeaderWriter suppresses duplicate WriteHeader calls. Applied globally
// (outside per-route chi Timeout): when the deadline fires, Timeout writes 504
// through this wrapper; handlers that finish later skip WriteOK/WriteError via
// headerWritten Unwrap (avoids "superfluous response.WriteHeader").
type onceHeaderWriter struct {
	http.ResponseWriter
	wrote atomic.Bool
}

func (w *onceHeaderWriter) WriteHeader(statusCode int) {
	if w.wrote.Swap(true) {
		return
	}
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *onceHeaderWriter) Write(b []byte) (int, error) {
	if !w.wrote.Load() {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(b)
}

func (w *onceHeaderWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func (w *onceHeaderWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (w *onceHeaderWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("http.Hijacker not supported")
	}
	return h.Hijack()
}

// SuppressDuplicateWriteHeader wraps the response writer so the first
// WriteHeader wins (timeout middleware or handler).
func SuppressDuplicateWriteHeader(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(&onceHeaderWriter{ResponseWriter: w}, r)
	})
}
