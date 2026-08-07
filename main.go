package main

import (
	"fmt"
	"net/http"
	"sync"
)

// LoggingMiddleware prevents duplicate log entries on request context cancellation.
// The bug: during graceful shutdown, requests get logged twice (once for cancel, once for completion).
// Fix: Track whether the request has already been logged using sync.Once.
type LoggingMiddleware struct {
	next http.Handler
}

func (m *LoggingMiddleware) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var logOnce sync.Once
	logFunc := func(msg string) {
		logOnce.Do(func() {
			fmt.Printf("[log] %s %s %s\n", r.Method, r.URL.Path, msg)
		})
	}

	r = r.WithContext(r.Context())
	wrapped := &responseWriter{ResponseWriter: w, logFunc: logFunc}
	m.next.ServeHTTP(wrapped, r)
	logFunc("completed")
}

type responseWriter struct {
	http.ResponseWriter
	logFunc func(string)
}

func main() {
	fmt.Println("Logging middleware: prevents duplicate entries via sync.Once")
}
