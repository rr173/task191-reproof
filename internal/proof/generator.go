// Package proof 生成可复现性证明、冻结基线与基线比对。
package proof

import (
	"fmt"
	"sort"
	"time"

	"task191-reproof/internal/model"
)

// Generator 生成证明与基线。
type Generator struct{}

// New 构造证明生成器。
func New() *Generator { return &Generator{} }

// ProofInput 是生成证明所需的输入。
type ProofInput struct {
	TargetID int64
	// GraphHash 是图摘要（含动作与依赖边）。
	GraphHash string
	// LogHash 是观测日志摘要。
	LogHash string
	// NoViolations 表示分析未发现违规。
	NoViolations bool
	// ActionsVerified 表示全部动作已声明完整。
	ActionsVerified bool
}

// MakeDraft 构造草案证明；仅当无违规且声明完整时才能变为有效。
func (g *Generator) MakeDraft(in ProofInput) (*model.Proof, error) {
	p := model.NewProof(in.TargetID, in.GraphHash, in.LogHash)
	if !in.NoViolations {
		return nil, model.ErrTargetNotProven
	}
	if !in.ActionsVerified {
		return nil, model.ErrDeclarationMissing
	}
	return p, nil
}

// BaselineInput 是冻结基线的输入。
type BaselineInput struct {
	TargetID int64
	ProofID  int64
	// FrozenItems 是冻结的输入路径哈希（写入产物的最终哈希）。
	FrozenItems []model.BaselineItem
}

// Freeze 构造基线（active 状态）。
func (g *Generator) Freeze(in BaselineInput) (*model.Baseline, []*model.BaselineItem) {
	b := &model.Baseline{
		TargetID: in.TargetID,
		ProofID:  in.ProofID,
		Status:   model.BaselineActive,
		FrozenAt: time.Now().UTC(),
	}
	items := append([]*model.BaselineItem(nil), toPtrs(in.FrozenItems)...)
	sort.Slice(items, func(i, j int) bool { return items[i].Path < items[j].Path })
	return b, items
}

func toPtrs(items []model.BaselineItem) []*model.BaselineItem {
	out := make([]*model.BaselineItem, 0, len(items))
	for i := range items {
		it := items[i]
		out = append(out, &it)
	}
	return out
}

// CompareResult 是基线比对的结果。
type CompareResult struct {
	BaselineID int64
	// Drifted 是哈希与基线不一致的路径。
	Drifted []string
	// Missing 是基线中存在但当前不再出现的路径。
	Missing []string
	// New 是当前出现但基线中不存在的路径。
	New []string
	// Clean 表示完全一致。
	Clean bool
	// Detail 汇总文本。
	Detail string
}

// Compare 将当前观测产物哈希与基线条目比对。
// 当前观测以 map[path]hash 传入；基线条目按 path 索引。
func (g *Generator) Compare(current map[string]string, baseline []*model.BaselineItem) CompareResult {
	base := make(map[string]string, len(baseline))
	for _, it := range baseline {
		base[it.Path] = it.Hash
	}
	res := CompareResult{Clean: true}
	if len(baseline) > 0 {
		res.BaselineID = baseline[0].BaselineID
	}
	var paths []string
	for p := range base {
		paths = append(paths, p)
	}
	for p := range current {
		if _, ok := base[p]; !ok {
			paths = append(paths, p)
		}
	}
	sort.Strings(paths)
	var b []string
	for _, p := range paths {
		bh, inBase := base[p]
		ch, inCur := current[p]
		switch {
		case inBase && !inCur:
			res.Missing = append(res.Missing, p)
			res.Clean = false
		case !inBase && inCur:
			res.New = append(res.New, p)
			res.Clean = false
		case inBase && inCur && bh != ch:
			res.Drifted = append(res.Drifted, p)
			res.Clean = false
		}
		_ = b
	}
	res.Detail = summarize(res)
	return res
}

func summarize(r CompareResult) string {
	if r.Clean {
		return "基线一致：全部输入哈希与基线匹配"
	}
	return fmt.Sprintf("基线漂移：%d 个路径哈希变化，%d 个新增，%d 个缺失",
		len(r.Drifted), len(r.New), len(r.Missing))
}
