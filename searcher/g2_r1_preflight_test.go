package searcher

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Explicit opt-in non-generating network check using the server's real transport.
func TestG2R1NetworkPreflight(t *testing.T) {
	dir := os.Getenv("G2_R1_NETWORK_DIR")
	if dir == "" {
		t.Skip("explicit non-generating network preflight only")
	}
	report := map[string]any{"generation_requests": 0, "key_used": false, "automatic_retries": 0, "transport": "newEvidenceHTTPClient (same as server)", "host_runtime": "WINDOWS_NATIVE_AMD64"}
	defer func() {
		b, _ := json.MarshalIndent(report, "", "  ")
		_ = os.WriteFile(filepath.Join(dir, "network-preflight.json"), b, 0600)
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	ips, err := net.DefaultResolver.LookupHost(ctx, "generativelanguage.googleapis.com")
	if err != nil {
		report["error"] = err.Error()
		t.Fatal(err)
	}
	report["api_dns_addresses"] = ips
	r, _ := http.NewRequestWithContext(ctx, "GET", "https://generativelanguage.googleapis.com/v1beta/models/gemini-3.8-flash", nil)
	proxy, err := http.ProxyFromEnvironment(r)
	if err != nil {
		t.Fatal(err)
	}
	route := "DIRECT"
	if proxy != nil {
		route = proxy.Scheme + "://" + proxy.Host
		_, err = net.DefaultResolver.LookupHost(ctx, proxy.Hostname())
		if err != nil {
			report["error"] = err.Error()
			t.Fatal(err)
		}
	}
	report["proxy_route"] = route
	h, err := newEvidenceHTTPClient(filepath.Join(dir, "network-audit"), "")
	if err != nil {
		t.Fatal(err)
	}
	resp, err := h.Do(r)
	if err != nil {
		report["error"] = err.Error()
		report["status"] = "BLOCKED_NETWORK"
		t.Fatal(err)
	}
	defer resp.Body.Close()
	report["http_status"] = resp.StatusCode
	report["tls_verified"] = resp.TLS != nil && len(resp.TLS.VerifiedChains) > 0
	if resp.TLS != nil {
		report["tls_version"] = tls.VersionName(resp.TLS.Version)
	}
	if resp.TLS == nil || len(resp.TLS.VerifiedChains) == 0 || (resp.StatusCode != 400 && resp.StatusCode != 401 && resp.StatusCode != 403) {
		report["status"] = "BLOCKED_NETWORK"
		t.Fatalf("unexpected noncredential preflight status %d", resp.StatusCode)
	}
	report["status"] = "PASS"
}
