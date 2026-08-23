// Package declaration 处理动作输入/输出声明：构建声明索引、校验声明完整性。
package declaration

import (
	"sort"

	"task191-reproof/internal/model"
)

// Index 是动作声明的只读索引，加速声明查询。
type Index struct {
	// byAction: actionID -> 该动作全部声明。
	byAction map[int64][]*model.Declaration
	// set: 声明唯一键集合。
	set model.DeclSet
	// readsByAction / writesByAction: 动作的读/写路径集合。
	readsByAction  map[int64]map[string]struct{}
	writesByAction map[int64]map[string]struct{}
}

// BuildIndex 从声明列表构建只读索引。
func BuildIndex(decls []*model.Declaration) *Index {
	idx := &Index{
		byAction:       make(map[int64][]*model.Declaration),
		set:            make(model.DeclSet),
		readsByAction:  make(map[int64]map[string]struct{}),
		writesByAction: make(map[int64]map[string]struct{}),
	}
	for _, d := range decls {
		idx.byAction[d.ActionID] = append(idx.byAction[d.ActionID], d)
		idx.set.Add(model.DeclKey{ActionID: d.ActionID, Path: d.Path, Direction: d.Direction})
		if d.Direction == model.DirRead {
			if idx.writesByAction[d.ActionID] == nil {
				idx.writesByAction[d.ActionID] = make(map[string]struct{})
			}
			idx.writesByAction[d.ActionID][d.Path] = struct{}{}
		} else {
			if idx.readsByAction[d.ActionID] == nil {
				idx.readsByAction[d.ActionID] = make(map[string]struct{})
			}
			idx.readsByAction[d.ActionID][d.Path] = struct{}{}
		}
	}
	return idx
}

// Declared 判断动作在指定方向声明了路径。
func (i *Index) Declared(actionID int64, path string, dir model.AccessDirection) bool {
	return i.set.Has(model.DeclKey{ActionID: actionID, Path: path, Direction: dir})
}

// ByAction 返回动作全部声明。
func (i *Index) ByAction(actionID int64) []*model.Declaration {
	out := append([]*model.Declaration(nil), i.byAction[actionID]...)
	sort.Slice(out, func(a, b int) bool {
		if out[a].Path != out[b].Path {
			return out[a].Path < out[b].Path
		}
		return out[a].Direction < out[b].Direction
	})
	return out
}

// Reads 返回动作声明的读路径（排序）。
func (i *Index) Reads(actionID int64) []string {
	return sortedKeys(i.readsByAction[actionID])
}

// Writes 返回动作声明的写路径（排序）。
func (i *Index) Writes(actionID int64) []string {
	return sortedKeys(i.writesByAction[actionID])
}

// ReadsSet 返回动作声明的读路径集合。
func (i *Index) ReadsSet(actionID int64) map[string]struct{} {
	return i.readsByAction[actionID]
}

// WritesSet 返回动作声明的写路径集合。
func (i *Index) WritesSet(actionID int64) map[string]struct{} {
	return i.writesByAction[actionID]
}

// HasDeclarations 判断动作是否已有任何声明。
func (i *Index) HasDeclarations(actionID int64) bool {
	return len(i.byAction[actionID]) > 0
}

// MissingDeclarations 返回动作读声明中在给定候选写入集中不存在的路径。
// 用于「读取不存在输入」检测：动作声明读某路径，但无任何前序动作写该路径，
// 且该路径也不在种子输入（seedWrites）中。
func (i *Index) MissingDeclarations(actionID int64, produced map[string]struct{}, seedWrites map[string]struct{}) []string {
	var missing []string
	for _, p := range i.Reads(actionID) {
		if _, ok := produced[p]; ok {
			continue
		}
		if _, ok := seedWrites[p]; ok {
			continue
		}
		missing = append(missing, p)
	}
	return missing
}

// sortedKeys 返回 map 键的排序切片。
func sortedKeys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
