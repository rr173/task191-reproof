// Package observation 归档观测到的实际文件访问，并提供按动作的访问索引。
package observation

import (
	"sort"

	"task191-reproof/internal/model"
)

// AccessIndex 是按动作索引的观测访问视图。
type AccessIndex struct {
	// reads / writes: actionID -> 路径集合（聚合去重）。
	reads  map[int64]map[string]struct{}
	writes map[int64]map[string]struct{}
	// tools: actionID -> 工具链名集合（从 kind=toolchain 日志提取）。
	tools map[int64]map[string]struct{}
	// allLogs: 全部日志条目。
	allLogs []model.LogEntry
}

// BuildAccessIndex 从日志条目构建访问索引。
// 工具链使用通过 kind=toolchain 的日志记录（path 为工具名）。
func BuildAccessIndex(logs []model.LogEntry, toolLogs []model.LogEntry) *AccessIndex {
	ai := &AccessIndex{
		reads:  make(map[int64]map[string]struct{}),
		writes: make(map[int64]map[string]struct{}),
		tools:  make(map[int64]map[string]struct{}),
	}
	for _, lg := range logs {
		switch lg.Direction {
		case model.DirRead:
			ensure(ai.reads, lg.ActionID)[lg.Path] = struct{}{}
		case model.DirWrite:
			ensure(ai.writes, lg.ActionID)[lg.Path] = struct{}{}
		}
	}
	for _, tl := range toolLogs {
		ensure(ai.tools, tl.ActionID)[tl.Path] = struct{}{}
	}
	ai.allLogs = logs
	return ai
}

// Reads 返回动作实际读取路径（排序）。
func (ai *AccessIndex) Reads(actionID int64) []string {
	return sortedKeys(ai.reads[actionID])
}

// Writes 返回动作实际写入路径（排序）。
func (ai *AccessIndex) Writes(actionID int64) []string {
	return sortedKeys(ai.writes[actionID])
}

// ReadsSet 返回动作实际读取路径集合。
func (ai *AccessIndex) ReadsSet(actionID int64) map[string]struct{} {
	return ai.reads[actionID]
}

// WritesSet 返回动作实际写入路径集合。
func (ai *AccessIndex) WritesSet(actionID int64) map[string]struct{} {
	return ai.writes[actionID]
}

// ToolsUsed 返回动作实际使用的工具链名集合。
func (ai *AccessIndex) ToolsUsed(actionID int64) map[string]struct{} {
	return ai.tools[actionID]
}

// HasObservations 判断动作是否有任何观测日志。
func (ai *AccessIndex) HasObservations(actionID int64) bool {
	return len(ai.reads[actionID])+len(ai.writes[actionID]) > 0
}

// All 返回全部日志条目。
func (ai *AccessIndex) All() []model.LogEntry {
	return ai.allLogs
}

// MergeLogs 合并两批日志：同 (action, seq) 去重，冲突内容保留双方（返回冲突条目）。
// 返回 (合并后日志, 冲突日志)。
func MergeLogs(a, b []model.LogEntry) ([]model.LogEntry, []model.LogEntry) {
	type key struct {
		action int64
		seq    int
	}
	seen := make(map[key]model.LogEntry)
	var order []key
	var conflicts []model.LogEntry
	for _, lg := range append(append([]model.LogEntry(nil), a...), b...) {
		k := key{action: lg.ActionID, seq: lg.Seq}
		if prev, ok := seen[k]; ok {
			if prev.ContentHash != lg.ContentHash {
				// 内容冲突：保留双方，标记后者为冲突。
				conflicts = append(conflicts, lg)
			}
			continue
		}
		seen[k] = lg
		order = append(order, k)
	}
	var merged []model.LogEntry
	for _, k := range order {
		merged = append(merged, seen[k])
	}
	return merged, conflicts
}

// ensure 返回（必要时创建）嵌套 map。
func ensure(m map[int64]map[string]struct{}, key int64) map[string]struct{} {
	if m[key] == nil {
		m[key] = make(map[string]struct{})
	}
	return m[key]
}

func sortedKeys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// SortByActionSeq 按 (action, seq) 稳定排序日志。
func SortByActionSeq(logs []model.LogEntry) {
	sort.SliceStable(logs, func(i, j int) bool {
		if logs[i].ActionID != logs[j].ActionID {
			return logs[i].ActionID < logs[j].ActionID
		}
		return logs[i].Seq < logs[j].Seq
	})
}
