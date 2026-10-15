package main

import (
	"fmt"
	"strings"
)

// 落盘前那一条 diff（议题 #47）。
//
// 提案里装的是**整份文件全文**（口径见 ingest.go：补丁要匹配上下文，人写错一行整条就落不下去），
// 代价是：人拿到一份 JSON，看不出「相对库里现在这一份，改了哪几行」。
// 「模型只提案、人 review 后再落」这一步就卡在这里 —— 没东西可看，review 就成了点头。
//
// 所以这一支只回答一个问题：**这份提案会把文件改成什么样**。输出是 patch 文本。
// 与 git 那份同形状，但**不依赖 git**：真相源不要求是 git 仓库
// （用例里那棵 vault 就没有 .git，`copyTree` 也刻意不搬它）。
//
// 自己算，不引模块：这一层是零依赖（go.mod 里一个 require 都没有），
// 这点算法不值得为它破例。

// diffLineCap 是 DP 的规模闸：old 行数 × new 行数。
// 真相源里的 markdown 到不了这个量级；真到了，人也不看逐行 diff 了 ——
// 那时如实说一句「太大，不逐行比」，不给一次内存炸弹。
const diffLineCap = 2 << 20

// diffContext 是 hunk 两边各留几行上下文（与 git 的默认值一致）。
const diffContext = 3

// unifiedDiff 生成 old → new 的 unified diff 文本行。
// 逐行相同时返回 nil。isNew = 库里本来没有这份文件（那 old 为空，左头写 /dev/null）。
func unifiedDiff(path, old, new string, isNew bool) []string {
	if !isNew && old == new {
		return nil
	}
	a, b := splitDiffLines(old), splitDiffLines(new)
	out := diffHeader(path, isNew)
	if len(a)*len(b) > diffLineCap {
		return append(out, fmt.Sprintf("（%d 行 → %d 行：太大，不逐行比 —— 整份替换）", len(a), len(b)))
	}
	if len(b) == 0 && len(a) == 0 {
		return append(out, "（这份是空文件：0 行）")
	}
	hunks := diffHunks(diffOps(a, b))
	if len(hunks) == 0 {
		// 逐行相同（连结尾那个换行都一样）—— 没有 diff 可说。
		return nil
	}
	return append(out, hunks...)
}

// splitDiffLines 切行，**留着行尾的 \n**。
//
// 留着才分得清「末行有没有换行」：`a\nb` 与 `a\nb\n` 不是同一份文件，
// 按「去尾换行」切会把这条差异抹平 —— 而这份 diff 的用途正是给人看差异。
func splitDiffLines(s string) []string {
	if s == "" {
		return nil
	}
	var out []string
	for len(s) > 0 {
		i := strings.IndexByte(s, '\n')
		if i < 0 {
			return append(out, s)
		}
		out = append(out, s[:i+1])
		s = s[i+1:]
	}
	// 循环里每条出口都 return（最后那一行在上面），这句只为让编译器安心。
	return out
}

// diffHeader 是那两行头。左头 a/ 右头 b/ —— 与 git 同形状，粘进 `git apply -p1` 也认。
func diffHeader(path string, isNew bool) []string {
	left := "a/" + path
	if isNew {
		left = "/dev/null"
	}
	return []string{"--- " + left, "+++ b/" + path}
}

// diffOp 是编辑脚本的一条。tag：' ' 两边都有 · '-' 只在 old · '+' 只在 new。
type diffOp struct {
	tag  byte
	text string
}

