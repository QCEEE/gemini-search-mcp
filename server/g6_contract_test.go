package server

import(
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

// Real handler -> Search -> installed SDK -> durable G6 reservation.
// The local Proxy hook rejects before DNS/TCP/TLS. No API request is sent.
func TestG6FullSDKContextAndRestartBudget(t *testing.T){
 dir:=t.TempDir();cfgPath:=filepath.Join(dir,"budget.json");ledger:=filepath.Join(dir,"ledger.json")
 os.WriteFile(cfgPath,[]byte(`{"schema":1,"enabled":true,"daily_limit":2,"task_limit":1,"run_limit":2,"run_id":"G6-OFFLINE"}`),0600)
 t.Setenv("GEMINI_RUN_ID","G6-OFFLINE");t.Setenv("GEMINI_BUDGET_CONFIG",cfgPath);t.Setenv("GEMINI_BUDGET_LEDGER",ledger);t.Setenv("GEMINI_VERIFY_MODEL","false");t.Setenv("GOOGLE_GENAI_USE_VERTEXAI","false");t.Setenv("GOOGLE_GENAI_USE_ENTERPRISE","false");t.Setenv("GOOGLE_GEMINI_BASE_URL","")
 old:=http.DefaultTransport;defer func(){http.DefaultTransport=old}();calls:=0
 http.DefaultTransport=&http.Transport{Proxy:func(r *http.Request)(*url.URL,error){calls++;return nil,errors.New("OFFLINE_G6_ABORT_BEFORE_NETWORK")}}
 cfg:=&config.Config{};cfg.Gemini.APIKey="offline-noncredential";cfg.Gemini.ModelName="gemini-3.8-flash";cfg.Gemini.MaxTokens=1536;cfg.Gemini.ThinkingLevel="LOW";cfg.Gemini.QueryTemplate="%s"
 msg,_:=json.Marshal(map[string]any{"jsonrpc":"2.0","id":1,"method":"tools/call","params":map[string]any{"name":"search","arguments":map[string]any{"question":"Offline SDK context propagation check","thinking_level":"LOW","max_token":1536}}})
 for i:=0;i<3;i++{
  t.Setenv("GEMINI_EVIDENCE_DIR",t.TempDir())
  s,_,err:=createMCPServer(cfg,"g6-offline-sdk","1","");if err!=nil{t.Fatal(err)}
  _=s.HandleMessage(context.Background(),msg)
  want:=i+1;if want>2{want=2};if calls!=want{t.Fatalf("native SDK forwards=%d want=%d at new server %d",calls,want,i)}
 }
 raw,err:=os.ReadFile(ledger);if err!=nil{t.Fatal(err)};var journal struct{Reservations []json.RawMessage `json:"reservations"`};if err=json.Unmarshal(raw,&journal);err!=nil{t.Fatal(err)};if len(journal.Reservations)!=2{t.Fatal("wrong persistent reservation count")}
}
