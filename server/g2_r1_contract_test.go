package server

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/cnosuke/mcp-gemini-grounded-search/config"
)

// This test traverses the real MCP handler, wrapper, SDK serializer and audited
// transport. Proxy interception aborts before any DNS/TCP/TLS or API request.
func TestG2R1SerializedLOWContract(t *testing.T) {
	for _, tc := range []struct {
		name     string
		cap      int
		explicit bool
	}{{"explicit-low-1536", 1536, true}, {"default-low-1536", 1536, false}, {"explicit-low-4096", 4096, true}} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("GEMINI_RUN_ID", "G2-R1-20261019")
			t.Setenv("GEMINI_EVIDENCE_DIR", t.TempDir())
			t.Setenv("GEMINI_VERIFY_MODEL", "false")
			t.Setenv("GOOGLE_GENAI_USE_VERTEXAI", "false")
			t.Setenv("GOOGLE_GENAI_USE_ENTERPRISE", "false")
			t.Setenv("GOOGLE_GEMINI_BASE_URL", "")
			old := http.DefaultTransport
			defer func() { http.DefaultTransport = old }()
			calls := 0
			var wire []byte
			var endpoint string
			http.DefaultTransport = &http.Transport{Proxy: func(r *http.Request) (*url.URL, error) {
				calls++
				endpoint = r.URL.String()
				wire, _ = io.ReadAll(r.Body)
				return nil, errors.New("OFFLINE_G2R1_ABORT_BEFORE_NETWORK")
			}}
			cfg := &config.Config{}
			cfg.Gemini.APIKey = "offline-noncredential"
			cfg.Gemini.ModelName = "gemini-3.8-flash"
			cfg.Gemini.MaxTokens = 1536
			cfg.Gemini.ThinkingLevel = "LOW"
			cfg.Gemini.QueryTemplate = "%s"
			s, _, err := createMCPServer(cfg, "offline-contract", "1", "")
			if err != nil {
				t.Fatal(err)
			}
			args := map[string]any{"question": g2R1Question, "max_token": tc.cap}
			if tc.explicit {
				args["thinking_level"] = "LOW"
			}
			msg, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": map[string]any{"name": "search", "arguments": args}})
			_ = s.HandleMessage(context.Background(), msg)
			if calls != 1 {
				t.Fatalf("SDK transport attempts=%d, want exactly 1; cap guard or contract failed", calls)
			}
			if endpoint != "https://generativelanguage.googleapis.com/v1beta/models/gemini-3.8-flash:generateContent" {
				t.Fatalf("wrong endpoint %s", endpoint)
			}
			var body map[string]any
			if err = json.Unmarshal(wire, &body); err != nil {
				t.Fatal(err)
			}
			gc := body["generationConfig"].(map[string]any)
			think := gc["thinkingConfig"].(map[string]any)
			if think["thinkingLevel"] != "LOW" || gc["maxOutputTokens"] != float64(tc.cap) {
				t.Fatalf("unexpected generation config: %v", gc)
			}
			if _, ok := think["thinkingBudget"]; ok {
				t.Fatal("unexpected thinking budget override")
			}
			tools := body["tools"].([]any)
			if _, ok := tools[0].(map[string]any)["googleSearch"]; !ok {
				t.Fatal("googleSearch disabled")
			}
			if out := os.Getenv("G2_R1_PREVIEW_DIR"); out != "" {
				if err = os.WriteFile(filepath.Join(out, tc.name+"-wire-preview.json"), wire, 0600); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

const g2R1Question = "\u641c\u7d22 Google \u5b98\u65b9 Gemini API \u5173\u4e8e Google Search Grounding \u7684\u6700\u65b0\u6587\u6863\u3002\u8bf7\u7ed9\u51fa\u5b98\u65b9\u6587\u6863\u540d\u79f0\u3001\u53ef\u6838\u67e5\u7684\u6765\u6e90 URL\uff0c\u4ee5\u53ca\u8be5\u529f\u80fd\u7684\u7b80\u77ed\u8bf4\u660e\u3002\u5fc5\u987b\u5b9e\u9645\u6267\u884c\u8054\u7f51\u641c\u7d22\uff0c\u4e0d\u5141\u8bb8\u4ec5\u51ed\u6a21\u578b\u5df2\u6709\u77e5\u8bc6\u56de\u7b54\u3002"
