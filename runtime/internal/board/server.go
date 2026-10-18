package board

import (
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"strings"
	"time"

	"anc/internal/approvals"
	"anc/internal/org"
)

// Server 是看板的 HTTP 出口。
//
// **默认只读**，这是设计约束，不是「暂时还没写」：
//   - 只有 GET / HEAD，别的方法一律 405 —— 没有写入口，就不存在越权写入口；
//   - **唯一例外**是审批信号（`POST /api/approvals/decide`，见 approvals.go）：
//     它不是第二条写入口，而是同一个变更管道的第三个前端（SPEC §7）——
//     与 CLI 走同一个入口、同一份留痕。除此之外没有任何写路由；
//   - 每次请求现读 vault（不缓存）：改完真相源刷新即见。看板的价值在「现在是什么样」，
//     缓存的看板会让人拿着过期信息做判断；
//   - 校验红档不隐藏：`/api/issues` 如实回显，前端把它摆最上面。
//
// 刻意不做的事：不写真相源、不碰机器、不装载服务、不碰凭据面字段（SPEC §2.3 / §6-3）。
type Server struct {
	Vault string           // vault 根（绝对路径）
	Now   func() time.Time // 注入时钟（测试用）；nil = time.Now

	// DataDir 是 gateway 的 data 目录（socket 与会话记录都在这）。
	// 空 = 运行态没接入：`/api/runtime` 会如实回 wired=false，而不是编一份绿灯。
	// 它和 Vault 是两类东西 —— Vault 是真相源，DataDir 是运行态现场，
	// 所以运行态走独立端点，不并进投影契约（见 runtime.go）。
	DataDir string

	// Approvals 是待批队列（热层）。nil = 没接上：`/api/approvals` 如实回 wired=false，
	// 写口也拒绝服务 —— 没有队列就没有地方落痕，绝不假装点头成功。
	Approvals approvals.Queue

	// Hot 是热层的**整体**视角（`/api/hot`，沉淀页用）。nil = 没接上，如实回 wired=false。
	// 与 Approvals 是同一个底层对象、问的是另一件事 —— 所以单开一个字段，
	// 不把「这一层现在什么情况」硬塞进 approvals.Queue 那个队列接口。
	Hot HotLayer

	// Agents 是同一个热层的**心跳**视角（`/api/agents`）。
	// nil = 没接上：视图如实回 wired=false，不拿「全是离线」冒充真实状态。
	Agents AgentLayer

	// UIFS 是前端静态资源的**根**。nil = 用内嵌的那份（默认）。
	//
	// 内嵌是默认，不是唯一：现场想换一份前端（改样式、临时补一页、拿真机产物核界面）
	// 不该被逼着重编译 —— 给了它（CLI 上是 `anc board serve --ui-dir <目录>`）就用外部那份。
	// **可覆盖 ≠ 拆掉默认**：不给它时行为一个字节都没变。
	//
	// 它只换「前端文件从哪来」，不动 HTTP 面的契约（路由、只读口径、端点还是那些）。
	UIFS fs.FS
}

func (s *Server) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// Handler 组装路由。静态资源来自内嵌文件系统（构建期产物）。
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/board", s.handleBoard)
	mux.HandleFunc("/api/issues", s.handleIssues)
	mux.HandleFunc("/api/runtime", s.handleRuntime)
	mux.HandleFunc("/api/dataflow", s.handleDataflow)
	mux.HandleFunc("/api/assets", s.handleAssets)
	mux.HandleFunc("/api/timeline", s.handleTimeline)
	mux.HandleFunc("/api/audit", s.handleAudit)
	mux.HandleFunc("/api/approvals", s.handleApprovals)
	mux.HandleFunc("/api/hot", s.handleHot)
	mux.HandleFunc("/api/agents", s.handleAgents)
	mux.HandleFunc("/api/context", s.handleContext)
	mux.HandleFunc("/api/grants", s.handleGrants)
	mux.HandleFunc(WritePath, s.handleApprovalsDecide)
	ui := s.UIFS
	if ui == nil {
		ui = UIFS()
	}
	mux.Handle("/", http.FileServerFS(ui))
	return readOnly(mux)
}

