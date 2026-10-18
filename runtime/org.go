package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"anc/internal/org"
)

const orgWhoUsage = `anc org who —— 把平台那一侧的定位信息翻成 org 里的谁（只查表，不猜）

用法：
  anc org who <vault> [--open-id <ou_…>] [--app-id <cli_…>] [--project <名>] [--json]

选项：
  --open-id <ou_…>   谁发的 —— 平台那边唯一的「人」标识（只可能落在成员上：业务 agent 不代表人）
  --app-id <cli_…>   这条发到哪个 app → 哪台 bot（成员 bot / 业务 agent 二选一）
  --project <名>     这台 bot 的 project 名（<公司 id>-<名字>）；与 --app-id 一起给时以 app_id 为准
  --json             机器读的出口

两头各自解、**互不代偿**：一头解不出就如实说解不出，不拿另一头顶上 ——
身份认错，后面每一条留痕都记在错的人头上，比不记还坏。
「属于谁」只在业务 agent 上答（域 → who 岗位 → 该岗位上启用中的成员）；人没有归属人。
`

// whoSide 是命令行/JSON 那一侧的一头：问的是什么、解出来是谁、凭什么。
type whoSide struct {
	Asked       string   `json:"asked"` // open_id | app_id | project
	Value       string   `json:"value"`
	Kind        string   `json:"kind,omitempty"`    // member | agent
	Ref         string   `json:"ref,omitempty"`     // 编号：成员名 / agent slug
	Name        string   `json:"name,omitempty"`    // 显示名
	Role        string   `json:"role,omitempty"`    // 岗位
	Domains     []string `json:"domains,omitempty"` // 归属域
	Inscription string   `json:"inscription,omitempty"`
	Project     string   `json:"project,omitempty"`
	Owner       string   `json:"owner,omitempty"`
	OwnerWhy    string   `json:"owner_why,omitempty"`
	Why         string   `json:"why,omitempty"` // 解不出时的原因（非空 = 这一头没解出来）
}

type whoOut struct {
	Vault string   `json:"vault"`
	From  *whoSide `json:"from,omitempty"`
	To    *whoSide `json:"to,omitempty"`
}

// resolveSide 把一次查询落成 whoSide：**解不出来也返回一侧**（带 Why），
// 让「没解出来」与「没问」在输出里能分开 —— 后者是没问，前者是问了没人认领。
func resolveSide(o *org.Org, asked, value string, lookup func(*org.Org, string) (org.Identity, bool), whyNot string) *whoSide {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	s := &whoSide{Asked: asked, Value: strings.TrimSpace(value)}
	id, ok := lookup(o, value)
	if !ok {
		s.Why = whyNot
		return s
	}
	s.Kind, s.Ref, s.Name = id.Kind, id.Ref, id.Name
	s.Role, s.Domains, s.Inscription = id.Role, id.Domains, id.Inscription
	s.Project = org.ProjectName(o.Company.ID, id.Ref)
	// 归属只在「代岗位」的那一类上有意义：人属于他自己，这一格留空。
	if id.Kind == org.KindAgent {
		s.Owner, s.OwnerWhy = o.Owner(id)
	}
	return s
}

