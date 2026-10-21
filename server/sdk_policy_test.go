package server

import (
 "context"
 "encoding/json"
 "errors"
 "net/http"
 "net/url"
 "os"
 "path/filepath"
 "testing"
 "github.com/cnosuke/mcp-gemini-grounded-search/config"
)

func TestPolicyNormalActualHandlerSDKAndRestartWithoutRetry(t *testing.T){
 dir:=t.TempDir();policy:=filepath.Join(dir,"policy.json");raw,_:=json.Marshal(map[string]any{"schema":1,"enabled":true,"mode":"LOCAL_USER_INITIATED","usage_ledger":filepath.Join(dir,"usage.json")});os.WriteFile(policy,raw,0600)
 t.Setenv("GEMINI_POLICY_CONFIG",policy);t.Setenv("GEMINI_BUDGET_CONFIG","");t.Setenv("GEMINI_VERIFY_MODEL","false");t.Setenv("GOOGLE_GENAI_USE_VERTEXAI","false");t.Setenv("GOOGLE_GENAI_USE_ENTERPRISE","false");t.Setenv("GOOGLE_GEMINI_BASE_URL","")
 old:=http.DefaultTransport;defer func(){http.DefaultTransport=old}();calls:=0
 http.DefaultTransport=&http.Transport{Proxy:func(r *http.Request)(*url.URL,error){calls++;return nil,errors.New("OFFLINE_ABORT_BEFORE_DNS_TCP_TLS")}}
 cfg:=&config.Config{};cfg.Gemini.APIKey="offline-noncredential";cfg.Gemini.ModelName="gemini-3.8-flash";cfg.Gemini.MaxTokens=1536;cfg.Gemini.ThinkingLevel="LOW";cfg.Gemini.QueryTemplate="%s"
 msg:=[]byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"search","arguments":{"question":"Offline policy test","thinking_level":"LOW","max_token":1536,"execution_mode":"TEST_BOUNDED"}}}`)
 for i:=0;i<5;i++{t.Setenv("GEMINI_EVIDENCE_DIR",t.TempDir());s,_,e:=createMCPServer(cfg,"offline-policy","1","");if e!=nil{t.Fatal(e)};s.HandleMessage(context.Background(),msg);if calls!=i+1{t.Fatalf("SDK sends=%d want=%d; retry or leaked cap",calls,i+1)}}
 // MCP argument asks for normal mode but host disables generation.
 raw,_=json.Marshal(map[string]any{"schema":1,"enabled":false,"mode":"TEST_BOUNDED"});os.WriteFile(policy,raw,0600)
 t.Setenv("GEMINI_EVIDENCE_DIR",t.TempDir());s,_,e:=createMCPServer(cfg,"offline-policy","1","");if e!=nil{t.Fatal(e)};s.HandleMessage(context.Background(),[]byte(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"search","arguments":{"question":"Offline bypass attempt","execution_mode":"LOCAL_USER_INITIATED","max_token":1536,"thinking_level":"LOW"}}}`));if calls!=5{t.Fatal("MCP input bypassed host policy")}
}