// readOnly 只放行读方法。405 带上 Allow 头 —— 让调用方知道该用什么方法，而不是自己去猜。
//
// 唯一的例外是审批信号那条（见 approvals.go 的 WritePath）：它不是「把看板改成可写」，
// 而是那一条本来就属于变更管道 —— CLI 与它走的是同一个入口。
//
// 方法在**这一层**判，不留给处理函数：否则 `GET` 那个写口会落到处理函数上，
// 报出一个「Content-Type 不对」的假原因（真原因是动词不对）—— 2026-10-10 实测踩到。
func readOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == WritePath {
			if r.Method != http.MethodPost {
				w.Header().Set("Allow", "POST")
				http.Error(w, "这个写口只认 POST："+WritePath, http.StatusMethodNotAllowed)
				return
			}
			next.ServeHTTP(w, r)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "看板只读：只有 GET / HEAD（写只有一处 "+WritePath+"，且只收 POST + application/json）", http.StatusMethodNotAllowed)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// handleBoard 出 `anc.board/v2` 投影。
//
// 加载不了（红档）就不给视图：**投影契约只在成功时出现** —— 消费方拿到的每一份 JSON
// 都必然是完整可吃的，不必先判断「这份是不是半成品」。哪里错了看 /api/issues。
func (s *Server) handleBoard(w http.ResponseWriter, r *http.Request) {
	o, err := org.Load(s.Vault)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"ok":    false,
			"error": s.scrub(err.Error()),
			"hint":  "看板读的是真相源；错在哪见 /api/issues 或 `anc org check <vault>`",
		})
		return
	}
	text, err := Of(o, s.now()).JSON()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"ok": false, "error": "投影序列化失败：" + err.Error(),
		})
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(text))
}

// Finding 是一条校验发现的对外形状：只带规则 id / 档位 / 定位 / 文案，不带本机路径。
type Finding struct {
	Rule  string `json:"rule"`
	Level string `json:"level"`
	Where string `json:"where"`
	Msg   string `json:"msg"`
}

// Issues 是 `/api/issues` 的形状。
//
// 刻意**不**并进 `anc.board/v2`：那是「真相源投影」的契约，这个是「真相源健康度」——
// 来源不同、变化节奏也不同（评审与修复会天天动它）。
type Issues struct {
	OK    bool      `json:"ok"`
	Fatal []Finding `json:"fatal"`
	Warn  []Finding `json:"warn"`
	Error string    `json:"error"`
}

// handleIssues 把「真相源现在有没有毛病」如实端出来。
// 用 200 而不是 4xx/5xx：这不是请求错，是内容有发现 —— 前端要把它**显示**出来，不是当故障处理。
func (s *Server) handleIssues(w http.ResponseWriter, r *http.Request) {
	o, err := org.Load(s.Vault)
	if err == nil {
		writeJSON(w, http.StatusOK, Issues{
			OK: true, Fatal: []Finding{}, Warn: findings(o.Warnings), Error: "",
		})
		return
	}
	var le *org.LoadError
	if errors.As(err, &le) {
		writeJSON(w, http.StatusOK, Issues{
			OK: false, Fatal: findings(le.Report.Fatal()), Warn: findings(le.Report.Warns()), Error: "",
		})
		return
	}
	// 到不了报告的错误（文件缺失、解析炸了…）：文案要摘掉本机路径再出去。
	writeJSON(w, http.StatusOK, Issues{
		OK: false, Fatal: []Finding{}, Warn: []Finding{}, Error: s.scrub(err.Error()),
	})
}

func findings(list []org.Issue) []Finding {
	out := make([]Finding, 0, len(list))
	for _, i := range list {
		out = append(out, Finding{Rule: i.Rule, Level: string(i.Level), Where: i.Where, Msg: i.Msg})
	}
	return out
}

// scrub 把本机绝对路径从对外文案里摘掉：看板是观测面，不该泄漏本机布局（SPEC §2.3）。
func (s *Server) scrub(msg string) string {
	if s.Vault == "" {
		return msg
	}
	return strings.ReplaceAll(msg, s.Vault, "<vault>")
}

// writeJSON 出稳定字节 + no-store：看板每次刷新都该是当前真相，不是缓存里的旧世界。
func writeJSON(w http.ResponseWriter, code int, v any) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		http.Error(w, "内部错误：序列化失败", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	_, _ = w.Write(append(b, '\n'))
}
