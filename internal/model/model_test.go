package model

import "testing"

func TestTargetStateTransitions(t *testing.T) {
	tgt := &Target{ID: 1, Status: TargetBuilding}
	cases := []struct {
		next TargetStatus
		ok   bool
	}{
		{TargetProven, true},
		{TargetIrreproducible, true},
		{TargetInsufficient, true},
		{TargetBaselined, true},
	}
	for _, c := range cases {
		if err := tgt.Validate(c.next); (err == nil) != c.ok {
			t.Fatalf("building -> %s ok=%v err=%v", c.next, c.ok, err)
		}
	}
	// 非法迁移。
	if err := (&Target{ID: 2, Status: TargetBaselined}).Validate(TargetInsufficient); err == nil {
		t.Fatal("baselined -> insufficient 应非法")
	}
}

func TestActionStateTransitions(t *testing.T) {
	a := &BuildAction{ID: 1, Status: ActionPending}
	if err := a.Validate(ActionDeclared); err != nil {
		t.Fatalf("pending -> declared: %v", err)
	}
	a.Status = ActionDeclared
	if err := a.Validate(ActionVerified); err != nil {
		t.Fatalf("declared -> verified: %v", err)
	}
	// pending 不允许直接 verified。
	if err := (&BuildAction{ID: 2, Status: ActionPending}).Validate(ActionVerified); err == nil {
		t.Fatal("pending -> verified 应非法")
	}
}

func TestProofStateTransitions(t *testing.T) {
	p := NewProof(1, "g", "l")
	if err := p.Validate(ProofValid); err != nil {
		t.Fatalf("draft -> valid: %v", err)
	}
	p.Status = ProofValid
	if err := p.Validate(ProofInvalidated); err != nil {
		t.Fatalf("valid -> invalidated: %v", err)
	}
	p.Status = ProofInvalidated
	if err := p.Validate(ProofValid); err == nil {
		t.Fatal("invalidated -> valid 应非法")
	}
}

func TestNormalizePath(t *testing.T) {
	if _, err := NormalizePath("   "); err == nil {
		t.Fatal("空路径应报错")
	}
	p, err := NormalizePath("  src/main.go  ")
	if err != nil || p != "src/main.go" {
		t.Fatalf("规范化失败: %q %v", p, err)
	}
}

func TestValidateDirection(t *testing.T) {
	if err := ValidateDirection(DirRead); err != nil {
		t.Fatal(err)
	}
	if err := ValidateDirection(AccessDirection("sideways")); err == nil {
		t.Fatal("非法方向应报错")
	}
}

func TestHashDeterminism(t *testing.T) {
	g1 := GraphDigest([]int64{2, 1}, []ActionPair{{From: 1, To: 2}})
	g2 := GraphDigest([]int64{1, 2}, []ActionPair{{From: 1, To: 2}})
	if g1 != g2 {
		t.Fatal("图摘要应不受输入顺序影响")
	}
	l1 := LogFingerprint([]LogEntry{{ActionID: 2, Seq: 1, Path: "a", Direction: DirRead, ContentHash: "x"}})
	l2 := LogFingerprint([]LogEntry{{ActionID: 2, Seq: 1, Path: "a", Direction: DirRead, ContentHash: "x"}})
	if l1 != l2 {
		t.Fatal("日志指纹应稳定")
	}
}
