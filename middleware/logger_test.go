package middleware

import (
	"bytes"
	"context"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestLogger_ContextCancellation_SingleLog(t *testing.T) {
	var buf bytes.Buffer
	logger := log.New(&buf, "", 0)

	handler := RequestLogger(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
		time.Sleep(10 * time.Millisecond)
	}))

	req := httptest.NewRequest("GET", "/test", nil)
	ctx, cancel := context.WithCancel(req.Context())
	req = req.WithContext(ctx)

	cancel() // Cancel context immediately to simulate context cancellation

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	// Ensure log buffer contains exactly one entry
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 1 || lines[0] == "" {
		t.Fatalf("Expected exactly 1 log line, got %d lines: %v", len(lines), lines)
	}
}
