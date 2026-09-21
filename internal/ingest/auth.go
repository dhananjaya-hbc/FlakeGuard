package ingest

import (
	"crypto/subtle"
	"net/http"
)

const apiKeyHeader = "X-API-Key"

func RequireAPIKey(next http.Handler, apiKey string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		provided := r.Header.Get(apiKeyHeader)
		if provided == "" || subtle.ConstantTimeCompare([]byte(provided), []byte(apiKey)) != 1 {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
