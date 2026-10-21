package lifecycle

import("os";"path/filepath";"sync";"sync/atomic";"testing";"time")

func TestPersistentAtomicDailyAndTaskBudgets(t *testing.T){
 path:=filepath.Join(t.TempDir(),"ledger.json");cfg:=BudgetConfig{Schema:1,DailyLimit:2,TaskLimit:1,RunLimit:2,RunID:"G6"};now:=time.Date(2026,10,20,2,0,0,0,time.UTC)
 var accepted atomic.Int32;var wg sync.WaitGroup
 for n:=0;n<32;n++{wg.Add(1);go func(n int){defer wg.Done();if _,err:=Reserve(path,cfg,now,string(rune('a'+n)));err==nil{accepted.Add(1)}}(n)}
 wg.Wait();if accepted.Load()!=2{t.Fatalf("accepted=%d",accepted.Load())}
 if _,err:=Reserve(path,cfg,now,"restart");err==nil{t.Fatal("restart reset daily/run budget")}
 cfg.RunLimit=0
 if _,err:=Reserve(path,cfg,now.Add(24*time.Hour),"next-day");err!=nil{t.Fatal(err)}
 if _,err:=Reserve(path,cfg,now.Add(24*time.Hour),"next-day");err==nil{t.Fatal("task cap reset")}
 if _,err:=Reserve(path,cfg,now,"clock-rollback");err==nil{t.Fatal("clock rollback accepted")}
}

func TestBudgetCorruptionAndTaskReservationsFailClosed(t *testing.T){
 path:=filepath.Join(t.TempDir(),"ledger.json");cfg:=BudgetConfig{Schema:1,DailyLimit:2,TaskLimit:1,RunID:"G6",RunLimit:2};now:=time.Now()
 if _,err:=Reserve(path,cfg,now,"same-task");err!=nil{t.Fatal(err)}
 if _,err:=Reserve(path,cfg,now,"same-task");err==nil{t.Fatal("tool retry consumed a second send")}
 if err:=os.WriteFile(path,[]byte("broken"),0600);err!=nil{t.Fatal(err)}
 if _,err:=Reserve(path,cfg,now,"new-task");err==nil{t.Fatal("corrupt ledger reset budget")}
}

func TestBudgetRejectsStructurallyValidButInvalidJournal(t *testing.T){
 path:=filepath.Join(t.TempDir(),"ledger.json");cfg:=BudgetConfig{Schema:1,DailyLimit:2,TaskLimit:1,RunID:"G6",RunLimit:2};now:=time.Date(2026,10,20,0,0,0,0,time.UTC)
 raw:=`{"schema":1,"last_day":"2026-10-20","reservations":[{"id":"0123456789abcdef0123456789abcdef","day":"invalid-day","task":"old","run":"G6","reserved_utc":"2026-10-20T00:00:00Z"}]}`
 if err:=os.WriteFile(path,[]byte(raw),0600);err!=nil{t.Fatal(err)}
 if _,err:=Reserve(path,cfg,now,"new");err==nil{t.Fatal("valid JSON with corrupt reservation date reset daily accounting")}
}
