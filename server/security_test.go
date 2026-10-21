package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMissingBearerFailsClosed(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	r := httptest.NewRequest("POST", "http://127.0.0.1:8765/mcp", nil)
	w := httptest.NewRecorder()
	withAuthMiddleware(next, "").ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("missing configured bearer: got %d, want 401", w.Code)
	}
}

func TestEmptyOriginAllowlistDeniesBrowserOrigin(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	r := httptest.NewRequest("POST", "http://127.0.0.1:8765/mcp", nil)
	r.Header.Set("Origin", "https://untrusted.example")
	w := httptest.NewRecorder()
	withOriginValidation(next, nil).ServeHTTP(w, r)
	if w.Code != http.StatusForbidden {
		t.Fatalf("untrusted Origin: got %d, want 403", w.Code)
	}
}

func TestBearerAndOriginBoundaries(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	h := withOriginValidation(withAuthMiddleware(next, "test-only-token"), []string{"http://127.0.0.1:8765"})
	for _, tc := range []struct {
		name, token, origin string
		want                int
	}{
		{"no_auth", "", "", 401},
		{"bad_auth", "Bearer wrong", "", 401},
		{"evil_origin", "Bearer test-only-token", "https://untrusted.example", 403},
		{"null_origin", "Bearer test-only-token", "null", 403},
		{"native_client", "Bearer test-only-token", "", 200},
		{"allowed_origin", "Bearer test-only-token", "http://127.0.0.1:8765", 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("POST", "http://127.0.0.1:8765/mcp", nil)
			r.Header.Set("Authorization", tc.token)
			r.Header.Set("Origin", tc.origin)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tc.want {
				t.Fatalf("got %d, want %d", w.Code, tc.want)
			}
		})
	}
}

func TestHostBoundary(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	h := withHostValidation(next, "127.0.0.1:8765")
	for _, host := range []string{"untrusted.example:8765", "127.0.0.1:9999", "localhost:8765", ""} {
		r := httptest.NewRequest("POST", "http://127.0.0.1:8765/mcp", nil)
		r.Host = host
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 403 {
			t.Fatalf("Host %q got %d, want 403", host, w.Code)
		}
	}
	r := httptest.NewRequest("POST", "http://127.0.0.1:8765/mcp", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("loopback host rejected: %d", w.Code)
	}
	r.Header.Set("X-Forwarded-Host", "untrusted.example")
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatalf("forwarded Host accepted: %d", w.Code)
	}
}
