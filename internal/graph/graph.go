// Package graph 维护构建动作 DAG：建图、环检测、拓扑排序与最短路径。
package graph

import (
	"fmt"
	"sort"

	"task191-reproof/internal/model"
)

// Graph 表示动作依赖图（邻接表）。
type Graph struct {
	// Nodes 是全部动作 ID。
	Nodes []int64
	// Edges 记录 from -> to 的有向边（to 依赖 from，from 先执行）。
	Edges map[int64][]int64
	// nodeSet 用于 O(1) 判断节点存在。
	nodeSet map[int64]struct{}
}

// New 构造空图。
func New() *Graph {
	return &Graph{
		Edges:   make(map[int64][]int64),
		nodeSet: make(map[int64]struct{}),
	}
}

// AddNode 添加节点；重复添加无副作用。
func (g *Graph) AddNode(id int64) {
	if _, ok := g.nodeSet[id]; ok {
		return
	}
	g.nodeSet[id] = struct{}{}
	g.Nodes = append(g.Nodes, id)
}

// AddEdge 添加边 from -> to（from 先于 to 执行）。
func (g *Graph) AddEdge(from, to int64) {
	for _, t := range g.Edges[from] {
		if t == to {
			return
		}
	}
	g.Edges[from] = append(g.Edges[from], to)
}

// HasNode 判断节点是否存在。
func (g *Graph) HasNode(id int64) bool {
	_, ok := g.nodeSet[id]
	return ok
}

// NodeCount 返回节点数。
func (g *Graph) NodeCount() int { return len(g.Nodes) }

// EdgeCount 返回边数。
func (g *Graph) EdgeCount() int {
	n := 0
	for _, tos := range g.Edges {
		n += len(tos)
	}
	return n
}

// HasEdge 判断边是否存在。
func (g *Graph) HasEdge(from, to int64) bool {
	for _, t := range g.Edges[from] {
		if t == to {
			return true
		}
	}
	return false
}

// Cycle 表示检测到的依赖环。
type Cycle struct {
	Path []int64
}

// Error 实现 error 接口。
func (c *Cycle) Error() string {
	return fmt.Sprintf("dependency cycle: %v", c.Path)
}

// DetectCycle 使用三色 DFS 检测环；无环返回 nil，有环返回第一个环的路径。
func (g *Graph) DetectCycle() error {
	const (
		white = 0
		gray  = 1
		black = 2
	)
	color := make(map[int64]int, len(g.Nodes))
	stack := []int64{}
	var dfs func(n int64) *Cycle
	dfs = func(n int64) *Cycle {
		color[n] = gray
		stack = append(stack, n)
		// 稳定遍历邻接（升序），保证可复现。
		neighbors := append([]int64(nil), g.Edges[n]...)
		sort.Slice(neighbors, func(i, j int) bool { return neighbors[i] < neighbors[j] })
		for _, m := range neighbors {
			switch color[m] {
			case gray:
				// 找到环：从 m 开始截取 stack。
				idx := -1
				for i, v := range stack {
					if v == m {
						idx = i
						break
					}
				}
				cyclePath := append([]int64(nil), stack[idx:]...)
				cyclePath = append(cyclePath, m)
				return &Cycle{Path: cyclePath}
			case white:
				if c := dfs(m); c != nil {
					return c
				}
			}
		}
		stack = stack[:len(stack)-1]
		color[n] = black
		return nil
	}
	nodes := append([]int64(nil), g.Nodes...)
	sort.Slice(nodes, func(i, j int) bool { return nodes[i] < nodes[j] })
	for _, n := range nodes {
		if color[n] == white {
			if c := dfs(n); c != nil {
				return c
			}
		}
	}
	return nil
}

