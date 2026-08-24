// Package service 编排 store / graph / declaration / observation / analysis / proof，
// 提供构建图可复现性隔离证明服务的用例级 API。
package service

import (
	"context"
	"fmt"
	"time"

	"task191-reproof/internal/analysis"
	"task191-reproof/internal/declaration"
	"task191-reproof/internal/graph"
	"task191-reproof/internal/model"
	"task191-reproof/internal/observation"
	"task191-reproof/internal/proof"
	"task191-reproof/internal/store"
)

// App 是应用服务入口。
type App struct {
	targets *store.TargetStore
	actions *store.ActionStore
	logs    *store.LogStore
	viols   *store.ViolationStore
	proofs  *store.ProofStore
	an      *analysis.Analyzer
	pg      *proof.Generator
}

// New 构造应用服务。
func New(st *store.Store) *App {
	return &App{
		targets: store.NewTargetStore(st),
		actions: store.NewActionStore(st),
		logs:    store.NewLogStore(st),
		viols:   store.NewViolationStore(st),
		proofs:  store.NewProofStore(st),
		an:      analysis.New(),
		pg:      proof.New(),
	}
}

// --- 目标 ---

// CreateTarget 创建构建目标。
func (a *App) CreateTarget(ctx context.Context, name, description string) (*model.Target, error) {
	return a.targets.Create(ctx, model.NewTarget(name, description))
}

// ListTargets 列出全部目标。
func (a *App) ListTargets(ctx context.Context) ([]*model.Target, error) {
	return a.targets.List(ctx)
}

// GetTarget 读取目标。
func (a *App) GetTarget(ctx context.Context, id int64) (*model.Target, error) {
	return a.targets.GetByID(ctx, id)
}

// AddSeed 登记目标的种子输入（外部提供，无需任何动作生产）。
func (a *App) AddSeed(ctx context.Context, targetID int64, path string) error {
	p, err := model.NormalizePath(path)
	if err != nil {
		return err
	}
	if _, err := a.targets.GetByID(ctx, targetID); err != nil {
		return err
	}
	return a.targets.AddSeed(ctx, targetID, p)
}

// ListSeeds 列出目标的种子输入。
func (a *App) ListSeeds(ctx context.Context, targetID int64) ([]string, error) {
	return a.targets.ListSeeds(ctx, targetID)
}

// --- 动作 ---

// CreateAction 在目标下创建动作。
func (a *App) CreateAction(ctx context.Context, targetID int64, name, command string) (*model.BuildAction, error) {
	if _, err := a.targets.GetByID(ctx, targetID); err != nil {
		return nil, err
	}
	return a.actions.Create(ctx, model.NewAction(targetID, name, command))
}

// ListActions 列出目标下动作。
func (a *App) ListActions(ctx context.Context, targetID int64) ([]*model.BuildAction, error) {
	return a.actions.ListByTarget(ctx, targetID)
}

// GetAction 读取动作。
func (a *App) GetAction(ctx context.Context, id int64) (*model.BuildAction, error) {
	return a.actions.GetByID(ctx, id)
}

// AddDep 添加动作依赖边；校验两端属于同一目标。
func (a *App) AddDep(ctx context.Context, actionID, dependsOn int64) (*model.ActionDep, error) {
	act, err := a.actions.GetByID(ctx, actionID)
	if err != nil {
		return nil, err
	}
	dep, err := a.actions.GetByID(ctx, dependsOn)
	if err != nil {
		return nil, err
	}
	if act.TargetID != dep.TargetID {
		return nil, fmt.Errorf("%w: 跨目标依赖", model.ErrInvalidTransition)
	}
	if actionID == dependsOn {
		return nil, model.ErrCycleDetected
	}
	d := &model.ActionDep{ActionID: actionID, DependsOn: dependsOn, CreatedAt: time.Now().UTC()}
	if err := a.actions.AddDep(ctx, d); err != nil {
		return nil, err
	}
	// 立即检查是否成环。
	if err := a.buildGraph(ctx, act.TargetID).DetectCycle(); err != nil {
		return nil, fmt.Errorf("%w: %v", model.ErrCycleDetected, err)
	}
	return d, nil
}