func cmdOrgWho(args []string) int {
	fs := flag.NewFlagSet("org who", flag.ContinueOnError)
	openID := fs.String("open-id", "", "谁发的（平台唯一的「人」标识）")
	appID := fs.String("app-id", "", "这条发到哪个 app")
	project := fs.String("project", "", "project 名")
	asJSON := fs.Bool("json", false, "机器读的出口")
	flagArgs, posArgs := splitArgs(args, map[string]bool{"json": true})
	if err := fs.Parse(flagArgs); err != nil {
		return 2
	}
	if len(posArgs) != 1 ||
		(strings.TrimSpace(*openID) == "" && strings.TrimSpace(*appID) == "" && strings.TrimSpace(*project) == "") {
		fmt.Fprint(os.Stderr, orgWhoUsage)
		return 2
	}
	abs, err := filepath.Abs(posArgs[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: %v\n", err)
		return 2
	}
	o, err := org.Load(abs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: %v\n", err)
		return 1
	}

	out := whoOut{Vault: abs}
	out.From = resolveSide(o, "open_id", *openID, (*org.Org).ByOpenID,
		fmt.Sprintf("open_id=%s 在 members/ 里没人认领 —— 自报的身份不算（同 gate 的口径）", strings.TrimSpace(*openID)))
	// app_id 比 project 名更贴平台事实：两个都给时按 app_id 解（project 那一头就不再报）。
	if strings.TrimSpace(*appID) != "" {
		out.To = resolveSide(o, "app_id", *appID, (*org.Org).ByAppID,
			fmt.Sprintf("app_id=%s 不挂在任何成员 bot 或业务 agent 上（这台还没接线，或是别人家的 app）", strings.TrimSpace(*appID)))
	} else {
		out.To = resolveSide(o, "project", *project, (*org.Org).ByProject,
			fmt.Sprintf("project=%s 既不是成员 bot、也不是业务 agent 的 project 名（本公司 id=%s）",
				strings.TrimSpace(*project), o.Company.ID))
	}

	if *asJSON {
		b, err := json.MarshalIndent(out, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "错误: %v\n", err)
			return 1
		}
		fmt.Println(string(b))
		return 0
	}

	fmt.Printf("anc org who —— 平台身份 → org 里的谁\n")
	fmt.Printf("  vault   %s\n", abs)
	printSide("谁发的", out.From)
	printSide("发到谁", out.To)
	return 0
}

// printSide 出一头。字段按「越贴身份越先」排：先是谁，再是挂在哪儿，最后才是凭什么。
func printSide(title string, s *whoSide) {
	if s == nil {
		return
	}
	fmt.Printf("\n  ── %s（%s=%s）\n", title, s.Asked, s.Value)
	if s.Why != "" {
		fmt.Printf("      解不出      %s\n", s.Why)
		return
	}
	if s.Kind == org.KindAgent {
		fmt.Printf("      业务 agent  %s（%s）\n", s.Ref, s.Name)
	} else {
		fmt.Printf("      人          %s（%s）\n", s.Ref, s.Name)
	}
	if s.Inscription != "" {
		fmt.Printf("      铭文        %s\n", s.Inscription)
	}
	if s.Role != "" {
		fmt.Printf("      岗位        %s\n", s.Role)
	}
	if len(s.Domains) > 0 {
		fmt.Printf("      域          %s\n", strings.Join(s.Domains, "、"))
	}
	if s.Owner != "" {
		fmt.Printf("      属于        %s\n", s.Owner)
	}
	if s.OwnerWhy != "" {
		fmt.Printf("      凭什么      %s\n", s.OwnerWhy)
	}
	fmt.Printf("      这台        %s\n", s.Project)
}

const orgAccessUsage = `anc org access —— 这个 open_id 在这家公司里是谁、能看哪些域（只查表，不猜）

用法：
  anc org access <vault> --open-id <ou_…> [--domain <slug>] [--json]

选项：
  --open-id <ou_…>   平台那边唯一的「人」标识
  --domain <slug>    额外判一次「能不能看这个域」
  --json             机器读的出口

白名单制：对不上 members/ 的 open_id 一律拒绝，停用（disabled）的成员同样拒绝。
跨域可见性由 company.md 的 access.cross_domain 决定，不写 = visible。
「拒绝」是判定结果，不是命令失败 —— 退出码仍是 0，脚本读 --json 的 allowed。
`

