package lifecycle

import (
 "net/http"
 "os"
 "path/filepath"
)

// The marker is created BEFORE reading authentication. Closing authentication
// first and then checking this directory safely drains admitted handlers, even
// when a handler has not yet reached the model reservation boundary.
func TrackRequests(next http.Handler,dir string)http.Handler{
 if dir==""{return next}
 return http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  if r.Method != http.MethodPost { next.ServeHTTP(w,r);return }
  path:=filepath.Join(dir,randomID()+".active")
  if err:=atomicWrite(path,[]byte("active"));err!=nil{http.Error(w,"Admission unavailable",http.StatusServiceUnavailable);return}
  defer os.Remove(path)
  next.ServeHTTP(w,r)
 })
}