// AddDeclaration 添加动作输入/输出声明，动作状态推进到 declared。
func (a *App) AddDeclaration(ctx context.Context, actionID int64, path string, dir model.AccessDirection, kind string) (*model.Declaration, error) {
	p, err := model.NormalizePath(path)
	if err != nil {
		return nil, err
	}
	if err := model.ValidateDirection(dir); err != nil {
		return nil, err
	}
	act, err := a.actions.GetByID(ctx, actionID)
	if err != nil {
		return nil, err
	}
	d := &model.Declaration{ActionID: actionID, Path: p, Direction: dir, Kind: kind, CreatedAt: time.Now().UTC()}
	if err := a.actions.AddDeclaration(ctx, d); err != nil {
		return nil, err
	}
	if act.Status == model.ActionPending {
		if err := a.actions.UpdateStatus(ctx, actionID, model.ActionDeclared); err != nil {
			return nil, err
		}
	}
	return d, nil
}

// ListDeclarations 列出目标的全部声明。
func (a *App) ListDeclarations(ctx context.Context, targetID int64) ([]*model.Declaration, error) {
	all, err := a.actions.ListDeclarations(ctx)
	if err != nil {
		return nil, err
	}
	actIDs := map[int64]bool{}
	acts, err := a.actions.ListByTarget(ctx, targetID)
	if err != nil {
		return nil, err
	}
	for _, act := range acts {
		actIDs[act.ID] = true
	}
	var out []*model.Declaration
	for _, d := range all {
		if actIDs[d.ActionID] {
			out = append(out, d)
		}
	}
	return out, nil
}

// AddToolchain 登记动作工具链版本。
func (a *App) AddToolchain(ctx context.Context, actionID int64, name, version, checksum string) (*model.Toolchain, error) {
	t := &model.Toolchain{ActionID: actionID, Name: name, Version: version, Checksum: checksum, CreatedAt: time.Now().UTC()}
	if err := a.actions.AddToolchain(ctx, t); err != nil {
		return nil, err
	}
	return t, nil
}

// --- 观测日志 ---

// AppendLogs 追加访问日志；同 seq 冲突内容保留（返回冲突提示）。
func (a *App) AppendLogs(ctx context.Context, entries []model.LogEntry) (int, error) {
	return a.logs.AppendLogs(ctx, entries)
}

// ListLogs 返回目标全部日志（按动作聚合）。
func (a *App) ListLogs(ctx context.Context, targetID int64) ([]*model.AccessLog, error) {
	acts, err := a.actions.ListByTarget(ctx, targetID)
	if err != nil {
		return nil, err
	}
	all, err := a.logs.ListAll(ctx)
	if err != nil {
		return nil, err
	}
	actIDs := map[int64]bool{}
	for _, act := range acts {
		actIDs[act.ID] = true
	}
	var out []*model.AccessLog
	for _, l := range all {
		if actIDs[l.ActionID] {
			out = append(out, l)
		}
	}
	return out, nil
}

// --- 分析 ---

// AnalyzeTarget 对目标运行隔离性分析并持久化违规与链。
// 返回 (目标新状态, 违规数, 错误)。
func (a *App) AnalyzeTarget(ctx context.Context, targetID int64) (model.TargetStatus, int, error) {
	t, err := a.targets.GetByID(ctx, targetID)
	if err != nil {
		return "", 0, err
	}
	in, err := a.collectInputs(ctx, t)
	if err != nil {
		return "", 0, err
	}
	res := a.an.Analyze(in)

	// 持久化违规与链（先清后写，幂等）。
	violations := res.Violations
	chains := res.Chains
	if err := a.viols.ReplaceForTarget(ctx, targetID, violations, chains); err != nil {
		return "", 0, err
	}
	// 回填链的 violation_id（由存储顺序决定：violations 按生成顺序插入）。
	for i, v := range violations {
		for _, c := range chains {
			if c.ViolationID == 0 && len(c.ActionIDs) > 0 && c.ActionIDs[len(c.ActionIDs)-1] == v.ActionID && c.Length == len(c.ActionIDs) {
				c.ViolationID = v.ID
				_ = i
				break
			}
		}
	}

	// 更新动作状态。
	for actionID, st := range res.ActionStatus {
		_ = a.actions.UpdateStatus(ctx, actionID, st)
	}
	// 判定目标状态。
	status := model.TargetProven
	if len(violations) > 0 {
		status = model.TargetIrreproducible
	} else if !a.allObserved(ctx, in.Graph) {
		status = model.TargetInsufficient
	}
	if err := a.targets.UpdateStatus(ctx, targetID, status); err != nil {
		return "", 0, err
	}
	return status, len(violations), nil
}

