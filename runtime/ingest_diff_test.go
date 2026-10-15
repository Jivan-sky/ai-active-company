package main

import (
	"fmt"
	"strings"
	"testing"
)

// ---------- 落盘前那条 diff（议题 #47）的用例 ----------
//
// 盯的是：① 行号与 hunk 头算得对（人要照着它 review）；② 「末行没有结尾换行」不许被抹平；
// ③ 没变化就一个字都不打（不许给人看一份空摆设）。

func lines(n int) string {
	var sb strings.Builder
	for i := 1; i <= n; i++ {
		fmt.Fprintf(&sb, "第 %d 行\n", i)
	}
	return sb.String()
}

func TestUnifiedDiffIdenticalIsNil(t *testing.T) {
	if got := unifiedDiff("projects.md", lines(20), lines(20), false); got != nil {
		t.Fatalf("逐字相同该一个字都不打，得到：%q", got)
	}
}

func TestUnifiedDiffOneChangeInMiddle(t *testing.T) {
	oldDoc := lines(20)
	newDoc := strings.Replace(oldDoc, "第 10 行\n", "第 10 行（改过）\n", 1)
	got := unifiedDiff("projects.md", oldDoc, newDoc, false)
	want := []string{
		"--- a/projects.md",
		"+++ b/projects.md",
		"@@ -7,7 +7,7 @@",
		" 第 7 行",
		" 第 8 行",
		" 第 9 行",
		"-第 10 行",
		"+第 10 行（改过）",
		" 第 11 行",
		" 第 12 行",
		" 第 13 行",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("diff 不对。\n拿到：\n%s\n该是：\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestUnifiedDiffNewFile(t *testing.T) {
	got := unifiedDiff("members/赵四/persona.md", "", "第一行\n第二行\n", true)
	want := []string{
		"--- /dev/null",
		"+++ b/members/赵四/persona.md",
		"@@ -0,0 +1,2 @@",
		"+第一行",
		"+第二行",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("新文件的 diff 不对：\n%s", strings.Join(got, "\n"))
	}
}

// 末行有没有结尾换行是一件真差异 —— 按「去尾换行」切行就会把它抹平。
func TestUnifiedDiffTrailingNewlineIsRealDifference(t *testing.T) {
	got := unifiedDiff("domains.md", "a\nb", "a\nb\n", false)
	if len(got) == 0 {
		t.Fatal("`a\\nb` 与 `a\\nb\\n` 不是同一份文件，diff 不该是空的")
	}
	joined := strings.Join(got, "\n")
	if !strings.Contains(joined, `\ No newline at end of file`) {
		t.Fatalf("该按 git 的写法标出「没有结尾换行」，得到：\n%s", joined)
	}
}

func TestUnifiedDiffTwoFarApartHunks(t *testing.T) {
	oldDoc := lines(20)
	newDoc := strings.Replace(strings.Replace(oldDoc, "第 3 行\n", "第 3 行（改）\n", 1), "第 17 行\n", "第 17 行（改）\n", 1)
	got := unifiedDiff("projects.md", oldDoc, newDoc, false)
	if n := strings.Count(strings.Join(got, "\n"), "@@ -"); n != 2 {
		t.Fatalf("隔得远的两处改动该出两个 hunk，得到 %d 个：\n%s", n, strings.Join(got, "\n"))
	}
}

func TestUnifiedDiffNearbyChangesMergeIntoOneHunk(t *testing.T) {
	oldDoc := lines(20)
	newDoc := strings.Replace(strings.Replace(oldDoc, "第 8 行\n", "第 8 行（改）\n", 1), "第 12 行\n", "第 12 行（改）\n", 1)
	got := unifiedDiff("projects.md", oldDoc, newDoc, false)
	if n := strings.Count(strings.Join(got, "\n"), "@@ -"); n != 1 {
		t.Fatalf("上下文会重叠的两处改动该合成一个 hunk，得到 %d 个：\n%s", n, strings.Join(got, "\n"))
	}
}

func TestUnifiedDiffHunkHeadersCountLines(t *testing.T) {
	// 删掉第 6 行（f）：上下文各留 3 行 → 老侧 3..9 共 7 行、新侧 3..8 共 6 行。
	oldDoc := "a\nb\nc\nd\ne\nf\ng\nh\ni\nj\nk\n"
	newDoc := "a\nb\nc\nd\ne\ng\nh\ni\nj\nk\n"
	got := unifiedDiff("domains.md", oldDoc, newDoc, false)
	joined := strings.Join(got, "\n")
	if !strings.Contains(joined, "@@ -3,7 +3,6 @@") {
		t.Fatalf("hunk 头该是 @@ -3,7 +3,6 @@（开头那两行不在上下文窗口里），得到：\n%s", joined)
	}
	if n := strings.Count(joined, "@@ -"); n != 1 {
		t.Fatalf("只有一处改动，该只有一个 hunk，得到 %d 个：\n%s", n, joined)
	}
	body := got[3:] // 跳过 --- / +++ / @@ 三行，只看正文
	del, add := false, false
	for _, ln := range body {
		switch ln {
		case "-f":
			del = true
		case "+f":
			add = true
		}
	}
	if !del {
		t.Fatalf("删掉的那一行该标成 -f，得到：\n%s", joined)
	}
	if add {
		t.Fatalf("f 是被删掉的行，不该出现在 + 侧：\n%s", joined)
	}
}
