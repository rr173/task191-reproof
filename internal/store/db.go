// Package store 提供 SQLite 持久化：迁移、事务与各实体的读写。
package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	_ "modernc.org/sqlite"
)

// Store 封装 SQLite 连接，所有写操作经互斥锁串行化（SQLite 单写者）。
type Store struct {
	db  *sql.DB
	mu  sync.Mutex
	dsn string
}

// Open 打开（必要时创建）SQLite 数据库并执行幂等迁移。
func Open(path string) (*Store, error) {
	if dir := filepath.Dir(path); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create db dir: %w", err)
		}
	}
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db, dsn: dsn}
	if err := s.migrate(context.Background()); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

// Close 关闭数据库。
func (s *Store) Close() error { return s.db.Close() }

// DB 返回底层句柄（仅供同包或特殊场景使用）。
func (s *Store) DB() *sql.DB { return s.db }

// WithTx 在互斥锁保护下执行一个读写事务。
func (s *Store) WithTx(ctx context.Context, fn func(tx *sql.Tx) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// Exec 执行一次写语句（加锁）。
func (s *Store) Exec(ctx context.Context, query string, args ...any) (sql.Result, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.db.ExecContext(ctx, query, args...)
}

// Query 执行一次查询（加锁，防止与写并发）。
func (s *Store) Query(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.db.QueryContext(ctx, query, args...)
}

// QueryRow 执行单行查询（加锁）。
func (s *Store) QueryRow(ctx context.Context, query string, args ...any) *sql.Row {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.db.QueryRowContext(ctx, query, args...)
}

// migrate 建表（幂等，IF NOT EXISTS）。
func (s *Store) migrate(ctx context.Context) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS targets (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL UNIQUE,
			description TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'building',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS actions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			target_id INTEGER NOT NULL REFERENCES targets(id),
			name TEXT NOT NULL,
			command TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'pending',
			created_at TEXT NOT NULL,
			UNIQUE(target_id, name)
		)`,
		`CREATE TABLE IF NOT EXISTS action_deps (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			action_id INTEGER NOT NULL REFERENCES actions(id),
			depends_on INTEGER NOT NULL REFERENCES actions(id),
			created_at TEXT NOT NULL,
			UNIQUE(action_id, depends_on)
		)`,
		`CREATE TABLE IF NOT EXISTS declarations (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			action_id INTEGER NOT NULL REFERENCES actions(id),
			path TEXT NOT NULL,
			direction TEXT NOT NULL,
			kind TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			UNIQUE(action_id, path, direction)
		)`,
		`CREATE TABLE IF NOT EXISTS toolchains (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			action_id INTEGER NOT NULL REFERENCES actions(id),
			name TEXT NOT NULL,
			version TEXT NOT NULL,
			checksum TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			UNIQUE(action_id, name)
		)`,
		`CREATE TABLE IF NOT EXISTS access_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			action_id INTEGER NOT NULL REFERENCES actions(id),
			seq INTEGER NOT NULL,
			path TEXT NOT NULL,
			direction TEXT NOT NULL,
			content_hash TEXT NOT NULL,
			size_bytes INTEGER NOT NULL DEFAULT 0,
			observed_at TEXT NOT NULL,
			UNIQUE(action_id, seq)
		)`,
		`CREATE TABLE IF NOT EXISTS artifacts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			path TEXT NOT NULL UNIQUE,
			status TEXT NOT NULL DEFAULT 'declared',
			hash TEXT NOT NULL DEFAULT '',
			size_bytes INTEGER NOT NULL DEFAULT 0,
			writer_action INTEGER NOT NULL DEFAULT 0,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS artifact_readers (
			artifact_id INTEGER NOT NULL REFERENCES artifacts(id),
			action_id INTEGER NOT NULL,
			PRIMARY KEY(artifact_id, action_id)
		)`,
		`CREATE TABLE IF NOT EXISTS violations (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			target_id INTEGER NOT NULL REFERENCES targets(id),
			action_id INTEGER NOT NULL,
			kind TEXT NOT NULL,
			path TEXT NOT NULL DEFAULT '',
			detail TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS violation_chains (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			target_id INTEGER NOT NULL REFERENCES targets(id),
			violation_id INTEGER NOT NULL,
			action_ids TEXT NOT NULL,
			length INTEGER NOT NULL,
			created_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS proofs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			target_id INTEGER NOT NULL REFERENCES targets(id),
			status TEXT NOT NULL DEFAULT 'draft',
			graph_hash TEXT NOT NULL,
			log_hash TEXT NOT NULL,
			created_at TEXT NOT NULL,
			invalidated_at TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS baselines (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			target_id INTEGER NOT NULL REFERENCES targets(id),
			proof_id INTEGER NOT NULL,
			status TEXT NOT NULL DEFAULT 'active',
			frozen_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS baseline_items (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			baseline_id INTEGER NOT NULL REFERENCES baselines(id),
			path TEXT NOT NULL,
			hash TEXT NOT NULL,
			size_bytes INTEGER NOT NULL DEFAULT 0,
			UNIQUE(baseline_id, path)
		)`,
		`CREATE TABLE IF NOT EXISTS target_seeds (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			target_id INTEGER NOT NULL REFERENCES targets(id),
			path TEXT NOT NULL,
			created_at TEXT NOT NULL,
			UNIQUE(target_id, path)
		)`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("migrate stmt: %w", err)
		}
	}
	return nil
}