// collectInputs 组装分析器输入。
func (a *App) collectInputs(ctx context.Context, t *model.Target) (analysis.Inputs, error) {
	acts, err := a.actions.ListByTarget(ctx, t.ID)
	if err != nil {
		return analysis.Inputs{}, err
	}
	g := graph.New()
	for _, act := range acts {
		g.AddNode(act.ID)
	}
	deps, err := a.actions.ListDeps(ctx)
	if err != nil {
		return analysis.Inputs{}, err
	}
	for _, d := range deps {
		if g.HasNode(d.ActionID) && g.HasNode(d.DependsOn) {
			g.AddEdge(d.DependsOn, d.ActionID)
		}
	}
	decls, err := a.actions.ListDeclarations(ctx)
	if err != nil {
		return analysis.Inputs{}, err
	}
	idx := declaration.BuildIndex(decls)
	entries, err := a.logs.AllLogEntries(ctx)
	if err != nil {
		return analysis.Inputs{}, err
	}
	acc := observation.BuildAccessIndex(entries, nil)
	seeds, err := a.targets.ListSeeds(ctx, t.ID)
	if err != nil {
		return analysis.Inputs{}, err
	}
	seedSet := make(map[string]struct{}, len(seeds))
	for _, s := range seeds {
		seedSet[s] = struct{}{}
	}
	return analysis.Inputs{
		TargetID:  t.ID,
		Graph:     g,
		DeclIndex: idx,
		Reads:     readsMap(acc, acts),
		Writes:    writesMap(acc, acts),
		Tools:     toolsMap(acc, acts),
		// 种子输入：外部提供、无需动作生产（由目标登记）。
		SeedWrites:     seedSet,
		SourceActionID: 0,
	}, nil
}

func readsMap(acc *observation.AccessIndex, acts []*model.BuildAction) map[int64]map[string]struct{} {
	out := make(map[int64]map[string]struct{})
	for _, act := range acts {
		if s := acc.ReadsSet(act.ID); len(s) > 0 {
			out[act.ID] = s
		}
	}
	return out
}

func writesMap(acc *observation.AccessIndex, acts []*model.BuildAction) map[int64]map[string]struct{} {
	out := make(map[int64]map[string]struct{})
	for _, act := range acts {
		if s := acc.WritesSet(act.ID); len(s) > 0 {
			out[act.ID] = s
		}
	}
	return out
}

func toolsMap(acc *observation.AccessIndex, acts []*model.BuildAction) map[int64]map[string]struct{} {
	out := make(map[int64]map[string]struct{})
	for _, act := range acts {
		if s := acc.ToolsUsed(act.ID); len(s) > 0 {
			out[act.ID] = s
		}
	}
	return out
}

// allObserved 判断是否所有动作都有观测日志。
func (a *App) allObserved(ctx context.Context, g *graph.Graph) bool {
	for _, n := range g.Nodes {
		logs, err := a.logs.ListByAction(ctx, n)
		if err != nil || len(logs) == 0 {
			return false
		}
	}
	return true
}

// buildGraph 构建目标 DAG（供环检测等）。
func (a *App) buildGraph(ctx context.Context, targetID int64) *graph.Graph {
	in, err := a.collectInputs(ctx, &model.Target{ID: targetID})
	if err != nil {
		return graph.New()
	}
	return in.Graph
}

// ListViolations 列出目标违规。
func (a *App) ListViolations(ctx context.Context, targetID int64) ([]*model.Violation, error) {
	return a.viols.ListByTarget(ctx, targetID)
}

// ListChains 列出目标违规链（最短）。
func (a *App) ListChains(ctx context.Context, targetID int64) ([]*model.ViolationChain, error) {
	return a.viols.ChainsByTarget(ctx, targetID)
}

