package searcher

import (
 "net/http"
 "os"
 "testing"
)

func TestG6ChildProxyPolicy(t *testing.T) {
 if os.Getenv("G6_PROXY_POLICY_CHECK")!="1" {t.Skip("explicit credential-free process proxy policy check")}
 for _, raw:=range []string{"http://127.0.0.1:8765/mcp","http://localhost:8765/mcp","http://[::1]:8765/mcp"} {
  req,_:=http.NewRequest("GET",raw,nil);proxy,err:=http.ProxyFromEnvironment(req)
  if err!=nil||proxy!=nil{t.Fatalf("loopback proxy bypass failed for %s: %v %v",raw,proxy,err)}
 }
 req,_:=http.NewRequest("GET","https://generativelanguage.googleapis.com/",nil)
 proxy,err:=http.ProxyFromEnvironment(req)
 if err!=nil||proxy==nil||proxy.String()!="http://127.0.0.1:11808"{t.Fatalf("Google proxy route mismatch: %v %v",proxy,err)}
 t.Log("Google uses the existing local proxy; all loopback MCP URLs bypass it; no network request was made")
}
