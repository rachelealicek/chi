package chi

import (
	"net/http"
	"sync"
)

type LoggerMiddleware struct {
	mu     sync.Mutex
	logged map[*http.Request]bool
}

func (l *LoggerMiddleware) Handle(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
		l.mu.Lock()
		defer l.mu.Unlock()
		if !l.logged[r] {
			l.logged[r] = true
		}
	})
}
