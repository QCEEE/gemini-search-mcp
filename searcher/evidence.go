package searcher

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"github.com/cnosuke/mcp-gemini-grounded-search/lifecycle"
)

// evidenceTransport is opt-in for bounded acceptance runs. It records bodies,
// never authentication headers, and refuses more than two model sends per folder.
type evidenceTransport struct {
	base          http.RoundTripper
	dir, secret   string
	mu            sync.Mutex
	metadataCount int
}

func (t *evidenceTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL.Scheme != "https" || r.URL.Host != "generativelanguage.googleapis.com" {
		return nil, fmt.Errorf("non-Gemini API destination rejected")
	}
	generation := r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, ":generateContent")
	metadata := r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1beta/models")
	if !generation && !metadata {
		return nil, fmt.Errorf("unapproved API operation rejected")
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	var body []byte
	var err error
	if r.Body != nil {
		body, err = io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if err != nil {
			return nil, err
		}
		_ = r.Body.Close()
		r.Body = io.NopCloser(bytes.NewReader(body))
	}
	prefix := ""
	if generation {
		var wire map[string]any
		if err = json.Unmarshal(body, &wire); err != nil {
			return nil, fmt.Errorf("invalid model request body")
		}
		tools, _ := wire["tools"].([]any)
		enabled := false
		for _, v := range tools {
			if m, ok := v.(map[string]any); ok {
				if _, ok = m["googleSearch"]; ok {
					enabled = true
				}
			}
		}
		gc, _ := wire["generationConfig"].(map[string]any)
		cap, _ := gc["maxOutputTokens"].(float64)
		maxCap := float64(2048)
		if os.Getenv("GEMINI_RUN_ID") == "G2-R1-20261019" {
			maxCap = 4096
			thinking, _ := gc["thinkingConfig"].(map[string]any)
			if r.URL.Path != "/v1beta/models/gemini-3.8-flash:generateContent" || thinking["thinkingLevel"] != "LOW" {
				return nil, fmt.Errorf("G2-R1 requires the fixed model and LOW thinking")
			}
		}
		if !enabled || cap <= 0 || cap > maxCap {
			return nil, fmt.Errorf("grounding tool and authorized output cap required")
		}
		if os.Getenv("GEMINI_BUDGET_CONFIG") != "" || os.Getenv("GEMINI_POLICY_CONFIG") != "" {
			thinking, _ := gc["thinkingConfig"].(map[string]any)
			if r.URL.Path != "/v1beta/models/gemini-3.8-flash:generateContent" || thinking["thinkingLevel"] != "LOW" || cap != 1536 {
				return nil, fmt.Errorf("G6 requires fixed model, LOW and 1536 output cap")
			}
			reservation, e := lifecycle.ReserveEnvironment(r.Context())
			if e != nil { return nil, e }
			prefix = "model-" + reservation.ID
			if e = t.write(prefix+"-reservation.json", reservation); e != nil { return nil, e }
			if e = os.WriteFile(filepath.Join(t.dir,prefix+"-request.json"),t.redact(body),0600); e != nil { return nil, e }
		} else {
		for n := 1; n <= 2; n++ {
			p := filepath.Join(t.dir, fmt.Sprintf("model-%03d-request.json", n))
			f, e := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
			if os.IsExist(e) {
				continue
			}
			if e != nil {
				return nil, e
			}
			safe := t.redact(body)
			_, e = f.Write(safe)
			_ = f.Close()
			if e != nil {
				return nil, e
			}
			prefix = fmt.Sprintf("model-%03d", n)
			break
		}
		}
		if prefix == "" {
			return nil, fmt.Errorf("two-request budget exhausted; no network request sent")
		}
	} else {
		t.metadataCount++
		prefix = fmt.Sprintf("metadata-%03d", t.metadataCount)
		if os.Getenv("GEMINI_RUN_ID") == "G2-R1-20261019" {
			prefix = fmt.Sprintf("metadata-%s-%03d", time.Now().UTC().Format("20060102T150405.000000000"), t.metadataCount)
		}
	}
	hash := sha256.Sum256(body)
	record := map[string]any{"timestamp_utc": time.Now().UTC().Format(time.RFC3339Nano), "method": r.Method, "host": r.URL.Host, "path": r.URL.Path, "wire_body_sha256": hex.EncodeToString(hash[:]), "generation": generation, "automatic_retries": false, "authentication_headers_saved": false}
	if os.Getenv("GEMINI_RUN_ID") == "G2-R1-20261019" {
		route := "DIRECT"
		if proxy, e := http.ProxyFromEnvironment(r); e != nil {
			return nil, e
		} else if proxy != nil {
			route = proxy.Scheme + "://" + proxy.Host
		}
		record["proxy_route"] = route
		record["host_runtime"] = "WINDOWS_NATIVE_AMD64"
		record["run_id"] = "G2-R1-20261019"
	}
	if err = t.write(prefix+"-send.json", record); err != nil {
		return nil, err
	}
	resp, err := t.base.RoundTrip(r)
	if err != nil {
		_ = t.write(prefix+"-network-error.json", map[string]any{"error": string(t.redact([]byte(err.Error())))})
		return nil, err
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	_ = resp.Body.Close()
	if err != nil {
		return nil, err
	}
	resp.Body = io.NopCloser(bytes.NewReader(raw))
	if err = os.WriteFile(filepath.Join(t.dir, prefix+"-response.json"), t.redact(raw), 0600); err != nil {
		return nil, err
	}
	if err = t.write(prefix+"-status.json", map[string]any{"http_status": resp.StatusCode, "timestamp_utc": time.Now().UTC().Format(time.RFC3339Nano)}); err != nil {
		return nil, err
	}
	return resp, nil
}

func (t *evidenceTransport) redact(b []byte) []byte {
	if t.secret != "" {
		return bytes.ReplaceAll(b, []byte(t.secret), []byte("[REDACTED]"))
	}
	return b
}
func (t *evidenceTransport) write(name string, v any) error {
	b, e := json.MarshalIndent(v, "", "  ")
	if e != nil {
		return e
	}
	return os.WriteFile(filepath.Join(t.dir, name), t.redact(b), 0600)
}
func newEvidenceHTTPClient(dir, key string) (*http.Client, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	base := http.DefaultTransport.(*http.Transport).Clone()
	base.DisableKeepAlives = true
	return &http.Client{Transport: &evidenceTransport{base: base, dir: dir, secret: key}, Timeout: 90 * time.Second, CheckRedirect: func(r *http.Request, via []*http.Request) error { return fmt.Errorf("API redirects disabled") }}, nil
}
