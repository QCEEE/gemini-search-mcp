package lifecycle

import (
 "context"
 "encoding/hex"
 "encoding/json"
 "fmt"
 "os"
 "time"
)

// ExecutionPolicy is read only from an operator-owned file selected at startup.
// MCP arguments cannot select or change this policy.
type ExecutionPolicy struct {
 Schema int `json:"schema"`
 Mode string `json:"mode"`
 Enabled bool `json:"enabled"`
 UsageLedger string `json:"usage_ledger"`
 TestAuthorization string `json:"test_authorization"`
}

func reservePolicy(ctx context.Context, path string)(Reservation,error){
 var p ExecutionPolicy;var empty Reservation
 raw,err:=os.ReadFile(path);if err!=nil{return empty,err};if err=json.Unmarshal(raw,&p);err!=nil{return empty,err}
 if p.Schema!=1||(p.Mode!="TEST_BOUNDED"&&p.Mode!="LOCAL_USER_INITIATED"){return empty,fmt.Errorf("invalid host execution policy; no send")}
 if !p.Enabled{return empty,fmt.Errorf("generation disabled by host policy; no send")}
 task,_:=ctx.Value(taskKey{}).(string)
 if p.Mode=="LOCAL_USER_INITIATED"{
  if p.UsageLedger==os.Getenv("GEMINI_BUDGET_LEDGER"){return empty,fmt.Errorf("normal accounting must be isolated from test ledger")}
  return reserveLocal(p.UsageLedger,time.Now().UTC(),task)
 }
 var cfg BudgetConfig
 raw,err=os.ReadFile(os.Getenv("GEMINI_BUDGET_CONFIG"));if err!=nil{return empty,err};if err=json.Unmarshal(raw,&cfg);err!=nil{return empty,err}
 var a struct{RunID string `json:"run_id"`;Closed bool `json:"run_closed"`;Maximum int `json:"maximum_gemini_attempts"`}
 raw,err=os.ReadFile(p.TestAuthorization);if err!=nil{return empty,err};if err=json.Unmarshal(raw,&a);err!=nil{return empty,err}
 if a.Closed||a.RunID!=cfg.RunID||a.Maximum<=0||cfg.RunLimit!=a.Maximum||!cfg.Enabled{return empty,fmt.Errorf("closed or mismatched test authorization; no send")}
 return Reserve(os.Getenv("GEMINI_BUDGET_LEDGER"),cfg,time.Now().UTC(),task)
}

type usageJournal struct {
 Schema int `json:"schema"`
 Mode string `json:"mode"`
 Reservations []Reservation `json:"reservations"`
}

// One reservation per execution prevents transport replay. It is not a cap on
// user business tasks, days, process starts, or successive MCP tool calls.
func reserveLocal(path string,now time.Time,execution string)(Reservation,error){
 var empty Reservation
 if path==""||execution==""{return empty,fmt.Errorf("missing normal accounting identity; no send")}
 lock,fresh,err:=lockLedger(path+".lock");if err!=nil{return empty,err};defer lock.Close()
 j:=usageJournal{Schema:1,Mode:"LOCAL_USER_INITIATED",Reservations:[]Reservation{}}
 raw,err:=os.ReadFile(path)
 if err==nil{if json.Unmarshal(raw,&j)!=nil||j.Schema!=1||j.Mode!="LOCAL_USER_INITIATED"||j.Reservations==nil{return empty,fmt.Errorf("corrupt normal accounting; no send")}}else if !os.IsNotExist(err)||!fresh{return empty,fmt.Errorf("missing normal accounting; no reset permitted")}
 seen:=map[string]bool{}
 for _,r:=range j.Reservations{
  b,e:=hex.DecodeString(r.ID)
  if e!=nil||len(b)!=16||hex.EncodeToString(b)!=r.ID||seen[r.ID]||r.Task==""||r.Run!="LOCAL_USER_INITIATED"||r.UTC.IsZero()||r.Day!=r.UTC.In(time.FixedZone("Asia/Shanghai",8*3600)).Format("2006-01-02"){return empty,fmt.Errorf("invalid normal accounting reservation; no send")}
  if r.Task==execution{return empty,fmt.Errorf("duplicate execution send; no retry")};seen[r.ID]=true
 }
 r:=Reservation{ID:randomID(),Day:now.In(time.FixedZone("Asia/Shanghai",8*3600)).Format("2006-01-02"),Task:execution,Run:"LOCAL_USER_INITIATED",UTC:now.UTC()}
 j.Reservations=append(j.Reservations,r);raw,err=json.MarshalIndent(j,"","  ");if err!=nil{return empty,err};if err=atomicWrite(path,raw);err!=nil{return empty,err};return r,nil
}
