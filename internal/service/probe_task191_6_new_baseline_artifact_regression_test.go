package service

import (
	"context"
	"testing"
	"task191-reproof/internal/model"
)

func TestBug06_NewArtifactInvalidatesBaseline(t *testing.T) {
	app, _ := openApp(t)
	ctx := context.Background()
	tid := buildDemoTarget(t, app)
	acts, _ := app.ListActions(ctx, tid)
	var bID int64
	for _, a := range acts { if a.Name == "b" { bID = a.ID } }
	if _, err := app.AddDeclaration(ctx, bID, "env.local", model.DirRead, ""); err != nil { t.Fatal(err) }
	if _, _, err := app.AnalyzeTarget(ctx, tid); err != nil { t.Fatal(err) }
	if _, err := app.GenerateProof(ctx, tid); err != nil { t.Fatal(err) }
	if _, err := app.FreezeBaseline(ctx, tid); err != nil { t.Fatal(err) }
	if _, err := app.AppendLogs(ctx, []model.LogEntry{{ActionID: bID, Seq: 9, Path: "new-output", Direction: model.DirWrite, ContentHash: "h-new"}}); err != nil { t.Fatal(err) }
	cmp, err := app.CompareBaseline(ctx, tid)
	if err != nil { t.Fatal(err) }
	if cmp.Clean || len(cmp.New) != 1 || cmp.New[0] != "new-output" { t.Fatalf("new artifact must be reported: %+v", cmp) }
}
