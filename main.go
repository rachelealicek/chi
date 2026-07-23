package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// LogEntry represents a single HTTP request log entry.
type LogEntry struct {
	Method  string
	Path    string
	Status  int
	Bytes   int
	Elapsed time.Duration
	Err     error
}

// Logger is the interface for writing log entries.
type Logger interface {
	Log(entry LogEntry)
}

// DefaultLogger writes log entries to an io.Writer.
type DefaultLogger struct {
	mu sync.Mutex
	w  io.Writer
}

func NewDefaultLogger(w io.Writer) *DefaultLogger {
	return &DefaultLogger{w: w}
}

func (l *DefaultLogger) Log(entry LogEntry) {
	l.mu.Lock()
	defer l.mu.Unlock()
	status := entry.Status
	if entry.Err != nil {
		if entry.Err == context.Canceled || entry.Err == context.DeadlineExceeded {
			if status == 0 {
				status = 499
			}
		}
	}
	if status == 0 {
		status = 200
	}
	fmt.Fprintf(l.w, "method=%s path=%s status=%d bytes=%d elapsed=%s", entry.Method, entry.Path, status, entry.Bytes, entry.Elapsed)
	if entry.Err != nil {
		fmt.Fprintf(l.w, " err=%s", entry.Err)
	}
	fmt.Fprintln(l.w)
}

// responseLogger wraps http.ResponseWriter and ensures exactly one log entry
// is written per request, even if the context is canceled mid-handler.
type responseLogger struct {
	w        http.ResponseWriter
	status   int
	bytes    int
	logged   int32 // atomic flag: 0 = not logged, 1 = logged
	logger   Logger
	method   string
	path     string
	start    time.Time
	cancelMu sync.Mutex
}

// newResponseLogger creates a responseLogger that guards against duplicate logs.
func newResponseLogger(w http.ResponseWriter, logger Logger, method, path string) *responseLogger {
	return &responseLogger{
		w:      w,
		logger: logger,
		method: method,
		path:   path,
		start:  time.Now(),
	}
}

// WriteLog writes the log entry exactly once using an atomic CAS guard.
// This is thread-safe — if called from both the deferred handler unwind
// and a context-cancellation watcher, only the first call wins.
func (rl *responseLogger) WriteLog(err error) {
	if !atomic.CompareAndSwapInt32(&rl.logged, 0, 1) {
		return // already logged — prevent duplicate
	}
	rl.logger.Log(LogEntry{
		Method:  rl.method,
		Path:    rl.path,
		Status:  rl.status,
		Bytes:   rl.bytes,
		Elapsed: time.Since(rl.start),
		Err:     err,
	})
}

func (rl *responseLogger) Header() http.Header {
	return rl.w.Header()
}

func (rl *responseLogger) Write(b []byte) (int, error) {
	n, err := rl.w.Write(b)
	rl.bytes += n
	return n, err
}

func (rl *responseLogger) WriteHeader(code int) {
	rl.status = code
	rl.w.WriteHeader(code)
}

// LoggerMiddleware wraps the handler with logging that guarantees exactly
// one log entry per request, even when the request context is canceled.
func LoggerMiddleware(logger Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rl := newResponseLogger(w, logger, r.Method, r.URL.Path)

		// Watch for context cancellation in a goroutine — if the client
		// disconnects or server shuts down mid-handler, log the cancellation.
		go func() {
			<-r.Context().Done()
			rl.WriteLog(r.Context().Err())
		}()

		// Defer the normal completion log. WriteLog's atomic CAS guard
		// ensures only one entry is written — whichever fires first wins.
		defer func() {
			// If context was canceled, use that error; otherwise nil (normal completion)
			err := r.Context().Err()
			rl.WriteLog(err)
		}()

		next.ServeHTTP(rl, r)
	})
}
