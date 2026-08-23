package service

import (
	"context"
	"testing"
	"task191-reproof/internal/model"
)

func TestBug07_DriftInvalidatesProof(t *testing.T) {
	app, _ := openApp(t); ctx := context.Background(); tid := buildDemoTarget(t, app)
	acts, _ := app.ListActions(ctx, tid); var bID int64
	for _, a := range acts { if a.Name == "b" { bID = a.ID } }
	if _, err := app.AddDeclaration(ctx, bID, "env.local", model.DirRead, ""); err != nil { t.Fatal(err) }
	if _, _, err := app.AnalyzeTarget(ctx, tid); err != nil { t.Fatal(err) }
	p, err := app.GenerateProof(ctx, tid); if err != nil { t.Fatal(err) }
	if _, err := app.FreezeBaseline(ctx, tid); err != nil { t.Fatal(err) }
	if _, err := app.AppendLogs(ctx, []model.LogEntry{{ActionID:bID, Seq:9, Path:"out_b_v2", Direction:model.DirWrite, ContentHash:"changed"}}); err != nil { t.Fatal(err) }
	cmp, err := app.CompareBaseline(ctx, tid); if err != nil || cmp.Clean { t.Fatalf("expected drift: %+v %v", cmp, err) }
	got, _ := app.GetProof(ctx, p.ID); if got.Status != model.ProofInvalidated { t.Fatalf("proof should be invalidated, got %s", got.Status) }
	target, _ := app.GetTarget(ctx, tid); if target.Status != model.TargetBuilding { t.Fatalf("target should return to building, got %s", target.Status) }
}
