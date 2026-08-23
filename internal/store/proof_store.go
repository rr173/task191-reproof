package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"task191-reproof/internal/model"
)

// ViolationStore 负责 violations / violation_chains 读写。
type ViolationStore struct{ s *Store }

// NewViolationStore 构造违规存储。
func NewViolationStore(s *Store) *ViolationStore { return &ViolationStore{s: s} }

// ReplaceForTarget 清空某目标的既有违规后写入新分析结果（幂等重分析）。
func (vs *ViolationStore) ReplaceForTarget(ctx context.Context, targetID int64, violations []*model.Violation, chains []*model.ViolationChain) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	return vs.s.WithTx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `DELETE FROM violation_chains WHERE target_id=?`, targetID); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM violations WHERE target_id=?`, targetID); err != nil {
			return err
		}
		for _, v := range violations {
			res, err := tx.ExecContext(ctx,
				`INSERT INTO violations(target_id, action_id, kind, path, detail, created_at) VALUES(?,?,?,?,?,?)`,
				targetID, v.ActionID, v.Kind, v.Path, v.Detail, now)
			if err != nil {
				return err
			}
			id, _ := res.LastInsertId()
			v.ID = id
			v.TargetID = targetID
			v.CreatedAt, _ = time.Parse(time.RFC3339Nano, now)
		}
		for _, c := range chains {
			idsJSON, err := json.Marshal(c.ActionIDs)
			if err != nil {
				return err
			}
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO violation_chains(target_id, violation_id, action_ids, length, created_at) VALUES(?,?,?,?,?)`,
				targetID, c.ViolationID, string(idsJSON), c.Length, now); err != nil {
				return err
			}
		}
		return nil
	})
}

// ListByTarget 返回目标全部违规。
func (vs *ViolationStore) ListByTarget(ctx context.Context, targetID int64) ([]*model.Violation, error) {
	rows, err := vs.s.Query(ctx,
		`SELECT id, target_id, action_id, kind, path, detail, created_at FROM violations WHERE target_id=? ORDER BY id`, targetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*model.Violation
	for rows.Next() {
		v := &model.Violation{}
		var created string
		if err := rows.Scan(&v.ID, &v.TargetID, &v.ActionID, &v.Kind, &v.Path, &v.Detail, &created); err != nil {
			return nil, err
		}
		v.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		out = append(out, v)
	}
	return out, rows.Err()
}

// ChainsByTarget 返回目标全部违规链。
func (vs *ViolationStore) ChainsByTarget(ctx context.Context, targetID int64) ([]*model.ViolationChain, error) {
	rows, err := vs.s.Query(ctx,
		`SELECT id, target_id, violation_id, action_ids, length, created_at FROM violation_chains WHERE target_id=? ORDER BY length, id`, targetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*model.ViolationChain
	for rows.Next() {
		c := &model.ViolationChain{}
		var idsJSON, created string
		if err := rows.Scan(&c.ID, &c.TargetID, &c.ViolationID, &idsJSON, &c.Length, &created); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(idsJSON), &c.ActionIDs)
		c.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		out = append(out, c)
	}
	return out, rows.Err()
}

// ProofStore 负责 proofs / baselines / baseline_items 读写。
type ProofStore struct{ s *Store }

// NewProofStore 构造证明存储。
func NewProofStore(s *Store) *ProofStore { return &ProofStore{s: s} }

// Create 创建证明。
func (ps *ProofStore) Create(ctx context.Context, p *model.Proof) (*model.Proof, error) {
	now := p.CreatedAt.Format(time.RFC3339Nano)
	err := ps.s.WithTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx,
			`INSERT INTO proofs(target_id, status, graph_hash, log_hash, created_at) VALUES(?,?,?,?,?)`,
			p.TargetID, p.Status, p.GraphHash, p.LogHash, now)
		if err != nil {
			return err
		}
		id, err := res.LastInsertId()
		if err != nil {
			return err
		}
		p.ID = id
		return nil
	})
	if err != nil {
		return nil, err
	}
	return p, nil
}

