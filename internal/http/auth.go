package http

import (
	"net/http"
	"os"
	"strings"
)

// authMiddleware checks for valid Bearer token
func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip auth for the docs endpoint so agents can read how to authenticate
		if r.URL.Path == "/" && r.Method == "GET" {
			next.ServeHTTP(w, r)
			return
		}

		// Get the expected token from environment
		expectedToken := os.Getenv("BEADS_API_SECRET")
		if expectedToken == "" {
			// If no secret is configured, allow all requests (development mode)
			next.ServeHTTP(w, r)
			return
		}

		// Check Authorization header
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			s.writeAuthError(w, r, "Missing Authorization header")
			return
		}

		// Expect "Bearer <token>"
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			s.writeAuthError(w, r, "Invalid Authorization header format. Expected: Authorization: Bearer <token>")
			return
		}

		token := parts[1]
		if token != expectedToken {
			s.writeAuthError(w, r, "Invalid token")
			return
		}

		// Token is valid, continue
		next.ServeHTTP(w, r)
	})
}

// writeAuthError writes an authentication error response
func (s *Server) writeAuthError(w http.ResponseWriter, r *http.Request, message string) {
	if s.wantsJSON(r) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("WWW-Authenticate", `Bearer realm="beads-api"`)
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"` + message + `","success":false}`))
	} else {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("WWW-Authenticate", `Bearer realm="beads-api"`)
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte("Error: " + message + "\n\nSee GET / for API documentation and authentication requirements.\n"))
	}
}
