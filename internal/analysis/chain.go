package analysis

import (
	"sort"

	"task191-reproof/internal/graph"
	"task191-reproof/internal/model"
)

// shortestChain 计算从 source 到 actionID 的最短动作链。
// source<=0 时以图内 ID 最小的动作作为起点（全局最小违规链）。
// 返回从源动作到违规动作的最短路径（边数最少的动作序列）；
// source 不可达 actionID 时退化为仅含违规动作的单点链。
func (a *Analyzer) shortestChain(g *graph.Graph, source, actionID int64) []int64 {
	if actionID == 0 {
		return nil
	}
	if source <= 0 {
		nodes := g.IDs()
		if len(nodes) == 0 {
			return []int64{actionID}
		}
		source = nodes[0]
	}
	if path := g.ShortestPath(source, actionID); path != nil {
		return path
	}
	return []int64{actionID}
}

// toolDeclared 判断动作是否声明了工具链（kind=toolchain 的声明）。
func toolDeclared(in Inputs, actionID int64, tool string) bool {
	return in.DeclIndex.Declared(actionID, tool, model.DirRead) ||
		in.DeclIndex.Declared(actionID, "toolchain:"+tool, model.DirWrite)
}

func sorted(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sortedPaths(m map[string][]int64) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func contains(list []int64, v int64) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}
