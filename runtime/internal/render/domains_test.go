package render

import (
	"strings"
	"testing"
)

// VisibleDomains 是 persona 段 8 与出站读出口共用的那把尺子：
// 自己的域全行（含数据在哪），别人的域只给三列（不许带 data / terms）。
func TestVisibleDomainsScoping(t *testing.T) {
	o, _ := fixtureOrg(t, "domains")
	alice, ok := o.Member("alice")
	if !ok {
		t.Fatal("取不到 alice")
	}
	mine, others := VisibleDomains(o, alice)
	if len(mine) != 1 || mine[0].Slug != "trade" {
		t.Fatalf("alice 自己的域应当只有 trade，实际 %+v", mine)
	}
	if mine[0].Data != "projects" {
		t.Errorf("自己的域要带上「数据在哪」，实际 %q", mine[0].Data)
	}
	if !strings.Contains(mine[0].Who, "Alice Wang") {
		t.Errorf("Who 要是派生的「人名（岗位）」，实际 %q", mine[0].Who)
	}
	if mine[0].Line == 0 {
		t.Error("行号要留着 —— 出站读出口拿它当证据指针")
	}
	if len(others) != 1 || others[0].Slug != "logistics" {
		t.Fatalf("别域目录应当只有 logistics，实际 %+v", others)
	}
	// 别人的域：只看得见名称 / 是什么 / 找谁。data 与 terms 连字段都没有。
	if others[0].Name == "" || others[0].What == "" || others[0].Who == "" {
		t.Errorf("别域目录三列不该空：%+v", others[0])
	}
}

// 一个域都没划：mine 空、others 全 —— 调用方照这个报「还没给你划域」，不编。
func TestVisibleDomainsWithoutAnyOwned(t *testing.T) {
	o, _ := fixtureOrg(t, "domains")
	devbot, ok := o.Member("devbot")
	if !ok {
		t.Fatal("取不到 devbot")
	}
	mine, others := VisibleDomains(o, devbot)
	if len(mine) != 0 {
		t.Fatalf("没划域就该是空的，实际 %+v", mine)
	}
	if len(others) != 2 {
		t.Fatalf("域表两行都该进目录，实际 %+v", others)
	}
}

// 域表没有这一栏：返回空，不是错（同 LoadDomains 的谦让）。
func TestVisibleDomainsWithoutTable(t *testing.T) {
	o, _ := fixtureOrg(t, "one")
	alice, ok := o.Member("alice")
	if !ok {
		t.Fatal("取不到 alice")
	}
	mine, others := VisibleDomains(o, alice)
	if len(mine) != 0 || len(others) != 0 {
		t.Fatalf("没有域表就该两个都空，实际 %+v / %+v", mine, others)
	}
}
