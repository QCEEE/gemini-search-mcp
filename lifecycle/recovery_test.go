package lifecycle

import (
 "net/http"
 "net/http/httptest"
 "os"
 "os/exec"
 "path/filepath"
 "sync"
 "testing"
 "time"
)

func TestPolicyNaturalDayRestartAndMissingCorruptAccounting(t *testing.T){
 p:=filepath.Join(t.TempDir(),"usage.json");a:=time.Date(2026,10,19,15,59,0,0,time.UTC)
 r,e:=reserveLocal(p,a,"before-midnight");if e!=nil||r.Day!="2026-10-19"{t.Fatal(r,e)}
 r,e=reserveLocal(p,a.Add(2*time.Minute),"after-midnight");if e!=nil||r.Day!="2026-10-20"{t.Fatal(r,e)}
 if _,e=reserveLocal(p,a.Add(2*time.Minute),"before-midnight");e==nil{t.Fatal("restart/day changed replay protection")}
 if _,e=reserveLocal(p,a.Add(24*time.Hour),"new-independent-request");e!=nil{t.Fatal(e)}
 os.Remove(p);if _,e=reserveLocal(p,a,"missing");e==nil{t.Fatal("deleted journal silently reset")}
 os.WriteFile(p,[]byte(`{"schema":1,"mode":"LOCAL_USER_INITIATED","reservations":[{}]}`),0600)
 if _,e=reserveLocal(p,a,"corrupt");e==nil{t.Fatal("corrupt journal admitted")}
}

func TestPolicyCrashProcessReservationSurvives(t *testing.T){
 if os.Getenv("POLICY_CRASH_CHILD")=="1"{if _,e:=reserveLocal(os.Getenv("POLICY_CRASH_LEDGER"),time.Now(),"crashed-execution");e!=nil{os.Exit(19)};os.Exit(17)}
 path:=filepath.Join(t.TempDir(),"usage.json");exe,_:=os.Executable();cmd:=exec.Command(exe,"-test.run=^TestPolicyCrashProcessReservationSurvives$");cmd.Env=append(os.Environ(),"POLICY_CRASH_CHILD=1","POLICY_CRASH_LEDGER="+path)
 err:=cmd.Run();if x,ok:=err.(*exec.ExitError);!ok||x.ExitCode()!=17{t.Fatal("crash fixture failed",err)}
 if _,err=reserveLocal(path,time.Now(),"crashed-execution");err==nil{t.Fatal("abnormal exit lost reservation")}
 if _,err=reserveLocal(path,time.Now(),"new-user-execution");err!=nil{t.Fatal(err)}
}

func TestPolicyStopDrainsAdmittedHandlerAndBlocksNewRequest(t *testing.T){
 dir:=t.TempDir();now:=time.Now();var mu sync.Mutex;accepting:=true;entered:=make(chan struct{});finish:=make(chan struct{});done:=make(chan int)
 load:=func()(AuthSnapshot,error){mu.Lock();defer mu.Unlock();return AuthSnapshot{Schema:1,OwnerSID:"owner",Accepting:accepting,Token:"token",Expires:now.Add(time.Hour)},nil}
 handler:=TrackRequests(AuthGuard(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){close(entered);<-finish;w.WriteHeader(200)}),load,"owner",func()time.Time{return now}),dir)
 request:=func()*http.Request{r:=httptest.NewRequest("POST","http://127.0.0.1/mcp",nil);r.Header.Set("Authorization","Bearer token");return r}
 go func(){w:=httptest.NewRecorder();handler.ServeHTTP(w,request());done<-w.Code}();<-entered
 files,_:=filepath.Glob(filepath.Join(dir,"*.active"));if len(files)!=1{t.Fatal("admitted handler invisible before model reservation")}
 mu.Lock();accepting=false;mu.Unlock();w:=httptest.NewRecorder();handler.ServeHTTP(w,request());if w.Code!=401{t.Fatal("stop admitted new request")}
 files,_=filepath.Glob(filepath.Join(dir,"*.active"));if len(files)!=1{t.Fatal("draining marker removed early")};close(finish);if code:=<-done;code!=200{t.Fatal("admitted handler interrupted")}
 files,_=filepath.Glob(filepath.Join(dir,"*.active"));if len(files)!=0{t.Fatal("completed marker leaked")}
}
