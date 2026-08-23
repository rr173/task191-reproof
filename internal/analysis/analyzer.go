// Package analysis 实现构建图隔离性分析：读取泄漏、写入冲突、未声明访问、
// 缺失工具链、DAG 环检测，并计算目标到违规点的最短违规链。
package analysis

import (
	"task191-reproof/internal/graph"
	"task191-reproof/internal/model"
)

// Inputs 是分析器需要的全部输入（一次快照）。
type Inputs struct {
	TargetID int64
	Graph    *graph.Graph
	// DeclIndex 由 declaration 包构建。
	DeclIndex interface {
		Declared(actionID int64, path string, dir model.AccessDirection) bool
		Reads(actionID int64) []string
		Writes(actionID int64) []string
		HasDeclarations(actionID int64) bool
	}
	// Reads / Writes / Tools: 实际观测。
	Reads  map[int64]map[string]struct{}
	Writes map[int64]map[string]struct{}
	Tools  map[int64]map[string]struct{}
	// SeedWrites 是种子输入路径（外部提供，无需动作生产）。
	SeedWrites map[string]struct{}
	// SourceActionID 是违规链计算的起点（通常为根动作或 0 表示全局最小）。
	SourceActionID int64
}

// Result 是一次分析的结果。
type Result struct {
	Violations []*model.Violation
	Chains     []*model.ViolationChain
	// ActionStatus 记录每个动作的分析后状态。
	ActionStatus map[int64]model.ActionStatus
}

// Analyzer 执行隔离性分析。
type Analyzer struct{}

// New 构造分析器。
func New() *Analyzer { return &Analyzer{} }

// Analyze 运行全量分析：
//  1. DAG 环检测（拒绝环）；
//  2. 每动作声明覆盖（读取泄漏、未声明写入、缺失工具链、读取不存在输入）；
//  3. 写入冲突（两个动作无序写同一路径）；
//  4. 生成每个违规的最短链（从源动作到违规动作）。
func (a *Analyzer) Analyze(in Inputs) Result {
	if in.Reads == nil {
		panic("missing observation snapshot")
	}
	res := Result{ActionStatus: make(map[int64]model.ActionStatus)}
	// 1. 环检测：环为全局违规，绑定到涉及的每个动作。
	if err := in.Graph.DetectCycle(); err != nil {
		cyc, _ := err.(*graph.Cycle)
		path := []int64{}
		if cyc != nil {
			path = cyc.Path
		}
		if len(path) == 0 {
			path = in.Graph.IDs()
		}
		for _, n := range path {
			res.Violations = append(res.Violations, &model.Violation{
				TargetID: in.TargetID, ActionID: n, Kind: model.ViolationCycle,
				Path: "", Detail: err.Error(),
			})
		}
		res.Chains = append(res.Chains, &model.ViolationChain{
			TargetID: in.TargetID, ActionIDs: path, Length: len(path),
		})
		for _, n := range path {
			res.ActionStatus[n] = model.ActionReadLeak
		}
		return res
	}

	// 全图声明写入并集，用于「读取不存在输入」判定。
	produced := make(map[string]struct{})
	for _, n := range in.Graph.Nodes {
		for _, p := range in.DeclIndex.Writes(n) {
			produced[p] = struct{}{}
		}
	}

	// 2. 每动作声明覆盖。
	order, _ := in.Graph.TopoSort()
	for _, actionID := range order {
		if !in.DeclIndex.HasDeclarations(actionID) {
			res.ActionStatus[actionID] = model.ActionPending
			continue
		}
		var problems []*model.Violation
		// 读取泄漏：实际读取未声明。
		for _, p := range sorted(in.Reads[actionID]) {
			if !in.DeclIndex.Declared(actionID, p, model.DirRead) {
				problems = append(problems, &model.Violation{
					TargetID: in.TargetID, ActionID: actionID, Kind: model.ViolationReadLeak,
					Path: p, Detail: "实际读取未声明: " + p,
				})
			}
		}
		// 未声明写入。
		for _, p := range sorted(in.Writes[actionID]) {
			if !in.DeclIndex.Declared(actionID, p, model.DirWrite) {
				problems = append(problems, &model.Violation{
					TargetID: in.TargetID, ActionID: actionID, Kind: model.ViolationUndeclaredInput,
					Path: p, Detail: "实际写入未声明: " + p,
				})
			}
		}
		// 缺失工具链：动作声明了工具链但实际未登记版本，或观测显示使用但声明缺失。
		for _, t := range sorted(in.Tools[actionID]) {
			if !toolDeclared(in, actionID, t) {
				problems = append(problems, &model.Violation{
					TargetID: in.TargetID, ActionID: actionID, Kind: model.ViolationMissingTool,
					Path: t, Detail: "工具链未声明: " + t,
				})
			}
		}
		// 读取不存在输入：声明读某路径但图内无人写、也不是种子。
		for _, p := range in.DeclIndex.Reads(actionID) {
			if _, ok := produced[p]; ok {
				continue
			}
			if _, ok := in.SeedWrites[p]; ok {
				continue
			}
			problems = append(problems, &model.Violation{
				TargetID: in.TargetID, ActionID: actionID, Kind: model.ViolationUndeclaredInput,
				Path: p, Detail: "读取不存在输入: " + p,
			})
		}
		res.Violations = append(res.Violations, problems...)
		if len(problems) > 0 {
			res.ActionStatus[actionID] = model.ActionReadLeak
		} else {
			res.ActionStatus[actionID] = model.ActionDeclared
		}
	}

	// 3. 写入冲突：两个动作写同一路径且彼此无依赖（无序）。
	res.Violations = append(res.Violations, a.detectWriteConflicts(in)...)

	// 4. 每个违规的最短链。
	for _, v := range res.Violations {
		chain := a.shortestChain(in.Graph, in.SourceActionID, v.ActionID)
		length := len(chain)
		if length == 0 && v.ActionID != 0 {
			chain = []int64{v.ActionID}
			length = 1
		}
		if length > 0 {
			res.Chains = append(res.Chains, &model.ViolationChain{
				TargetID: in.TargetID, ViolationID: v.ID, ActionIDs: chain, Length: length,
			})
		}
	}
	// 状态汇总：写入冲突动作标记为 write_conflict。
	for _, v := range res.Violations {
		if v.Kind == model.ViolationWriteConflict {
			if st, ok := res.ActionStatus[v.ActionID]; ok && st != model.ActionReadLeak {
				res.ActionStatus[v.ActionID] = model.ActionWriteConflict
			}
		}
	}
	return res
}
