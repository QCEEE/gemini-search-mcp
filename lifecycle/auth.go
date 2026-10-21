package lifecycle

import (
 "crypto/subtle"
 "encoding/json"
 "fmt"
 "net/http"
 "os"
 "time"
)

type AuthSnapshot struct {
 Schema int `json:"schema"`
 OwnerSID string `json:"owner_sid"`
 Accepting bool `json:"accepting"`
 Token string `json:"token"`
 Expires time.Time `json:"expires_utc"`
 Generation string `json:"generation"`
}

func AuthGuard(next http.Handler,load func()(AuthSnapshot,error),owner string,now func()time.Time)http.Handler{
 return http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  s,err:=load()
  if err!=nil||owner==""||s.Schema!=1||s.OwnerSID!=owner||!s.Accepting||s.Token==""||!now().Before(s.Expires)||subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")),[]byte("Bearer "+s.Token))!=1{
   http.Error(w,"Unauthorized",http.StatusUnauthorized);return
  }
  next.ServeHTTP(w,r)
 })
}

func ReadAuth(path string)(AuthSnapshot,error){
 var s AuthSnapshot
 raw,err:=os.ReadFile(path);if err!=nil{return s,err}
 plain,err:=unprotect(raw);if err!=nil{return s,fmt.Errorf("authentication snapshot unavailable")}
 defer clear(plain)
 err=json.Unmarshal(plain,&s);return s,err
}

func WriteAuth(path string,s AuthSnapshot)error{
 plain,err:=json.Marshal(s);if err!=nil{return err};defer clear(plain)
 raw,err:=protect(plain);if err!=nil{return err};return atomicWrite(path,raw)
}
