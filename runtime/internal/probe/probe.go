// Package probe 观测**运行态**：不烧 token、不往机器上装东西，只读磁盘事实，
// 回答一个问题 —— 「这个 bot 到底能不能回话」。
//
// 为什么不是「看进程在不在」：2026-10-07 一天踩了两次 —— 进程活着、
// `engine started` 打着绿字、长连接也建着，但每条消息都起不来 agent。
// 所以判据只能是「真回话过没有」。
//
// 只读两源，都在磁盘上、跨平台、零依赖：
//
//  1. <data>/run/api.sock —— **真拨一次**，不看文件在不在
//     （残留的 socket 文件会把「已经挂了」看成「在跑」，这正是假绿的经典长相）；
//  2. <data>/sessions/*.json —— 会话事实：谁说过话、agent 起没起来、最后谁回的。
//
// 四档，且刻意让「绿」很难拿到（「没消息 = 没事」是反模式，见 DESIGN §「观测三源交叉」）：
//
//	ok      窗口内有过**真实回复**（assistant 真回了话）；
//	warn    没观测到 / 久无成功交互 —— 这一档**不许**报绿；
//	fail    有消息但 agent 起不来，或最后一轮迟迟不回；
//	unwired 声明过「还没接平台凭据」—— 本来就不该期待它回话，单列灰，不挡绿。
//
// 「真实回复」的机械口径：assistant 记录的内容**非空**。空内容（只调了工具、或异常返回空）
// 不算回过话 —— 语义对不对仍留给人（议题 #51）：能机械判定的漏判不许留，判不了的明确不判。
package probe

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"anc/internal/gateway"
	renderpkg "anc/internal/render"
)

// Schema 是这个报告的版本。有消费方（看板、watchdog）之后，字段增删一律改它。
const Schema = "anc.runtime/v1"

// State 是四档判定。
type State string

const (
	StateOK   State = "ok"
	StateWarn State = "warn"
	StateFail State = "fail"
	// StateUnwired 是第四档（灰）：这个 bot **还没接平台凭据**，本来就不该期待它回话。
	//
	// 为什么必须单列一档：只有「黄」的话，「没接凭据」与「接了但没动静」混在一格里 ——
	// 前者不是事故、后者要查，混着看会让人整份报告都不信。
	// 2026-10-08 沙箱实测逼出这一档：SPEC 要求一个公司恰好有一个启用中的 devbot，
	// 而只有一个真飞书 app 时它必然报黄 —— 于是「已接凭据的都绿了」这个事实被一颗假黄盖住。
	//
	// 口径走**数据**（vault 里声明 `unwired: true`），不按 app_id 的长相猜、也不写死白名单。
	StateUnwired State = "unwired"
)

// Finding 是一个 bot 的运行态判定。
type Finding struct {
	Project string `json:"project"`
	State   State  `json:"state"`
	Why     string `json:"why"`
}

// Report 是一次观测的全量结果。
type Report struct {
	Schema     string    `json:"schema"`
	Vault      string    `json:"vault"`
	Config     string    `json:"config"`
	DataDir    string    `json:"data_dir"`
	Gateway    string    `json:"gateway"` // up / down
	GatewayWhy string    `json:"gateway_why,omitempty"`
	Bots       []Finding `json:"bots"`
	Extras     []string  `json:"extras,omitempty"`
	// Handlers：探针只做观测，**裁判权留给人工**。所以要顺手说清「出问题该交给谁」，
	// 否则一份红报告等于把问题丢在地上。来源是 company.admins（SPEC §「谁能发特权命令」）。
	Handlers   []string `json:"handlers"`
	HandlerWhy string   `json:"handler_why,omitempty"`
}

// Options 是观测输入。Projects / Admins 由调用方从真相源算好传进来 —— 这个包不认识 org。
type Options struct {
	Vault    string
	Config   string
	DataDir  string
	Stale    time.Duration // 多久没有成功交互算「不新鲜」
	Stall    time.Duration // 最后一轮没人回多久算卡住
	Now      time.Time
	Admins   []string // 出问题该交给谁（显示用）
	Projects []string // 该有哪些 project；nil = 从 config 读
	// Unwired 是**声明**「还没接平台凭据」的 project 名（vault 里成员的 `unwired: true`）。
	// 声明了的不判黄也不判绿，单列灰；CLI 侧从 org 算好传进来（这个包不认识 org）。
	Unwired []string
}

// DefaultStale / DefaultStall 是出厂阈值。
// 两个数都是按 2026-10-07 的实测手感取的（一次真回复 5~12s），不是算出来的 ——
// 客户现场大概率要按业务节奏调，所以做成参数而不是常量。
const (
	DefaultStale = 24 * time.Hour
	DefaultStall = 10 * time.Minute
)

