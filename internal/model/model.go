// Package model 定义构建图可复现性隔离证明服务的领域实体、状态机与领域错误。
package model

import (
	"time"
)

// 目标状态：构建中 -> 可证明 / 不可复现 / 证据不足 -> 已基线化。
type TargetStatus string

const (
	TargetBuilding       TargetStatus = "building"
	TargetProven         TargetStatus = "proven"
	TargetIrreproducible TargetStatus = "irreproducible"
	TargetInsufficient   TargetStatus = "insufficient"
	TargetBaselined      TargetStatus = "baselined"
)

// 构建动作状态：待采集 -> 声明完整 -> 已验证 / 读取泄漏 / 写入冲突。
type ActionStatus string

const (
	ActionPending       ActionStatus = "pending"
	ActionDeclared      ActionStatus = "declared"
	ActionVerified      ActionStatus = "verified"
	ActionReadLeak      ActionStatus = "read_leak"
	ActionWriteConflict ActionStatus = "write_conflict"
)

// 输入/输出产物状态：声明 / 实际访问 / 未声明 / 被污染 / 已固定。
type ArtifactStatus string

const (
	ArtifactDeclared   ArtifactStatus = "declared"
	ArtifactObserved   ArtifactStatus = "observed"
	ArtifactUndeclared ArtifactStatus = "undeclared"
	ArtifactPolluted   ArtifactStatus = "polluted"
	ArtifactPinned     ArtifactStatus = "pinned"
)

// 证明状态：草案 -> 有效 -> 已失效 / 已替代。
type ProofStatus string

const (
	ProofDraft       ProofStatus = "draft"
	ProofValid       ProofStatus = "valid"
	ProofInvalidated ProofStatus = "invalidated"
	ProofSuperseded  ProofStatus = "superseded"
)

// 基线状态。
type BaselineStatus string

const (
	BaselineActive     BaselineStatus = "active"
	BaselineSuperseded BaselineStatus = "superseded"
)

// 访问方向。
type AccessDirection string

const (
	DirRead  AccessDirection = "read"
	DirWrite AccessDirection = "write"
)

// 违规类型。
type ViolationKind string

const (
	ViolationReadLeak        ViolationKind = "read_leak"
	ViolationWriteConflict   ViolationKind = "write_conflict"
	ViolationUndeclaredInput ViolationKind = "undeclared_input"
	ViolationMissingTool     ViolationKind = "missing_toolchain"
	ViolationCycle           ViolationKind = "dag_cycle"
	ViolationStaleBaseline   ViolationKind = "stale_baseline"
)

// Target 表示一个可被证明可复现的构建目标。
type Target struct {
	ID          int64        `json:"id"`
	Name        string       `json:"name"`
	Description string       `json:"description"`
	Status      TargetStatus `json:"status"`
	CreatedAt   time.Time    `json:"created_at"`
	UpdatedAt   time.Time    `json:"updated_at"`
}

// BuildAction 表示一个构建动作（DAG 节点）。
type BuildAction struct {
	ID        int64        `json:"id"`
	TargetID  int64        `json:"target_id"`
	Name      string       `json:"name"`
	Command   string       `json:"command"`
	Status    ActionStatus `json:"status"`
	CreatedAt time.Time    `json:"created_at"`
}

// ActionDep 表示动作依赖边：DependsOn 必须先于 Action 执行。
type ActionDep struct {
	ID        int64     `json:"id"`
	ActionID  int64     `json:"action_id"`
	DependsOn int64     `json:"depends_on"`
	CreatedAt time.Time `json:"created_at"`
}

// Declaration 表示动作对输入/输出的声明。
type Declaration struct {
	ID        int64           `json:"id"`
	ActionID  int64           `json:"action_id"`
	Path      string          `json:"path"`
	Direction AccessDirection `json:"direction"`
	Kind      string          `json:"kind"`
	CreatedAt time.Time       `json:"created_at"`
}