// ShortestPollution 返回目标的最短污染链（违规链中最短的一条）。
func (a *App) ShortestPollution(ctx context.Context, targetID int64) (*model.ViolationChain, error) {
	chains, err := a.viols.ChainsByTarget(ctx, targetID)
	if err != nil {
		return nil, err
	}
	if len(chains) == 0 {
		return nil, model.ErrNotFound
	}
	shortest := chains[0]
	for _, c := range chains[1:] {
		if c.Length < shortest.Length {
			shortest = c
		}
	}
	return shortest, nil
}

// --- 证明与基线 ---

// GenerateProof 为目标生成证明；无违规且声明完整才可发布有效证明。
func (a *App) GenerateProof(ctx context.Context, targetID int64) (*model.Proof, error) {
	t, err := a.targets.GetByID(ctx, targetID)
	if err != nil {
		return nil, err
	}
	in, err := a.collectInputs(ctx, t)
	if err != nil {
		return nil, err
	}
	res := a.an.Analyze(in)
	if len(res.Violations) > 0 {
		return nil, model.ErrTargetNotProven
	}
	graphHash := model.GraphDigest(in.Graph.IDs(), in.Graph.Pairs())
	logs, err := a.logs.AllLogEntries(ctx)
	if err != nil {
		return nil, err
	}
	logHash := model.LogFingerprint(logs)
	// 相同图和日志哈希复用结论。
	if reuse, err := a.proofs.FindReusable(ctx, targetID, graphHash, logHash); err == nil {
		return reuse, nil
	}
	p, err := a.pg.MakeDraft(proof.ProofInput{
		TargetID:        targetID,
		GraphHash:       graphHash,
		LogHash:         logHash,
		NoViolations:    true,
		ActionsVerified: a.allObserved(ctx, in.Graph),
	})
	if err != nil {
		return nil, err
	}
	if p.Status == model.ProofDraft {
		// 无违规且全部观测 → 直接有效。
		p.Status = model.ProofValid
	}
	created, err := a.proofs.Create(ctx, p)
	if err != nil {
		return nil, err
	}
	if created.Status == model.ProofValid {
		if err := a.proofs.InvalidateOthers(ctx, targetID, created.ID); err != nil {
			return nil, err
		}
	}
	return created, nil
}

// FreezeBaseline 冻结目标输入哈希为基线；目标须已 proven。
func (a *App) FreezeBaseline(ctx context.Context, targetID int64) (*model.Baseline, error) {
	t, err := a.targets.GetByID(ctx, targetID)
	if err != nil {
		return nil, err
	}
	if t.Status != model.TargetProven && t.Status != model.TargetBaselined {
		return nil, model.ErrTargetNotProven
	}
	p, err := a.proofs.LatestByTarget(ctx, targetID)
	if err != nil {
		if err == model.ErrNotFound {
			return nil, model.ErrTargetNotProven
		}
		return nil, err
	}
	if p.Status != model.ProofValid {
		return nil, model.ErrTargetNotProven
	}
	// 收集全部写入产物最终哈希。
	items, err := a.frozenItems(ctx, targetID)
	if err != nil {
		return nil, err
	}
	b, itemsPtrs := a.pg.Freeze(proof.BaselineInput{
		TargetID:    targetID,
		ProofID:     p.ID,
		FrozenItems: items,
	})
	created, err := a.proofs.CreateBaseline(ctx, b, itemsPtrs)
	if err != nil {
		return nil, err
	}
	// 标记产物已固定。
	var paths []string
	for _, it := range items {
		paths = append(paths, it.Path)
	}
	if err := a.logs.MarkArtifactsPinned(ctx, paths); err != nil {
		return nil, err
	}
	if err := a.targets.UpdateStatus(ctx, targetID, model.TargetBaselined); err != nil {
		return nil, err
	}
	return created, nil
}

