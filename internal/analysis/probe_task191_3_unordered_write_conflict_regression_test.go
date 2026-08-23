package analysis

import (
	"testing"

	"task191-reproof/internal/declaration"
	"task191-reproof/internal/graph"
	"task191-reproof/internal/model"
)

func TestBug03_UnorderedWritersRemainConflicting(t *testing.T) {
	g := graph.New()
	g.AddNode(1)
	g.AddNode(2)
	idx := declaration.BuildIndex([]*model.Declaration{
		{ActionID: 1, Path: "artifact.bin", Direction: model.DirWrite},
		{ActionID: 2, Path: "artifact.bin", Direction: model.DirWrite},
	})
	res := New().Analyze(Inputs{
		TargetID: 7,
		Graph:    g,
		DeclIndex: idx,
		Writes: map[int64]map[string]struct{}{
			1: {"artifact.bin": {}},
			2: {"artifact.bin": {}},
		},
	})
	for _, v := range res.Violations {
		if v.Kind == model.ViolationWriteConflict && v.Path == "artifact.bin" {
			return
		}
	}
	t.Fatal("expected an unordered write conflict")
}
