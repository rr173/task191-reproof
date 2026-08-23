package analysis

import (
	"testing"

	"task191-reproof/internal/declaration"
	"task191-reproof/internal/graph"
	"task191-reproof/internal/model"
)

type fakeDecl struct {
	byAction map[int64][]*model.Declaration
}

func (f *fakeDecl) Declared(actionID int64, path string, dir model.AccessDirection) bool {
	for _, d := range f.byAction[actionID] {
		if d.Path == path && d.Direction == dir {
			return true
		}
	}
	return false
}
func (f *fakeDecl) Reads(actionID int64) []string {
	var out []string
	for _, d := range f.byAction[actionID] {
		if d.Direction == model.DirRead {
			out = append(out, d.Path)
		}
	}
	return out
}
func (f *fakeDecl) Writes(actionID int64) []string {
	var out []string
	for _, d := range f.byAction[actionID] {
		if d.Direction == model.DirWrite {
			out = append(out, d.Path)
		}
	}
	return out
}
func (f *fakeDecl) HasDeclarations(actionID int64) bool { return len(f.byAction[actionID]) > 0 }

func buildGraph(pairs [][2]int64) *graph.Graph {
	g := graph.New()
	seen := map[int64]bool{}
	for _, p := range pairs {
		if !seen[p[0]] {
			g.AddNode(p[0])
			seen[p[0]] = true
		}
		if !seen[p[1]] {
			g.AddNode(p[1])
			seen[p[1]] = true
		}
		g.AddEdge(p[0], p[1])
	}
	return g
}

func TestReadLeakDetected(t *testing.T) {
	g := buildGraph([][2]int64{{1, 2}})
	fd := &fakeDecl{byAction: map[int64][]*model.Declaration{
		1: {{Path: "a", Direction: model.DirRead}, {Path: "b", Direction: model.DirWrite}},
		2: {{Path: "b", Direction: model.DirRead}, {Path: "c", Direction: model.DirWrite}},
	}}
	in := Inputs{
		TargetID:  1,
		Graph:     g,
		DeclIndex: fd,
		Reads:     map[int64]map[string]struct{}{1: {"a": {}}, 2: {"b": {}, "secret.env": {}}},
		Writes:    map[int64]map[string]struct{}{1: {"b": {}}, 2: {"c": {}}},
		Tools:     map[int64]map[string]struct{}{},
	}
	res := New().Analyze(in)
	found := false
	for _, v := range res.Violations {
		if v.Kind == model.ViolationReadLeak && v.Path == "secret.env" {
			found = true
		}
	}
	if !found {
		t.Fatalf("应检测到读取泄漏 secret.env: %+v", res.Violations)
	}
	// 应有违规链。
	if len(res.Chains) == 0 {
		t.Fatal("应有违规链")
	}
}

func TestWriteConflictUnordered(t *testing.T) {
	// 1 -> 3, 2 -> 3；1 和 2 都写 same.out 且互不依赖 → 冲突。
	g := buildGraph([][2]int64{{1, 3}, {2, 3}})
	fd := &fakeDecl{byAction: map[int64][]*model.Declaration{
		1: {{Path: "same.out", Direction: model.DirWrite}},
		2: {{Path: "same.out", Direction: model.DirWrite}},
		3: {{Path: "same.out", Direction: model.DirRead}},
	}}
	in := Inputs{
		TargetID:  1,
		Graph:     g,
		DeclIndex: fd,
		Reads:     map[int64]map[string]struct{}{3: {"same.out": {}}},
		Writes:    map[int64]map[string]struct{}{1: {"same.out": {}}, 2: {"same.out": {}}},
		Tools:     map[int64]map[string]struct{}{},
	}
	res := New().Analyze(in)
	conflicts := 0
	for _, v := range res.Violations {
		if v.Kind == model.ViolationWriteConflict {
			conflicts++
		}
	}
	if conflicts < 1 {
		t.Fatalf("应检测到无序写冲突: %+v", res.Violations)
	}
}

