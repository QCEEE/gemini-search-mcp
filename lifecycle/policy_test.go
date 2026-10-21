package lifecycle

import (
 "context"
 "encoding/json"
 "fmt"
 "os"
 "path/filepath"
 "sync"
 "sync/atomic"
 "testing"
)

func policyFixture(t *testing.T, mode string) (string,string,func(bool)) {
 t.Helper();dir:=t.TempDir();cfg:=filepath.Join(dir,"test.json");legacy:=filepath.Join(dir,"test-ledger.json");normal:=filepath.Join(dir,"usage.json");policy:=filepath.Join(dir,"policy.json");auth:=filepath.Join(dir,"authorization.json")
 os.WriteFile(cfg,[]byte(`{"schema":1,"enabled":true,"daily_limit":2,"task_limit":1,"run_limit":2,"run_id":"TEST"}`),0600)
 os.WriteFile(auth,[]byte(`{"run_id":"TEST","run_closed":false,"maximum_gemini_attempts":2}`),0600)
 write:=func(enabled bool){raw,_:=json.Marshal(map[string]any{"schema":1,"mode":mode,"enabled":enabled,"usage_ledger":normal,"test_authorization":auth});if err:=os.WriteFile(policy,raw,0600);err!=nil{t.Fatal(err)}};write(true)
 t.Setenv("GEMINI_POLICY_CONFIG",policy);t.Setenv("GEMINI_BUDGET_CONFIG",cfg);t.Setenv("GEMINI_BUDGET_LEDGER",legacy)
 return normal,legacy,write
}

// Catches leakage of test daily/run counters into normal requests.
func TestPolicyNormalNoBusinessTaskOrDailyCap(t *testing.T){
 normal,legacy,_:=policyFixture(t,"LOCAL_USER_INITIATED")
 for i:=0;i<8;i++{if _,err:=ReserveEnvironment(BeginTask(context.Background()));err!=nil{t.Fatalf("normal request %d blocked: %v",i+1,err)}}
 raw,err:=os.ReadFile(normal);if err!=nil{t.Fatal("normal accounting missing",err)};var j struct{Reservations []Reservation `json:"reservations"`};json.Unmarshal(raw,&j);if len(j.Reservations)!=8{t.Fatal("normal accounting lost requests")}
 if _,err=os.Stat(legacy);!os.IsNotExist(err){t.Fatal("normal mutated test ledger")}
}

func TestPolicyNormalConcurrentAccountingAndDuplicateSend(t *testing.T){
 normal,_,_:=policyFixture(t,"LOCAL_USER_INITIATED");var n atomic.Int32;var wg sync.WaitGroup
 for i:=0;i<32;i++{wg.Add(1);go func(){defer wg.Done();if _,err:=ReserveEnvironment(BeginTask(context.Background()));err==nil{n.Add(1)}}()};wg.Wait();if n.Load()!=32{t.Fatalf("normal accepted=%d",n.Load())}
 ctx:=BeginTask(context.Background());if _,err:=ReserveEnvironment(ctx);err!=nil{t.Fatal(err)};if _,err:=ReserveEnvironment(ctx);err==nil{t.Fatal("same execution replay admitted")}
 raw,_:=os.ReadFile(normal);var j struct{Reservations []Reservation `json:"reservations"`};json.Unmarshal(raw,&j);if len(j.Reservations)!=33{t.Fatal("concurrent reservations lost")}
}

func TestPolicyModeIsHostControlledAndFailsClosed(t *testing.T){
 _,legacy,_:=policyFixture(t,"UNRECOGNIZED")
 if _,err:=ReserveEnvironment(BeginTask(context.Background()));err==nil{t.Fatal("unknown host policy admitted")}
 if _,err:=os.Stat(legacy);!os.IsNotExist(err){t.Fatal("invalid policy mutated test ledger")}
}

func TestPolicyDisableBlocksBeforeReservation(t *testing.T){
 normal,legacy,write:=policyFixture(t,"LOCAL_USER_INITIATED");write(false)
 if _,err:=ReserveEnvironment(BeginTask(context.Background()));err==nil{t.Fatal("disabled policy admitted")}
 for _,p:=range []string{normal,legacy}{if _,err:=os.Stat(p);!os.IsNotExist(err){t.Fatal("disabled policy changed accounting")}}
}

func TestPolicyTestBoundedKeepsBudgetAndSealedAuthorization(t *testing.T){
 normal,legacy,_:=policyFixture(t,"TEST_BOUNDED")
 for i:=0;i<3;i++{_,err:=ReserveEnvironment(BeginTask(context.Background()));if (i<2)!=(err==nil){t.Fatalf("test attempt %d: %v",i,err)}}
 if _,err:=os.Stat(normal);!os.IsNotExist(err){t.Fatal("test mutated normal ledger")}
 // A closed run cannot be reopened by a new journal or a mode input.
 raw:=fmt.Sprintf(`{"schema":1,"mode":"TEST_BOUNDED","enabled":true,"test_authorization":%q}`,filepath.Join(filepath.Dir(legacy),"authorization.json"));os.WriteFile(os.Getenv("GEMINI_POLICY_CONFIG"),[]byte(raw),0600)
 os.WriteFile(filepath.Join(filepath.Dir(legacy),"authorization.json"),[]byte(`{"run_id":"TEST","run_closed":true,"maximum_gemini_attempts":2}`),0600)
 t.Setenv("GEMINI_BUDGET_LEDGER",filepath.Join(t.TempDir(),"fresh.json"))
 if _,err:=ReserveEnvironment(BeginTask(context.Background()));err==nil{t.Fatal("closed test reopened")}
}
