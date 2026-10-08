package main

import "net/http"

// securityHeadersList is applied to every response. QuickNotes is a JSON API,
// so the policies are as strict as possible: nothing may be embedded,
// framed, cached or loaded cross-origin.
var securityHeadersList = map[string]string{
	"X-Content-Type-Options":       "nosniff",
	"Content-Security-Policy":      "default-src 'none'; frame-ancestors 'none'",
	"X-Frame-Options":              "DENY",
	"Cache-Control":                "no-store",
	"Cross-Origin-Resource-Policy": "same-origin",
	"Cross-Origin-Embedder-Policy": "require-corp",
	"Cross-Origin-Opener-Policy":   "same-origin",
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for k, v := range securityHeadersList {
			w.Header().Set(k, v)
		}
		next.ServeHTTP(w, r)
	})
}
