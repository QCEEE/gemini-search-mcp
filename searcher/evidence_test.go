package searcher

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

type offlineTransport struct{ sends int }

func (t *offlineTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	t.sends++
	return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"test_fixture":true}`))}, nil
}
func TestEvidenceTransportRefusesThirdSend(t *testing.T) {
	base := &offlineTransport{}
	a := &evidenceTransport{base: base, dir: t.TempDir()}
	for i := 0; i < 3; i++ {
		r, _ := http.NewRequest("POST", "https://generativelanguage.googleapis.com/v1beta/models/test:generateContent", strings.NewReader(`{"tools":[{"googleSearch":{}}],"generationConfig":{"maxOutputTokens":1024}}`))
		_, err := a.RoundTrip(r)
		if (i < 2) != (err == nil) {
			t.Fatalf("send %d unexpected error: %v", i, err)
		}
	}
	if base.sends != 2 {
		t.Fatalf("base sends=%d, want 2", base.sends)
	}
}
func TestEvidenceTransportRejectsUnsafeRequests(t *testing.T) {
	for _, tc := range []struct{ url, body string }{
		{"https://untrusted.example/v1beta/models/test:generateContent", `{}`},
		{"https://generativelanguage.googleapis.com/v1beta/models/test:generateContent", `{"generationConfig":{"maxOutputTokens":1024}}`},
		{"https://generativelanguage.googleapis.com/v1beta/models/test:generateContent", `{"tools":[{"googleSearch":{}}],"generationConfig":{"maxOutputTokens":5000}}`},
	} {
		base := &offlineTransport{}
		a := &evidenceTransport{base: base, dir: t.TempDir()}
		r, _ := http.NewRequest("POST", tc.url, strings.NewReader(tc.body))
		_, err := a.RoundTrip(r)
		if err == nil || base.sends != 0 {
			t.Fatalf("unsafe request reached base transport")
		}
	}
}
