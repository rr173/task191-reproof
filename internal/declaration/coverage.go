package declaration

import (
	"task191-reproof/internal/model"
)

// Coverage 是一次分析中对单个动作声明覆盖情况的结论。
type Coverage struct {
	ActionID int64
	// UndeclaredReads 是实际读取但未声明的路径。
	UndeclaredReads []string
	// UndeclaredWrites 是实际写入但未声明的路径。
	UndeclaredWrites []string
	// MissingToolchains 是动作使用了但未声明版本/校验和的工具链名。
	MissingToolchains []string
	// MissingInputs 是声明读取但图内无人生产、且非种子输入的路径。
	MissingInputs []string
}

// Evaluate 评估单个动作的声明覆盖情况。
//   - reads / writes: 实际访问（来自观测日志）按方向分组。
//   - usedTools: 实际使用的工具链名集合。
//   - produced: 全图动作声明写入的路径并集（供 MissingInputs 判定）。
//   - seedWrites: 种子输入（外部注入、不需要任何动作生产）。
func Evaluate(actionID int64, idx *Index, reads, writes map[string]struct{}, usedTools map[string]struct{}, produced, seedWrites map[string]struct{}) Coverage {
	c := Coverage{ActionID: actionID}
	for p := range reads {
		if !idx.Declared(actionID, p, model.DirRead) {
			c.UndeclaredReads = append(c.UndeclaredReads, p)
		}
	}
	for p := range writes {
		if !idx.Declared(actionID, p, model.DirWrite) {
			c.UndeclaredWrites = append(c.UndeclaredWrites, p)
		}
	}
	for t := range usedTools {
		if !toolDeclared(idx, actionID, t) {
			c.MissingToolchains = append(c.MissingToolchains, t)
		}
	}
	// 声明读取但无人生产：合并 produced 与 seedWrites 判断。
	// produced 是全图所有动作写声明的并集；seedWrites 是外部种子。
	for _, p := range idx.Reads(actionID) {
		if _, ok := produced[p]; ok {
			continue
		}
		if _, ok := seedWrites[p]; ok {
			continue
		}
		c.MissingInputs = append(c.MissingInputs, p)
	}
	sortStrings(c.UndeclaredReads)
	sortStrings(c.UndeclaredWrites)
	sortStrings(c.MissingToolchains)
	sortStrings(c.MissingInputs)
	return c
}

// toolDeclared 判断动作是否声明了指定工具链（带版本）。
func toolDeclared(idx *Index, actionID int64, tool string) bool {
	for _, d := range idx.byAction[actionID] {
		if d.Kind == "toolchain" && d.Path == tool {
			return true
		}
	}
	return false
}

// sortStrings 原地排序字符串切片。
func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

// AllClean 判断覆盖结论是否无任何问题。
func (c Coverage) AllClean() bool {
	return len(c.UndeclaredReads) == 0 &&
		len(c.UndeclaredWrites) == 0 &&
		len(c.MissingToolchains) == 0 &&
		len(c.MissingInputs) == 0
}

// FirstProblem 返回第一个问题的描述（供详情展示）。
func (c Coverage) FirstProblem() string {
	if len(c.UndeclaredReads) > 0 {
		return "未声明读取: " + c.UndeclaredReads[0]
	}
	if len(c.UndeclaredWrites) > 0 {
		return "未声明写入: " + c.UndeclaredWrites[0]
	}
	if len(c.MissingToolchains) > 0 {
		return "工具链未声明版本: " + c.MissingToolchains[0]
	}
	if len(c.MissingInputs) > 0 {
		return "读取不存在输入: " + c.MissingInputs[0]
	}
	return ""
}
