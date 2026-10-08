package main

import (
	"net/http"
	"testing"
)

func TestSecurityHeadersOnRoutes(t *testing.T) {
	cases := []struct {
		path   string
		status int
	}{
		{"/health", http.StatusOK},
		{"/notes", http.StatusOK},
		{"/metrics", http.StatusOK},
		{"/notes/999", http.StatusNotFound},
		{"/does-not-exist", http.StatusNotFound},
	}

	want := map[string]string{
		"X-Content-Type-Options":       "nosniff",
		"Content-Security-Policy":      "default-src 'none'; frame-ancestors 'none'",
		"X-Frame-Options":              "DENY",
		"Cache-Control":                "no-store",
		"Cross-Origin-Resource-Policy": "same-origin",
		"Cross-Origin-Embedder-Policy": "require-corp",
		"Cross-Origin-Opener-Policy":   "same-origin",
	}

	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			srv := newTestServer(t)
			rec := do(t, srv, http.MethodGet, tc.path, nil)

			if rec.Code != tc.status {
				t.Fatalf("status: got %d, want %d", rec.Code, tc.status)
			}
			for h, v := range want {
				if got := rec.Header().Get(h); got != v {
					t.Errorf("%s: got %q, want %q", h, got, v)
				}
			}
		})
	}
}