func TestOrderedWritesNoConflict(t *testing.T) {
	// 1 -> 2（1 先写，2 后写同一路径）→ 有顺序，无冲突。
	g := buildGraph([][2]int64{{1, 2}})
	fd := &fakeDecl{byAction: map[int64][]*model.Declaration{
		1: {{Path: "f", Direction: model.DirWrite}},
		2: {{Path: "f", Direction: model.DirWrite}},
	}}
	in := Inputs{
		TargetID:  1,
		Graph:     g,
		DeclIndex: fd,
		Reads:     map[int64]map[string]struct{}{},
		Writes:    map[int64]map[string]struct{}{1: {"f": {}}, 2: {"f": {}}},
		Tools:     map[int64]map[string]struct{}{},
	}
	res := New().Analyze(in)
	for _, v := range res.Violations {
		if v.Kind == model.ViolationWriteConflict {
			t.Fatalf("有序写不应冲突: %+v", v)
		}
	}
}

func TestMissingInputDetected(t *testing.T) {
	g := buildGraph([][2]int64{{1, 2}})
	fd := &fakeDecl{byAction: map[int64][]*model.Declaration{
		1: {{Path: "src.go", Direction: model.DirRead}, {Path: "out", Direction: model.DirWrite}},
		2: {{Path: "out", Direction: model.DirRead}},
	}}
	in := Inputs{
		TargetID:  1,
		Graph:     g,
		DeclIndex: fd,
		Reads:     map[int64]map[string]struct{}{1: {"src.go": {}}, 2: {"out": {}}},
		Writes:    map[int64]map[string]struct{}{1: {"out": {}}},
		Tools:     map[int64]map[string]struct{}{},
		// 种子无 src.go → 读取不存在输入。
		SeedWrites: map[string]struct{}{},
	}
	res := New().Analyze(in)
	found := false
	for _, v := range res.Violations {
		if v.Kind == model.ViolationUndeclaredInput && v.Path == "src.go" {
			found = true
		}
	}
	if !found {
		t.Fatalf("应检测到读取不存在输入 src.go: %+v", res.Violations)
	}
}

func TestSeedInputAllowed(t *testing.T) {
	g := buildGraph([][2]int64{{1, 2}})
	fd := &fakeDecl{byAction: map[int64][]*model.Declaration{
		1: {{Path: "src.go", Direction: model.DirRead}, {Path: "out", Direction: model.DirWrite}},
	}}
	in := Inputs{
		TargetID:   1,
		Graph:      g,
		DeclIndex:  fd,
		Reads:      map[int64]map[string]struct{}{1: {"src.go": {}}},
		Writes:     map[int64]map[string]struct{}{1: {"out": {}}},
		Tools:      map[int64]map[string]struct{}{},
		SeedWrites: map[string]struct{}{"src.go": {}},
	}
	res := New().Analyze(in)
	for _, v := range res.Violations {
		if v.Kind == model.ViolationUndeclaredInput {
			t.Fatalf("种子输入不应报未声明: %+v", v)
		}
	}
}

func TestCycleViolation(t *testing.T) {
	g := buildGraph([][2]int64{{1, 2}, {2, 3}, {3, 1}})
	fd := &fakeDecl{}
	in := Inputs{TargetID: 1, Graph: g, DeclIndex: fd}
	res := New().Analyze(in)
	found := false
	for _, v := range res.Violations {
		if v.Kind == model.ViolationCycle {
			found = true
		}
	}
	if !found {
		t.Fatal("应检测到 DAG 环违规")
	}
}

func TestDeclarationCoverageViaIndex(t *testing.T) {
	decls := []*model.Declaration{
		{ActionID: 1, Path: "a", Direction: model.DirRead},
		{ActionID: 1, Path: "b", Direction: model.DirWrite},
	}
	idx := declaration.BuildIndex(decls)
	if !idx.Declared(1, "a", model.DirRead) {
		t.Fatal("a 应已声明")
	}
	if idx.Declared(1, "a", model.DirWrite) {
		t.Fatal("a 不应是写声明")
	}
	if !idx.HasDeclarations(1) || idx.HasDeclarations(2) {
		t.Fatal("声明存在性判断错误")
	}
}
