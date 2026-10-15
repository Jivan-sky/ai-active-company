package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

// ---------- 议题 #19：拷一个 exe 就能 init ----------
//
// 模板有两个来处：磁盘（--templates / exe 同级 / cwd，优先，现场改不用重编）与**内嵌**（编进 exe，
// 三处都没有时兜底）。这一组盯的是**内嵌那一份与磁盘那一份一样全** ——
// 少了任何一份，`anc init` 会在半路挂，而它是装机第一步。

func embeddedTemplates() templateSource {
	return templateSource{Kind: "内嵌（测试）", fsys: embeddedRuntime, prefix: "templates/"}
}

// `resolveTemplates` 认「这是模板目录」靠的就是 README.md.tmpl：内嵌这一份必须有。
// 顺带把 tplMap 上每一条都读一遍 —— 少一条，init 就走到一半挂。
func TestEmbeddedTemplatesCoverEveryTemplate(t *testing.T) {
	src := embeddedTemplates()
	if _, err := src.Read("README.md.tmpl"); err != nil {
		t.Fatalf("内嵌那一份连 README.md.tmpl 都没有（resolveTemplates 认它靠这一份）：%v", err)
	}
	for _, e := range tplMap {
		if _, err := src.Read(e.Src); err != nil {
			t.Errorf("内嵌模板缺 %s：%v", e.Src, err)
		}
	}
	if got, want := countTemplates(src), len(tplMap); got != want {
		t.Errorf("内嵌模板数 %d，tplMap 有 %d 条", got, want)
	}
	// org/ 骨架（`anc org init` 走它）整棵都得在。
	sub, err := src.Sub("org")
	if err != nil {
		t.Fatalf("内嵌树里没有 org/ 这一棵：%v", err)
	}
	for _, name := range []string{"company.md.tmpl", "domains.md.tmpl", "projects.md.tmpl",
		"member-persona.md.tmpl", "role-manager.md.tmpl", "role-devbot.md.tmpl",
		"charter-dir-CLAUDE.md.tmpl", "context-dir-CLAUDE.md.tmpl", "data-dir-CLAUDE.md.tmpl"} {
		if _, err := sub.Read(name); err != nil {
			t.Errorf("内嵌 org/ 骨架缺 %s：%v", name, err)
		}
	}
	// assets 那份也编进去了：只有 exe 的机器上 `anc assets` 要能用得上（`anc assets` 读它）。
	if _, err := src.Open("inventory/assets.md"); err != nil {
		t.Errorf("内嵌树里没有 inventory/assets.md：%v", err)
	}
}

// 内嵌那一份必须与磁盘那一份**逐字节相同、一份不缺**：两处漂了，等于「拷 exe 装」与「在仓库里装」
// 得到两个不同的东西。用例的 cwd 就是 runtime/（go test 会切到包目录），磁盘那一份读的是包内的 templates/。
func TestEmbeddedTemplatesMatchDisk(t *testing.T) {
	src := embeddedTemplates()
	err := filepath.WalkDir("templates", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, _ := filepath.Rel("templates", p)
		onDisk, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		inExe, err := src.Read(filepath.ToSlash(rel))
		if err != nil {
			t.Errorf("磁盘上有 %s，内嵌那一份里没有：%v", rel, err)
			return nil
		}
		if string(onDisk) != string(inExe) {
			t.Errorf("%s 两份不一致（长度 %d / %d）", rel, len(onDisk), len(inExe))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
