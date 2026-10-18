package distill

import (
	"strings"
	"testing"

	"anc/internal/timeline"
)

func goodDraft(name, caseKey string) string {
	return "---\nname: " + name + "\nstatus: draft\ncase: " + caseKey + "\n---\n\n# 对账\n\n" +
		"## 何时用\n月底对账时。\n\n## 输入\n账单 CSV。\n\n## 步骤\n1. 对齐单号。\n\n## 验证\n差异为零。\n"
}

func checkInput(name, draft string) CheckInput {
	return CheckInput{
		Name: name, DraftExists: true, Draft: draft,
		RolesWired: true, Roles: []RoleSkills{{Role: "manager", Skills: []string{name}}},
		TimelineWired: true,
		Entries: []timeline.Entry{
			row("c1", "case-md", "2026-10-18T10:00:00+08:00", "done", "对账", "trade", "做完了"),
		},
	}
}

func itemOf(r CheckReport, id string) *CheckItem {
	for i := range r.Items {
		if r.Items[i].ID == id {
			return &r.Items[i]
		}
	}
	return nil
}

func TestCheckPassesCompleteDraft(t *testing.T) {
	r := Check(checkInput("reconcile", goodDraft("reconcile", "case-md")))
	if !r.Ok || r.Fails != 0 || r.Warns != 0 {
		t.Fatalf("完整草稿应直接通过：ok=%v fails=%d warns=%d items=%+v", r.Ok, r.Fails, r.Warns, r.Items)
	}
	if it := itemOf(r, "case.provenance"); it == nil || it.Level != LevelOK {
		t.Fatalf("来源 case 应判绿：%+v", it)
	}
}

func TestCheckBlocksMissingSections(t *testing.T) {
	draft := strings.Replace(goodDraft("reconcile", "case-md"), "## 验证\n差异为零。\n", "", 1)
	r := Check(checkInput("reconcile", draft))
	if r.Ok || r.Fails != 1 {
		t.Fatalf("缺「验证」应拦发布：ok=%v fails=%d", r.Ok, r.Fails)
	}
	if it := itemOf(r, "draft.section.验证"); it == nil || it.Level != LevelFail {
		t.Fatalf("没点名缺「验证」：%+v", it)
	}
	empty := strings.Replace(goodDraft("reconcile", "case-md"), "## 步骤\n1. 对齐单号。\n", "## 步骤\n   \n", 1)
	r = Check(checkInput("reconcile", empty))
	if it := itemOf(r, "draft.section.步骤"); it == nil || it.Level != LevelFail {
		t.Fatalf("空的「步骤」应判 fail：%+v", it)
	}
}

func TestCheckBlocksPlaceholderAndSecret(t *testing.T) {
	draft := strings.Replace(goodDraft("reconcile", "case-md"), "1. 对齐单号。", "1. TODO 对齐单号。", 1)
	if r := Check(checkInput("reconcile", draft)); r.Ok || itemOf(r, "draft.placeholder").Level != LevelFail {
		t.Fatalf("TODO 应拦发布：%+v", r.Items)
	}
	draft = strings.Replace(goodDraft("reconcile", "case-md"), "账单 CSV。", "账单 CSV。\napi_key: sk-test-123456", 1)
	r := Check(checkInput("reconcile", draft))
	if r.Ok || itemOf(r, "draft.secret").Level != LevelFail {
		t.Fatalf("明文密钥应拦发布：%+v", r.Items)
	}
	if strings.Contains(itemOf(r, "draft.secret").Detail, "sk-test-123456") {
		t.Fatal("检查结果里不许回显明文")
	}
}

func TestCheckWarnsButDoesNotBlock(t *testing.T) {
	in := checkInput("reconcile", goodDraft("reconcile", "case-md"))
	in.Roles = []RoleSkills{{Role: "manager", Skills: []string{"now"}}}
	in.Draft = strings.Replace(in.Draft, "账单 CSV。", "账单 CSV。\n见 D:\\work\\a.md", 1)
	r := Check(in)
	if !r.Ok || r.Fails != 0 {
		t.Fatalf("只有 warn 时不该拦发布：ok=%v fails=%d", r.Ok, r.Fails)
	}
	for _, id := range []string{"roles.declared", "draft.absolute_path"} {
		if it := itemOf(r, id); it == nil || it.Level != LevelWarn {
			t.Fatalf("%s 应是 warn：%+v", id, it)
		}
	}
}

func TestCheckBlocksNameMismatchAndCollision(t *testing.T) {
	r := Check(checkInput("reconcile", goodDraft("other-name", "case-md")))
	if r.Ok || itemOf(r, "draft.name_matches").Level != LevelFail {
		t.Fatalf("frontmatter name 不一致应拦：%+v", r.Items)
	}
	in := checkInput("reconcile", goodDraft("reconcile", "case-md"))
	in.OfficialExists = true
	r = Check(in)
	if r.Ok || itemOf(r, "official.collision").Level != LevelFail {
		t.Fatalf("正式落点已存在应拦：%+v", r.Items)
	}
}

func TestCheckInvalidNameStopsEarly(t *testing.T) {
	r := Check(CheckInput{Name: "Bad Name", DraftExists: true, Draft: "x"})
	if r.Ok || r.Fails != 1 || len(r.Items) != 1 {
		t.Fatalf("非法名字应只回一条 fail：%+v", r.Items)
	}
}
