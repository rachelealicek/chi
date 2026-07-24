package middleware

import (
	"context"
	"log"
	"net/http"
	"sync/atomic"
	"time"
)

type LogFormatter interface {
	WriteLog(status int, bytes int, elapsed time.Duration)
}

type ResponseLogger struct {
	http.ResponseWriter
	status  int
	bytes   int
	logged  int32
	Logger  *log.Logger
}

func (l *ResponseLogger) WriteHeader(code int) {
	l.status = code
	l.ResponseWriter.WriteHeader(code)
}

func (l *ResponseLogger) Write(b []byte) (int, error) {
	if l.status == 0 {
		l.status = http.StatusOK
	}
	n, err := l.ResponseWriter.Write(b)
	l.bytes += n
	return n, err
}

func (l *ResponseLogger) LogOnce(elapsed time.Duration) {
	if atomic.CompareAndSwapInt32(&l.logged, 0, 1) {
		if l.Logger != nil {
			l.Logger.Printf("[%d] %d bytes in %v", l.status, l.bytes, elapsed)
		}
	}
}

func RequestLogger(logger *log.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rl := &ResponseLogger{
				ResponseWriter: w,
				status:         http.StatusOK,
				Logger:         logger,
			}

			ctx, cancel := context.WithCancel(r.Context())
			defer cancel()

			defer func() {
				rl.LogOnce(time.Since(start))
			}()

			go func() {
				<-ctx.Done()
				if ctx.Err() == context.Canceled {
					rl.LogOnce(time.Since(start))
				}
			}()

			next.ServeHTTP(rl, r.WithContext(ctx))
		})
	}
}
