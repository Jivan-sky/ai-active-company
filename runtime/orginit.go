package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"anc/internal/org"
)

const orgInitUsage = `anc org init —— 生成一棵 org 真相源骨架（vault）

用法：
  anc org init <目录> --client <客户名> --id <ascii-id> [选项]

选项：
  --id <ascii>     公司 id：会进服务名与 project 前缀，必须是小写 ASCII（^[a-z][a-z0-9-]*$）
  --members <名单> 成员名，逗号分隔（默认 alice,bob）；devbot 会自动补上，一个公司恰好一个
  --admin <成员>   管理员（默认第一个成员）；它进 admin_from，能发特权命令
  --data <目录名>  顶层数据目录，逗号分隔（默认 10-knowledge,20-ops）
  --date <日期>    写进 company.md（默认今天）
  --templates <目录> 模板目录
  --force          目标目录非空时仍继续
  --git            生成后 git init + 首次提交（局部身份，不碰全局配置）

生成的是**骨架**，不是成品：真实 app_id / open_id 要人工填，角色的 allowed_tools 要人工定。
生成完会自动跑一次校验并把非红档发现打出来。
`

func cmdOrgInit(args []string) int {
	fs := flag.NewFlagSet("org init", flag.ContinueOnError)
	client := fs.String("client", "", "客户名（必填）")
	id := fs.String("id", "", "公司 id（必填，小写 ASCII）")
	members := fs.String("members", "alice,bob", "成员名，逗号分隔")
	admin := fs.String("admin", "", "管理员（默认第一个成员）")
	data := fs.String("data", "10-knowledge,20-ops", "顶层数据目录，逗号分隔")
	date := fs.String("date", time.Now().Format("2006-01-02"), "日期 YYYY-MM-DD")
	tplDir := fs.String("templates", "", "模板目录")
	force := fs.Bool("force", false, "目录非空仍继续")
	doGit := fs.Bool("git", false, "生成后 git init + 首次提交")
	flagArgs, posArgs := splitArgs(args, map[string]bool{"force": true, "git": true})
	if err := fs.Parse(flagArgs); err != nil {
		return 2
	}
	if *client == "" || *id == "" || len(posArgs) == 0 {
		fmt.Fprint(os.Stderr, orgInitUsage)
		return 2
	}
	if !org.ValidCompanyID(*id) {
		fmt.Fprintf(os.Stderr, "错误：--id=%q 不合法。必须是小写 ASCII（^[a-z][a-z0-9-]*$）——\n"+
			"它会进服务名（launchd label / 计划任务名）与 project 前缀，中文或大写会在那里出问题。\n", *id)
		return 2
	}
	if _, err := time.Parse("2006-01-02", *date); err != nil {
		fmt.Fprintf(os.Stderr, "错误：--date 需为 YYYY-MM-DD，收到 %s\n", *date)
		return 2
	}
	names := splitList(*members)
	if len(names) == 0 {
		fmt.Fprintln(os.Stderr, "错误：--members 为空")
		return 2
	}
	if strings.Contains(strings.Join(names, ","), "devbot") {
		fmt.Fprintln(os.Stderr, "错误：devbot 会自动生成，别写进 --members")
		return 2
	}
	names = append(names, "devbot")
	adminName := *admin
	if adminName == "" {
		adminName = names[0]
	}
	if !contains(names, adminName) {
		fmt.Fprintf(os.Stderr, "错误：--admin=%q 不在成员名单里（%s）\n", adminName, strings.Join(names, ", "))
		return 2
	}
	dirs := splitList(*data)
	if len(dirs) == 0 {
		fmt.Fprintln(os.Stderr, "错误：--data 为空（至少要有一个数据目录，否则 persona 的路由表是空的）")
		return 2
	}

	abs, err := filepath.Abs(posArgs[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误：%v\n", err)
		return 1
	}
	if !*force {
		if entries, err := os.ReadDir(abs); err == nil && len(entries) > 0 {
			fmt.Fprintf(os.Stderr, "错误：%s 已存在且非空（要往里写请加 --force）\n", abs)
			return 1
		}
	}
	tpl, err := resolveOrgTemplates(*tplDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误：%v\n", err)
		return 1
	}

	base := map[string]string{
		"{{CLIENT}}":      *client,
		"{{ID}}":          *id,
		"{{ADMIN}}":       adminName,
		"{{DATE}}":        *date,
		"{{ANC_VERSION}}": version,
	}
	written := 0
	write := func(src, dst string, vars map[string]string) error {
		raw, err := tpl.Read(src)
		if err != nil {
			return err
		}
		v := map[string]string{}
		for k, val := range base {
			v[k] = val
		}
		for k, val := range vars {
			v[k] = val
		}
		out := strings.TrimRight(render(string(raw), v), "\n") + "\n"
		p := filepath.Join(abs, filepath.FromSlash(dst))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(p, []byte(out), 0o644); err != nil {
			return err
		}
		written++
		return nil
	}

	if err := write("company.md.tmpl", "company/company.md", nil); err != nil {
		fmt.Fprintf(os.Stderr, "错误：%v\n", err)
		return 1
	}
	// 域表（罗盘）随骨架一起给：能力默认可用，内容默认空着 —— 由 agent 在访谈后自己填。
	if err := write("domains.md.tmpl", "domains.md", map[string]string{"{{DOMAIN_DATA}}": dirs[0]}); err != nil {
		fmt.Fprintf(os.Stderr, "错误：%v\n", err)
		return 1
	}
	// 项目表（立项书汇总）同样随骨架给：能力默认可用、内容默认空着 ——
	// 立项书本身由各人的 agent 那边生成，ANC 只留落点与汇总。
	if err := write("projects.md.tmpl", "projects.md", nil); err != nil {
		fmt.Fprintf(os.Stderr, "错误：%v\n", err)
		return 1
	}
	// 立项书副本落点（接入链的落点那一半）：目录建好、约定写在里面的 CLAUDE.md 里，
	// 内容默认空着 —— 副本本身由客户 / FDE 那边同步进来。
	if err := write("charter-dir-CLAUDE.md.tmpl", "charters/CLAUDE.md", nil); err != nil {
		fmt.Fprintf(os.Stderr, "错误：%v\n", err)
		return 1
	}
	// 工作上下文冻结副本的落点：与 charters/ 同一类 —— 落点先建好、说明先写好，
	// 冻不冻、冻几份是运行态的事。
	if err := write("context-dir-CLAUDE.md.tmpl", "context/CLAUDE.md", nil); err != nil {
		fmt.Fprintf(os.Stderr, "错误：%v\n", err)
		return 1
	}
	if err := write("role-manager.md.tmpl", "roles/manager/persona.md",
		map[string]string{"{{ROLE_TITLE}}": "经理"}); err != nil {
		fmt.Fprintf(os.Stderr, "错误：%v\n", err)
		return 1
	}
	if err := write("role-devbot.md.tmpl", "roles/devbot/persona.md", nil); err != nil {
		fmt.Fprintf(os.Stderr, "错误：%v\n", err)
		return 1
	}
	for _, n := range names {
		role := "manager"
		if n == "devbot" {
			role = "devbot"
		}
		if err := write("member-persona.md.tmpl", "members/"+n+"/persona.md", map[string]string{
			"{{MEMBER_NAME}}":    n,
			"{{MEMBER_DISPLAY}}": displayName(n),
			"{{MEMBER_ROLE}}":    role,
			"{{APP_ID}}":         "cli_" + *id + "_" + n,
			"{{OPEN_ID}}":        "ou_" + n,
		}); err != nil {
			fmt.Fprintf(os.Stderr, "错误：%v\n", err)
			return 1
		}
	}
	for _, d := range dirs {
		if err := write("data-dir-CLAUDE.md.tmpl", d+"/CLAUDE.md",
			map[string]string{"{{DATA_DIR}}": d}); err != nil {
			fmt.Fprintf(os.Stderr, "错误：%v\n", err)
			return 1
		}
	}

	fmt.Printf("✅ 已生成 org 真相源骨架：%s\n\n", abs)
	fmt.Printf("   公司   %s（id=%s）\n", *client, *id)
	fmt.Printf("   成员   %s\n", strings.Join(names, ", "))
	fmt.Printf("   数据目录 %s\n", strings.Join(dirs, ", "))
	fmt.Printf("   文件   %d 个\n\n", written)

	// 自检：骨架里「故意留白」的红档是预期内的（留给人填），其余红档 = 我模板写错了。
	// 两者必须分开 —— 否则每次 init 都会把「请你填」误报成「模板 bug，请报给我」。
	blanks := map[string]bool{"role.allowed_tools.empty": true}
	var expectedFatal []org.Issue
	o, loadErr := org.Load(abs)
	if loadErr == nil {
		printIssues(o.Warnings)
	} else {
		le, ok := loadErr.(*org.LoadError)
		if !ok {
			fmt.Fprintf(os.Stderr, "错误：生成的骨架没通过校验（模板有问题，请报给我）：\n%v\n", loadErr)
			return 1
		}
		var unexpected []org.Issue
		for _, i := range le.Report.Fatal() {
			if blanks[i.Rule] {
				expectedFatal = append(expectedFatal, i)
				continue
			}
			unexpected = append(unexpected, i)
		}
		if len(unexpected) > 0 {
			fmt.Fprintf(os.Stderr, "错误：生成的骨架没通过校验（模板有问题，请报给我）：\n  - %s\n",
				strings.Join(org.IssueStrings(unexpected), "\n  - "))
			return 1
		}
		printIssues(le.Report.Warns())
	}
	fmt.Println()
	if len(expectedFatal) > 0 {
		fmt.Printf("   骨架里 %d 条红档是**故意**的（第 2 条要你填的就是它）：\n", len(expectedFatal))
		for _, i := range expectedFatal {
			fmt.Printf("     - %s\n", i)
		}
	}
	fmt.Println("   下一步（骨架不是成品）：")
	fmt.Println("   1) members/*/persona.md 里的 app_id / open_id 是**占位符**，换成真实的；")
	fmt.Println("      发 /whoami 给 bot 可以拿 open_id。不换就连不上、也认不出谁在说话。")
	fmt.Println("   2) roles/*/persona.md 的 allowed_tools 是故意留空的，所以校验现在是红的：")
	fmt.Println("      实测 dontAsk 下没预授权的工具会被自动拒绝，空着 = bot 连得上却干不了活。")
	fmt.Println("      填上这个角色真正需要的工具名，红档才会消，anc render 也才会放行。")
	fmt.Println("   3) 每个数据目录的 CLAUDE.md 首句会进 persona 路由表，写清楚它才找得着路。")
	fmt.Println("   4) 立项书副本放 charters/<slug>/（目录名 = projects.md 的 slug）；")
	fmt.Println("      真源在客户侧，projects.md 的 source 列指向它。")
	fmt.Println("   5) 工作上下文要冻一份存档就落 context/：anc context freeze <vault> --who <编号>；")
	fmt.Println("      只是要看一眼就去看板的「上下文」页（同一份装配，换一个摆法）。")
	fmt.Printf("   6) anc org check %s\n", abs)

	if *doGit {
		if _, err := exec.LookPath("git"); err != nil {
			fmt.Println("\n⚠️  未找到 git，跳过 --git。")
			return 0
		}
		steps := [][]string{
			{"init", "-q"},
			{"add", "-A"},
			{"-c", "user.name=anc", "-c", "user.email=anc@local", "commit", "-q", "-m",
				fmt.Sprintf("anc org init: %s (id=%s)", *client, *id)},
		}
		for _, s := range steps {
			c := exec.Command("git", s...)
			c.Dir = abs
			if out, err := c.CombinedOutput(); err != nil {
				fmt.Printf("\n⚠️  git %s 失败：%v %s\n", strings.Join(s, " "), err, strings.TrimSpace(string(out)))
				return 0
			}
		}
		fmt.Println("\n   git：已 init 并完成首次提交（局部身份 anc/anc@local）")
	}
	return 0
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// displayName 是成员名的默认显示名（alice → Alice）；中文名原样保留。
func displayName(name string) string {
	r := []rune(name)
	if len(r) == 0 {
		return name
	}
	if r[0] >= 'a' && r[0] <= 'z' {
		r[0] = r[0] - 'a' + 'A'
	}
	return string(r)
}

// resolveOrgTemplates 找 org/ 骨架模板（`templates/org/`）—— 磁盘与内嵌两个来处都取得到。
func resolveOrgTemplates(explicit string) (templateSource, error) {
	base, err := resolveTemplates(explicit)
	if err != nil {
		return templateSource{}, err
	}
	sub, err := base.Sub("org")
	if err != nil {
		return templateSource{}, fmt.Errorf("模板里缺 org/ 这一棵：%v", err)
	}
	// 认得出这一棵要靠里头那几份真在（缺了就是模板不全，报出来别装作能用）。
	if _, err := sub.Read("company.md.tmpl"); err != nil {
		return templateSource{}, fmt.Errorf("模板里缺 org/company.md.tmpl：%v", err)
	}
	return sub, nil
}
