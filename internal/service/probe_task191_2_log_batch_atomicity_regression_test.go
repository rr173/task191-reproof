package service

import (
	"context"
	"errors"
	"testing"

	"task191-reproof/internal/model"
	"task191-reproof/internal/store"
)

func TestBug02_ConflictingLogBatchIsAtomic(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(t.TempDir() + "/bug02.db")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	app := New(st)
	target, err := app.CreateTarget(ctx, "log-atomicity", "")
	if err != nil {
		t.Fatal(err)
	}
	action, err := app.CreateAction(ctx, target.ID, "compile", "build")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := app.AppendLogs(ctx, []model.LogEntry{{ActionID: action.ID, Seq: 1, Path: "input", Direction: model.DirRead, ContentHash: "old"}}); err != nil {
		t.Fatal(err)
	}
	_, err = app.AppendLogs(ctx, []model.LogEntry{
		{ActionID: action.ID, Seq: 2, Path: "new-input", Direction: model.DirRead, ContentHash: "new"},
		{ActionID: action.ID, Seq: 1, Path: "input", Direction: model.DirRead, ContentHash: "changed"},
	})
	if !errors.Is(err, model.ErrConflictLog) {
		t.Fatalf("expected conflict error, got %v", err)
	}
	logs, err := store.NewLogStore(st).ListByAction(ctx, action.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) != 1 || logs[0].Seq != 1 || logs[0].ContentHash != "old" {
		t.Fatalf("conflicting batch must roll back all entries, got %+v", logs)
	}
}
