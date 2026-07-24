package main

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// Test 1: Exactly one log entry on context cancellation
func TestLogger_ContextCancellation_SingleLog(t *testing.T) {
	var mu sync.Mutex
	var logCount int

	logger := &countingLogger{
		onLog: func() {
			mu.Lock()
			logCount++
			mu.Unlock()
		},
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Block until context is canceled
		<-r.Context().Done()
	})

	mw := LoggerMiddleware(logger, handler)

	// Create a request with a cancelable context
	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest("GET", "/cancel", nil).WithContext(ctx)
	rec := httptest.NewRecorder()

	// Cancel the request context after a short delay
	go func() {
		time.Sleep(5 * time.Millisecond)
		cancel()
	}()

	mw.ServeHTTP(rec, req)

	// WriteLog runs synchronously via defer before ServeHTTP returns,
	// so logCount is already updated — no sleep needed.
	mu.Lock()
	if logCount != 1 {
		t.Errorf("expected exactly 1 log entry, got %d", logCount)
	}
	mu.Unlock()
}

// Test 2: Exactly one log entry on normal request completion
func TestLogger_NormalCompletion_SingleLog(t *testing.T) {
	var mu sync.Mutex
	var logCount int
	var buf bytes.Buffer

	logger := &countingLogger{
		w: &buf,
		onLog: func() {
			mu.Lock()
			logCount++
			mu.Unlock()
		},
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("hello"))
	})

	mw := LoggerMiddleware(logger, handler)

	req := httptest.NewRequest("GET", "/normal", nil)
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)

	mu.Lock()
	if logCount != 1 {
		t.Errorf("expected exactly 1 log entry, got %d", logCount)
	}
	mu.Unlock()
}

// Test 3: Canceled request logs 499 status
func TestLogger_ContextCanceled_LogsError(t *testing.T) {
	var buf bytes.Buffer
	logger := NewDefaultLogger(&buf)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	})

	mw := LoggerMiddleware(logger, handler)

	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest("GET", "/cancel", nil).WithContext(ctx)
	rec := httptest.NewRecorder()

	go func() {
		time.Sleep(5 * time.Millisecond)
		cancel()
	}()

	mw.ServeHTTP(rec, req)

	output := buf.String()
	if !strings.Contains(output, "status=499") {
		t.Errorf("expected status=499 in log for canceled request, got: %s", output)
	}
}

// Test 4: Normal request logs 200 status
func TestLogger_NormalCompletion_Logs200(t *testing.T) {
	var buf bytes.Buffer
	logger := NewDefaultLogger(&buf)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("ok"))
	})

	mw := LoggerMiddleware(logger, handler)

	req := httptest.NewRequest("GET", "/ok", nil)
	rec := httptest.NewRecorder()
	mw.ServeHTTP(rec, req)

	output := buf.String()
	if !strings.Contains(output, "status=200") {
		t.Errorf("expected status=200 in log, got: %s", output)
	}
}

// Test 5: Thread safety — concurrent WriteLog calls don't produce duplicates
func TestResponseLogger_WriteLog_ThreadSafe(t *testing.T) {
	var mu sync.Mutex
	var logCount int

	logger := &countingLogger{
		onLog: func() {
			mu.Lock()
			logCount++
			mu.Unlock()
		},
	}

	rl := newResponseLogger(nil, logger, "GET", "/test")

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rl.WriteLog(nil)
		}()
	}
	wg.Wait()

	mu.Lock()
	if logCount != 1 {
		t.Errorf("expected exactly 1 log entry from concurrent calls, got %d", logCount)
	}
	mu.Unlock()
}

// Test 6: Multiple requests each produce exactly one log
func TestLogger_MultipleRequests_EachOneLog(t *testing.T) {
	var mu sync.Mutex
	var logCount int

	logger := &countingLogger{
		onLog: func() {
			mu.Lock()
			logCount++
			mu.Unlock()
		},
	}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	})

	mw := LoggerMiddleware(logger, handler)

	for i := 0; i < 5; i++ {
		req := httptest.NewRequest("GET", "/test", nil)
		rec := httptest.NewRecorder()
		mw.ServeHTTP(rec, req)
	}

	mu.Lock()
	if logCount != 5 {
		t.Errorf("expected 5 log entries for 5 requests, got %d", logCount)
	}
	mu.Unlock()
}

// countingLogger wraps DefaultLogger and counts Log calls.
type countingLogger struct {
	w      *bytes.Buffer
	onLog  func()
	logger *DefaultLogger
}

func (c *countingLogger) Log(entry LogEntry) {
	c.onLog()
	if c.w != nil {
		if c.logger == nil {
			c.logger = NewDefaultLogger(c.w)
		}
		c.logger.Log(entry)
	}
}
