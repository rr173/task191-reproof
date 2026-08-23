package analysis

import (
	"sort"

	"task191-reproof/internal/model"
)

// detectWriteConflicts 找出两个动作无序写同一路径的冲突。
// 判定：存在写者 w1 != w2 且 w1 不能到达 w2、w2 不能到达 w1（无顺序关系）。
func (a *Analyzer) detectWriteConflicts(in Inputs) []*model.Violation {
	writersByPath := make(map[string][]int64)
	for actionID, paths := range in.Writes {
		for p := range paths {
			writersByPath[p] = append(writersByPath[p], actionID)
		}
	}
	var out []*model.Violation
	for _, path := range sortedPaths(writersByPath) {
		ws := writersByPath[path]
		if len(ws) < 2 {
			continue
		}
		sort.Slice(ws, func(i, j int) bool { return ws[i] < ws[j] })
		for i := 0; i < len(ws); i++ {
			for j := i + 1; j < len(ws); j++ {
				w1, w2 := ws[i], ws[j]
				reach1 := in.Graph.ReachableFrom(w1)
				reach2 := in.Graph.ReachableFrom(w2)
				hasOrder := contains(reach1, w2) && contains(reach2, w1)
				if !hasOrder {
					out = append(out, &model.Violation{
						TargetID: in.TargetID, ActionID: w2, Kind: model.ViolationWriteConflict,
						Path: path, Detail: "无序写冲突: " + path,
					})
				}
			}
		}
	}
	return out
}
