package lifecycle

import (
 "net/http"
 "net/http/httptest"
 "path/filepath"
 "testing"
)

func TestPolicyProtocolStreamsDoNotBlockModelDrain(t *testing.T){
 dir:=t.TempDir();entered:=make(chan struct{});finish:=make(chan struct{});done:=make(chan struct{})
 h:=TrackRequests(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){close(entered);<-finish}),dir)
 go func(){h.ServeHTTP(httptest.NewRecorder(),httptest.NewRequest("GET","http://127.0.0.1/mcp",nil));close(done)}()
 <-entered;files,_:=filepath.Glob(filepath.Join(dir,"*.active"));close(finish);<-done
 if len(files)!=0{t.Fatal("long-lived protocol GET stream incorrectly blocks model drain")}
}