// FindReusable 查找与图/日志哈希一致且仍有效的证明（相同图和日志哈希复用结论）。
func (ps *ProofStore) FindReusable(ctx context.Context, targetID int64, graphHash, logHash string) (*model.Proof, error) {
	row := ps.s.QueryRow(ctx,
		`SELECT id, target_id, status, graph_hash, log_hash, created_at, invalidated_at FROM proofs
		 WHERE target_id=? AND graph_hash=? AND log_hash=? AND status='valid' ORDER BY id ASC LIMIT 1`,
		targetID, graphHash, logHash)
	p, err := scanProof(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, model.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return p, nil
}

// LatestByTarget 返回目标最新证明。
func (ps *ProofStore) LatestByTarget(ctx context.Context, targetID int64) (*model.Proof, error) {
	row := ps.s.QueryRow(ctx,
		`SELECT id, target_id, status, graph_hash, log_hash, created_at, invalidated_at FROM proofs
		 WHERE target_id=? ORDER BY id DESC LIMIT 1`, targetID)
	p, err := scanProof(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, model.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return p, nil
}

// ListByTarget 返回目标全部证明。
func (ps *ProofStore) ListByTarget(ctx context.Context, targetID int64) ([]*model.Proof, error) {
	rows, err := ps.s.Query(ctx,
		`SELECT id, target_id, status, graph_hash, log_hash, created_at, invalidated_at FROM proofs WHERE target_id=? ORDER BY id DESC`, targetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*model.Proof
	for rows.Next() {
		p, err := scanProof(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// GetByID 按 ID 读取证明。
func (ps *ProofStore) GetByID(ctx context.Context, id int64) (*model.Proof, error) {
	row := ps.s.QueryRow(ctx,
		`SELECT id, target_id, status, graph_hash, log_hash, created_at, invalidated_at FROM proofs WHERE id=?`, id)
	p, err := scanProof(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, model.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return p, nil
}

// UpdateStatus 更新证明状态（可携带失效时间）。
func (ps *ProofStore) UpdateStatus(ctx context.Context, id int64, status model.ProofStatus, invalidatedAt *time.Time) error {
	inv := ""
	if invalidatedAt != nil {
		inv = invalidatedAt.Format(time.RFC3339Nano)
	}
	_, err := ps.s.Exec(ctx, `UPDATE proofs SET status=?, invalidated_at=? WHERE id=?`, status, inv, id)
	return err
}

// InvalidateOthers 将目标下其它有效证明全部置为已替代（新证明生成时调用）。
func (ps *ProofStore) InvalidateOthers(ctx context.Context, targetID, keepID int64) error {
	_, err := ps.s.Exec(ctx,
		`UPDATE proofs SET status='superseded' WHERE target_id=? AND id<>? AND status='valid'`, targetID, keepID)
	return err
}

// CreateBaseline 创建基线并冻结输入哈希。
func (ps *ProofStore) CreateBaseline(ctx context.Context, b *model.Baseline, items []*model.BaselineItem) (*model.Baseline, error) {
	frozen := b.FrozenAt.Format(time.RFC3339Nano)
	err := ps.s.WithTx(ctx, func(tx *sql.Tx) error {
		// 已存在的 active 基线先降级为 superseded（同一目标只允许一条 active）。
		if _, err := tx.ExecContext(ctx,
			`UPDATE baselines SET status='superseded' WHERE target_id=? AND status='active'`, b.TargetID); err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx,
			`INSERT INTO baselines(target_id, proof_id, status, frozen_at) VALUES(?,?,?,?)`,
			b.TargetID, b.ProofID, b.Status, frozen)
		if err != nil {
			return err
		}
		id, _ := res.LastInsertId()
		b.ID = id
		for _, it := range items {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO baseline_items(baseline_id, path, hash, size_bytes) VALUES(?,?,?,?)`,
				b.ID, it.Path, it.Hash, it.SizeBytes); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return b, nil
}

// ActiveBaseline 返回目标当前 active 基线。
func (ps *ProofStore) ActiveBaseline(ctx context.Context, targetID int64) (*model.Baseline, error) {
	row := ps.s.QueryRow(ctx,
		`SELECT id, target_id, proof_id, status, frozen_at FROM baselines WHERE target_id=? AND status='active' ORDER BY id DESC LIMIT 1`, targetID)
	b := &model.Baseline{}
	var frozen string
	if err := row.Scan(&b.ID, &b.TargetID, &b.ProofID, &b.Status, &frozen); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, model.ErrNotFound
		}
		return nil, err
	}
	b.FrozenAt, _ = time.Parse(time.RFC3339Nano, frozen)
	return b, nil
}

// ListBaselines 返回目标全部基线。
func (ps *ProofStore) ListBaselines(ctx context.Context, targetID int64) ([]*model.Baseline, error) {
	rows, err := ps.s.Query(ctx,
		`SELECT id, target_id, proof_id, status, frozen_at FROM baselines WHERE target_id=? ORDER BY id DESC`, targetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*model.Baseline
	for rows.Next() {
		b := &model.Baseline{}
		var frozen string
		if err := rows.Scan(&b.ID, &b.TargetID, &b.ProofID, &b.Status, &frozen); err != nil {
			return nil, err
		}
		b.FrozenAt, _ = time.Parse(time.RFC3339Nano, frozen)
		out = append(out, b)
	}
	return out, rows.Err()
}

// BaselineItems 返回基线冻结条目。
func (ps *ProofStore) BaselineItems(ctx context.Context, baselineID int64) ([]*model.BaselineItem, error) {
	rows, err := ps.s.Query(ctx,
		`SELECT id, baseline_id, path, hash, size_bytes FROM baseline_items WHERE baseline_id=? ORDER BY path`, baselineID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*model.BaselineItem
	for rows.Next() {
		it := &model.BaselineItem{}
		if err := rows.Scan(&it.ID, &it.BaselineID, &it.Path, &it.Hash, &it.SizeBytes); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

func scanProof(row interface{ Scan(dest ...any) error }) (*model.Proof, error) {
	p := &model.Proof{}
	var created string
	var invalidated sql.NullString
	if err := row.Scan(&p.ID, &p.TargetID, &p.Status, &p.GraphHash, &p.LogHash, &created, &invalidated); err != nil {
		return nil, err
	}
	p.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	if invalidated.Valid && invalidated.String != "" {
		t, err := time.Parse(time.RFC3339Nano, invalidated.String)
		if err == nil {
			p.InvalidatedAt = &t
		}
	}
	return p, nil
}
