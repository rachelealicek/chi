package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
)

func LoggerMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
		if r.Context().Err() == context.Canceled {
			return
		}
		log.Printf("Handled %s %s", r.Method, r.URL.Path)
	})
}

func main() {
	fmt.Println("Hello, Bounty Hunter!")
}