// Toolchain 表示动作声明的工具链版本。
type Toolchain struct {
	ID        int64     `json:"id"`
	ActionID  int64     `json:"action_id"`
	Name      string    `json:"name"`
	Version   string    `json:"version"`
	Checksum  string    `json:"checksum"`
	CreatedAt time.Time `json:"created_at"`
}

// AccessLog 表示观测到的实际文件访问事件。
type AccessLog struct {
	ID          int64           `json:"id"`
	ActionID    int64           `json:"action_id"`
	Seq         int             `json:"seq"`
	Path        string          `json:"path"`
	Direction   AccessDirection `json:"direction"`
	ContentHash string          `json:"content_hash"`
	SizeBytes   int64           `json:"size_bytes"`
	ObservedAt  time.Time       `json:"observed_at"`
}

// Artifact 表示一个路径上的产物状态聚合。
type Artifact struct {
	Path      string         `json:"path"`
	Status    ArtifactStatus `json:"status"`
	Hash      string         `json:"hash"`
	SizeBytes int64          `json:"size_bytes"`
	Writer    int64          `json:"writer_action"`
	Readers   []int64        `json:"reader_actions"`
	UpdatedAt time.Time      `json:"updated_at"`
}

// Violation 表示一次分析发现的隔离/污染违规。
type Violation struct {
	ID        int64         `json:"id"`
	TargetID  int64         `json:"target_id"`
	ActionID  int64         `json:"action_id"`
	Kind      ViolationKind `json:"kind"`
	Path      string        `json:"path"`
	Detail    string        `json:"detail"`
	CreatedAt time.Time     `json:"created_at"`
}

// ViolationChain 表示目标到违规点的最短违规链（动作序列）。
type ViolationChain struct {
	ID          int64     `json:"id"`
	TargetID    int64     `json:"target_id"`
	ViolationID int64     `json:"violation_id"`
	ActionIDs   []int64   `json:"action_ids"`
	Length      int       `json:"length"`
	CreatedAt   time.Time `json:"created_at"`
}

// Proof 表示目标可复现性的证明。
type Proof struct {
	ID            int64       `json:"id"`
	TargetID      int64       `json:"target_id"`
	Status        ProofStatus `json:"status"`
	GraphHash     string      `json:"graph_hash"`
	LogHash       string      `json:"log_hash"`
	CreatedAt     time.Time   `json:"created_at"`
	InvalidatedAt *time.Time  `json:"invalidated_at,omitempty"`
}

// Baseline 表示已冻结输入哈希的可复现基线。
type Baseline struct {
	ID       int64          `json:"id"`
	TargetID int64          `json:"target_id"`
	ProofID  int64          `json:"proof_id"`
	Status   BaselineStatus `json:"status"`
	FrozenAt time.Time      `json:"frozen_at"`
}

// BaselineItem 表示基线中冻结的一个输入路径哈希。
type BaselineItem struct {
	ID         int64  `json:"id"`
	BaselineID int64  `json:"baseline_id"`
	Path       string `json:"path"`
	Hash       string `json:"hash"`
	SizeBytes  int64  `json:"size_bytes"`
}

// NewTarget 构造新目标。
func NewTarget(name, description string) *Target {
	now := time.Now().UTC()
	return &Target{
		Name:        name,
		Description: description,
		Status:      TargetBuilding,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

// NewAction 构造新动作。
func NewAction(targetID int64, name, command string) *BuildAction {
	return &BuildAction{
		TargetID:  targetID,
		Name:      name,
		Command:   command,
		Status:    ActionPending,
		CreatedAt: time.Now().UTC(),
	}
}

// NewProof 构造草案证明。
func NewProof(targetID int64, graphHash, logHash string) *Proof {
	return &Proof{
		TargetID:  targetID,
		Status:    ProofDraft,
		GraphHash: graphHash,
		LogHash:   logHash,
		CreatedAt: time.Now().UTC(),
	}
}
