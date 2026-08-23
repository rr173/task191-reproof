package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"task191-reproof/internal/model"
)

// TargetStore 负责 targets 表读写。
type TargetStore struct{ s *Store }

// NewTargetStore 构造目标存储。
func NewTargetStore(s *Store) *TargetStore { return &TargetStore{s: s} }

// AddSeed 登记目标的种子输入路径（外部提供，无需动作生产）。
func (ts *TargetStore) AddSeed(ctx context.Context, targetID int64, path string) error {
	return ts.s.WithTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx,
			`INSERT INTO target_seeds(target_id, path, created_at) VALUES(?,?,?)`,
			targetID, path, time.Now().UTC().Format(time.RFC3339Nano))
		if err != nil {
			if isUniqueViolation(err) {
				return model.ErrDuplicateName
			}
			return err
		}
		_ = res
		return nil
	})
}

// ListSeeds 返回目标的种子输入路径。
func (ts *TargetStore) ListSeeds(ctx context.Context, targetID int64) ([]string, error) {
	rows, err := ts.s.Query(ctx,
		`SELECT path FROM target_seeds WHERE target_id=? ORDER BY path`, targetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

// Create 插入新目标并返回完整实体。
func (ts *TargetStore) Create(ctx context.Context, t *model.Target) (*model.Target, error) {
	err := ts.s.WithTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx,
			`INSERT INTO targets(name, description, status, created_at, updated_at) VALUES(?,?,?,?,?)`,
			t.Name, t.Description, t.Status, t.CreatedAt.Format(time.RFC3339Nano), t.UpdatedAt.Format(time.RFC3339Nano))
		if err != nil {
			if isUniqueViolation(err) {
				return model.ErrDuplicateName
			}
			return err
		}
		id, err := res.LastInsertId()
		if err != nil {
			return err
		}
		t.ID = id
		return nil
	})
	if err != nil {
		return nil, err
	}
	return t, nil
}

