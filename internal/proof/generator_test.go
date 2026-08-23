package proof

import (
	"testing"

	"task191-reproof/internal/model"
)

func TestMakeDraftRejectsUnproven(t *testing.T) {
	g := New()
	if _, err := g.MakeDraft(ProofInput{TargetID: 1, NoViolations: false}); err == nil {
		t.Fatal("有违规不应生成证明")
	}
	if _, err := g.MakeDraft(ProofInput{TargetID: 1, NoViolations: true, ActionsVerified: false}); err == nil {
		t.Fatal("声明未完整不应生成证明")
	}
	p, err := g.MakeDraft(ProofInput{TargetID: 1, NoViolations: true, ActionsVerified: true, GraphHash: "g", LogHash: "l"})
	if err != nil {
		t.Fatalf("应可生成草案: %v", err)
	}
	if p.Status != model.ProofDraft || p.GraphHash != "g" {
		t.Fatalf("草案字段错误: %+v", p)
	}
}

func TestCompareCleanAndDrift(t *testing.T) {
	g := New()
	base := []*model.BaselineItem{
		{BaselineID: 1, Path: "out/a", Hash: "ha", SizeBytes: 1},
		{BaselineID: 1, Path: "out/b", Hash: "hb", SizeBytes: 2},
	}
	current := map[string]string{"out/a": "ha", "out/b": "hb"}
	res := g.Compare(current, base)
	if !res.Clean || len(res.Drifted) != 0 {
		t.Fatalf("一致比对应 clean: %+v", res)
	}
	current["out/b"] = "hb-CHANGED"
	res = g.Compare(current, base)
	if res.Clean || len(res.Drifted) != 1 || res.Drifted[0] != "out/b" {
		t.Fatalf("漂移比对错误: %+v", res)
	}
	current2 := map[string]string{"out/a": "ha", "out/new": "hn"}
	res = g.Compare(current2, base)
	if len(res.Missing) != 1 || len(res.New) != 1 {
		t.Fatalf("新增/缺失检测错误: %+v", res)
	}
}

func TestFreezeSortsItems(t *testing.T) {
	g := New()
	b, items := g.Freeze(BaselineInput{
		TargetID: 1,
		ProofID:  2,
		FrozenItems: []model.BaselineItem{
			{Path: "z", Hash: "hz"},
			{Path: "a", Hash: "ha"},
		},
	})
	if b.TargetID != 1 || b.ProofID != 2 || b.Status != model.BaselineActive {
		t.Fatalf("基线字段错误: %+v", b)
	}
	if items[0].Path != "a" || items[1].Path != "z" {
		t.Fatalf("基线条目应排序: %+v", items)
	}
}
