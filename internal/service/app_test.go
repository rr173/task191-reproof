package service

import (
	"context"
	"path/filepath"
	"testing"

	"task191-reproof/internal/model"
	"task191-reproof/internal/store"
)

func openApp(t *testing.T) (*App, string) {
	t.Helper()
	db := filepath.Join(t.TempDir(), "app.db")
	st, err := store.Open(db)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return New(st), db
}

// buildDemoTarget 构造一个带读取泄漏的 demo 目标。
func buildDemoTarget(t *testing.T, app *App) int64 {
	t.Helper()
	ctx := context.Background()
	tgt, err := app.CreateTarget(ctx, "demo", "demo")
	if err != nil {
		t.Fatalf("create target: %v", err)
	}
	if err := app.AddSeed(ctx, tgt.ID, "seed"); err != nil {
		t.Fatalf("add seed: %v", err)
	}
	if err := app.AddSeed(ctx, tgt.ID, "env.local"); err != nil {
		t.Fatalf("add seed env.local: %v", err)
	}
	names := []string{"a", "b", "c"}
	ids := map[string]int64{}
	for _, n := range names {
		a, err := app.CreateAction(ctx, tgt.ID, n, "cmd "+n)
		if err != nil {
			t.Fatalf("create action %s: %v", n, err)
		}
		ids[n] = a.ID
	}
	if _, err := app.AddDep(ctx, ids["b"], ids["a"]); err != nil {
		t.Fatal(err)
	}
	if _, err := app.AddDep(ctx, ids["c"], ids["b"]); err != nil {
		t.Fatal(err)
	}
	decls := []struct {
		act  string
		path string
		dir  model.AccessDirection
	}{
		{"a", "seed", model.DirRead},
		{"a", "out_a", model.DirWrite},
		{"b", "out_a", model.DirRead},
		{"b", "out_b", model.DirWrite},
		{"c", "out_b", model.DirRead},
		{"c", "out_c", model.DirWrite},
	}
	for _, d := range decls {
		if _, err := app.AddDeclaration(ctx, ids[d.act], d.path, d.dir, ""); err != nil {
			t.Fatalf("decl %v: %v", d, err)
		}
	}
	logs := []model.LogEntry{
		{ActionID: ids["a"], Seq: 1, Path: "seed", Direction: model.DirRead, ContentHash: "h-seed", SizeBytes: 8},
		{ActionID: ids["a"], Seq: 2, Path: "out_a", Direction: model.DirWrite, ContentHash: "h-a", SizeBytes: 8},
		{ActionID: ids["b"], Seq: 1, Path: "out_a", Direction: model.DirRead, ContentHash: "h-a", SizeBytes: 8},
		{ActionID: ids["b"], Seq: 2, Path: "out_b", Direction: model.DirWrite, ContentHash: "h-b", SizeBytes: 8},
		{ActionID: ids["b"], Seq: 3, Path: "env.local", Direction: model.DirRead, ContentHash: "h-env", SizeBytes: 4}, // 泄漏
		{ActionID: ids["c"], Seq: 1, Path: "out_b", Direction: model.DirRead, ContentHash: "h-b", SizeBytes: 8},
		{ActionID: ids["c"], Seq: 2, Path: "out_c", Direction: model.DirWrite, ContentHash: "h-c", SizeBytes: 8},
	}
	if _, err := app.AppendLogs(ctx, logs); err != nil {
		t.Fatalf("append logs: %v", err)
	}
	return tgt.ID
}

func TestAnalyzeFindsLeakThenFixes(t *testing.T) {
	app, _ := openApp(t)
	ctx := context.Background()
	tid := buildDemoTarget(t, app)

	status, n, err := app.AnalyzeTarget(ctx, tid)
	if err != nil {
		t.Fatalf("analyze: %v", err)
	}
	if status != model.TargetIrreproducible {
		t.Fatalf("期望 irreproducible，得到 %s", status)
	}
	if n < 1 {
		t.Fatalf("期望违规 ≥1，得到 %d", n)
	}
	viols, err := app.ListViolations(ctx, tid)
	if err != nil || len(viols) == 0 {
		t.Fatalf("violations: %v", err)
	}
	foundLeak := false
	for _, v := range viols {
		if v.Kind == model.ViolationReadLeak && v.Path == "env.local" {
			foundLeak = true
		}
	}
	if !foundLeak {
		t.Fatalf("应发现 env.local 读取泄漏: %+v", viols)
	}
	shortest, err := app.ShortestPollution(ctx, tid)
	if err != nil {
		t.Fatalf("shortest pollution: %v", err)
	}
	if len(shortest.ActionIDs) == 0 {
		t.Fatal("最短污染链不应为空")
	}

	// 补齐声明后重新分析。
	acts, _ := app.ListActions(ctx, tid)
	var bAct int64
	for _, a := range acts {
		if a.Name == "b" {
			bAct = a.ID
		}
	}
	if _, err := app.AddDeclaration(ctx, bAct, "env.local", model.DirRead, ""); err != nil {
		t.Fatal(err)
	}
	status, n, err = app.AnalyzeTarget(ctx, tid)
	if err != nil {
		t.Fatalf("re-analyze: %v", err)
	}
	if status != model.TargetProven || n != 0 {
		t.Fatalf("补齐后应 proven，得到 %s/%d", status, n)
	}
}

