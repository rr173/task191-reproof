package model

import "errors"

// 领域错误集合。
var (
	ErrNotFound           = errors.New("resource not found")
	ErrInvalidTransition  = errors.New("invalid state transition")
	ErrDuplicateName      = errors.New("duplicate name")
	ErrCycleDetected      = errors.New("dependency cycle detected")
	ErrUndeclaredRead     = errors.New("undeclared read access")
	ErrWriteConflict      = errors.New("unordered write conflict")
	ErrMissingToolchain   = errors.New("missing toolchain version")
	ErrBaselineFrozen     = errors.New("baseline already frozen")
	ErrStaleGraph         = errors.New("graph hash mismatch with proof")
	ErrConflictLog        = errors.New("conflicting log content preserved")
	ErrTargetNotProven    = errors.New("target not proven yet")
	ErrDeclarationMissing = errors.New("declaration missing")
	ErrEmptyPath          = errors.New("empty path")
	ErrInvalidDirection   = errors.New("invalid access direction")
)