// GetByID 按 ID 读取目标；不存在返回 ErrNotFound。
func (ts *TargetStore) GetByID(ctx context.Context, id int64) (*model.Target, error) {
	row := ts.s.QueryRow(ctx,
		`SELECT id, name, description, status, created_at, updated_at FROM targets WHERE id=?`, id)
	t, err := scanTarget(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, model.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return t, nil
}

// GetByName 按名称读取目标。
func (ts *TargetStore) GetByName(ctx context.Context, name string) (*model.Target, error) {
	row := ts.s.QueryRow(ctx,
		`SELECT id, name, description, status, created_at, updated_at FROM targets WHERE name=?`, name)
	t, err := scanTarget(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, model.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return t, nil
}

// List 返回全部目标（按 ID 升序）。
func (ts *TargetStore) List(ctx context.Context) ([]*model.Target, error) {
	rows, err := ts.s.Query(ctx,
		`SELECT id, name, description, status, created_at, updated_at FROM targets ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*model.Target
	for rows.Next() {
		t, err := scanTarget(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// UpdateStatus 更新目标状态并推进 updated_at。
func (ts *TargetStore) UpdateStatus(ctx context.Context, id int64, status model.TargetStatus) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	res, err := ts.s.Exec(ctx,
		`UPDATE targets SET status=?, updated_at=? WHERE id=?`, status, now, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return model.ErrNotFound
	}
	return nil
}

// ActionStore 负责 actions / action_deps / declarations / toolchains 表读写。
type ActionStore struct{ s *Store }

// NewActionStore 构造动作存储。
func NewActionStore(s *Store) *ActionStore { return &ActionStore{s: s} }

// Create 插入动作。
func (as *ActionStore) Create(ctx context.Context, a *model.BuildAction) (*model.BuildAction, error) {
	err := as.s.WithTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx,
			`INSERT INTO actions(target_id, name, command, status, created_at) VALUES(?,?,?,?,?)`,
			a.TargetID, a.Name, a.Command, a.Status, a.CreatedAt.Format(time.RFC3339Nano))
		if err != nil {
			if isUniqueViolation(err) {
				return model.ErrDuplicateName
			}
			return err
		}
		id, err := res.LastInsertId()
		if err != nil {
			return err
		}
		a.ID = id
		return nil
	})
	if err != nil {
		return nil, err
	}
	return a, nil
}

// GetByID 读取动作。
func (as *ActionStore) GetByID(ctx context.Context, id int64) (*model.BuildAction, error) {
	row := as.s.QueryRow(ctx,
		`SELECT id, target_id, name, command, status, created_at FROM actions WHERE id=?`, id)
	a, err := scanAction(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, model.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return a, nil
}

// ListByTarget 返回目标下全部动作。
func (as *ActionStore) ListByTarget(ctx context.Context, targetID int64) ([]*model.BuildAction, error) {
	rows, err := as.s.Query(ctx,
		`SELECT id, target_id, name, command, status, created_at FROM actions WHERE target_id=? ORDER BY id`, targetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*model.BuildAction
	for rows.Next() {
		a, err := scanAction(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// UpdateStatus 更新动作状态。
func (as *ActionStore) UpdateStatus(ctx context.Context, id int64, status model.ActionStatus) error {
	_, err := as.s.Exec(ctx, `UPDATE actions SET status=? WHERE id=?`, status, id)
	return err
}

// AddDep 添加依赖边；重复边返回 ErrDuplicateName。
func (as *ActionStore) AddDep(ctx context.Context, dep *model.ActionDep) error {
	return as.s.WithTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx,
			`INSERT INTO action_deps(action_id, depends_on, created_at) VALUES(?,?,?)`,
			dep.ActionID, dep.DependsOn, dep.CreatedAt.Format(time.RFC3339Nano))
		if err != nil {
			if isUniqueViolation(err) {
				return model.ErrDuplicateName
			}
			return err
		}
		id, err := res.LastInsertId()
		if err != nil {
			return err
		}
		dep.ID = id
		return nil
	})
}

// ListDeps 返回全部依赖边（action_id -> depends_on）。
func (as *ActionStore) ListDeps(ctx context.Context) ([]*model.ActionDep, error) {
	rows, err := as.s.Query(ctx,
		`SELECT id, action_id, depends_on, created_at FROM action_deps ORDER BY action_id, depends_on`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*model.ActionDep
	for rows.Next() {
		d := &model.ActionDep{}
		var created string
		if err := rows.Scan(&d.ID, &d.ActionID, &d.DependsOn, &created); err != nil {
			return nil, err
		}
		d.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		out = append(out, d)
	}
	return out, rows.Err()
}

// AddDeclaration 添加输入/输出声明。
func (as *ActionStore) AddDeclaration(ctx context.Context, d *model.Declaration) error {
	return as.s.WithTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx,
			`INSERT INTO declarations(action_id, path, direction, kind, created_at) VALUES(?,?,?,?,?)`,
			d.ActionID, d.Path, d.Direction, d.Kind, d.CreatedAt.Format(time.RFC3339Nano))
		if err != nil {
			if isUniqueViolation(err) {
				return model.ErrDuplicateName
			}
			return err
		}
		id, err := res.LastInsertId()
		if err != nil {
			return err
		}
		d.ID = id
		return nil
	})
}

// ListDeclarations 返回全部声明。
func (as *ActionStore) ListDeclarations(ctx context.Context) ([]*model.Declaration, error) {
	rows, err := as.s.Query(ctx,
		`SELECT id, action_id, path, direction, kind, created_at FROM declarations ORDER BY action_id, path`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*model.Declaration
	for rows.Next() {
		d := &model.Declaration{}
		var created string
		if err := rows.Scan(&d.ID, &d.ActionID, &d.Path, &d.Direction, &d.Kind, &created); err != nil {
			return nil, err
		}
		d.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		out = append(out, d)
	}
	return out, rows.Err()
}

// DeclarationsByAction 返回某动作的声明。
func (as *ActionStore) DeclarationsByAction(ctx context.Context, actionID int64) ([]*model.Declaration, error) {
	rows, err := as.s.Query(ctx,
		`SELECT id, action_id, path, direction, kind, created_at FROM declarations WHERE action_id=? ORDER BY path`, actionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*model.Declaration
	for rows.Next() {
		d := &model.Declaration{}
		var created string
		if err := rows.Scan(&d.ID, &d.ActionID, &d.Path, &d.Direction, &d.Kind, &created); err != nil {
			return nil, err
		}
		d.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		out = append(out, d)
	}
	return out, rows.Err()
}

// AddToolchain 添加工具链声明。
func (as *ActionStore) AddToolchain(ctx context.Context, t *model.Toolchain) error {
	return as.s.WithTx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx,
			`INSERT INTO toolchains(action_id, name, version, checksum, created_at) VALUES(?,?,?,?,?)`,
			t.ActionID, t.Name, t.Version, t.Checksum, t.CreatedAt.Format(time.RFC3339Nano))
		if err != nil {
			if isUniqueViolation(err) {
				return model.ErrDuplicateName
			}
			return err
		}
		id, err := res.LastInsertId()
		if err != nil {
			return err
		}
		t.ID = id
		return nil
	})
}

// ToolchainsByAction 返回某动作的工具链。
func (as *ActionStore) ToolchainsByAction(ctx context.Context, actionID int64) ([]*model.Toolchain, error) {
	rows, err := as.s.Query(ctx,
		`SELECT id, action_id, name, version, checksum, created_at FROM toolchains WHERE action_id=? ORDER BY name`, actionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*model.Toolchain
	for rows.Next() {
		t := &model.Toolchain{}
		var created string
		if err := rows.Scan(&t.ID, &t.ActionID, &t.Name, &t.Version, &t.Checksum, &created); err != nil {
			return nil, err
		}
		t.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
		out = append(out, t)
	}
	return out, rows.Err()
}

// ScanHelpers — 行扫描函数。
func scanTarget(row interface{ Scan(dest ...any) error }) (*model.Target, error) {
	t := &model.Target{}
	var created, updated string
	if err := row.Scan(&t.ID, &t.Name, &t.Description, &t.Status, &created, &updated); err != nil {
		return nil, err
	}
	t.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	t.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	return t, nil
}

func scanAction(row interface{ Scan(dest ...any) error }) (*model.BuildAction, error) {
	a := &model.BuildAction{}
	var created string
	if err := row.Scan(&a.ID, &a.TargetID, &a.Name, &a.Command, &a.Status, &created); err != nil {
		return nil, err
	}
	a.CreatedAt, _ = time.Parse(time.RFC3339Nano, created)
	return a, nil
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return contains(msg, "UNIQUE constraint failed") || contains(msg, "constraint failed")
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

var _ = fmt.Sprintf