// TopoSort 返回拓扑序（升序稳定）。图有环时返回错误。
func (g *Graph) TopoSort() ([]int64, error) {
	if err := g.DetectCycle(); err != nil {
		return nil, err
	}
	indeg := make(map[int64]int, len(g.Nodes))
	for _, n := range g.Nodes {
		indeg[n] = 0
	}
	for _, tos := range g.Edges {
		for _, to := range tos {
			indeg[to]++
		}
	}
	var queue []int64
	for _, n := range g.Nodes {
		if indeg[n] == 0 {
			queue = append(queue, n)
		}
	}
	sort.Slice(queue, func(i, j int) bool { return queue[i] < queue[j] })
	var order []int64
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		order = append(order, n)
		neighbors := append([]int64(nil), g.Edges[n]...)
		sort.Slice(neighbors, func(i, j int) bool { return neighbors[i] < neighbors[j] })
		for _, m := range neighbors {
			indeg[m]--
			if indeg[m] == 0 {
				queue = append(queue, m)
				sort.Slice(queue, func(i, j int) bool { return queue[i] < queue[j] })
			}
		}
	}
	if len(order) != len(g.Nodes) {
		return nil, &Cycle{Path: order}
	}
	return order, nil
}

// ShortestPath 计算从 from 到 to 的最短路径（边数最少）；不可达返回 nil。
func (g *Graph) ShortestPath(from, to int64) []int64 {
	if from == to {
		return []int64{from}
	}
	if !g.HasNode(from) || !g.HasNode(to) {
		return nil
	}
	prev := make(map[int64]int64)
	visited := map[int64]bool{from: true}
	queue := []int64{from}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		neighbors := append([]int64(nil), g.Edges[cur]...)
		sort.Slice(neighbors, func(i, j int) bool { return neighbors[i] < neighbors[j] })
		for _, nb := range neighbors {
			if visited[nb] {
				continue
			}
			visited[nb] = true
			prev[nb] = cur
			if nb == to {
				// 回溯构造路径。
				var path []int64
				for p := to; ; p = prev[p] {
					path = append([]int64{p}, path...)
					if p == from {
						break
					}
				}
				return path
			}
			queue = append(queue, nb)
		}
	}
	return nil
}

// ReachableFrom 返回从 start 出发可到达的全部节点（含自身）。
// 仅沿有向边传播；没有顺序关系的节点不应被误判为可达，
// 否则会令无序写冲突检测（detectWriteConflicts）漏报。
func (g *Graph) ReachableFrom(start int64) []int64 {
	if !g.HasNode(start) {
		return nil
	}
	visited := map[int64]bool{start: true}
	queue := []int64{start}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, nb := range g.Edges[cur] {
			if !visited[nb] {
				visited[nb] = true
				queue = append(queue, nb)
			}
		}
	}
	var out []int64
	for n := range visited {
		out = append(out, n)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// TransitiveDeps 返回节点 n 的全部传递依赖（先于 n 执行的动作集合，不含 n）。
func (g *Graph) TransitiveDeps(n int64) map[int64]bool {
	result := make(map[int64]bool)
	var visit func(cur int64)
	visit = func(cur int64) {
		for _, pred := range g.Predecessors(cur) {
			if !result[pred] {
				result[pred] = true
				visit(pred)
			}
		}
	}
	visit(n)
	return result
}

// Predecessors 返回节点的直接前驱（依赖方：pred -> n）。
func (g *Graph) Predecessors(n int64) []int64 {
	var out []int64
	for from, tos := range g.Edges {
		for _, to := range tos {
			if to == n {
				out = append(out, from)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

// Subgraph 提取只含指定节点集合的子图。
func (g *Graph) Subgraph(ids []int64) *Graph {
	set := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		set[id] = struct{}{}
	}
	sg := New()
	for _, id := range ids {
		sg.AddNode(id)
	}
	for _, from := range ids {
		for _, to := range g.Edges[from] {
			if _, ok := set[to]; ok {
				sg.AddEdge(from, to)
			}
		}
	}
	return sg
}

// Pairs 导出全部边对（供指纹计算）。
func (g *Graph) Pairs() []model.ActionPair {
	var pairs []model.ActionPair
	var froms []int64
	for from := range g.Edges {
		froms = append(froms, from)
	}
	sort.Slice(froms, func(i, j int) bool { return froms[i] < froms[j] })
	for _, from := range froms {
		tos := append([]int64(nil), g.Edges[from]...)
		sort.Slice(tos, func(i, j int) bool { return tos[i] < tos[j] })
		for _, to := range tos {
			pairs = append(pairs, model.ActionPair{From: from, To: to})
		}
	}
	return pairs
}

// IDs 返回全部节点 ID。
func (g *Graph) IDs() []int64 {
	out := append([]int64(nil), g.Nodes...)
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
