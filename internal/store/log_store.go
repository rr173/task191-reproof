package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"task191-reproof/internal/model"
)

// LogStore 负责 access_logs / artifacts / artifact_readers 读写。
type LogStore struct{ s *Store }

// NewLogStore 构造日志存储。
func NewLogStore(s *Store) *LogStore { return &LogStore{s: s} }

// AppendLogs 批量追加访问日志（同动作按 seq 合并）。
// 规则：
//   - 同 (action_id, seq) 已存在且内容一致 → 跳过（幂等）。
//   - 同 (action_id, seq) 已存在但内容不一致 → 保留冲突，不覆盖，返回 ErrConflictLog。
//   - 新 seq → 正常插入。
//
// 合并后同步聚合 artifacts：写入方、读取方与最新哈希。
func (ls *LogStore) AppendLogs(ctx context.Context, logs []model.LogEntry) (appended int, err error) {
	if len(logs) == 0 {
		return 0, nil
	}
	err = ls.s.WithTx(ctx, func(tx *sql.Tx) error {
		for _, lg := range logs {
			if err := model.ValidateDirection(lg.Direction); err != nil {
				return err
			}
			if err := validatePath(lg.Path); err != nil {
				return err
			}
			var exists int
			var oldHash string
			qErr := tx.QueryRowContext(ctx,
				`SELECT count(*), COALESCE((SELECT content_hash FROM access_logs WHERE action_id=? AND seq=?), '') FROM access_logs WHERE action_id=? AND seq=?`,
				lg.ActionID, lg.Seq, lg.ActionID, lg.Seq).Scan(&exists, &oldHash)
			if qErr != nil {
				return qErr
			}
			if exists > 0 {
				if oldHash != lg.ContentHash {
					return fmt.Errorf("%w: action %d seq %d 内容冲突", model.ErrConflictLog, lg.ActionID, lg.Seq)
				}
				continue
			}
			now := time.Now().UTC().Format(time.RFC3339Nano)
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO access_logs(action_id, seq, path, direction, content_hash, size_bytes, observed_at) VALUES(?,?,?,?,?,?,?)`,
				lg.ActionID, lg.Seq, lg.Path, lg.Direction, lg.ContentHash, lg.SizeBytes, now); err != nil {
				return err
			}
			appended++
			// 聚合 artifacts
			if err := upsertArtifact(tx, ctx, lg); err != nil {
				return err
			}
		}
		return nil
	})
	return appended, err
}

// upsertArtifact 更新产物聚合行。
func upsertArtifact(tx *sql.Tx, ctx context.Context, lg model.LogEntry) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	var id int64
	err := tx.QueryRowContext(ctx, `SELECT id FROM artifacts WHERE path=?`, lg.Path).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		res, iErr := tx.ExecContext(ctx,
			`INSERT INTO artifacts(path, status, hash, size_bytes, writer_action, updated_at) VALUES(?,?,?,?,?,?)`,
			lg.Path, model.ArtifactObserved, lg.ContentHash, lg.SizeBytes, writerFor(lg), now)
		if iErr != nil {
			return iErr
		}
		id, _ = res.LastInsertId()
	} else if err != nil {
		return err
	} else {
		if lg.Direction == model.DirWrite {
			if _, err := tx.ExecContext(ctx,
				`UPDATE artifacts SET hash=?, size_bytes=?, writer_action=?, status=?, updated_at=? WHERE id=?`,
				lg.ContentHash, lg.SizeBytes, lg.ActionID, model.ArtifactObserved, now, id); err != nil {
				return err
			}
		}
	}
	if lg.Direction == model.DirRead {
		if _, err := tx.ExecContext(ctx,
			`INSERT OR IGNORE INTO artifact_readers(artifact_id, action_id) VALUES(?,?)`, id, lg.ActionID); err != nil {
			return err
		}
	}
	return nil
}

func writerFor(lg model.LogEntry) int64 {
	if lg.Direction == model.DirWrite {
		return lg.ActionID
	}
	return 0
}

// ListByAction 返回某动作的访问日志（按 seq 升序）。
func (ls *LogStore) ListByAction(ctx context.Context, actionID int64) ([]*model.AccessLog, error) {
	rows, err := ls.s.Query(ctx,
		`SELECT id, action_id, seq, path, direction, content_hash, size_bytes, observed_at FROM access_logs WHERE action_id=? ORDER BY seq`, actionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanLogs(rows)
}

// ListAll 返回全部访问日志。
func (ls *LogStore) ListAll(ctx context.Context) ([]*model.AccessLog, error) {
	rows, err := ls.s.Query(ctx,
		`SELECT id, action_id, seq, path, direction, content_hash, size_bytes, observed_at FROM access_logs ORDER BY action_id, seq`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanLogs(rows)
}

// AllLogEntries 返回全部日志条目（用于指纹计算）。
func (ls *LogStore) AllLogEntries(ctx context.Context) ([]model.LogEntry, error) {
	rows, err := ls.s.Query(ctx,
		`SELECT action_id, seq, path, direction, content_hash, size_bytes FROM access_logs ORDER BY action_id, seq`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.LogEntry
	for rows.Next() {
		var e model.LogEntry
		if err := rows.Scan(&e.ActionID, &e.Seq, &e.Path, &e.Direction, &e.ContentHash, &e.SizeBytes); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ListArtifacts 返回全部产物聚合。
func (ls *LogStore) ListArtifacts(ctx context.Context) ([]*model.Artifact, error) {
	rows, err := ls.s.Query(ctx,
		`SELECT path, status, hash, size_bytes, writer_action, updated_at FROM artifacts ORDER BY path`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*model.Artifact
	for rows.Next() {
		ar := &model.Artifact{}
		var updated string
		if err := rows.Scan(&ar.Path, &ar.Status, &ar.Hash, &ar.SizeBytes, &ar.Writer, &updated); err != nil {
			return nil, err
		}
		ar.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
		out = append(out, ar)
	}
	for _, ar := range out {
		rrows, err := ls.s.Query(ctx,
			`SELECT ar.action_id FROM artifact_readers ar JOIN artifacts a ON a.id=ar.artifact_id WHERE a.path=?`, ar.Path)
		if err != nil {
			return nil, err
		}
		for rrows.Next() {
			var aid int64
			if err := rrows.Scan(&aid); err != nil {
				rrows.Close()
				return nil, err
			}
			ar.Readers = append(ar.Readers, aid)
		}
		rrows.Close()
	}
	return out, nil
}

// ArtifactByPath 返回单产物聚合。
func (ls *LogStore) ArtifactByPath(ctx context.Context, path string) (*model.Artifact, error) {
	row := ls.s.QueryRow(ctx,
		`SELECT path, status, hash, size_bytes, writer_action, updated_at FROM artifacts WHERE path=?`, path)
	ar := &model.Artifact{}
	var updated string
	if err := row.Scan(&ar.Path, &ar.Status, &ar.Hash, &ar.SizeBytes, &ar.Writer, &updated); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, model.ErrNotFound
		}
		return nil, err
	}
	ar.UpdatedAt, _ = time.Parse(time.RFC3339Nano, updated)
	rrows, err := ls.s.Query(ctx,
		`SELECT ar.action_id FROM artifact_readers ar JOIN artifacts a ON a.id=ar.artifact_id WHERE a.path=?`, path)
	if err != nil {
		return nil, err
	}
	defer rrows.Close()
	for rrows.Next() {
		var aid int64
		if err := rrows.Scan(&aid); err != nil {
			return nil, err
		}
		ar.Readers = append(ar.Readers, aid)
	}
	return ar, nil
}

// MarkArtifactsPinned 将一批路径标记为已固定（基线冻结）。
func (ls *LogStore) MarkArtifactsPinned(ctx context.Context, paths []string) error {
	if len(paths) == 0 {
		return nil
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	return ls.s.WithTx(ctx, func(tx *sql.Tx) error {
		for _, p := range paths {
			if _, err := tx.ExecContext(ctx,
				`UPDATE artifacts SET status=?, updated_at=? WHERE path=?`, model.ArtifactPinned, now, p); err != nil {
				return err
			}
		}
		return nil
	})
}

func scanLogs(rows *sql.Rows) ([]*model.AccessLog, error) {
	var out []*model.AccessLog
	for rows.Next() {
		l := &model.AccessLog{}
		var observed string
		if err := rows.Scan(&l.ID, &l.ActionID, &l.Seq, &l.Path, &l.Direction, &l.ContentHash, &l.SizeBytes, &observed); err != nil {
			return nil, err
		}
		l.ObservedAt, _ = time.Parse(time.RFC3339Nano, observed)
		out = append(out, l)
	}
	return out, rows.Err()
}

func validatePath(p string) error {
	p = strings.TrimSpace(p)
	if p == "" {
		return model.ErrEmptyPath
	}
	return nil
}