func TestProofAndBaselineLifecycle(t *testing.T) {
	app, _ := openApp(t)
	ctx := context.Background()
	tid := buildDemoTarget(t, app)

	// 先补齐泄漏再分析。
	acts, _ := app.ListActions(ctx, tid)
	var bAct int64
	for _, a := range acts {
		if a.Name == "b" {
			bAct = a.ID
		}
	}
	_, _ = app.AddDeclaration(ctx, bAct, "env.local", model.DirRead, "")
	status, _, err := app.AnalyzeTarget(ctx, tid)
	if err != nil || status != model.TargetProven {
		t.Fatalf("analyze: %v %s", err, status)
	}

	p, err := app.GenerateProof(ctx, tid)
	if err != nil {
		t.Fatalf("generate proof: %v", err)
	}
	if p.Status != model.ProofValid {
		t.Fatalf("证明应为 valid: %s", p.Status)
	}
	// 相同图与日志哈希复用结论。
	reuse, err := app.GenerateProof(ctx, tid)
	if err != nil {
		t.Fatalf("reuse: %v", err)
	}
	if reuse.ID != p.ID {
		t.Fatalf("应复用证明 %d，得到 %d", p.ID, reuse.ID)
	}

	base, err := app.FreezeBaseline(ctx, tid)
	if err != nil {
		t.Fatalf("freeze: %v", err)
	}
	if base.ID == 0 {
		t.Fatal("基线应有 ID")
	}
	tgt, _ := app.GetTarget(ctx, tid)
	if tgt.Status != model.TargetBaselined {
		t.Fatalf("目标应 baselined: %s", tgt.Status)
	}

	// 基线比对一致。
	cmp, err := app.CompareBaseline(ctx, tid)
	if err != nil {
		t.Fatalf("compare: %v", err)
	}
	if !cmp.Clean {
		t.Fatalf("基线应一致: %s", cmp.Detail)
	}

	// 未 proven 前冻结应拒绝。
	t2, _ := app.CreateTarget(ctx, "raw", "")
	if _, err := app.FreezeBaseline(ctx, t2.ID); err == nil {
		t.Fatal("未 proven 目标冻结应拒绝")
	}
}

func TestCycleRejectedOnAddDep(t *testing.T) {
	app, _ := openApp(t)
	ctx := context.Background()
	tgt, _ := app.CreateTarget(ctx, "cyc", "")
	a1, _ := app.CreateAction(ctx, tgt.ID, "a1", "")
	a2, _ := app.CreateAction(ctx, tgt.ID, "a2", "")
	a3, _ := app.CreateAction(ctx, tgt.ID, "a3", "")
	if _, err := app.AddDep(ctx, a2.ID, a1.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := app.AddDep(ctx, a3.ID, a2.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := app.AddDep(ctx, a1.ID, a3.ID); err == nil {
		t.Fatal("成环依赖应被拒绝")
	}
}

func TestInvalidateProof(t *testing.T) {
	app, _ := openApp(t)
	ctx := context.Background()
	tid := buildDemoTarget(t, app)
	acts, _ := app.ListActions(ctx, tid)
	var bAct int64
	for _, a := range acts {
		if a.Name == "b" {
			bAct = a.ID
		}
	}
	_, _ = app.AddDeclaration(ctx, bAct, "env.local", model.DirRead, "")
	_, _, _ = app.AnalyzeTarget(ctx, tid)
	p, err := app.GenerateProof(ctx, tid)
	if err != nil {
		t.Fatalf("proof: %v", err)
	}
	inv, err := app.InvalidateProof(ctx, p.ID)
	if err != nil {
		t.Fatalf("invalidate: %v", err)
	}
	if inv.Status != model.ProofInvalidated {
		t.Fatalf("状态应为 invalidated: %s", inv.Status)
	}
}

func TestCrossTargetDepRejected(t *testing.T) {
	app, _ := openApp(t)
	ctx := context.Background()
	t1, _ := app.CreateTarget(ctx, "t1", "")
	t2, _ := app.CreateTarget(ctx, "t2", "")
	a1, _ := app.CreateAction(ctx, t1.ID, "a", "")
	a2, _ := app.CreateAction(ctx, t2.ID, "b", "")
	if _, err := app.AddDep(ctx, a1.ID, a2.ID); err == nil {
		t.Fatal("跨目标依赖应被拒绝")
	}
}