// ccSessions 只取探针要用的字段；别的字段上游会变，不跟着它跑。
type ccSessions struct {
	Sessions map[string]struct {
		AgentSessionID string `json:"agent_session_id"`
		History        []struct {
			Role      string `json:"role"`
			Content   string `json:"content"`
			Timestamp string `json:"timestamp"`
		} `json:"history"`
	} `json:"sessions"`
}

// Run 做一次观测。返回 error 只在「连该有哪些 bot 都不知道」时发生（config 读不到）。
func Run(opt Options) (Report, error) {
	if opt.Stale <= 0 {
		opt.Stale = DefaultStale
	}
	if opt.Stall <= 0 {
		opt.Stall = DefaultStall
	}
	if opt.Now.IsZero() {
		opt.Now = time.Now()
	}
	rep := Report{
		Schema:   Schema,
		Vault:    opt.Vault,
		Config:   opt.Config,
		DataDir:  opt.DataDir,
		Bots:     []Finding{},
		Handlers: opt.Admins,
	}

	// 声明：该有哪些 bot。声明了但没有任何会话 = 「没观测到」，不是「没事」——
	// 那是假绿的另一种长相。
	declared := opt.Projects
	if declared == nil {
		raw, err := os.ReadFile(opt.Config)
		if err != nil {
			return rep, fmt.Errorf("读不到 config %s: %w", opt.Config, err)
		}
		declared = renderpkg.ProjectsIn(string(raw))
	}

	if why := DialUnix(gateway.CCConnect.SocketPath(opt.DataDir)); why == "" {
		rep.Gateway = "up"
	} else {
		rep.Gateway = "down"
		rep.GatewayWhy = why
	}

	files, _ := filepath.Glob(filepath.Join(gateway.CCConnect.SessionsDir(opt.DataDir), "*.json"))
	byProject := map[string][]string{}
	for _, f := range files {
		p := ProjectOfSessionFile(filepath.Base(f))
		byProject[p] = append(byProject[p], f)
	}

	unwired := map[string]bool{}
	for _, p := range opt.Unwired {
		unwired[p] = true
	}
	for _, p := range declared {
		// 声明「还没接凭据」的先落灰，且**在 gateway 挂没挂之前就判** ——
		// 没接凭据的 bot 本来就不该回话，拿它去凑红/黄都是噪声。
		if unwired[p] {
			rep.Bots = append(rep.Bots, Finding{p, StateUnwired, "未接平台凭据（真相源里声明 unwired）—— 不该期待它回话，不计绿也不计黄"})
			delete(byProject, p)
			continue
		}
		if rep.Gateway == "down" {
			rep.Bots = append(rep.Bots, Finding{p, StateFail, "gateway 没在跑，这个 bot 必然回不了话"})
			continue
		}
		rep.Bots = append(rep.Bots, Judge(p, byProject[p], opt.Now, opt.Stale, opt.Stall))
		delete(byProject, p)
	}
	// 配置里没有、磁盘上却有会话 → 删过 bot 却留着它的记录。
	for p, fs := range byProject {
		rep.Extras = append(rep.Extras, fmt.Sprintf("%s（%d 个会话文件，配置里已无此 project）", p, len(fs)))
	}
	sort.Strings(rep.Extras)
	return rep, nil
}

// DialUnix 判活：**拨一次**，不靠 socket 文件在不在。
// 「不许假定 socket 形态」（DESIGN §7）到这一步已经统一了：Windows 是文件、Linux 是 unix
// socket 节点，而**存在性**两边都成立 —— 所以它反而是错判据，进程崩了文件会留下。
func DialUnix(path string) string {
	if _, err := os.Stat(path); err != nil {
		return "socket 不存在：" + path
	}
	c, err := net.DialTimeout("unix", path, 3*time.Second)
	if err != nil {
		return "socket 在但拨不通（多半是进程已经没了，留下的是残留文件）：" + err.Error()
	}
	c.Close()
	return ""
}

// ProjectOfSessionFile 从 `demo-alice_00a71faa.json` 取出 project 名。
func ProjectOfSessionFile(base string) string {
	name := strings.TrimSuffix(base, ".json")
	if i := strings.LastIndex(name, "_"); i > 0 {
		return name[:i]
	}
	return name
}

