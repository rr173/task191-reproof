package store

import (
	"context"
	"path/filepath"
	"testing"

	"task191-reproof/internal/model"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	st, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func TestTargetCRUD(t *testing.T) {
	st := openTestStore(t)
	ts := NewTargetStore(st)
	ctx := context.Background()

	created, err := ts.Create(ctx, model.NewTarget("app", "应用构建"))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.ID == 0 {
		t.Fatal("应分配 ID")
	}
	// 重名拒绝。
	if _, err := ts.Create(ctx, model.NewTarget("app", "")); err != model.ErrDuplicateName {
		t.Fatalf("重名应拒绝，得到 %v", err)
	}
	got, err := ts.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Name != "app" || got.Status != model.TargetBuilding {
		t.Fatalf("实体错误: %+v", got)
	}
	if err := ts.UpdateStatus(ctx, created.ID, model.TargetProven); err != nil {
		t.Fatalf("update status: %v", err)
	}
	got, _ = ts.GetByID(ctx, created.ID)
	if got.Status != model.TargetProven {
		t.Fatal("状态未更新")
	}
	if _, err := ts.GetByID(ctx, 9999); err != model.ErrNotFound {
		t.Fatalf("不存在应报 ErrNotFound，得到 %v", err)
	}
}

func TestActionAndDeclarations(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	tgt, _ := NewTargetStore(st).Create(ctx, model.NewTarget("t", ""))
	as := NewActionStore(st)

	a, err := as.Create(ctx, model.NewAction(tgt.ID, "compile", "go build"))
	if err != nil {
		t.Fatalf("create action: %v", err)
	}
	if _, err := as.Create(ctx, model.NewAction(tgt.ID, "compile", "")); err != model.ErrDuplicateName {
		t.Fatalf("动作重名应拒绝，得到 %v", err)
	}
	d := &model.Declaration{ActionID: a.ID, Path: "src/main.go", Direction: model.DirRead, CreatedAt: a.CreatedAt}
	if err := as.AddDeclaration(ctx, d); err != nil {
		t.Fatalf("add decl: %v", err)
	}
	decls, err := as.DeclarationsByAction(ctx, a.ID)
	if err != nil || len(decls) != 1 {
		t.Fatalf("declarations: %v len=%d", err, len(decls))
	}
	// 依赖边。
	dep := &model.ActionDep{ActionID: a.ID, DependsOn: 1, CreatedAt: a.CreatedAt}
	if err := as.AddDep(ctx, dep); err != nil {
		t.Fatalf("add dep: %v", err)
	}
	deps, err := as.ListDeps(ctx)
	if err != nil || len(deps) != 1 {
		t.Fatalf("deps: %v len=%d", err, len(deps))
	}
}

func TestAppendLogsIdempotentAndConflict(t *testing.T) {
	st := openTestStore(t)
	ls := NewLogStore(st)
	ctx := context.Background()
	tgt, err := NewTargetStore(st).Create(ctx, model.NewTarget("t", ""))
	if err != nil {
		t.Fatal(err)
	}
	act, err := NewActionStore(st).Create(ctx, model.NewAction(tgt.ID, "a", ""))
	if err != nil {
		t.Fatal(err)
	}
	aid := act.ID

	entries := []model.LogEntry{
		{ActionID: aid, Seq: 1, Path: "a", Direction: model.DirRead, ContentHash: "h1", SizeBytes: 10},
		{ActionID: aid, Seq: 2, Path: "b", Direction: model.DirWrite, ContentHash: "h2", SizeBytes: 20},
	}
	n, err := ls.AppendLogs(ctx, entries)
	if err != nil || n != 2 {
		t.Fatalf("append: n=%d err=%v", n, err)
	}
	// 幂等重放：同 seq 同内容 → 0 新增。
	n, err = ls.AppendLogs(ctx, entries)
	if err != nil || n != 0 {
		t.Fatalf("重放应幂等: n=%d err=%v", n, err)
	}
	// 冲突：同 seq 不同内容 → 保留冲突并报错。
	conflict := []model.LogEntry{
		{ActionID: aid, Seq: 2, Path: "b", Direction: model.DirWrite, ContentHash: "h2-CHANGED", SizeBytes: 99},
	}
	if _, err := ls.AppendLogs(ctx, conflict); err == nil {
		t.Fatal("内容冲突应报错")
	}
	// 冲突后原内容仍在。
	all, err := ls.ListAll(ctx)
	if err != nil || len(all) != 2 {
		t.Fatalf("冲突不应覆盖原内容: len=%d err=%v", len(all), err)
	}
	// 产物聚合。
	arts, err := ls.ListArtifacts(ctx)
	if err != nil || len(arts) != 2 {
		t.Fatalf("artifacts: len=%d err=%v", len(arts), err)
	}
	byPath := map[string]*model.Artifact{}
	for _, ar := range arts {
		byPath[ar.Path] = ar
	}
	if byPath["b"].Writer != aid {
		t.Fatalf("b 的写入方应为 action %d: %+v", aid, byPath["b"])
	}
}

func TestAppendLogsBatchConflictAtomic(t *testing.T) {
	st := openTestStore(t)
	ls := NewLogStore(st)
	ctx := context.Background()
	tgt, err := NewTargetStore(st).Create(ctx, model.NewTarget("t", ""))
	if err != nil {
		t.Fatal(err)
	}
	act, err := NewActionStore(st).Create(ctx, model.NewAction(tgt.ID, "a", ""))
	if err != nil {
		t.Fatal(err)
	}
	aid := act.ID

	// 先落 2 条历史日志。
	if _, err := ls.AppendLogs(ctx, []model.LogEntry{
		{ActionID: aid, Seq: 1, Path: "a", Direction: model.DirRead, ContentHash: "h1", SizeBytes: 10},
		{ActionID: aid, Seq: 2, Path: "b", Direction: model.DirWrite, ContentHash: "h2", SizeBytes: 20},
	}); err != nil {
		t.Fatal(err)
	}

	// 一批：前面是全新 seq，最后一条与历史 seq=1 冲突。
	// 整批都不应留下任何新日志（原子回滚），已有日志保持不变。
	batch := []model.LogEntry{
		{ActionID: aid, Seq: 3, Path: "c", Direction: model.DirRead, ContentHash: "h3", SizeBytes: 30},
		{ActionID: aid, Seq: 4, Path: "d", Direction: model.DirWrite, ContentHash: "h4", SizeBytes: 40},
		{ActionID: aid, Seq: 1, Path: "a", Direction: model.DirRead, ContentHash: "h1-CHANGED", SizeBytes: 99},
	}
	n, err := ls.AppendLogs(ctx, batch)
	if err == nil {
		t.Fatal("内容冲突应报错")
	}
	if n != 0 {
		t.Fatalf("冲突批次不应计入新增: n=%d", n)
	}
	// 仅剩历史 2 条，新 seq 3/4 不应残留。
	all, err := ls.ListAll(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		var seqs []int
		for _, lg := range all {
			seqs = append(seqs, lg.Seq)
		}
		t.Fatalf("冲突批次应整批回滚，仅剩历史 2 条，得到 %d: seq=%v", len(all), seqs)
	}
	for _, lg := range all {
		if lg.Seq == 1 && lg.ContentHash != "h1" {
			t.Fatalf("已有 seq=1 内容被破坏: %s", lg.ContentHash)
		}
	}
}

func TestPinnedAndProofPersist(t *testing.T) {
	st := openTestStore(t)
	ctx := context.Background()
	ls := NewLogStore(st)
	ps := NewProofStore(st)
	tgt, err := NewTargetStore(st).Create(ctx, model.NewTarget("t2", ""))
	if err != nil {
		t.Fatal(err)
	}
	act, err := NewActionStore(st).Create(ctx, model.NewAction(tgt.ID, "a", ""))
	if err != nil {
		t.Fatal(err)
	}

	entries := []model.LogEntry{
		{ActionID: act.ID, Seq: 1, Path: "out", Direction: model.DirWrite, ContentHash: "h", SizeBytes: 1},
	}
	if _, err := ls.AppendLogs(ctx, entries); err != nil {
		t.Fatal(err)
	}
	if err := ls.MarkArtifactsPinned(ctx, []string{"out"}); err != nil {
		t.Fatal(err)
	}
	ar, err := ls.ArtifactByPath(ctx, "out")
	if err != nil {
		t.Fatal(err)
	}
	if ar.Status != model.ArtifactPinned {
		t.Fatalf("产物应 pinned: %s", ar.Status)
	}
	// 证明持久化。
	p := model.NewProof(tgt.ID, "g", "l")
	created, err := ps.Create(ctx, p)
	if err != nil || created.ID == 0 {
		t.Fatalf("create proof: %v", err)
	}
	got, err := ps.GetByID(ctx, created.ID)
	if err != nil || got.GraphHash != "g" || got.Status != model.ProofDraft {
		t.Fatalf("get proof: %v %+v", err, got)
	}
	// 基线 + 冻结条目。
	b := &model.Baseline{TargetID: tgt.ID, ProofID: created.ID, Status: model.BaselineActive, FrozenAt: created.CreatedAt}
	items := []*model.BaselineItem{{Path: "out", Hash: "h", SizeBytes: 1}}
	base, err := ps.CreateBaseline(ctx, b, items)
	if err != nil {
		t.Fatalf("create baseline: %v", err)
	}
	activeBase, err := ps.ActiveBaseline(ctx, tgt.ID)
	if err != nil || activeBase.ID != base.ID {
		t.Fatalf("active baseline: %v", err)
	}
	gotItems, err := ps.BaselineItems(ctx, base.ID)
	if err != nil || len(gotItems) != 1 || gotItems[0].Hash != "h" {
		t.Fatalf("baseline items: %v %+v", err, gotItems)
	}
}
