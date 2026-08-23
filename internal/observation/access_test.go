package observation

import (
	"reflect"
	"testing"

	"task191-reproof/internal/model"
)

func TestMergeLogs(t *testing.T) {
	a := []model.LogEntry{
		{ActionID: 1, Seq: 1, Path: "x", Direction: model.DirRead, ContentHash: "h1"},
		{ActionID: 1, Seq: 2, Path: "y", Direction: model.DirWrite, ContentHash: "h2"},
	}
	b := []model.LogEntry{
		{ActionID: 1, Seq: 1, Path: "x", Direction: model.DirRead, ContentHash: "h1"},       // 重复，忽略
		{ActionID: 1, Seq: 3, Path: "z", Direction: model.DirRead, ContentHash: "h3"},       // 新增
		{ActionID: 1, Seq: 2, Path: "y", Direction: model.DirWrite, ContentHash: "h2-DIFF"}, // 冲突
	}
	merged, conflicts := MergeLogs(a, b)
	if len(merged) != 3 {
		t.Fatalf("合并后应为 3 条，得到 %d: %+v", len(merged), merged)
	}
	if len(conflicts) != 1 {
		t.Fatalf("应有 1 条冲突，得到 %d", len(conflicts))
	}
	// 冲突内容保留：合并集中仍包含原始 h2。
	for _, lg := range merged {
		if lg.ContentHash == "h2-DIFF" {
			t.Fatal("冲突内容不应覆盖原内容")
		}
	}
}

func TestAccessIndex(t *testing.T) {
	logs := []model.LogEntry{
		{ActionID: 1, Seq: 1, Path: "a", Direction: model.DirRead, ContentHash: "h"},
		{ActionID: 1, Seq: 2, Path: "b", Direction: model.DirWrite, ContentHash: "h2"},
		{ActionID: 1, Seq: 3, Path: "a", Direction: model.DirRead, ContentHash: "h"},
	}
	toolLogs := []model.LogEntry{
		{ActionID: 1, Seq: 4, Path: "go", Direction: model.DirWrite, ContentHash: ""},
	}
	ai := BuildAccessIndex(logs, toolLogs)
	reads := ai.Reads(1)
	writes := ai.Writes(1)
	if !reflect.DeepEqual(reads, []string{"a"}) {
		t.Fatalf("reads 应为 [a]，得到 %v", reads)
	}
	if !reflect.DeepEqual(writes, []string{"b"}) {
		t.Fatalf("writes 应为 [b]，得到 %v", writes)
	}
	tools := ai.ToolsUsed(1)
	if len(tools) != 1 {
		t.Fatalf("tools 应为 1 个，得到 %v", tools)
	}
	if !ai.HasObservations(1) || ai.HasObservations(2) {
		t.Fatal("观测存在性判断错误")
	}
}

func TestSortByActionSeq(t *testing.T) {
	logs := []model.LogEntry{
		{ActionID: 2, Seq: 1, Path: "b"},
		{ActionID: 1, Seq: 2, Path: "a"},
		{ActionID: 1, Seq: 1, Path: "c"},
	}
	SortByActionSeq(logs)
	if logs[0].Path != "c" || logs[1].Path != "a" || logs[2].Path != "b" {
		t.Fatalf("排序错误: %+v", logs)
	}
}
