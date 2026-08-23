package model

import "fmt"

// Validate 校验目标状态迁移是否合法。
func (t *Target) Validate(next TargetStatus) error {
	valid := map[TargetStatus][]TargetStatus{
		TargetBuilding:       {TargetProven, TargetIrreproducible, TargetInsufficient, TargetBaselined},
		TargetProven:         {TargetBaselined, TargetBuilding},
		TargetIrreproducible: {TargetBuilding},
		TargetInsufficient:   {TargetBuilding},
		TargetBaselined:      {TargetBuilding},
	}
	for _, s := range valid[t.Status] {
		if s == next {
			return nil
		}
	}
	return fmt.Errorf("%w: target %d 不允许 %s -> %s", ErrInvalidTransition, t.ID, t.Status, next)
}

// Validate 校验动作状态迁移是否合法。
func (a *BuildAction) Validate(next ActionStatus) error {
	valid := map[ActionStatus][]ActionStatus{
		ActionPending:       {ActionDeclared},
		ActionDeclared:      {ActionVerified, ActionReadLeak, ActionWriteConflict},
		ActionVerified:      {ActionReadLeak, ActionWriteConflict, ActionDeclared},
		ActionReadLeak:      {ActionDeclared, ActionVerified},
		ActionWriteConflict: {ActionDeclared, ActionVerified},
	}
	for _, s := range valid[a.Status] {
		if s == next {
			return nil
		}
	}
	return fmt.Errorf("%w: action %d 不允许 %s -> %s", ErrInvalidTransition, a.ID, a.Status, next)
}

// Validate 校验证明状态迁移是否合法。
func (p *Proof) Validate(next ProofStatus) error {
	valid := map[ProofStatus][]ProofStatus{
		ProofDraft:       {ProofValid, ProofInvalidated},
		ProofValid:       {ProofSuperseded},
		ProofInvalidated: {},
		ProofSuperseded:  {},
	}
	for _, s := range valid[p.Status] {
		if s == next {
			return nil
		}
	}
	return fmt.Errorf("%w: proof %d 不允许 %s -> %s", ErrInvalidTransition, p.ID, p.Status, next)
}
