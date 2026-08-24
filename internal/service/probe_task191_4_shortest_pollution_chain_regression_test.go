package service

import (
	"context"
	"testing"

	"task191-reproof/internal/model"
	"task191-reproof/internal/store"
)

func TestBug04_PollutionChainIsShortest(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(t.TempDir() + "/bug04.db")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	app := New(st)
	target, err := app.CreateTarget(ctx, "shortest-chain", "")
	if err != nil {
		t.Fatal(err)
	}
	root, _ := app.CreateAction(ctx, target.ID, "root", "")
	side, _ := app.CreateAction(ctx, target.ID, "side", "")
	bad, _ := app.CreateAction(ctx, target.ID, "bad", "")
	if _, err := app.AddDeclaration(ctx, bad.ID, "out", model.DirWrite, ""); err != nil { t.Fatal(err) }
	if _, err := app.AddDep(ctx, bad.ID, root.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := app.AddDep(ctx, side.ID, root.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := app.AppendLogs(ctx, []model.LogEntry{{ActionID: bad.ID, Seq: 1, Path: "secret", Direction: model.DirRead, ContentHash: "h"}}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := app.AnalyzeTarget(ctx, target.ID); err != nil {
		t.Fatal(err)
	}
	chains, err := app.ListChains(ctx, target.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(chains) == 0 || chains[0].Length != 2 || len(chains[0].ActionIDs) != 2 || chains[0].ActionIDs[0] != root.ID || chains[0].ActionIDs[1] != bad.ID {
		t.Fatalf("expected shortest root->bad chain, got %+v", chains)
	}
}
