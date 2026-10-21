package lifecycle

import (
 "encoding/json"
 "net/http"
 "net/http/httptest"
 "os"
 "path/filepath"
 "sync"
 "testing"
 "time"
)

func TestHotRotationPreservesAdmittedRequestAndRevokesOldToken(t *testing.T) {
 now:=time.Now().UTC(); var mu sync.Mutex
 snapshot:=AuthSnapshot{Schema:1,OwnerSID:"test-owner",Accepting:true,Token:"old-token",Expires:now.Add(time.Hour)}
 load:=func()(AuthSnapshot,error){mu.Lock();defer mu.Unlock();return snapshot,nil}
 admitted:=make(chan struct{}); finish:=make(chan struct{}); done:=make(chan int)
 h:=AuthGuard(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){close(admitted);<-finish;w.WriteHeader(200)}),load,"test-owner",func()time.Time{return now})
 go func(){r:=httptest.NewRequest("POST","http://127.0.0.1/mcp",nil);r.Header.Set("Authorization","Bearer old-token");w:=httptest.NewRecorder();h.ServeHTTP(w,r);done<-w.Code}()
 <-admitted;mu.Lock();snapshot.Token="new-token";mu.Unlock()
 denied:=httptest.NewRecorder();r:=httptest.NewRequest("POST","http://127.0.0.1/mcp",nil);r.Header.Set("Authorization","Bearer old-token");h.ServeHTTP(denied,r)
 if denied.Code!=401 {t.Fatalf("old token accepted: %d",denied.Code)}
 close(finish);if code:=<-done;code!=200{t.Fatalf("in-flight request interrupted: %d",code)}
 called:=false
 fresh:=AuthGuard(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){called=true}),load,"test-owner",func()time.Time{return now})
 r.Header.Set("Authorization","Bearer new-token");fresh.ServeHTTP(httptest.NewRecorder(),r);if !called{t.Fatal("new token rejected")}
}

func TestAuthFailureModesDenyWithoutInvokingHandler(t *testing.T){
 now:=time.Now().UTC()
 for _,s:=range []AuthSnapshot{
  {Schema:1,OwnerSID:"owner",Accepting:true,Token:"token",Expires:now},
  {Schema:1,OwnerSID:"owner",Accepting:false,Token:"token",Expires:now.Add(time.Hour)},
  {Schema:1,OwnerSID:"foreign",Accepting:true,Token:"token",Expires:now.Add(time.Hour)},
  {Schema:2,OwnerSID:"owner",Accepting:true,Token:"token",Expires:now.Add(time.Hour)},
 }{
  h:=AuthGuard(http.HandlerFunc(func(http.ResponseWriter,*http.Request){t.Error("denied request reached handler")}),func()(AuthSnapshot,error){return s,nil},"owner",func()time.Time{return now})
  r:=httptest.NewRequest("POST","http://127.0.0.1/mcp",nil);r.Header.Set("Authorization","Bearer token");w:=httptest.NewRecorder();h.ServeHTTP(w,r);if w.Code!=401{t.Fatal(w.Code)}
 }
}

func TestDPAPISnapshotRoundtripAndCorruption(t *testing.T){
 path:=filepath.Join(t.TempDir(),"auth.dpapi")
 value:=AuthSnapshot{Schema:1,OwnerSID:"owner",Accepting:true,Token:"test-secret-token-never-plaintext",Expires:time.Now().Add(time.Hour)}
 if err:=WriteAuth(path,value);err!=nil{t.Fatal(err)}
 raw,_:=os.ReadFile(path);if json.Valid(raw){t.Fatal("plaintext snapshot persisted")}
 got,err:=ReadAuth(path);if err!=nil||got.Token!=value.Token{t.Fatalf("roundtrip failed: %v",err)}
 raw[len(raw)/2]^=255;if err=os.WriteFile(path,raw,0600);err!=nil{t.Fatal(err)}
 if _,err=ReadAuth(path);err==nil{t.Fatal("corruption accepted")}
}
