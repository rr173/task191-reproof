package service

import (
 "context"
 "testing"
 "task191-reproof/internal/model"
)
func TestBug08_NewBaselineSupersedesOld(t *testing.T) {
 app,_:=openApp(t); ctx:=context.Background(); tid:=buildDemoTarget(t,app)
 acts,_:=app.ListActions(ctx,tid); var bid int64; for _,a:=range acts { if a.Name=="b" { bid=a.ID } }
 if _,err:=app.AddDeclaration(ctx,bid,"env.local",model.DirRead,"");err!=nil{t.Fatal(err)}
 if _,_,err:=app.AnalyzeTarget(ctx,tid);err!=nil{t.Fatal(err)}; if _,err:=app.GenerateProof(ctx,tid);err!=nil{t.Fatal(err)}
 b1,err:=app.FreezeBaseline(ctx,tid);if err!=nil{t.Fatal(err)}; b2,err:=app.FreezeBaseline(ctx,tid);if err!=nil{t.Fatal(err)}
 if b1.ID==b2.ID{t.Fatal("expected a new baseline")}; bs,err:=app.ListBaselines(ctx,tid);if err!=nil{t.Fatal(err)}
 if len(bs)!=2||bs[0].Status!=model.BaselineActive||bs[1].Status!=model.BaselineSuperseded{t.Fatalf("bad baseline statuses: %+v",bs)}
}
