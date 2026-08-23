package service
import("context";"testing";"task191-reproof/internal/model")
func TestBug09_EmptyObservationReturnsInsufficient(t *testing.T){app,_:=openApp(t);ctx:=context.Background();tg,_:=app.CreateTarget(ctx,"empty-observation","");a,_:=app.CreateAction(ctx,tg.ID,"compile","");if _,e:=app.AddDeclaration(ctx,a.ID,"out",model.DirWrite,"");e!=nil{t.Fatal(e)};st,_,e:=app.AnalyzeTarget(ctx,tg.ID);if e!=nil{t.Fatal(e)};if st!=model.TargetInsufficient{t.Fatalf("expected insufficient got %s",st)}}
