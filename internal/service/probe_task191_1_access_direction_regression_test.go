package service

import (
	"context"
	"testing"

	"task191-reproof/internal/model"
	"task191-reproof/internal/store"
)

func TestBug01_AccessDirectionLeakRemainsVisible(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(t.TempDir() + "/bug01.db")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	app := New(st)
	target, err := app.CreateTarget(ctx, "direction-regression", "")
	if err != nil {
		t.Fatal(err)
	}
	action, err := app.CreateAction(ctx, target.ID, "compile", "build")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.AddDeclaration(ctx, action.ID, "out.bin", model.DirWrite, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := app.AppendLogs(ctx, []model.LogEntry{
		{ActionID: action.ID, Seq: 1, Path: "secret.env", Direction: model.DirRead, ContentHash: "h-secret"},
		{ActionID: action.ID, Seq: 2, Path: "out.bin", Direction: model.DirWrite, ContentHash: "h-out"},
	}); err != nil {
		t.Fatal(err)
	}
	status, violations, err := app.AnalyzeTarget(ctx, target.ID)
	if err != nil {
		t.Fatal(err)
	}
	if status != model.TargetIrreproducible || violations == 0 {
		t.Fatalf("undeclared read must make the target irreproducible: status=%s violations=%d", status, violations)
	}
	items, err := app.ListViolations(ctx, target.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range items {
		if item.Kind == model.ViolationReadLeak && item.Path == "secret.env" {
			return
		}
	}
	t.Fatal("expected a read_leak violation for secret.env")
}