// diffOps 走一遍 LCS：先算长度表，再从左上回溯成编辑脚本。
// 规模已被 unifiedDiff 那道闸限住（n × m ≤ diffLineCap）。
//
// tag 为 ' ' 时 text 是两边共有的那一行；'-' / '+' 各取自己那一侧的原样行。
func diffOps(a, b []string) []diffOp {
	n, m := len(a), len(b)
	// lcs[i][j] = a[i:] 与 b[j:] 的最长公共子序列长度（表从右下往左上填）。
	w := m + 1
	lcs := make([]int32, (n+1)*w)
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			switch {
			case a[i] == b[j]:
				lcs[i*w+j] = lcs[(i+1)*w+j+1] + 1
			case lcs[(i+1)*w+j] >= lcs[i*w+j+1]:
				lcs[i*w+j] = lcs[(i+1)*w+j]
			default:
				lcs[i*w+j] = lcs[i*w+j+1]
			}
		}
	}
	ops := make([]diffOp, 0, n+m)
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case a[i] == b[j]:
			ops = append(ops, diffOp{' ', a[i]})
			i, j = i+1, j+1
		case lcs[(i+1)*w+j] >= lcs[i*w+j+1]:
			ops = append(ops, diffOp{'-', a[i]})
			i++
		default:
			ops = append(ops, diffOp{'+', b[j]})
			j++
		}
	}
	for ; i < n; i++ {
		ops = append(ops, diffOp{'-', a[i]})
	}
	for ; j < m; j++ {
		ops = append(ops, diffOp{'+', b[j]})
	}
	return ops
}

// diffHunks 把编辑脚本切成带上下文的 hunk（行首那条 @@）。
//
// 上下文是为了让人判得出「这一段是在什么旁边改的」；重叠的两段合成一个 hunk，
// 免得同一段上下文被打印两遍。
func diffHunks(ops []diffOp) []string {
	var changed []int
	for i, op := range ops {
		if op.tag != ' ' {
			changed = append(changed, i)
		}
	}
	if len(changed) == 0 {
		return nil
	}
	type span struct{ lo, hi int } // [lo, hi)
	spans := make([]span, 0, len(changed))
	for _, c := range changed {
		lo := c - diffContext
		if lo < 0 {
			lo = 0
		}
		hi := c + diffContext + 1
		if hi > len(ops) {
			hi = len(ops)
		}
		if n := len(spans); n > 0 && lo <= spans[n-1].hi {
			if hi > spans[n-1].hi {
				spans[n-1].hi = hi
			}
			continue
		}
		spans = append(spans, span{lo, hi})
	}

	// 行号是「这份文件里的第几行」，不是编辑脚本里的第几条：'-' 推左边，'+' 推右边。
	aLine, bLine, idx := 1, 1, 0
	var out []string
	for _, s := range spans {
		for ; idx < s.lo; idx++ {
			switch ops[idx].tag {
			case '-':
				aLine++
			case '+':
				bLine++
			default:
				aLine++
				bLine++
			}
		}
		aStart, bStart, aCount, bCount := aLine, bLine, 0, 0
		body := make([]string, 0, s.hi-s.lo)
		for ; idx < s.hi; idx++ {
			body = append(body, diffMarkedLines(ops[idx])...)
			switch ops[idx].tag {
			case '-':
				aCount++
			case '+':
				bCount++
			default:
				aCount++
				bCount++
			}
		}
		aLine += aCount
		bLine += bCount
		// 一侧 0 行时，起点写它前面那一行的行号（git 的规矩：新增文件是 @@ -0,0 +1,N @@）。
		// 不这么写，git apply 认不出这是新文件。
		aHdr, bHdr := aStart, bStart
		if aCount == 0 {
			aHdr = aStart - 1
		}
		if bCount == 0 {
			bHdr = bStart - 1
		}
		out = append(out, fmt.Sprintf("@@ -%d,%d +%d,%d @@", aHdr, aCount, bHdr, bCount))
		out = append(out, body...)
	}
	return out
}

// diffMarkedLines 给一条编辑脚本加行首标记（' ' / '-' / '+'）。
//
// 末行没有结尾换行时，按 git 的写法补一句 —— 那份文件真的没有结尾换行，
// 不写就等于把这条差异藏起来（而它正是 `\n` 末尾这一处的差异）。
func diffMarkedLines(op diffOp) []string {
	if strings.HasSuffix(op.text, "\n") {
		return []string{string(op.tag) + strings.TrimSuffix(op.text, "\n")}
	}
	return []string{string(op.tag) + op.text, `\ No newline at end of file`}
}
