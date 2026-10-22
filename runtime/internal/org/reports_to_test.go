package org

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 上级只记成员编号；人名（岗位）是读取时派生的，真相源里不存第二份。
func TestMemberReportsToAndLabel(t *testing.T) {
	o, err := Load(fixture(t, "domains"))
	if err != nil {
		t.Fatal(err)
	}
	bob, ok := o.Member("bob")
	if !ok {
		t.Fatal("取不到 bob")
	}
	if bob.ReportsTo != "alice" {
		t.Fatalf("reports_to 没读进来：%q", bob.ReportsTo)
	}
	if got := o.MemberLabel(bob.ReportsTo); got != "Alice Wang（经理）" {
		t.Fatalf("上级没派生成人名（岗位）：%q", got)
	}
	if got := o.MemberLabel(""); got != "" {
		t.Fatalf("空编号应当派生空串，实际 %q", got)
	}
	if got := o.MemberLabel("nobody"); got != "" {
		t.Fatalf("解不出的编号应当如实空串，实际 %q", got)
	}
}

func setReportsTo(t *testing.T, vault, member, value string) {
	t.Helper()
	path := filepath.Join(vault, "members", member, "persona.md")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := strings.ReplaceAll(string(b), "\r\n", "\n")
	text = strings.Replace(text, "reports_to: alice", "reports_to: "+value, 1)
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
}

func hasRule(issues []Issue, rule string) bool {
	for _, i := range issues {
		if i.Rule == rule {
			return true
		}
	}
	return false
}

// 写错上级不拦启动，但必须被看见：原样保留编号，看板不会把它抹成「—」。
func TestReportsToUnknownIsWarned(t *testing.T) {
	v := copyVault(t, "domains")
	setReportsTo(t, v, "bob", "ghost")
	o, err := Load(v)
	if err != nil {
		t.Fatalf("写错上级不该拦启动：%v", err)
	}
	if !hasRule(o.Warnings, "member.reports_to.unknown") {
		t.Fatalf("没有报 member.reports_to.unknown：%+v", o.Warnings)
	}
	bob, _ := o.Member("bob")
	if bob.ReportsTo != "ghost" {
		t.Fatalf("写错的编号被吞了：%q", bob.ReportsTo)
	}
	if got := o.MemberLabel(bob.ReportsTo); got != "" {
		t.Fatalf("解不出的上级不该编一个人名，实际 %q", got)
	}
}

func TestReportsToSelfIsWarned(t *testing.T) {
	v := copyVault(t, "domains")
	setReportsTo(t, v, "bob", "bob")
	o, err := Load(v)
	if err != nil {
		t.Fatalf("指向自己不该拦启动：%v", err)
	}
	if !hasRule(o.Warnings, "member.reports_to.self") {
		t.Fatalf("没有报 member.reports_to.self：%+v", o.Warnings)
	}
}
