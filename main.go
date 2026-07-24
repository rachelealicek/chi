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
	w      http.ResponseWriter
	status int
	bytes  int
	logged int32 // atomic flag: 0 = not logged, 1 = logged
	logger Logger
	method string
	path   string
	start  time.Time
	mu     sync.Mutex // protects status and bytes
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
	rl.mu.Lock()
	status := rl.status
	n := rl.bytes
	rl.mu.Unlock()
	// Default to 200 only for normal completion (no error). If the handler
	// wrote a body without calling WriteHeader, status is still 0 → 200.
	// When there's a context cancellation error, DefaultLogger.Log will
	// convert status 0 → 499, so we must NOT override it here.
	if status == 0 && err == nil {
		status = 200
	}
	rl.logger.Log(LogEntry{
		Method:  rl.method,
		Path:    rl.path,
		Status:  status,
		Bytes:   n,
		Elapsed: time.Since(rl.start),
		Err:     err,
	})
}

func (rl *responseLogger) Header() http.Header {
	return rl.w.Header()
}

func (rl *responseLogger) Write(b []byte) (int, error) {
	n, err := rl.w.Write(b)
	rl.mu.Lock()
	rl.bytes += n
	rl.mu.Unlock()
	return n, err
}

func (rl *responseLogger) WriteHeader(code int) {
	rl.mu.Lock()
	rl.status = code
	rl.mu.Unlock()
	rl.w.WriteHeader(code)
}

// LoggerMiddleware wraps the handler with logging that guarantees exactly
// one log entry per request, even when the request context is canceled.
func LoggerMiddleware(logger Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rl := newResponseLogger(w, logger, r.Method, r.URL.Path)

		// done signals the context-watcher goroutine that the handler has
		// finished, so it can exit without waiting for r.Context().Done().
		done := make(chan struct{})
		// goroutineDone signals that the watcher goroutine has fully exited
		// (or completed its WriteLog), so the deferred WriteLog can safely
		// proceed without racing on logger state.
		goroutineDone := make(chan struct{})

		// Watch for context cancellation in a goroutine — if the client
		// disconnects or server shuts down mid-handler, log the cancellation.
		go func() {
			defer close(goroutineDone)
			select {
			case <-r.Context().Done():
				rl.WriteLog(r.Context().Err())
			case <-done:
				// Handler completed normally; the deferred WriteLog handles logging.
			}
		}()

		// Defer the normal completion log. WriteLog's atomic CAS guard
		// ensures only one entry is written — whichever fires first wins.
		defer func() {
			close(done)
			<-goroutineDone // wait for watcher goroutine to finish
			rl.WriteLog(r.Context().Err())
		}()

		next.ServeHTTP(rl, r)
	})
}

func main() {}