// accessOut 是 `anc org access` 的出口。判定和「凭什么」分开写：
// 拒绝时 allowed=false、why 说清是没人认领还是已经停用，调用方不用猜。
type accessOut struct {
	Vault       string   `json:"vault"`
	OpenID      string   `json:"open_id"`
	Allowed     bool     `json:"allowed"`
	Member      string   `json:"member,omitempty"`
	Name        string   `json:"name,omitempty"`
	Role        string   `json:"role,omitempty"`
	Admin       bool     `json:"admin"`
	Domains     []string `json:"domains,omitempty"`
	AllDomains  bool     `json:"all_domains"`
	CrossDomain string   `json:"cross_domain"` // 生效值：visible / hidden
	Domain      string   `json:"domain,omitempty"`
	CanSee      *bool    `json:"can_see,omitempty"`
	Why         string   `json:"why,omitempty"`
}

func cmdOrgAccess(args []string) int {
	fs := flag.NewFlagSet("org access", flag.ContinueOnError)
	openID := fs.String("open-id", "", "平台那边唯一的「人」标识")
	domain := fs.String("domain", "", "额外判一次能不能看这个域")
	asJSON := fs.Bool("json", false, "机器读的出口")
	flagArgs, posArgs := splitArgs(args, map[string]bool{"json": true})
	if err := fs.Parse(flagArgs); err != nil {
		return 2
	}
	if len(posArgs) != 1 || strings.TrimSpace(*openID) == "" {
		fmt.Fprint(os.Stderr, orgAccessUsage)
		return 2
	}
	abs, err := filepath.Abs(posArgs[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: %v\n", err)
		return 2
	}
	o, err := org.Load(abs)
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误: %v\n", err)
		return 1
	}

	out := accessOut{
		Vault:       abs,
		OpenID:      strings.TrimSpace(*openID),
		CrossDomain: o.Company.Access.EffectiveCrossDomain(),
	}
	v, ok := o.ResolveViewer(*openID)
	if ok {
		out.Allowed = true
		out.Member, out.Name, out.Role = v.Member.Name, v.Member.DisplayName, v.Member.Role
		out.Admin, out.Domains, out.AllDomains = v.Admin, v.Domains, v.AllDomains
	} else {
		out.Why = "对不上 members/ 里的 feishu.open_id，或该成员已停用 —— 白名单制，拒绝"
	}
	if d := strings.TrimSpace(*domain); d != "" {
		out.Domain = d
		can := ok && v.CanSeeDomain(d)
		out.CanSee = &can
	}

	if *asJSON {
		b, err := json.MarshalIndent(out, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "错误: %v\n", err)
			return 1
		}
		fmt.Println(string(b))
		return 0
	}

	fmt.Printf("anc org access —— 这个 open_id 能看什么\n")
	fmt.Printf("  vault      %s\n", abs)
	fmt.Printf("  open_id    %s\n", out.OpenID)
	if !out.Allowed {
		fmt.Printf("  判定       拒绝\n")
		fmt.Printf("  为什么     %s\n", out.Why)
		return 0
	}
	fmt.Printf("  判定       放行\n")
	fmt.Printf("  人         %s（%s）\n", out.Member, out.Name)
	if out.Role != "" {
		fmt.Printf("  岗位       %s\n", out.Role)
	}
	fmt.Printf("  管理员     %s\n", yesNo(out.Admin))
	if len(out.Domains) > 0 {
		fmt.Printf("  所属域     %s\n", strings.Join(out.Domains, "、"))
	} else {
		fmt.Printf("  所属域     —（没写 domains）\n")
	}
	fmt.Printf("  跨域口径   %s\n", out.CrossDomain)
	if out.AllDomains {
		fmt.Printf("  可见域     全部\n")
	} else if len(out.Domains) > 0 {
		fmt.Printf("  可见域     %s（跨域不可见）\n", strings.Join(out.Domains, "、"))
	} else {
		fmt.Printf("  可见域     只有公司级资料（跨域不可见，且没写 domains）\n")
	}
	if out.CanSee != nil {
		fmt.Printf("  domain=%s  %s\n", out.Domain, yesNo(*out.CanSee))
	}
	return 0
}

func yesNo(b bool) string {
	if b {
		return "是"
	}
	return "否"
}
