// Package server implements the OpenBISS HTTPS server and BISS-compatible
// HTTP handlers.
package server

import "net/http"

// corsMiddleware adds CORS headers to every response and handles OPTIONS
// preflight requests. BISS allows all origins (*) because the API is local-only
// (127.0.0.1) and any security is provided by the signedContents mechanism,
// not by origin restrictions.
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Accept")
		w.Header().Set("Access-Control-Max-Age", "86400")

		// Respond immediately to OPTIONS preflight without invoking the handler.
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}