// frozenItems 收集目标当前全部写入产物的最终哈希作为基线条目。
// 与 currentHashes 保持一致的口径：凡有写入方的产物即纳入冻结，
// 不依赖 pin 状态（pin 是冻结后的标记，冻结时尚未置位）。
func (a *App) frozenItems(ctx context.Context, targetID int64) ([]model.BaselineItem, error) {
	arts, err := a.logs.ListArtifacts(ctx)
	if err != nil {
		return nil, err
	}
	acts, err := a.actions.ListByTarget(ctx, targetID)
	if err != nil {
		return nil, err
	}
	actIDs := map[int64]bool{}
	for _, act := range acts {
		actIDs[act.ID] = true
	}
	var out []model.BaselineItem
	for _, ar := range arts {
		if ar.Writer == 0 || !actIDs[ar.Writer] {
			continue
		}
		out = append(out, model.BaselineItem{Path: ar.Path, Hash: ar.Hash, SizeBytes: ar.SizeBytes})
	}
	return out, nil
}

// CompareBaseline 比对当前观测与活动基线。
func (a *App) CompareBaseline(ctx context.Context, targetID int64) (*proof.CompareResult, error) {
	base, err := a.proofs.ActiveBaseline(ctx, targetID)
	if err != nil {
		return nil, err
	}
	items, err := a.proofs.BaselineItems(ctx, base.ID)
	if err != nil {
		return nil, err
	}
	current, err := a.currentHashes(ctx, targetID)
	if err != nil {
		return nil, err
	}
	res := a.pg.Compare(current, items)
	res.BaselineID = base.ID
	if !res.Clean {
		// 漂移 → 目标退回 building，证明失效。
		if p, err := a.proofs.GetByID(ctx, base.ProofID); err == nil {
			now := time.Now().UTC()
			_ = a.proofs.UpdateStatus(ctx, p.ID, model.ProofInvalidated, &now)
		}
		_ = a.targets.UpdateStatus(ctx, targetID, model.TargetBuilding)
	}
	return &res, nil
}

// currentHashes 收集目标当前写入产物哈希。
func (a *App) currentHashes(ctx context.Context, targetID int64) (map[string]string, error) {
	arts, err := a.logs.ListArtifacts(ctx)
	if err != nil {
		return nil, err
	}
	acts, err := a.actions.ListByTarget(ctx, targetID)
	if err != nil {
		return nil, err
	}
	actIDs := map[int64]bool{}
	for _, act := range acts {
		actIDs[act.ID] = true
	}
	out := make(map[string]string)
	for _, ar := range arts {
		if ar.Writer == 0 || !actIDs[ar.Writer] {
			continue
		}
		out[ar.Path] = ar.Hash
	}
	return out, nil
}

// ListProofs 列出目标证明。
func (a *App) ListProofs(ctx context.Context, targetID int64) ([]*model.Proof, error) {
	return a.proofs.ListByTarget(ctx, targetID)
}

// GetProof 读取证明。
func (a *App) GetProof(ctx context.Context, id int64) (*model.Proof, error) {
	return a.proofs.GetByID(ctx, id)
}

// InvalidateProof 使证明失效。
func (a *App) InvalidateProof(ctx context.Context, id int64) (*model.Proof, error) {
	p, err := a.proofs.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := p.Validate(model.ProofInvalidated); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	if err := a.proofs.UpdateStatus(ctx, id, model.ProofInvalidated, &now); err != nil {
		return nil, err
	}
	p.Status = model.ProofInvalidated
	p.InvalidatedAt = &now
	return p, nil
}

// ListBaselines 列出目标基线。
func (a *App) ListBaselines(ctx context.Context, targetID int64) ([]*model.Baseline, error) {
	return a.proofs.ListBaselines(ctx, targetID)
}

// Stats 返回统计信息。
func (a *App) Stats(ctx context.Context) (map[string]any, error) {
	targets, err := a.targets.List(ctx)
	if err != nil {
		return nil, err
	}
	allLogs, err := a.logs.ListAll(ctx)
	if err != nil {
		return nil, err
	}
	violCount := 0
	for _, t := range targets {
		vs, err := a.viols.ListByTarget(ctx, t.ID)
		if err != nil {
			return nil, err
		}
		violCount += len(vs)
	}
	return map[string]any{
		"targets":       len(targets),
		"access_logs":   len(allLogs),
		"violations":    violCount,
		"status_counts": statusCounts(targets),
	}, nil
}

func statusCounts(targets []*model.Target) map[string]int {
	out := map[string]int{}
	for _, t := range targets {
		out[string(t.Status)]++
	}
	return out
}
