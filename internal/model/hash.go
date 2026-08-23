package model

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// HashBytes 计算字节内容 SHA-256 十六进制摘要。
func HashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// HashString 计算字符串 SHA-256 摘要。
func HashString(s string) string {
	return HashBytes([]byte(s))
}

// ActionPair 表示一条有序边（先执行 From，后执行 To）。
type ActionPair struct {
	From int64
	To   int64
}

// GraphDigest 根据动作集合与依赖边集合计算图摘要。
// 输入顺序对摘要无影响：动作 ID 升序、边按 (from,to) 排序后拼接。
func GraphDigest(actionIDs []int64, pairs []ActionPair) string {
	sortedActions := append([]int64(nil), actionIDs...)
	sort.Slice(sortedActions, func(i, j int) bool { return sortedActions[i] < sortedActions[j] })
	sortedPairs := append([]ActionPair(nil), pairs...)
	sort.Slice(sortedPairs, func(i, j int) bool {
		if sortedPairs[i].From != sortedPairs[j].From {
			return sortedPairs[i].From < sortedPairs[j].From
		}
		return sortedPairs[i].To < sortedPairs[j].To
	})
	var b strings.Builder
	for _, id := range sortedActions {
		b.WriteString("n:")
		b.WriteString(strconv.FormatInt(id, 10))
		b.WriteString(";")
	}
	for _, p := range sortedPairs {
		b.WriteString("e:")
		b.WriteString(strconv.FormatInt(p.From, 10))
		b.WriteString(">")
		b.WriteString(strconv.FormatInt(p.To, 10))
		b.WriteString(";")
	}
	return HashString(b.String())
}

// LogFingerprint 计算访问日志集合的摘要：按 (action,seq) 排序后拼接路径、方向与内容哈希。
func LogFingerprint(entries []LogEntry) string {
	sorted := append([]LogEntry(nil), entries...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].ActionID != sorted[j].ActionID {
			return sorted[i].ActionID < sorted[j].ActionID
		}
		return sorted[i].Seq < sorted[j].Seq
	})
	var b strings.Builder
	for _, e := range sorted {
		b.WriteString(fmt.Sprintf("%d@%d:%s:%s:%s:%d;", e.ActionID, e.Seq, e.Path, e.Direction, e.ContentHash, e.SizeBytes))
	}
	return HashString(b.String())
}

// LogEntry 表示一次访问事件（供指纹计算与合并使用）。
type LogEntry struct {
	ActionID    int64
	Seq         int
	Path        string
	Direction   AccessDirection
	ContentHash string
	SizeBytes   int64
}

// NormalizePath 规范化路径：去除首尾空白；空路径返回错误。
func NormalizePath(p string) (string, error) {
	p = strings.TrimSpace(p)
	if p == "" {
		return "", ErrEmptyPath
	}
	return p, nil
}

// ValidateDirection 校验访问方向。
func ValidateDirection(d AccessDirection) error {
	if d != DirRead && d != DirWrite {
		return ErrInvalidDirection
	}
	return nil
}

// DeclKey 唯一标识一个动作的一条声明。
type DeclKey struct {
	ActionID  int64
	Path      string
	Direction AccessDirection
}

// DeclSet 是动作声明集合，支持 O(1) 查询。
type DeclSet map[DeclKey]struct{}

// Add 添加声明。
func (d DeclSet) Add(k DeclKey) { d[k] = struct{}{} }

// Has 判断声明是否存在。
func (d DeclSet) Has(k DeclKey) bool { _, ok := d[k]; return ok }
