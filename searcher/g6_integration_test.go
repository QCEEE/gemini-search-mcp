package searcher

import(
 "context"
 "net/http"
 "os"
 "path/filepath"
 "strings"
 "testing"
 "github.com/cnosuke/mcp-gemini-grounded-search/lifecycle"
)

func TestG6TransportEnforcesTaskAndPersistentBudget(t *testing.T){
 dir:=t.TempDir();cfg:=filepath.Join(dir,"config.json");ledger:=filepath.Join(dir,"ledger.json")
 if err:=os.WriteFile(cfg,[]byte(`{"schema":1,"enabled":true,"daily_limit":2,"task_limit":1,"run_limit":2,"run_id":"G6"}`),0600);err!=nil{t.Fatal(err)}
 t.Setenv("GEMINI_BUDGET_CONFIG",cfg);t.Setenv("GEMINI_BUDGET_LEDGER",ledger)
 base:=&offlineTransport{};tr:=&evidenceTransport{dir:dir,base:base};task:=lifecycle.BeginTask(context.Background())
 request:=func(ctx context.Context)*http.Request{r,_:=http.NewRequestWithContext(ctx,"POST","https://generativelanguage.googleapis.com/v1beta/models/gemini-3.8-flash:generateContent",strings.NewReader(`{"tools":[{"googleSearch":{}}],"generationConfig":{"maxOutputTokens":1536,"thinkingConfig":{"thinkingLevel":"LOW"}}}`));return r}
 if _,err:=tr.RoundTrip(request(task));err!=nil{t.Fatal(err)}
 if _,err:=tr.RoundTrip(request(task));err==nil{t.Fatal("SDK/tool retry forwarded")}
 if _,err:=tr.RoundTrip(request(lifecycle.BeginTask(context.Background())));err!=nil{t.Fatal(err)}
 restarted:=&evidenceTransport{dir:t.TempDir(),base:base}
 if _,err:=restarted.RoundTrip(request(lifecycle.BeginTask(context.Background())));err==nil{t.Fatal("new transport/folder reset persistent budget")}
 if base.sends!=2{t.Fatalf("forwarded=%d",base.sends)}
}

func TestG6GenerationDisabledBeforeAnyReservation(t *testing.T){
 dir:=t.TempDir();cfg:=filepath.Join(dir,"config.json");ledger:=filepath.Join(dir,"ledger.json")
 os.WriteFile(cfg,[]byte(`{"schema":1,"enabled":false,"daily_limit":2,"task_limit":1,"run_limit":2,"run_id":"G6"}`),0600)
 t.Setenv("GEMINI_BUDGET_CONFIG",cfg);t.Setenv("GEMINI_BUDGET_LEDGER",ledger)
 if _,err:=lifecycle.ReserveEnvironment(lifecycle.BeginTask(context.Background()));err==nil{t.Fatal("disabled generation admitted")}
 if _,err:=os.Stat(ledger);!os.IsNotExist(err){t.Fatal("disabled call changed budget")}
}