// Judge 只看会话事实，不看进程、不烧 token。
func Judge(project string, files []string, now time.Time, stale, stall time.Duration) Finding {
	if len(files) == 0 {
		return Finding{project, StateWarn, "没观测到任何会话 —— 「没消息 = 没事」是反模式，所以这里不许报绿"}
	}
	var lastUser, lastAssistant, lastEmpty time.Time
	neverStarted := false
	msgs := 0
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var s ccSessions
		if err := json.Unmarshal(b, &s); err != nil {
			return Finding{project, StateWarn, fmt.Sprintf("会话文件读不动（%s）：%v", filepath.Base(f), err)}
		}
		for _, one := range s.Sessions {
			for _, h := range one.History {
				msgs++
				ts, err := time.Parse(time.RFC3339Nano, h.Timestamp)
				if err != nil {
					continue
				}
				switch h.Role {
				case "user":
					if ts.After(lastUser) {
						lastUser = ts
					}
				case "assistant":
					// 空回复不是「回过话」：只调了工具、或异常返回空，都证明不了它能说话。
					// 单记一个时间，最后一轮是不是空回复要看得见（议题 #51）。
					if strings.TrimSpace(h.Content) == "" {
						if ts.After(lastEmpty) {
							lastEmpty = ts
						}
						continue
					}
					if ts.After(lastAssistant) {
						lastAssistant = ts
					}
				}
			}
			// 有消息、却从没拿到过 agent 会话 id = agent 一次都没起来过。
			// 这正是「进程活着 / engine started 是绿的 / 但回不了话」的签名。
			if one.AgentSessionID == "" && len(one.History) > 0 {
				neverStarted = true
			}
		}
	}
	if msgs == 0 {
		return Finding{project, StateWarn, "会话文件是空的 —— 没观测到实际交互"}
	}
	if neverStarted {
		return Finding{project, StateFail, "有消息、但 agent 从没起来过（agent_session_id 为空）—— 多半是 work_dir / 工具白名单 / agent 二进制的问题，查 gateway 日志"}
	}
	if !lastUser.IsZero() && lastUser.After(lastAssistant) {
		if lastEmpty.After(lastUser) {
			return Finding{project, StateWarn, "最后一轮 assistant 记录是空内容 —— 空回复不算「回过话」（可能只调了工具，也可能异常）；不报绿，请人看一眼"}
		}
		if since := now.Sub(lastUser); since > stall {
			return Finding{project, StateFail, fmt.Sprintf("最后一轮消息发出 %s 还没有回复（超过 --stall %s）", since.Round(time.Second), stall)}
		}
		return Finding{project, StateWarn, fmt.Sprintf("最后一轮正在处理中（发出 %s）", now.Sub(lastUser).Round(time.Second))}
	}
	switch {
	case lastAssistant.IsZero():
		if !lastEmpty.IsZero() {
			return Finding{project, StateWarn, "只有过空回复（assistant 记录为空），还没有一次有内容的回复"}
		}
		return Finding{project, StateWarn, "只有用户消息，还没有过一次回复"}
	case now.Sub(lastAssistant) > stale:
		return Finding{project, StateWarn, fmt.Sprintf("最近一次成功回复在 %s 前，超过 --stale %s —— 没人打扰它的话，这个窗口里它到底能不能回话是不知道的",
			now.Sub(lastAssistant).Round(time.Minute), stale)}
	default:
		return Finding{project, StateOK, fmt.Sprintf("最近一次成功回复在 %s 前", now.Sub(lastAssistant).Round(time.Second))}
	}
}

// AllGreen 是「这次的观测算不算全绿」。两条都不许松：
//
//   - **灰（未接凭据）不挡绿** —— 这是这一档存在的全部意义：门要能过，
//     但结论里必须看得见「哪几个本来就没接」。所以绿的口径是
//     「**已接凭据的都绿了**」，不是「所有 bot 都绿了」。
//   - **空集不判绿** —— 一个绿的都没有时（全被声明成未接、或 config 里压根没有 project），
//     「已接凭据的都绿了」是**空真**，报绿就是最纯的那种假绿：什么都没验过。
//     这与「没消息 = 没事」是反模式是同一条纪律（2026-10-09 实测逼出：
//     把三个 bot 全声明成 unwired，第一版 `AllGreen()` 照旧返回 true）。
func (r Report) AllGreen() bool {
	if r.Gateway != "up" || len(r.Extras) > 0 {
		return false
	}
	green := 0
	for _, f := range r.Bots {
		switch f.State {
		case StateUnwired:
			continue
		case StateOK:
			green++
		default:
			return false
		}
	}
	return green > 0
}

// Tally 是给回显用的一行计数：已接的里有几个绿、几个黄、几个红，另有多少个未接凭据。
// 分开数是有意的 —— 一句「3/3 绿」会把「没接凭据」也糊进绿里。
func (r Report) Tally() (ok, warn, fail, unwired int) {
	for _, f := range r.Bots {
		switch f.State {
		case StateOK:
			ok++
		case StateWarn:
			warn++
		case StateFail:
			fail++
		case StateUnwired:
			unwired++
		}
	}
	return
}
