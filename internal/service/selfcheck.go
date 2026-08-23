package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"task191-reproof/internal/model"
	"task191-reproof/internal/store"
)

// SmokeOptions 是自检参数。
type SmokeOptions struct {
	// DBPath 是自检使用的数据库路径；为空则使用临时目录。
	DBPath string
	// KeepDB 是否保留数据库文件（调试用）。
	KeepDB bool
}

// RunSmoke 执行端到端自检：创建目标与动作 DAG、登记声明与工具链、
// 写入访问日志、分析并发现读取泄漏、补齐声明后证明可复现、
// 冻结基线、关闭并重开数据库验证恢复与基线比对。
// 全部通过返回 nil。
func RunSmoke(opts SmokeOptions) error {
	tmpDir, err := os.MkdirTemp("", "reproof-smoke-*")
	if err != nil {
		return fmt.Errorf("make temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	dbPath := opts.DBPath
	if dbPath == "" {
		dbPath = filepath.Join(tmpDir, "smoke.db")
	}
	if err := smokePhase1(dbPath); err != nil {
		return err
	}
	if err := smokePhase2Recovery(dbPath); err != nil {
		return err
	}
	if !opts.KeepDB {
		_ = os.Remove(dbPath)
	}
	return nil
}

// smokePhase1 创建完整闭环并冻结基线。
func smokePhase1(dbPath string) error {
	st, err := store.Open(dbPath)
	if err != nil {
		return fmt.Errorf("open store: %w", err)
	}
	defer st.Close()
	app := New(st)
	ctx := context.Background()

	// 1. 创建目标。
	t, err := app.CreateTarget(ctx, "web-app-release", "前端应用可复现发布")
	if err != nil {
		return fmt.Errorf("create target: %w", err)
	}

	// 2. 创建动作 DAG：fetch -> compile -> bundle -> sign。
	actionNames := []struct{ name, cmd string }{
		{"fetch-deps", "go mod download"},
		{"compile", "go build -o app"},
		{"bundle", "tar czf app.tar.gz app"},
		{"sign", "gpg --detach-sign app.tar.gz"},
	}
	actionIDs := map[string]int64{}
	for _, an := range actionNames {
		a, err := app.CreateAction(ctx, t.ID, an.name, an.cmd)
		if err != nil {
			return fmt.Errorf("create action %s: %w", an.name, err)
		}
		actionIDs[an.name] = a.ID
	}
	// 依赖边：fetch -> compile -> bundle -> sign。
	deps := [][2]string{{"fetch-deps", "compile"}, {"compile", "bundle"}, {"bundle", "sign"}}
	for _, d := range deps {
		if _, err := app.AddDep(ctx, actionIDs[d[1]], actionIDs[d[0]]); err != nil {
			return fmt.Errorf("add dep %v: %w", d, err)
		}
	}

	// 3. 登记种子输入（外部提供：源码、依赖清单、环境文件）。
	for _, seed := range []string{"go.mod", "go.sum", "src/main.go", "env.local"} {
		if err := app.AddSeed(ctx, t.ID, seed); err != nil {
			return fmt.Errorf("add seed %s: %w", seed, err)
		}
	}

	// 4. 声明输入/输出。
	decls := []struct {
		action string
		path   string
		dir    model.AccessDirection
	}{
		{"fetch-deps", "go.mod", model.DirRead},
		{"fetch-deps", "go.sum", model.DirRead},
		{"fetch-deps", "deps.lock", model.DirWrite},
		{"compile", "deps.lock", model.DirRead},
		{"compile", "src/main.go", model.DirRead},
		{"compile", "app", model.DirWrite},
		{"bundle", "app", model.DirRead},
		{"bundle", "app.tar.gz", model.DirWrite},
		{"sign", "app.tar.gz", model.DirRead},
		{"sign", "app.tar.gz.sig", model.DirWrite},
	}
	for _, d := range decls {
		if _, err := app.AddDeclaration(ctx, actionIDs[d.action], d.path, d.dir, ""); err != nil {
			return fmt.Errorf("add declaration %v: %w", d, err)
		}
	}

	// 4. 登记工具链。
	if _, err := app.AddToolchain(ctx, actionIDs["compile"], "go", "1.26.3", "hash-go"); err != nil {
		return fmt.Errorf("add toolchain: %w", err)
	}

	// 5. 写入访问日志（compile 动作偷偷读取未声明的 env.local → 读取泄漏）。
	logs := []model.LogEntry{
		{ActionID: actionIDs["fetch-deps"], Seq: 1, Path: "go.mod", Direction: model.DirRead, ContentHash: "h-gomod", SizeBytes: 120},
		{ActionID: actionIDs["fetch-deps"], Seq: 2, Path: "go.sum", Direction: model.DirRead, ContentHash: "h-gosum", SizeBytes: 3200},
		{ActionID: actionIDs["fetch-deps"], Seq: 3, Path: "deps.lock", Direction: model.DirWrite, ContentHash: "h-lock", SizeBytes: 88},
		{ActionID: actionIDs["compile"], Seq: 1, Path: "deps.lock", Direction: model.DirRead, ContentHash: "h-lock", SizeBytes: 88},
		{ActionID: actionIDs["compile"], Seq: 2, Path: "src/main.go", Direction: model.DirRead, ContentHash: "h-main", SizeBytes: 2048},
		{ActionID: actionIDs["compile"], Seq: 3, Path: "env.local", Direction: model.DirRead, ContentHash: "h-env", SizeBytes: 64}, // 未声明！
		{ActionID: actionIDs["compile"], Seq: 4, Path: "app", Direction: model.DirWrite, ContentHash: "h-app", SizeBytes: 5120},
		{ActionID: actionIDs["bundle"], Seq: 1, Path: "app", Direction: model.DirRead, ContentHash: "h-app", SizeBytes: 5120},
		{ActionID: actionIDs["bundle"], Seq: 2, Path: "app.tar.gz", Direction: model.DirWrite, ContentHash: "h-tgz", SizeBytes: 2048},
		{ActionID: actionIDs["sign"], Seq: 1, Path: "app.tar.gz", Direction: model.DirRead, ContentHash: "h-tgz", SizeBytes: 2048},
		{ActionID: actionIDs["sign"], Seq: 2, Path: "app.tar.gz.sig", Direction: model.DirWrite, ContentHash: "h-sig", SizeBytes: 512},
	}
	if _, err := app.AppendLogs(ctx, logs); err != nil {
		return fmt.Errorf("append logs: %w", err)
	}

	// 6. 分析：应发现读取泄漏 env.local。
	status, nViol, err := app.AnalyzeTarget(ctx, t.ID)
	if err != nil {
		return fmt.Errorf("analyze: %w", err)
	}
	if status != model.TargetIrreproducible {
		return fmt.Errorf("期望不可复现，得到 %s", status)
	}
	if nViol < 1 {
		return fmt.Errorf("期望至少 1 个违规，得到 %d", nViol)
	}
	chains, err := app.ListChains(ctx, t.ID)
	if err != nil {
		return fmt.Errorf("list chains: %w", err)
	}
	if len(chains) == 0 {
		return fmt.Errorf("期望违规链存在")
	}
	// 7. 补齐声明后重新分析 → proven。
	if _, err := app.AddDeclaration(ctx, actionIDs["compile"], "env.local", model.DirRead, ""); err != nil {
		return fmt.Errorf("补齐声明: %w", err)
	}
	status, nViol, err = app.AnalyzeTarget(ctx, t.ID)
	if err != nil {
		return fmt.Errorf("re-analyze: %w", err)
	}
	if status != model.TargetProven || nViol != 0 {
		return fmt.Errorf("补齐后应可证明，得到 status=%s viol=%d", status, nViol)
	}

	// 8. 生成证明。
	p, err := app.GenerateProof(ctx, t.ID)
	if err != nil {
		return fmt.Errorf("generate proof: %w", err)
	}
	if p.Status != model.ProofValid {
		return fmt.Errorf("证明应为 valid，得到 %s", p.Status)
	}

	// 9. 冻结基线。
	base, err := app.FreezeBaseline(ctx, t.ID)
	if err != nil {
		return fmt.Errorf("freeze baseline: %w", err)
	}
	tAfter, err := app.GetTarget(ctx, t.ID)
	if err != nil {
		return err
	}
	if tAfter.Status != model.TargetBaselined {
		return fmt.Errorf("目标应为 baselined，得到 %s", tAfter.Status)
	}
	_ = base
	return nil
}

// smokePhase2Recovery 关闭并重开同一数据库，验证恢复与基线比对。
func smokePhase2Recovery(dbPath string) error {
	st, err := store.Open(dbPath)
	if err != nil {
		return fmt.Errorf("重开 store: %w", err)
	}
	defer st.Close()
	app := New(st)
	ctx := context.Background()

	// 1. 目标与状态恢复。
	t, err := app.GetTarget(ctx, 1)
	if err != nil {
		return fmt.Errorf("恢复目标: %w", err)
	}
	if t.Status != model.TargetBaselined {
		return fmt.Errorf("重启后目标状态应为 baselined，得到 %s", t.Status)
	}
	// 2. 动作状态恢复。
	acts, err := app.ListActions(ctx, t.ID)
	if err != nil {
		return fmt.Errorf("恢复动作: %w", err)
	}
	if len(acts) != 4 {
		return fmt.Errorf("期望 4 个动作，得到 %d", len(acts))
	}
	// 3. 证明恢复。
	proofs, err := app.ListProofs(ctx, t.ID)
	if err != nil {
		return fmt.Errorf("恢复证明: %w", err)
	}
	if len(proofs) != 1 || proofs[0].Status != model.ProofValid {
		return fmt.Errorf("期望 1 个有效证明，得到 %d", len(proofs))
	}
	// 4. 基线比对：当前观测与基线一致。
	cmp, err := app.CompareBaseline(ctx, t.ID)
	if err != nil {
		return fmt.Errorf("基线比对: %w", err)
	}
	if !cmp.Clean {
		return fmt.Errorf("重启后基线应一致，得到 %s", cmp.Detail)
	}
	// 5. 相同图和日志哈希复用结论：再次生成证明返回同一证明。
	reuse, err := app.GenerateProof(ctx, t.ID)
	if err != nil {
		return fmt.Errorf("复用证明: %w", err)
	}
	if reuse.ID != proofs[0].ID {
		return fmt.Errorf("期望复用既有证明，得到新证明 %d", reuse.ID)
	}
	// 6. 变更产物哈希后比对应漂移：bundle 以不同哈希重写 app.tar.gz。
	acts2, err := app.ListActions(ctx, t.ID)
	if err != nil {
		return fmt.Errorf("恢复动作2: %w", err)
	}
	var bundleID int64
	for _, a := range acts2 {
		if a.Name == "bundle" {
			bundleID = a.ID
		}
	}
	if bundleID == 0 {
		return fmt.Errorf("未找到 bundle 动作")
	}
	driftLogs := []model.LogEntry{
		{ActionID: bundleID, Seq: 3, Path: "app.tar.gz", Direction: model.DirWrite, ContentHash: "h-tgz-drift", SizeBytes: 2100},
	}
	if _, err := app.AppendLogs(ctx, driftLogs); err != nil {
		return fmt.Errorf("追加漂移日志: %w", err)
	}
	cmp2, err := app.CompareBaseline(ctx, t.ID)
	if err != nil {
		return fmt.Errorf("漂移比对: %w", err)
	}
	if cmp2.Clean {
		return fmt.Errorf("产物哈希变化后基线应漂移")
	}
	// 漂移后目标退回 building，证明失效。
	t2, err := app.GetTarget(ctx, t.ID)
	if err != nil {
		return err
	}
	if t2.Status != model.TargetBuilding {
		return fmt.Errorf("漂移后目标应退回 building，得到 %s", t2.Status)
	}
	proofs2, err := app.ListProofs(ctx, t.ID)
	if err != nil {
		return err
	}
	if len(proofs2) == 0 || proofs2[0].Status != model.ProofInvalidated {
		return fmt.Errorf("漂移后证明应失效")
	}
	return nil
}
