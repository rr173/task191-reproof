package graph

import (
	"reflect"
	"testing"
)

func TestCycleDetection(t *testing.T) {
	g := New()
	g.AddNode(1)
	g.AddNode(2)
	g.AddNode(3)
	g.AddEdge(1, 2)
	g.AddEdge(2, 3)
	if err := g.DetectCycle(); err != nil {
		t.Fatalf("无环图不应报环: %v", err)
	}
	g.AddEdge(3, 1)
	if err := g.DetectCycle(); err == nil {
		t.Fatal("1->2->3->1 应检测出环")
	}
}

func TestTopoSort(t *testing.T) {
	g := New()
	g.AddNode(1)
	g.AddNode(2)
	g.AddNode(3)
	g.AddEdge(1, 2)
	g.AddEdge(2, 3)
	g.AddEdge(1, 3)
	order, err := g.TopoSort()
	if err != nil {
		t.Fatalf("topo: %v", err)
	}
	// 1 必须先于 2、3；2 必须先于 3。
	pos := map[int64]int{}
	for i, n := range order {
		pos[n] = i
	}
	if pos[1] > pos[2] || pos[2] > pos[3] || pos[1] > pos[3] {
		t.Fatalf("拓扑序违反约束: %v", order)
	}
}

func TestShortestPath(t *testing.T) {
	g := New()
	g.AddNode(1)
	g.AddNode(2)
	g.AddNode(3)
	g.AddNode(4)
	g.AddEdge(1, 2)
	g.AddEdge(1, 3)
	g.AddEdge(2, 4)
	g.AddEdge(3, 4)
	path := g.ShortestPath(1, 4)
	if len(path) != 3 {
		t.Fatalf("期望最短路径 3 节点，得到 %v", path)
	}
	if path[0] != 1 || path[2] != 4 {
		t.Fatalf("路径端点错误: %v", path)
	}
	if g.ShortestPath(4, 1) != nil {
		t.Fatal("反向不可达应为 nil")
	}
}

func TestTransitiveDeps(t *testing.T) {
	g := New()
	g.AddNode(1)
	g.AddNode(2)
	g.AddNode(3)
	g.AddNode(4)
	g.AddEdge(1, 2)
	g.AddEdge(2, 3)
	g.AddEdge(1, 4)
	deps := g.TransitiveDeps(3)
	if !deps[1] || !deps[2] || deps[4] {
		t.Fatalf("传递依赖计算错误: %v", deps)
	}
}

func TestSubgraph(t *testing.T) {
	g := New()
	g.AddNode(1)
	g.AddNode(2)
	g.AddNode(3)
	g.AddEdge(1, 2)
	g.AddEdge(2, 3)
	sg := g.Subgraph([]int64{1, 3})
	if sg.HasEdge(1, 2) || sg.HasEdge(2, 3) {
		t.Fatal("子图不应含被裁剪节点边")
	}
	if !sg.HasNode(1) || !sg.HasNode(3) || sg.NodeCount() != 2 {
		t.Fatal("子图节点错误")
	}
}

func TestReachableFrom(t *testing.T) {
	g := New()
	g.AddNode(1)
	g.AddNode(2)
	g.AddNode(3)
	g.AddEdge(1, 2)
	g.AddEdge(2, 3)
	reach := g.ReachableFrom(1)
	want := []int64{1, 2, 3}
	if !reflect.DeepEqual(reach, want) {
		t.Fatalf("可达集应为 %v，得到 %v", want, reach)
	}
}
