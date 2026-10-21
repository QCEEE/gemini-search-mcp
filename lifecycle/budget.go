package lifecycle

import(
 "context"
 "crypto/rand"
 "encoding/hex"
 "encoding/json"
 "fmt"
 "os"
 "time"
)

type BudgetConfig struct{
 Enabled bool `json:"enabled"`
 Schema int `json:"schema"`
 DailyLimit int `json:"daily_limit"`
 TaskLimit int `json:"task_limit"`
 RunID string `json:"run_id"`
 RunLimit int `json:"run_limit"`
}
type Reservation struct{ID string `json:"id"`;Day string `json:"day"`;Task string `json:"task"`;Run string `json:"run"`;UTC time.Time `json:"reserved_utc"`}
type journal struct{Schema int `json:"schema"`;LastDay string `json:"last_day"`;Reservations []Reservation `json:"reservations"`}
type taskKey struct{}
func randomID()string{var b [16]byte;if _,err:=rand.Read(b[:]);err!=nil{panic(err)};return hex.EncodeToString(b[:])}
func BeginTask(ctx context.Context)context.Context{return context.WithValue(ctx,taskKey{},randomID())}
func ReserveEnvironment(ctx context.Context)(Reservation,error){
 if path:=os.Getenv("GEMINI_POLICY_CONFIG");path!=""{return reservePolicy(ctx,path)}
 var cfg BudgetConfig;var empty Reservation
 raw,err:=os.ReadFile(os.Getenv("GEMINI_BUDGET_CONFIG"));if err!=nil{return empty,err}
 if err=json.Unmarshal(raw,&cfg);err!=nil{return empty,err}
	if !cfg.Enabled{return empty,fmt.Errorf("generation disabled; no network send")}
 task,_:=ctx.Value(taskKey{}).(string)
 return Reserve(os.Getenv("GEMINI_BUDGET_LEDGER"),cfg,time.Now().UTC(),task)
}
func Reserve(path string,cfg BudgetConfig,now time.Time,task string)(Reservation,error){
 var empty Reservation
 if path==""||task==""||cfg.Schema!=1||cfg.DailyLimit<0||cfg.TaskLimit<1||cfg.RunLimit<0||cfg.RunID==""{return empty,fmt.Errorf("invalid budget contract; no send")}
 lock,fresh,err:=lockLedger(path+".lock");if err!=nil{return empty,err};defer lock.Close()
 data:=journal{Schema:1,Reservations:[]Reservation{}}
 raw,err:=os.ReadFile(path)
 if err==nil{
  if err=json.Unmarshal(raw,&data);err!=nil{return empty,fmt.Errorf("corrupt budget journal; no send")}
  if data.Schema!=1||data.Reservations==nil{return empty,fmt.Errorf("invalid budget journal; no send")}
 }else if !os.IsNotExist(err)||!fresh{return empty,fmt.Errorf("missing budget journal; no reset permitted")}
 day:=now.In(time.FixedZone("Asia/Shanghai",8*3600)).Format("2006-01-02")
 if data.LastDay>day{return empty,fmt.Errorf("budget clock rollback; no send")}
 daily,taskUsed,runUsed:=0,0,0
 seen:=map[string]bool{};lastRecordedDay:=""
 for _,r:=range data.Reservations{
  idBytes,idError:=hex.DecodeString(r.ID)
  if idError!=nil||len(idBytes)!=16||hex.EncodeToString(idBytes)!=r.ID||seen[r.ID]||r.Task==""||r.Run==""||r.UTC.IsZero()||r.Day!=r.UTC.In(time.FixedZone("Asia/Shanghai",8*3600)).Format("2006-01-02"){return empty,fmt.Errorf("invalid reservation; no send")}
  seen[r.ID]=true;if r.Day>lastRecordedDay{lastRecordedDay=r.Day}
  if r.Day==day{daily++};if r.Task==task{taskUsed++};if r.Run==cfg.RunID{runUsed++}
 }
 if data.LastDay!=lastRecordedDay{return empty,fmt.Errorf("inconsistent budget journal date; no send")}
 if daily>=cfg.DailyLimit||taskUsed>=cfg.TaskLimit||(cfg.RunLimit>0&&runUsed>=cfg.RunLimit){return empty,fmt.Errorf("persistent daily/task/run budget exhausted; no network send")}
 r:=Reservation{ID:randomID(),Day:day,Task:task,Run:cfg.RunID,UTC:now.UTC()};data.LastDay=day;data.Reservations=append(data.Reservations,r)
 encoded,err:=json.MarshalIndent(data,"","  ");if err!=nil{return empty,err};if err=atomicWrite(path,encoded);err!=nil{return empty,err};return r,nil
}
