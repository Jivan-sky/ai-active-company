# ANC 运行层落地设计（v0.1 草案）

> 本文是 `../SPEC.md` 的**工程派生视图**：SPEC 管口径，本文管「口径怎么落到一台客户机器上」。冲突一律以 SPEC 为准；实现过程中若被迫改口径，先改 SPEC，再回来改本文。
>
> **验证状态：设计稿，未在任何客户现场跑过。** 已实测的是 `anc init / doctor / assets / version`，§2 阶段 A 的 `anc render / anc org check`，以及 **阶段 A 的外部验证：真 cc-connect 已接受这份配置，且 Windows 与 Linux 两腿结果一致**（2026-10-07 烟测，v1.3.4，三项目全部 `platform ready` / `engine started`；细节见 §7.1 与 `../docs/M1-DESIGN.md` §10.1。Linux 腿跑在 WSL2 Ubuntu，`anc` 由 HEAD 交叉编译）。**会话与 agent 真交互已打通**（2026-10-07：飞书真人发消息 → agent 真回话，`turn complete tools=3`；同日两次「引擎全绿但回不了话」的故障已定位并修复 —— `work_dir` 从未创建、空白名单在 `dontAsk` 下等于全拒，见 `../docs/M1-DESIGN.md` 与议题 #37）。**`anc probe` 只读两源已落地**（socket 真拨 + 会话事实），并已抓过残留 socket 与「agent 从没起来过」两类假绿；**探针多了第四档「灰」**（2026-10-09：`unwired: true` **声明**驱动，灰不挡绿、四个消费方共用一支算，真 VM 实测 `anc probe` 退出码 0 —— 见 §7.1.20）；**看板运行态页已接入**（2026-10-07：看板与 CLI 共用 internal/probe 同一份判据，走独立端点 /api/runtime，anc board serve --data 起；活体经 HTTP 与真实浏览器引擎各验一次，响应里逐条摘掉本机路径）。**尚未实测**：**macOS 腿（零覆盖）**、**我们自己那套**服务单元（systemd / launchd / schtasks 一个都没装过 —— D4 之后常驻交给上游 daemon，装载走 `anc apply`；上游那份 Windows 计划任务已实测装载成功）、探针的**功能级 ping**（那要真发一条消息、烧 token —— 已拍板**不做**，裁判权留给人工，见 ../SPEC.md §13 Q17）。**`anc notify` 已落地**（2026-10-07：判据做成纯函数 + 18 条档位用例，活体 dry-run 与「无 socket」负例均已实测，**真发到飞书并跑完整条闭环**，见 §7.1.5）。**`anc trail` 已落地**（2026-10-07：留痕的事实层 —— 只读聚合 cc-connect 会话与 harness 原生记录；三条口径与 claude 自己的 cost-state **对拍到逐字段相等**；活体 + 负例 + JSON 均已实测，见 §7.1.6）。**`anc trail` 的判断层已落地**（2026-10-07：判据是**数据**不是代码，`--rules` 可整份替换、不用重编译；只出结论不设门禁，每条结论都带来源与证据，见 §7.1.7）。**`anc apply` 已落地（Windows 腿 2026-10-07；**Linux 腿 2026-10-08**）**：一条命令走完七步 —— 校验 org → 渲染（与 `anc render` 同一套门禁：同一份 `commitConfig`）→ 凭据体检 → `cc-connect daemon install --no-capture-secrets --force` → 凭据桥 → `restart --force` → 回读（platform ready + 只读探针）；沙箱真跑通（`platform ready 1/1`、探针绿、任务定义里既无键名也无密钥值、重跑幂等），dry-run 与真装载各一次，见 §7.1.8。**仍未实测**：**macOS 腿的凭据注入**（零实现）；Windows 腿的**登录自启**（上游任务写的是 `-AtLogOn`，要注销 / 重启才验得到 —— 归 #42）。Linux 腿的凭据注入与「重启 / 断电后自己起来」都已实测（§7.1.10 / §7.1.11）。**`anc doctor` 多了一组「断电兜底」只读自检**（Linux 三条前提 / Windows 触发器 + 电源条件；真机实测，见 §7.1.11）。**看板三分区接齐了**（2026-10-08：运行态 = 防假绿探针、数据流 = 策略面 + 执行面事实、原料 = 数据目录清单；**事件面与沉淀层仍未实现**，页内如实列缺 —— 见 `README.md` 看板一节）。**授权表（`grants/`）已有落点与结构校验**（2026-10-08：一个 grant 一个文件、十条规则整组 `warn`、看板投影全量；**执行层与按人过滤视图未做** —— 见 §7.1.15）。**看板「授权」页已接入**（2026-10-15：全量逐条搬 + 按主体换视角；一页一个问题，「数据流」页的问题句因此收窄到执行面那两件 —— 见 §7.1.53）。**凭据键名不再回读文本，改为渲染时记账**（2026-10-08：成员名非 ASCII 或带 `-` 时，曾经发出一个要不到凭据、静默起不来的 bot；同日把「名 → 键」改成**全函数**（ASCII 直通、其余按码点转义）—— 中文成员名不再需要改名、也不再需要降级，规则只拦「当不了目录名」的字符 —— 真二进制 dry-run 与真上游 cc-connect 各实测一次，见 §7.1.16）。**`anc gate` 已落地**（2026-10-08：一个引擎两个作用域 —— 会话层事实来自 harness 记录、新增的「交付层」事实来自真相源仓库（org + timeline）；出厂五门 × 两条 = 11 条判据，门名与判据全在数据里；沙箱真留痕走通「记一条绿 + 记一条红 → 结论与证据跟着变」，见 §7.1.17）。**看板状态灯改成形状 + 颜色双编码**（2026-10-08：四档从 8px 同形圆点改成 实心圆 / 带光环的圆 / 菱形 / 空心虚线圈，灰刻意不许像绿；真浏览器逐档读计算样式 + 整页截图各一次，见 §7.1.18）。**工作上下文并进同一次读出口调用**（2026-10-09：`skills` / `projects` / `gaps` 三段挂上同一个 `anc_read_context` —— 技能正文在不在逐条标、项目按同一把尺子过滤、答不了的**报缺不编**；**本机实测**：单元 5 条 + 十四包全绿，**真 VM 未跑** —— 见 §7.1.23）。§7 列出必须实测的项、怎么验、通过判据 —— 在跑通之前，本文任何一条都不许当「已实现」讲。
>
> **语言交代：Go**（复用 `anc` 单 exe，新增 `bootstrap` / `render` / `service` / `serve`）。理由：同一份源码跨 macOS / Windows / Linux，常驻进程不许自带运行时，装配器已经是 Go。代价：现场改**逻辑**要重编（约 10 秒）；对应缓解是 persona、模板、业务规则全部外置成纯文件，改这些不用重编译。

---

## 0. 待拍板（先拍这四条，再往下实现）

| # | 决策 | 选项 | 代价 / 影响 |
|---|---|---|---|
| **D1** | **成员 Bot 的隔离档** | (a) 严格一人一 OS 账号（SPEC §6-9 原义）<br>(b) 分档：成员 Bot 共享账号 + 独立进程 / 独立 HOME / 独立凭据 + harness 白名单；业务 Bot（碰 C 档）才独立账号<br>(c) 独立账号只给「需要碰不可逆动作」的 bot | (a) 隔离最强，但 30~200 人 = 30~200 个 macOS 账号，装机与运维成本随人数线性涨，这是本设计里最大的一处规模冲突<br>(b) 装机量降到个位数，但成员 Bot 的隔离从 OS 级降到「进程 + 文件权限 + harness 规则」级<br>(c) 折中，但"谁算高风险"要每次现场判断 |
| **D2** | **无人值守恢复** | (a) 不开 FileVault + 自动登录<br>(b) 开 FileVault + 重启后人工登录<br>(c) 开 FileVault + 计划内重启 + 掉线外部告警催人 | macOS 上 **FileVault 与自动登录互斥**，只能二选一：(a) 断电重启后 bot 自己回来，但磁盘明文，客户资料有被搬走的面；(b) 数据加密，但每次重启都要人到场，否则 bot 不回话 |
| **D3** | **会话连续性** | (a) 单轮：一次问答一个进程，跨消息状态只靠 vault 落盘<br>(b) 多轮：按会话落 jsonl，保留上下文<br>(c) 混合：IM 线程内多轮，换题或超时即断 | (a) 最简、最可审计、无内存态，但用户要重复交代背景；(b) 体验好，但上下文成本与"读了过期上下文答错"的风险升高 |
| **D4** | **与上游 `anc` CLI 的关系**（= SPEC Q6） | (a) 全部自建到能用<br>(b) 只自建 `bootstrap / init / render`，`serve` 等上游<br>(c) 最小 `serve` 先只服务成员 Bot（只读），业务 Bot 等上游 | 上游 CLI **未发布**，等它就是等一个不确定的时间点；全自建则 `serve` 这一块要自己承担 IM 长连接、会话调度、出口审计。<br>**2026-10-07 新证据（已据此拍板，结论见下方「拍板记录」）**：我们依赖的运行时 `cc-connect` 自己已经带 **daemon 管理** —— `daemon install / uninstall / start / stop / restart / status / logs`，Windows 走 `schtasks`（实测 `daemon status` 返回 `Status: Not installed / Platform: schtasks`）。也就是说 **IM 长连接 + 会话调度 + 常驻这件事，上游运行时已经做完了**，`serve` 真正要自己写的只剩「org → 渲染 → 装载 → 探针」这一层编排 —— 这直接压缩了 (a) 的规模。另有一处安全相关：`daemon install` 默认会把 config.toml 里的 `${ENV}` 占位**捕获成实际值**写进服务文件，须显式用 `--no-capture-secrets` 或 `CC_DAEMON_NO_CAPTURE_SECRETS=1` 关掉 —— 与我们「config / 备份 / diff 全程无明文」的承诺直接冲突，若走 (b)/(c) 必须带上这个开关 |
| **D5** | **两套骨架的关系** | (a) 合成一棵树：客户库（`anc init`：inbox / ops / delivery）与 org 真相源（`anc org init`：company / roles / members）放到同一个仓库根<br>(b) 保持两棵：客户库是交付物，org 真相源是运行层，各自独立<br>(c) 客户库作为 org 真相源的一个数据目录挂进去 | 现在两个命令生成的是两棵独立树，且**都有「角色」这个概念**（`40-roles/` vs `roles/`）—— 撞上 SPEC §3.5「同一事实只在一处维护」，双源必然漂移。合成一棵树要先定谁是根、谁挂谁；不合并就得在两边写清楚各自的权威范围。**未拍板**，暂不合并 |
| **D6** | **bot 的家目录布局** | (a) 按 §2：账号 HOME 下 `bot/`（cwd）与 `state/`（会话、日志）<br>(b) 按当前实现：`<homes>/<成员>/` 作 cwd，`<homes>/<成员>/state/` 放日志 | §2 写的是「一个账号一个 HOME」，而 `render` / `service` 现在用的是「一个 homes 根 + 每个成员一个子目录」。D1 若选「严格一人一 OS 账号」，两者要对齐（homes 根就得落到各账号 HOME 里）；若选共享账号，当前实现就是对的。**等服务单元与 D1 一起定** |

**拍板记录（2026-10-07）**

- **D4**：采纳 **(c′)** —— `serve` 降级为 `apply`。上游 `cc-connect daemon` 已承担长连接 / 会话 / 常驻
  （已实测 `daemon install / start / status` 全在），所以 v1 的 `anc serve` 其实是 `anc apply`：
  **render → 校验 → 装载上游 daemon（必须带 `--no-capture-secrets`）→ 装只读探针**，**不自建常驻进程**。
  阶段 C 的启动条件（先拍本条）由此满足。
- **D5**：采纳 **(a)** —— 合成一棵树，**以客户库为根**；`anc org init` 降级为「只生成 org 子树」的子集命令。
  加一条更根本的：**一个仓库根 = 一个公司**（「一机一公司」是它的推论，不是权宜）。
- **D6**：**不等 D1** —— 用一层间接解耦：布局固定为 `<homes_root>/<成员>/`，
  D1 只决定 `<homes_root>` 是共享账号下的 `~/anc/homes`，还是各账号的 `$HOME`。**模板只有一份**。
- **D3（部分，2026-10-07 拍板）**：**空闲重置关掉** —— `reset_on_idle_mins` 出厂值 = `0`，
  换新会话由人**显式发 `/new`**，不靠计时器替人猜。它现在来自数据
  （`company.md` 的 `defaults.reset_on_idle_mins`），**不再是代码里的字面量**
  —— 这一行原本写死成 30、没有任何拍板依据，而且客户改不了（见议题 #41）。
  D3 的其余部分（单轮 / 多轮 / 混合）**仍未拍板**。
- **`company.devbot.count` 出厂档（2026-10-07 拍板）**：**保持 fatal、不改代码**。
  理由：SPEC §1 那条是红线，而「首跑只服务一个 bot」这个现场问题**已经有现成的数据出口** ——
  `company.md` 的 `policy:` 段一行（`company.devbot.count = warn`）就能降档。沙箱实测：`anc org check`
  打印「规则覆盖 company.devbot.count」+「⚠️ 非红档发现 1 条」，退出码 0。
  不给它开实现里的特例：门禁要留一条能一键降级的数据出口，而不是逐个在代码里放行。
- **阶段 C 的装载（2026-10-07 落地）**：`anc serve` **不写** —— 装载 = 上游 daemon（见 D4）。
  一条 `anc apply`：渲染 → 校验 → `cc-connect daemon install --no-capture-secrets --force`
  → 凭据桥（把 secrets.env 读进 daemon 进程环境）→ `restart --force` → 回读。实测见 §7.1.8。
- **D1（服务侧那半，2026-10-08 由 SPEC §6-9 拍定）**：**跑在服务侧的 bot（业务 Bot、devbot）一律独立 OS 账号**；
  成员 Bot 跑在成员**自己的机器**上，共用人机身份 —— 所以「一人一系统账号」不是本档的口径。
  五件套（独立账号 / HOME / 凭据 / cwd / `allowed_tools` 非空）与接法见 `../SPEC.md` **§4.7③**。
  **D1 剩下的**只是成员 Bot 那一侧的落法。
- **D2 与 D3 的其余部分**：**仍未拍板**（D1 见上一条）。
- **域表（罗盘）落点**（2026-10-07 拍板）：一条业务域 = `domains.md` 里一行；
  `slug`（ASCII）/ `name`（中文）分列，`who` **填岗位**、人名渲染时从 `members/` 现算，
  `data` 是**指针**（顶层目录名）不是说明，列序按表头认、多出来的列忽略。
  **「怎么做」不进表**（活的，由 agent 自己沉淀）；**任务进度也待定**（见 SPEC §13 Q15，
  倾向「表里只留指针、进度落沉淀层」）。粒度判据（防繁琐）：① 会改变 agent 行为吗
  ② 能自动维护吗（人手维护必腐烂）③ 是不是已在别处（在则引用不复制）。
  拒掉的列：成员名单（与 org 双源）、KPI、状态 / 成熟度、审批流程、子域嵌套、常见任务清单。

---

## 1. 进程模型（每个 bot 一套）

| 进程 | 生命周期 | 职责 | 不做什么 |
|---|---|---|---|
| `cc-connect daemon`（上游，**常驻**） | **常驻**（Windows 计划任务 / systemd --user / launchd） | IM 长连接、消息路由、会话调度、**单一出口** | **不跑 agent loop**、不解析业务规则（规则在数据里，见 §3） |
| `anc apply`（我们，**一次**） | 按需执行，装完即退 | 渲染 + 校验 + 装载 `cc-connect daemon` + 凭据桥 + 重启 + 回读 | 不做常驻、不跑 agent loop、不自己实现长连接（D4） |
| harness 进程（官方 CLI） | **按需拉起**，一次问答一个进程 | 真正的推理与工具调用 | 不持有跨会话状态；不直接对外发消息（回给 serve） |
| `ingest`（服务 Bot） | 常驻 | 把 IM 附件 / 工单 / 邮件里的原件落 `_originals/`（只追加） | 不改写原件、不外发 |
| `watchdog`（服务 Bot） | 常驻 | 跨 bot 健康交叉 + 告警外发 | 不碰业务数据 |
| `devbot` | 按需 | 唯一的全权限 bot，cwd = 仓库本体 | 改动必须走 git，可回滚 |

**为什么 harness 不常驻**：官方 CLI 是交互式程序，常驻吃内存、占订阅并发额度；按需拉起才能做到「一次问答 = 一个可审计的进程」。这同时是 §4 会话边界的基础。

**拉起方式**：装载 = `anc apply` —— 它调上游 `cc-connect daemon install --no-capture-secrets --force`，单元形态（Windows 计划任务 / Linux `systemd` / macOS `launchd`）由上游按平台决定；凭据由**凭据桥**在拉起 gateway 之前读进进程环境（§7.1.8）。这是**用户级**服务，不写系统级 daemon。

> **装载已实现（阶段 C，2026-10-07）**：`anc apply` —— 常驻交给上游 daemon（D4），我们**不写** `serve`。
> 上面表里 `cc-connect daemon` 那一行就是它；实测证据见 §7.1.8。
> **阶段 B 的 `anc service` 已退役（2026-10-07）**：它生成的单元跑的是 `anc serve --bot`，而 serve 永不存在 ——
> 装上去只会得到一个反复退出的服务。退役记录与教训见 §7.1.9。

---

## 2. 账号与目录布局

```
/opt/anc/vault/          root 拥有、bot 只读；git ff-only 副本（客户库真相源在这里）
/opt/anc/harness/        官方 CLI 本体，root 拥有、只读；升级一次全 bot 生效
/opt/anc/bin/anc         装配器 + serve（单 exe）
/Users/anc-<bot>/        bot 账号 HOME（0700）
  .anc/creds/            该账号专属凭据（0600）；不进 git、不进日志
  bot/                   cwd：AGENTS.md / CLAUDE.md（渲染产物，带指纹头）、skills/、inbox/
  state/                 会话记录、探针游标、审计暂存（不进 git）
```

- **共享 CLI 本体 + 独立 HOME = 独立凭据**：各官方 CLI 的凭据落在 `$HOME` 或该账号的登录钥匙串里，按账号天然隔离；共享本体只省磁盘与升级次数，**不承担隔离职责**。
- 隔离靠 **OS 账号 + 独立进程 + 独立 HOME + 独立凭据**（SPEC §6-9）。cwd 只是挂载点，**不是授权边界**。
- **布局与隔离档解耦（2026-10-07 拍板，D6）**：布局固定为 `<homes_root>/<成员>/`；
  D1 只决定 `<homes_root>` 是共享账号下的 `~/anc/homes`，还是各账号的 `$HOME`。**模板只有一份**，不再等 D1。
- `anc bootstrap` 是**唯一需要管理员权限**的动作，且只做上面列出的这几类目录与账号。**必须先 `--dry-run` 打印将要做的每一步**，人核对后再真跑（危险操作，见守则）。

---

## 3. org 真相源 → 渲染 → 发布

- **真相源** = git 仓库里的 markdown + frontmatter（公司 / 角色 / 成员 / 岗位），**只存岗位与职责，不存个人标识**（SPEC §2.3）。
- `anc render` **全量生成、不打补丁**：产出每个 bot 的 `AGENTS.md` / `CLAUDE.md`、目录路由表、persona、skill 挂载清单、权限声明文件。
- **persona 三层叠加**：公共层 × 角色层 × 成员层 → 渲染进上述产物；公共段**只存在于公共层**，避免手抄漂移。
- **段 8「业务域」由 `domains.md` 驱动**：表在才出这一段。**自己所属的域整行**（含 `data`），
  **别人的域只给目录行**（域 / 名称 / 是什么 / 找谁），不给它的 `data` 与 `terms`。
  `who` 存岗位，人名渲染时现算。**没有这张表的 vault，persona 逐字不变**（golden 未动即为证）。
- 生成物带**指纹头**；`anc render --check` 报告漂移。**手改生成物 = 事故**（检测到就报错退出，不静默覆盖）。
- **校验规则住在数据里，不埋在实现里**：出厂规则表在 `internal/org/rules.go`（每条有 id、默认档、理由），
  客户在 `company.md` 的 `policy:` 段逐条覆盖（`fatal` / `warn` / `off`）。分档判据只有一条 ——
  **拦不可逆的**（bot 下线、对外答错、覆盖人的手写文件），可逆的小事只告警；SPEC 的无例外红线（§6-5 / §171）
  标 `[红线]` 不许降级。规则 id 拼错 = 你以为关了其实没关，所以策略段自己也要被校验。
  非红档发现必须回显，门禁不许静默。全表：`anc org check <vault> --rules`。
- **两道差分门**：`--adopt`（目标配置没有 anc 指纹 = 别人的文件，不覆盖）、`--allow-scale`
  （新增 / 删除 project 会静默上下线 bot）。两者独立，接管他源配置时都要显式过。
- **`--check` 的口径要说全**：它比的是「现在这份 == anc 上一轮生成的」，抓得到人的手改与 org 变化，
  **抓不到渲染器自己的 bug**。渲染器的外部验证只有真拉起 + 探针（阶段 B）。
  **2026-10-07 已补上半步**：真 cc-connect v1.3.4 吃下了渲染产物（三项目 `platform ready` + `engine started`），
  即「gateway 接受这份配置」成立；但**上游对未知键是静默忽略的**（实测），所以这份产物「被接受」不等于
  「字段都对」—— 字段正确性只能由我们自己的白名单测试兜（见 §7）。
- **发布**：运维机 `render` → commit → 生产机 `anc pull`（ff-only）→ 重渲染 → `anc serve reload`（只对新会话生效，不打断在跑的会话）。
- 硬指标不变：**重装一次 ≤ 10 分钟**（§6）。
- **安全相关段完全显式（2026-10-07 已实现）**：产物恒定显式输出 `[relay] timeout_secs = 0`（v1 零绑定）。
  上游对这个段自带默认值且默认是「开着」（120），不写就等于静默放行 bot 间通道（SPEC §6 不变量 3）。
  两道拦：`Build` 自校验（产物缺段直接失败，无覆盖开关）、`anc render --check` 把「段缺失」与「值非 0」
  都当安全缺口报出来并以退出码 1 结束 —— 指纹比对只看 persona / inputs，**手改 `timeout_secs` 不会动指纹**，
  只有这条能看见它。

---

## 4. 执行契约（一次会话长什么样）

1. `serve` 校验发送者：这个 bot 只服务它对应的那个人（＋ 被授权的群 / 流程）。
2. 组装输入：消息正文 + persona 指针 + 路由表 + 附件落点（**原件指针，不是原件副本**）。
3. 拉起 harness：cwd = `$HOME/bot`，环境变量注入该账号凭据，流式读输出。
4. 回 IM：按段 / 卡片输出，**只走 serve 的单一出口**。
5. 落审计行：谁问的、读了哪些源、做了什么分档动作、消耗多少 token、耗时多久。

**状态不留在内存里**：换进程不丢事实，跨消息上下文按 §0-D3 拍板结果处理。

---

## 5. 权限落点（三层，位置比写法重要）

| 层 | 放什么 | 为什么放这层 |
|---|---|---|
| **harness 权限规则** | bypass 关闭、工具白名单、目录白名单、出站域名白名单 | 声明式、可审计、改一行生效、不用发版 |
| **`anc serve` 代码兜底** | 发送者鉴权、输入校验、路径白名单、密钥注入、**C 档人确认卡（IM 按钮）** | 硬约束至少一层代码兜底：只有提示词的硬约束等于没有约束 |
| **OS 层** | 账号隔离、文件权限、只读挂载、独立凭据 | 破一个 = 破一个 |
| **授权表（真相源）** | grant：谁 / 什么动作 / 什么客体 / 期限 / 署名（SPEC §6 授权模型） | 声明式、落 git 可 diff 可回滚；**运行时每次行使重读真相源**，撤回即时生效。落点与结构校验见 **§7.1.15**（执行层未落） |

- 角色 bot：只读 vault，只写自己的 `$HOME/bot` 与 `state`；`_originals/` 只追加。
- **渲染期过滤（2026-10-07）**：agent **不判权**，只工作在**已被过滤好的世界**里；
  **授权表不进 bot 可读的 vault**，bot 的 cwd 是渲染产物、不是仓库根（SPEC §3 / §6）。
- devbot：全权限，cwd = 仓库本体，改坏靠 git 回滚。
- **单一出口**：所有对外动作只经 serve 的 egress，出口处统一做四件事 —— 幂等键、C 档人确认、审计记录、失败可见。检验：**「客户收到的那条消息是从哪来的」必须只有一个答案。**

---

## 6. 装配、重建与观测

**装配链（现场按序执行）**

```
anc bootstrap --dry-run      # 打印将要建什么，人核对
anc bootstrap                # 建账号 / 目录 / 只读挂载 / 本体（需管理员）
anc org init <客户库> ...     # 已实现并实测通过
anc render --apply           # org → gateway config（落盘；persona / 路由表 / 权限声明都在这一份里）
anc apply                    # 渲染 + 校验 + 装载上游 daemon + 凭据桥 + 重启 + 回读（一条命令，幂等）
anc doctor                   # 环境与网络自检（已实现）
→  给 bot 发第一条私聊，等回复  # 这才是「装好了」
```

**重建演练是交付物**：在新机器上跑同一条链，记录「到第一条回复」的墙钟时长；压不到 10 分钟就按缺陷立项（SPEC §3.5）。

**观测三源交叉（防假绿）**：服务在册（launchd / systemd / 计划任务状态）＋ 进程存在 ＋ **功能级探针**（给 bot 发一条私聊 ping，等它回）。「没消息 = 没事」是反模式。

**计量**：token 按 bot / provider 落账；业务流只量「一件事从发生到处理完要多久」与「卡住时间占比」。

---

## 7. 必须实测的清单（未验证，不许当已实现说）

| 要验的 | 怎么验 | 通过判据 |
|---|---|---|
| 官方 CLI 的非交互入口与流式输出 | 在某个 bot 账号下跑一次 headless 问答 | 能拿到流式事件与结束码；能中途取消并留下记录 —— **部分实测（§7.1.29）**：某 bot 账号 headless 出字、rc=0；流式事件与中途取消仍未验 |
| 每账号凭据隔离 | 两个账号各自登录同一 CLI | A 账号的凭据不能被 B 账号读取或复用 —— **部分实测（§7.1.29）**：两个 OS 账号各一份 `secrets.env`（0600）、互读家目录 `Permission denied`；凭据走进程环境注入，没走 GUI 登录态 |
| 无人值守启动（含钥匙串能否解锁） | 按 §0-D2 拍板的档位重启机器，不人工登录 GUI | 服务被拉起，且凭据可用（能回话） |
| `bootstrap` 的动作清单 | `--dry-run` 逐条核对后再真跑 | 实际动作与清单完全一致，没有清单外的系统改动 |
| 会话回收与订阅额度 | 并发 N 个会话，观察额度、内存与回收 | 不触发上游限流；空闲进程被回收 |
| Windows 计划任务装载 | 在目标机上 `anc apply --apply`（= 上游 `daemon install --no-capture-secrets --force`） | 任务出现在计划任务库里；`platform ready` 到齐；凭据不进任务定义 —— **已实测，见 §7.1.8** | 
| Linux / macOS 腿的装载 | 在目标机上跑 `anc apply` | 凭据注入按平台落地（systemd `EnvironmentFile=` / launchd 包装脚本）—— **Linux 已实测，见 §7.1.10；macOS 未实现，命令直接报错** |
| 登录自启（`-AtLogOn` / linger / launchd） | 注销或重启一次，不人工登录 GUI | 服务被拉起且凭据可用 —— **Linux 已实测，含硬断电，见 §7.1.11；Windows 只有登录触发器，未验**；`anc doctor` 现在会把这三条前提（Windows 是触发器 + 电源条件）自检一遍，只读 |
| `anc trail` 在别的 harness 上 | ~~codex / hermes 各跑一轮~~ **codex / openclaw 已在真记录上跑通（§7.1.25 / §7.1.26）**；hermes / dsh 仍未验 | codex 记录里本就没有金额（已明说“读不到”）；openclaw 的用量与它自己记录里的 usage 逐数相等 |
| 审计的归集在别的 harness 上 | 用 codex / hermes 各跑一轮，再 `anc audit collect` | 行使能归集出来（工具表是数据，加条目即可）—— **未实测**，现在归集只认 claude 会话记录的格式 |

> 上面最后两行的**接法**已定形：`../SPEC.md` **§4.7②**（H3 原生记录可读 + 用量三口径 / H4 工具名映射是数据）。**换腿 = 加一张表 + 一个 adapter，不是写第二套**；本表这两行在真跑通之前保持「未实测」。

### 7.1 已实测（2026-10-07，Windows + Linux 两腿，cc-connect v1.3.4 / commit 27c1de8f）

**「gateway 是否接受这份配置」—— 已通过。** `anc org init` + `anc render --apply` 产出的三项目 config
（alice / bob = `dontAsk`，devbot = `bypassPermissions`，`append_system_prompt` 为七段 persona 的 TOML
literal block，平台段用 `[[projects.platforms]]` + `[projects.platforms.options]`）交给真 gateway 拉起：

```
INFO msg="platform ready" project=smoke-alice platform=feishu
INFO msg="engine started" project=smoke-alice agent=claudecode platforms=1
…（smoke-bob / smoke-devbot 同形）
INFO msg="api server started" socket=%TEMP%\anc-smoke\data\run\api.sock
INFO msg="cc-connect is running" projects=3
```

派生的三条设计结论：

1. **`data_dir` 是有效键** —— 会话数据落在我们指定的目录，没回流默认 `~/.cc-connect`。
2. **长 persona 不撞命令行长** —— cc-connect 把合入内容写临时文件、用 `--append-system-prompt-file`
   传给 claude（规避 Windows 8192 字节上限），所以 persona 可以继续写长。
3. **上游不校验未知键**（实测：塞 `bogus_key_xyz` 后 `config loaded` 照常、零告警）—— 所以
   「渲染产物被 gateway 接受」**不构成**「字段名都对」的证据。表里下面两行因此升格为必须做：

| 要验的 | 怎么验 | 通过判据 |
|---|---|---|
| 渲染产物的字段名白名单 | 拿 1.3.4 的 `config.example.toml` 当 schema 语料，对我们产出的每个键断言「在样例里出现过」 | 键名全部命中；命中不了的要么补样例、要么删字段 |
| 每次升级后的形态回归 | 升级 cc-connect 时先跑字段白名单，再跑一次烟测拉起 | 两项都过才允许换 pin |

#### 7.1.1 跨平台对照（Linux 腿，WSL2 Ubuntu x86_64）

同一份 `anc` 源码交叉编译出 linux/amd64（`GOOS=linux GOARCH=amd64 go build -trimpath`），
配 cc-connect 的 `cc-connect-v1.3.4-linux-amd64` 发布件（SHA256 `86a8c00d…4c997`，
`sha256sum -c` 通过），在 WSL2 Ubuntu 上重跑同一条链路。**结论与 Windows 一致：**

```
INFO msg="platform ready" project=smoke-alice platform=feishu
INFO msg="engine started" project=smoke-alice agent=claudecode platforms=1
…（smoke-bob / smoke-devbot 同形）
INFO msg="api server started" socket=/home/<user>/anc-linux-smoke/data/run/api.sock
INFO msg="cc-connect is running" projects=3
```

`org init` + `render --apply` 在 Linux 上直接产出 POSIX 路径（`work_dir = /home/<user>/…`、
`data_dir = /home/<user>/…`），**没有 Windows 路径泄漏**；未定义 `${ENV}` 的行为与 Windows 逐字一致
（三条 `WARN` → `config loaded` → `failed to create platform … app_id and app_secret are required`、exit 1）。
`data_dir` 下同样长出 `agent-prompts/`（长 prompt 落文件那条路径在 Linux 也走通）、`crons/`、`timers/`、`run/`。

三处必须记住的平台差异：

1. **socket 形态不同** —— Windows 是文件路径，Linux 是**Unix domain socket**。探针/看门狗不许假定其中一种。
2. **渲染路径是本机原生的** —— 我们只验过「在哪个 OS 渲染就给哪个 OS 的路径」；
   **跨 OS 渲染（Windows 上渲染给 Linux 用）未验，也没设计**。发布流程（§3「运维机 render → 生产机 pull → 重渲染」）
   靠的是生产机本地重渲染，所以这条路是安全的，但别想着把渲染产物跨 OS 搬运。
3. **我们自己那套服务单元从没在真 systemd / launchd 上跑过** —— WSL2 默认 PID 1 是 `init(Ubuntu)`、
   `systemctl --user` 返回 `offline`，要验 systemd 用户单元得先开 systemd（改本机 WSL 全局配置，
   属需授权动作），`launchd` 更需要真 mac。**D4 之后这件事不再由我们承担**（常驻交给上游 daemon）——
   这套代码已于 2026-10-07 退役，教训见 §7.1.9。

#### 7.1.2 服务单元：Linux 腿的离线校验（2026-10-07）—— 抓到一个硬 bug

> **这一节讲的是已退役的 `internal/service`（2026-10-07 退役，见 §7.1.9）。代码删了，教训留着。**

真装载还没做，但**单元文件能不能被 systemd 接受**这一层可以离线判：`systemd-analyze verify`
（WSL2 Ubuntu 里的 systemd 259）。**不需要 systemd 当 PID 1**，只要以 root 跑一次
—— WSL 默认没有 `/run/systemd/`，非 root 会 `Permission denied`。

**当场抓到**：`anc service render --goos linux` 的产出一处**前缀重复**，单元根本装不上。

```
StandardOutput=append:append:<homes>/alice/state/serve.log
StandardError=append:append:<homes>/alice/state/serve.err.log
```

systemd 的判定（修复前，原文）：

```
anc-alice.service:12: StandardOutput= path is not absolute: append:/home/<user>/.../state/serve.log
anc-alice.service:13: StandardError= path is not absolute: append:/home/<user>/.../state/serve.err.log
```

成因：模板里写的是 `StandardOutput=append:%s`，传参时又拼了一次 `"append:"+path`。
systemd 剥掉一个 `append:` 后，剩下的 `append:/...` 不是绝对路径 → 拒收。
修复：模板去掉 `append:`（改为 `StandardOutput=%s`，前缀只由参数带）。
**修复后 `systemd-analyze verify` 无输出（通过）。**

这个 bug 从阶段 B 首次提交（`1a5d03e`）就在，`internal/service` 的测试**没覆盖这两行**，
而且它从没在任何真 systemd 上跑过 —— 正是 N5「装上了 ≠ 跑起来了」的实例。

**同批做掉的**：darwin 的 plist 与 windows 的 task XML 用 XML 解析器验证**良构**
（Windows 侧，两份都 OK）。**注意这只证明「结构合法」，不证明「launchd / schtasks 接受」**
—— 后者要真机。

**仍未覆盖**：`systemctl --user enable --now` 真装载、linger、`launchctl load`、
`schtasks /Create /XML`（要往本机任务库写东西，属需授权动作）、以及 `anc serve` 真跑（尚未实现）。
**这些都不再由我们承担** —— D4 之后装载走上游 daemon（§7.1.8），代码在 §7.1.9 退役。

#### 7.1.3 端到端真腿：真飞书 app 走通一轮对话（2026-10-07）

前两节验的都是「gateway 起得来」。这一段验的是**消息真的能进去、agent 真的会答、答复真的发得回来**。
用的是一条**专用测试腿**（运维者个人飞书应用，**未绑公司账号**），不碰生产。

现场：`anc org init`（单成员 alice）→ 填真实 `feishu.app_id` / `feishu.open_id` → `anc render --apply`
→ `cc-connect --config …`，密钥只走**进程环境变量**（`ANC_FEISHU_SECRET_ALICE`），不落盘。

```
INFO msg="feishu: bot identified" open_id=ou_05e1…4de1
INFO msg="platform ready" project=live-alice platform=feishu
INFO msg="engine started" project=live-alice agent=claudecode platforms=1
INFO msg="cc-connect is running" projects=1
[Info] [connected to wss://msg-frontier.feishu.cn/ws/v2 …]
```

用户发「你好」之后：

```
level=INFO msg="message received" platform=feishu session=feishu:oc_1837…:ou_8759… content_len=6
level=INFO msg="session spawned" agent_session="" is_resume=false elapsed=61.9052ms
level=INFO msg="turn complete" session=s1 tools=0 response_len=166 turn_duration=6.9600825s
  input_tokens=42923 output_tokens=223 silent=false
```

**四个结论**：

1. **渲染产物能驱动一次真实对话** —— persona（七段）确实进了 claude 的上下文：bot 回的是
   「冒烟在。日程、消息、入库，说吧。」，与我们在 `members/alice/persona.md` 里写的服务对象逐字对应。
   「gateway 接受配置」由此从「进程起得来」升级为「**能干活**」。
2. **`allow_from` 用渲染进 config 的 open_id 生效** —— 用 app owner 的 open_id 投递即达，
   说明渲染器把 `member.feishu.open_id` → `allow_from` 这条链路是通的。
3. **长连接（websocket）形态可行** —— 飞书 `callback_type=websocket`，cc-connect 直连
   `wss://msg-frontier.feishu.cn/ws/v2`，**不需要公网回调地址**。这对「一台 Mac mini 在内网」的部署形态是关键前提。
4. **逐轮 usage 是现成的**（详见 §4.6 / GitHub #12 的评论）：`cc-connect` 的 `turn complete`
   自带 `input_tokens` / `output_tokens` / `turn_duration` / `tools`。**但**它只是 stdout 日志行，
   会话落盘 JSON（`data/sessions/<project>_<hash>.json`）里**没有** usage 字段 —— 长期归集的结构化出口**未定**。

**同时抓到一个隔离缺口**：bot 家目录 `homes/alice` 是**空的**，但 bot 的回复里冒出了运维者个人
`~/.claude` 的内容（caveman skill / `settings.json`）。**persona 不是 bot 上下文的全集。**
详见 GitHub **#28**。

**仍未覆盖**：多 bot 并发（本次只一个 project）；订阅制下 usage 的真伪（`input_tokens` 的来源未核）；
bot 家目录与个人配置的隔离（#28）。

#### 7.1.4 usage 的取数路径：读 harness 原生记录（2026-10-07）—— 三方对拍

§4.6 承诺了 token 计量 / 成本分账，但一直没定「数从哪来」。查清了，结论是**读 harness 原生记录**。

**先排掉三条不行的路**：内部 HTTP API（`data_dir/run/api.sock`）的 `GET /sessions` 只回
`project` / `session_key` / `platform`，**没有 usage**；会话落盘 JSON（`data/sessions/*.json`）只有 `history`；
cc-connect 的 `turn complete` 日志行**只打 `input_tokens` / `output_tokens`**，
而 `core.Event` 上明明有 `CacheCreationInputTokens` / `CacheReadInputTokens`（`engine.go:5101` 没打）。
回复页脚有全量（`<model> · out N · in N cw N cr N · ctx N%`），但它在 interactive card 里，解析脆弱。

**日志这条可以用**：`CC_LOG_FILE` 把 slog 重定向到可轮转文件（实测落地）。但**轮转只留 `.1` 一个备份**，
最旧的丢 —— 归集必须**持续消费**，不能事后翻。且格式是 TextHandler，**没有 JSON 选项**。

**真正可用的是原生记录。** cc-connect 的 `agent_session` 就是 claude 的 session id，用它直接定位
`~/.claude/projects/<slug>/<session>.jsonl`，逐轮抽 `message.usage`：

| 轮 | 原生 `in` / `out` | 原生 `cache_read` | cc-connect 日志 |
|---|---|---|---|
| 1 | 42923 / 223 | 0 | `input_tokens=42923 output_tokens=223` **逐字一致** |
| 2 | 203 / 2 | 43008 | `input_tokens=203 output_tokens=2`（cr 缺） |
| 3 | 23194 / 88 | 27008 | 无 `turn complete` 行 |

全天汇总（Agent=Claude）：原生逐轮相加 = `ccusage daily` = `66320 / 313 / 70016`，
而 cc-connect 日志累加只有 `66320 / 313 / **0**`（字段不存在）。

**三个结论**：

1. **cc-connect 的 in/out 是转述不是估算** —— 与原生记录逐字一致。
2. **原生记录字段最全**（in/out/cr/cw），连 `costUSD` 都有（见下）。
3. **不必为 cw/cr 去 fork cc-connect，也不必抓页脚。**

**代价（必须记住）**：这条路**绑死 claude 的存储格式**
（`<HOME>/.claude/projects/<slug>/<session>.jsonl`、行式 JSON、`message.usage`），**不是稳定契约**
—— 同一次就撞见 `cost-state` / `atis-latch` / `last-prompt` 这些未公开行类型。换 harness 就要换一套解析。

**意外收获**：原生记录里有 `type: "cost-state"` 行，claude **自己算成本并按模型分账**：
`totalCostUSD` + `modelUsage{ "<model>[<window>]": { inputTokens, outputTokens, cacheReadInputTokens,
cacheCreationInputTokens, thinkingTokens, costUSD } }`，且**未知模型会标 `hasUnknownModelCost: true`**。
「未知模型显式报无价格、不猜」这个口径**上游已经实现了**，照抄即可。
**但**：本文件里 `cost-state` 只出现 1 条且只含第 1 轮，是**快照不是账本**，写盘时机**未验**。

**参照实现**：本机装了 `ccusage`，一次认出 Claude / Codex / Hermes / OpenClaw / Qwen，
覆盖 14 种 harness，把 cache create / cache read 单列，并对缺价格的模型打 WARN。
**说明「每种 harness 写一个 adapter」不是天量工作**；用它当参照（或依赖）比自己从零写更划算。

> **修正（2026-10-07，见 §7.1.6）**：本节那句「原生逐轮相加 = `ccusage daily`」是按**行**相加得出的。
> 在同一现场复算：按行相加 = `172549 / 17184 / 3392000`，按 `message.id` 合并 = `85726 / 8517 / 1659520`
> —— **正好一倍**。所以「按行相加」不能当口径用，正确做法是**按 `message.id` 合并**（一条消息会写多行）。
> 本节那次为什么能对上没查清（可能那份样本里没有重复行）；**以 §7.1.6 的口径为准**。

### 7.1.5 报红送人：`anc notify`（2026-10-07）

§6 的观测只做到「看得见」。用户拍板：**报红要由 bot 主动送到负责人面前**，
不能躺在 CLI / 看板里等人来看。落地成 `anc notify`，与 `probe` 分成两条命令 ——
probe 只读、永远无副作用；notify 有副作用（往人的聊天里推消息）。合在一起会毁掉
probe 的立身之本（只读命令被随手跑），所以判据仍是同一份 `internal/probe`，不写两套。

**判据（纯函数 `notify.Decide`，无时钟、无盘、无网，18 条档位用例覆盖）**：

- **只看红。** 黄 = 运行中 / 这一窗口没观测到，不要人动手；把黄也推给人 → 人静音 → 红的告警一起死。
- **边沿触发 + 冷却**（默认 30m，同 probe 的 `--stale` / `--stall` 一样是参数）：
  没喊过 + 红 → `alert`；红着且过冷却期 → `reminder`（否则就成了「没消息 = 没事」）；
  红转绿 → `recovery` 并清账；冷却期内 → 只落 `Plan.Silent`（**让「没喊」说得出来**）。
- **网关挂了只喊 `gateway` 一条**，且此时不评估 bot —— 底下每个 bot 判红是同一根因，逐个喊就是刷屏。
- 没喊过的账转绿 → **不欠谁一声**（不发 recovery）。

**通道**：走 cc-connect 的 unix socket `POST /send`（`{"project","message"}`，`core/api.go`）。
与探针判活**同一条通道** —— 不直连飞书 API，换平台（钉钉 / 企业微信）是 cc-connect 那侧的事。

**游标**：`<data_dir>/state/notify.json`（不进 git）。**dry-run 不写**（写了会吞掉下一次真发的告警）；
推送失败（一个都没送到）也不写 —— 不把失败当成「喊过了」；坏 JSON 报错，不当空游标静默吞。

**推给谁**：`company.admins` 各自的 bot，**不是出故障那个 bot**（它可能连话都回不了）。
不是成员 / 已停用的收进 `unresolved` 明说，不静默少喊一个人。

**实测**：`gofmt` / `go vet` / `go test ./...` 全绿（notify 包 18 条，含真 unix socket 上验请求体与错误路径）；
活体 dry-run 沙箱 🟢 →「没有要喊的」exit 0；负例（临时现场无 socket）→ `[新告警] gateway`、exit 1、
**且未落游标**；无动作时 `--send` 写空游标成功。

**真发已实测（2026-10-07，用户在场，沙箱）** —— 整条闭环都走过真通道，没有一步是 mock：

| 步 | 做了什么 | 结果 |
|---|---|---|
| 造红 | 沙箱里放一个「有消息、但 `agent_session_id` 为空」的假会话 | `anc probe` 判 🔴（`Judge` 合并同项目所有会话文件，这条签名就是「引擎绿但回不了话」） |
| 喊人 | `notify --send` | `[新告警] demo-alice` 推到 `demo-alice`，gateway 回 200，**负责人聊天里真收到**；退出码 0，游标落盘 |
| 不唠叨 | 红档还在时**再跑一次** | 「没有要喊的（红 1 / 黄 0）」，1 条落 `Silent` —— 冷却真压住了，没重复喊 |
| 报平安 | 删掉假会话，再 `--send` | `[已恢复] demo-alice` 推到人面前，退出码 0，游标清回 `{}` |

沙箱已复原（🟢）。**仍未覆盖**：真网关真故障（不造假会自己变红的那种）下的端到端 —— 
那要等真出事，或者专门搭一个坏掉的 gateway。

### 7.1.6 留痕的事实层：`anc trail`（2026-10-07）

§4 第 5 条要「落审计行：谁问的、读了哪些源、做了什么分档动作、消耗多少 token、耗时多久」，
§5 要「客户收到的那条消息从哪来只有一个答案」。**写入端 harness 自己已经有了**（记录就是账本），
缺的是把账翻成人看得懂、并能按 bot / 按轮对上的那一层。这就是 `anc trail`：**只读聚合，零新增写入**。

两份现成记录，缺一份都不完整：

| 源 | 位置 | 它回答什么 |
|---|---|---|
| cc-connect 会话落盘 | `<data>/sessions/<project>_<hash>.json` | 哪个 bot 的哪个会话槽对应哪个 harness 会话 id —— **两份记录之间唯一的桥** |
| harness 原生记录 | `<记录根>/projects/<slug(work_dir)>/<id>.jsonl` ＋ `<id>/subagents/*.jsonl`（**这套布局来自接入口径表**，见下） | 逐轮的 token / 工具 / 被拒 / 耗时 / 成本 |

**三条口径是实测出来的，不是看着像**（把聚合结果与 claude 自己的 `cost-state` 对拍，**逐字段相等**）：

1. **不按行累加。** 同一条 assistant 消息写成多行（一行一个 content block，流式中间态
   `output_tokens=0`）。按行相加 = 正确值的**两倍**（本现场 `172549` vs `85726`）。
   正确做法：按 `message.id` 合并 —— 用量取各次出现的最大值，工具名取并集。
2. **子 agent 的账算在这个 bot 头上。** `<id>/subagents/*.jsonl` 是 Agent 工具拉起的旁路会话
   （本现场 1 个 Explore，`in 60279 / out 7969`）。不并进来会少报一大截；`meta.json` 的 `toolUseId`
   能对上主记录里的 `tool_use.id`，所以还能**精确归到拉起它的那一轮**（本现场 = 第 6 轮）。
3. **成本用 harness 自己算的**（`cost-state.totalCostUSD`），它连「未知模型没价格」都标了
   （`hasUnknownModelCost`）。自己不维护价格表。

对拍结果（同一段会话）：

```
trail 聚合（主 + 子 agent，按 message.id 合并）  in 146005  out 16486  cr 2071680  cw 0
claude cost-state（harness 自己的账本）          in 146005  out 16486  cr 2071680  cw 0
```

**坑（都写进代码注释了）**：一行可能很大（整段工具输出塞在行里），`bufio.Scanner` 默认 64KB
会直接报错 —— 那是**读不到**，不是「没有」；解析不了的行要**数出来报**；`work_dir` 住在
`[projects.agent.options]` 里（项目名之后还有段头），按「见 `[` 就重置」扫会把整段丢掉。

**读不到就明说**：原生记录找不到 / 解析不了时，`trail` **报「归集不上」并以退出码 1 退出**，
合计行也标注「其中 N 段读不到，没算进来」—— 一个光秃秃的 `0` 和「真的一分没花」长得一模一样，
那正是我们一直在防的假绿。

**实测**：`gofmt` / `go vet` / `go test ./...` 全绿（trail 包 12 条 + render 2 条）；活体在沙箱跑出
3 段会话、6 轮时间线、子任务归属与逐轮耗时（不含空闲）；负例（临时现场 work_dir 指向不存在的地方）
→ 3 段全报「归集不上」、Exit 1、合计注明「3 段读不到」；`--json` 结构与字段核对过。

**接入口径是数据，不是代码**（2026-10-09）：上面那套布局（记录根从哪个环境变量来、项目目录怎么起名、
主记录 / 子任务记录叫什么）已经从代码里搬进 `internal/harness` 的 `harnesses.json`：

- `--harness <id>` 选哪一家，`--harnesses <文件>` **整份替换**那张表（同 `--rules` 的做法）；
- 加一家 = 加一行数据；`reader` 为空的家**只登记了口径、读不了**，代码会明说
  「只登记了口径，没有读取器」并以退出码 1 退出 —— 不装作 0 消耗；
- 仍绑在代码上的只剩**解析语义**：一家一个读取器（`claude-jsonl` / `codebuddy-jsonl`），
  读取器名写在表里。名字不认识**报错**，不退回默认读法 —— 按错的口径读出来会把别家的记录
  「读成一堆数字」，那比读不到更糟。

**第二条腿：CodeBuddy / WorkBuddy CLI 读取器（2026-10-09）**。落盘位置与文件名跟 claude 同形
（`projects/<cwd 转义>/<会话 id>.jsonl`），但**行的语义不一样**，所以是另一个读取器。四条差异，
全部来自沙箱真跑（用户提问 → `Read` 工具调用 → 工具结果 → 收尾回答，四种记录都见到了）：

1. **用量按步挂**（`providerData.usage`），没有 claude 那种 `cost-state` 累计快照；同一步的
   `function_call` 与 `function_call_result` 带**同一份**用量 → 按 `providerData.messageId` 去重取最大值。
   这和 claude 是同一类坑（按行相加账翻倍），但触发方式不同，所以去重键也不同。
2. **工具调用是独立的行**（`type: function_call` / `function_call_result`），不是 assistant 消息里的
   `tool_use` block；`function_call_result.status != completed` 才算「没成功」。
3. **时间戳是毫秒 epoch 数字**，不是 RFC3339 字符串；这家还会写十几种非消息行
   （`file-history-snapshot` / `summary` / `turn-metrics`…），一律跳过 —— 跳过不是丢账，它们本就不是账。
4. **记录里没有成本字段** → 成本记 0，并在 `problems` 里明说是**读不到**、不是花了 0 元。

**项目目录的转义规则也不一样**（这条不实测就一定会「读不到」）：claude 是「每个非字母数字 → `-`」，
CodeBuddy 是「**路径分隔符连续的一段 → 一个 `-`**，点 / 空格 / 中文原样保留，盘符小写」
（`D:\某目录\Probe_X` → `d-某目录-Probe_X`；四个路径实测）。所以规则也进了表（`dir_rule`）。

**证据链要交代清楚**：沙箱里没有腾讯账号（`codebuddy` 未登录时只回一句「Authentication required」），
所以把 CLI 指向一个**本地桩端点**（`models.json` 里一张 `custom-local:` 自定义模型 + 一个 OpenAI
兼容的本地 SSE 桩）来产出记录。**模型那一跳不是真的**（桩按固定文本回话），
**记录的形状是真的** —— 上面四条差异全部来自这些真记录，不是照文档猜的。

**实测**：`gofmt` / `go build` / `go vet` / `go test ./...` 15 包全绿（新增 5 条用例：4 条解析 +
1 条目录规则；另修 1 条会因「表里那家已有读取器」而**静默跳过**的旧用例 —— 覆盖悄悄归零是假绿的一种）；
沙箱活体（真 cc-connect 会话
文件 + 真记录）跑出 1 轮、`in 100 / out 18`（= 40+60 / 12+6，按步去重对得上）、`Read ×1`、耗时 600ms，
两条 problems 如实报出、退出码 1；**同一现场用错的口径**（`--harness claude`）跑 → 目录名算成 `D--ANC--`、
明说「原生记录找不到」，一个数字都不报。

**未覆盖**：只认**已经验过的两种**记录形态（claude 的 jsonl、CodeBuddy 的 jsonl）——
codex / hermes / openclaw / dsh 还没读取器，表里 `reader` 为空的一律明说读不了；
CodeBuddy 的**子任务记录**（`Agent` 工具拉起的旁路会话）落不落盘、落在哪，未验，
所以表里没声明子任务布局（→ 当作没有，而不是编一个目录出来）。**判断层没做**：「决定是什么」「失败根因」不在此列，
`trail` 只出事实，不替 agent 编理由。

### 7.1.7 留痕的判断层：`anc trail` 的判据（2026-10-07）

事实层只回答「发生了什么」。这一层回答「这算什么」，**且判断与事实在输出里分开摆** ——
混在一起，读的人就分不清哪句能当证据用。

**判据是数据，不是代码。** 引擎里没有任何一个 `if` 认识某条具体判据：

- 判据 = 一张表（`id / scope / level / title / say / when`），`when` 是对**指标**的比较
  （`denied >= 2`），`say` 是带占位符的模板（`{denied_tools}`）；
- 出厂 4 条（`internal/judge` 的 `Builtin`），`--rules <文件>` 可以**整份替换**；
- **加一条判据 = 加一行数据**，不用改引擎、不用重编译；换一套完全不同的判法，换的是那份文件。

**这里没有门禁。** judge 只出结论：不拦任何事、不改退出码、不自动动手。
判错了的代价应该只是「多了一行话」，不是「一件本该发生的事没发生」。

**三条自我约束**（都写进代码了）：

1. **来源与证据缺一不可。** 每条结论带 `source`（现在是 `rule`，将来模型路线会写 `model` ——
   这条字段就是「判据」与「猜测」的分界线）与 `evidence`（点回记录原话）；**没有证据不发**。
2. **证据只列相关的那一类。** 一条「被权限挡下」的结论，证据里混进「文件不存在」，
   读的人就没法判断这条结论成不成立。相关性从判据自己写的指标推出来，不在引擎里写死。
3. **写错的判据要被拒。** 指标名、比较符、占位符都校验；不认识的直接报错 ——
   一条**永远不会命中**的判据，比一条写错的更难发现。新增**事实**才需要动事实层与指标表，
   判据可以随便长，事实必须来自记录。

**事实层为此补的一格**：`Failure{kind, tool, why}` —— `denied`（被权限规则挡下）与
`failed`（真执行失败）**分开记**。把「权限没给」说成「故障」，人会去查一个不存在的问题；
把真失败说成「权限不够」，人会去加权限、问题还在。`why` **照抄记录原话**（截掉给模型看的
`IMPORTANT:` 提示尾巴），不转述。工具名靠 `tool_use_id` 回查主记录，查不到就留空、**不猜**。

**实测**：`gofmt` / `go vet` / `go test ./...` 全绿（judge 10 条 + trail 13 条）；
沙箱活体跑出 6 条结论（`session-denied-heavy` ×1、`turn-denied-repeat` ×2、`turn-tool-failed` ×3），
证据各自只列相关那一类；**换一份判据文件**（自定义阈值与措辞）行为随之改变，**没有重编译**；
`--no-judge` 只出事实；`--json` 结构为 `sessions: [{session, findings}]`，字段核对过；
负例（读不到）只命中 `session-unreadable` 并以退出码 1 退出。

**未覆盖**：出厂只有 4 条判据（**覆盖窄是有意的** —— 它是起点不是标准答案，现场该改的是那张表）；
「失败根因」的**模型路线**没做，将来也只允许作为「提案」出现，不能当结论。

---

### 7.1.8 装载：`anc apply`（2026-10-07，Windows 腿，沙箱真跑）

**一条命令七步**：校验 org → 渲染（与 `anc render` 共用同一份落盘实现与门禁）→ 凭据体检 →
`cc-connect daemon install --no-capture-secrets --force` → 凭据桥 → `daemon restart --force` → 回读。

**为什么「凭据桥」要单独一步**：上游 daemon 的装载体**没有 dotenv**（实测 v1.3.4 二进制里搜不到），
`${ENV}` 只从**进程环境**解析；而 `daemon install` 默认会把 `${ENV}` **捕获成明文**写进服务文件。
所以两头都必须在：装载带 `--no-capture-secrets`，拉起 gateway 之前把 `secrets.env` 读进进程环境。
注入的是**上游生成的** `cc-connect-daemon.ps1`，因此托管区带边界标记与指纹，幂等可重入、被冲掉看得出来。

**实测（沙箱 `anc-demo`，真跑）**：

| 验的 | 怎么验 | 结果 |
|---|---|---|
| dry-run 不碰任何东西 | `anc apply <vault>` | 退出 0；`config.toml` 不存在、没跑任何 daemon 命令 |
| 真装载 | `anc apply <vault> --apply` | `platform ready 1/1`、探针 🟢；装载体顶部出现托管区（`fp 720cbfde3d11`），上游原文逐字保留在下面 |
| **凭据不进任务定义** | `Export-ScheduledTask -TaskName cc-connect` 的 XML 里找键名与密钥值 | 键名 → False，32 位密钥值 → False；任务动作只是一句 `powershell.exe -File <家目录>\.cc-connect\cc-connect-daemon.ps1` |
| 幂等 | 重跑 `--apply`，比对装载体 SHA256 | 哈希不变、`platform ready 1/1` |
| 缺键 = 停 | 凭据文件留空跑一次 | 退出 1，**在装载之前**就停（不装载一个必然起不来的 daemon，否则这台机器上原来在跑的 bot 会全部下线） |

**一处如实记**：上游 `install --force` 每次都会把装载体整份重写回它自己那份，所以重跑在**文件层面**
是「重写回原样 → 再放回托管区」。命令不把它说成「没动过」，而是说「重注（内容与上次一致）」。

**踩到并修掉的一处**：`render.EnvRefs` 第一次跑就把产物头部注释里的 `${ENV}`（讲口径用的那句话）
当成了真实的键名，于是体检报「缺键 ENV」。修法：**整行注释不算引用**。这是「先跑 dry-run」的价值。

**没做（明确标出）**：

- **macOS 腿**：launchd 包装脚本未实现（议题 #23）。**未实现即报错**：`anc apply` 在 macOS 上直接拒绝，
  不退化成「前四步做完、留个读不到凭据的 daemon」。Linux 腿见 §7.1.10。
- **登录自启**：上游写的是 `-AtLogOn`，要注销 / 重启才验得到；本轮只验了「现在这一次拉起」。
- **ACL**：`~/.cc-connect` 整棵树与 `~/.anc` 其余部分仍是默认继承（归议题 #4；`secrets.env` 已收紧）。
### 7.1.9 退役记录：`anc service`（2026-10-07）

**为什么退**：D4 把 `serve` 降级成 `apply`、且 serve 永不存在，而 `anc service render/install`
生成的单元跑的就是 `anc serve --bot` —— 装载它只会得到一个反复退出的服务。
留着等于留一条「看起来能用、装了就坏」的路径，**比没有更糟**。

**删了什么**：`internal/service/`（三平台单元生成 + 指纹 + `~/` 展开）、`runtime/service.go`（CLI）、
`runtime/service_cli_test.go`（4 条 CLI 测试）。装载的职责归 `anc apply`（§7.1.8）。
删除前先在机器上查过：没有我们自己装过的计划任务（`ANC\*`）、也没有 `~/anc/tasks/` 残留，
所以**没有任何东西需要卸载**。

**留下的教训**（代码删了，这三条不许忘）：

1. **模板 + 参数拼接是 bug 温床**：模板写 `StandardOutput=append:%s`、传参又拼一次 `"append:"+path`，
   产出 `append:append:/…`，systemd 直接拒收（§7.1.2 抓到的那处）。
2. **「结构合法」不等于「对方接受」**：XML 良构、`systemd-analyze verify` 通过，都只是离线判据。
3. **单测没覆盖的行等于没写**：那个 bug 从阶段 B 首次提交就在，测试恰好绕过了它。

**顺带得到的判据**：常驻这件事上游已经做完了（`cc-connect daemon`），我们自研的那一套
除了多一个失败面没有任何增量 —— **能不自研就不自研**；同理，「一个 bot 一套进程」在
上游「一个 daemon 管全部 project」的现实下本来就不成立。
### 7.1.10 装载：Linux 腿（2026-10-08，Ubuntu 24.04 / systemd 255，真 VM 真跑）

**沙箱**：VMware Workstation 里的 Ubuntu 24.04 cloud image —— `pid1=systemd`、`systemctl is-system-running=running`、
用户级 `systemctl --user` 也是 running、linger 已开。WSL 不行：没有 systemd 就没有 `systemctl --user`，
而 `daemon install` 在 Linux 上只走 systemd，没有用户会话它会直接拒绝。

**机制：我们自己的 drop-in，不碰上游的单元文件。**
Windows 腿接管的是**上游生成**的 `cc-connect-daemon.ps1`（所以要维护边界 + 指纹 + 「被重装冲掉看得出来」）。
Linux 腿反过来：上游按 euid 落单元（root → `/etc/systemd/system/cc-connect.service`，
非 root → `~/.config/systemd/user/cc-connect.service`），`daemon install --force` 每次把它整份重写回自己那份；
而 systemd 的 drop-in（`<单元名>.d/anc-secrets.conf`）是**我们完全拥有**的文件，上游不碰它 ——
天然幂等，也不用去解析别人的文件。装载时只去这两个位置里**找**真落点（不猜），找不到就明说试过哪些。

**两个真 bug（都是实测抓出来的，别再踩）**：

1. **systemd 的 `EnvironmentFile=` 不给去引号。** 写成 `EnvironmentFile="…"` → systemd 报
   `EnvironmentFile= path is not absolute, ignoring: "/home/anc/.anc/secrets.env"`，整条被忽略，
   daemon 起来后报「引用了没定义的环境变量」。修：**不加引号**，且路径不能带空白（测试里加了「不许有引号」的断言）。
2. **上游 `daemon restart` 不认新写的 drop-in，必须我们自己 reload。** 实测：写完 drop-in 后
   `systemctl --user show cc-connect -p EnvironmentFiles` 是空的，跑一次 `systemctl --user daemon-reload`
   之后才出现 `/home/anc/.anc/secrets.env (ignore_errors=no)`。修：`bridgeLinux` 写完文件自己 reload 并回显。

**这两条正好解释「引擎绿但起不来」**：不 reload 或带引号 → daemon 起来了却报
`config: env var placeholder references unset variable`，而表面上 `platform ready` 还在。

**实测（真跑，逐条可复现）**：

| 验的 | 怎么验 | 结果 |
|---|---|---|
| 装得上 | `anc apply <vault> --apply` | 第 4 步 `cc-connect daemon installed and started. Platform: systemd (user)` |
| **systemd 真认这份 drop-in** | `systemctl --user show cc-connect -p EnvironmentFiles` | `EnvironmentFiles=/home/anc/.anc/secrets.env (ignore_errors=no)`（reload 之前为空） |
| **凭据真进了进程环境** | `/proc/<MainPID>/environ` 里数键名 | `ANC_FEISHU_SECRET_{ALICE,BOB,DEVBOT}` 三个都在（只报键名，不打印值） |
| **凭据不进单元文件** | 在 `~/.config/systemd/user/` 下 grep 键名与密钥值 | 空 —— 单元与 drop-in 里没有任何明文密钥 |
| 幂等 | 重跑 `--apply`，比对 drop-in SHA256 | `0970b11a…` 不变，`fp 1104018a058f` 不变 |
| 端到端 | 日志里数 `platform ready` | `platform ready 3/3` + `engine started … agent=claudecode` |
| 探针 | `anc probe <vault>` | 三个 project 都是 🟡「没观测到任何会话」—— 没发消息就不许报绿 |
 探针的退出码口径：**不是全绿就 `exit 1`**（`probe.go:108` 的 `rep.AllGreen()`）—— 于是刚装好、还没人
发过消息的机器上 `anc probe` **必然返回 1**（三个 🟡）。这是「不许报绿」的直接后果，**别把它当
健康门禁串进 `&&`**：`anc apply` 第 7 步就是因此只回显、不采信它的退出码。

**一处已更正（2026-10-09）**：上面那句「刚装好的机器必然返回 1」在**单腿客户**身上已经不成立 ——
只有一个真飞书 app 时，另外几个占位 bot 会在真相源里声明 `unwired: true`，落灰、**不挡绿**，
于是 `anc probe` 在这些机器上正常返回 0。判据见 §7.1.20。

**一处已更正（2026-10-08 复测）**：曾记「VM 的 NAT 出不去飞书（`open.feishu.cn` 连接被 reset）」——
那是**当时主机侧透明代理那一分钟的状态**，不是这台沙箱的固有属性。取证：日志里 `connection reset by peer`
只出现在 2026-10-07T17:40Z 前后（对端是 fake-IP `198.18.0.75:443`，Clash 类代理的特征段）；此后同一条
出网路径拿到的是**真飞书响应** —— `curl -X POST https://open.feishu.cn/open-apis/auth/v3/tenant_access_token/internal`
→ `{"code":10003,"msg":"invalid param"}`（HTTP 200，真服务端 JSON），cc-connect 自己的日志也从传输错误
变成服务端错误（`1000040346: app_id is invalid`、`msg:invalid param,code:10003`）。结论：**Linux 腿「真回话」
只缺一个真 app_id，不缺出网。** `platform ready` 判的仍是 gateway 自己起没起来。

**没做（明确标出）**：

- **macOS 腿**：零覆盖（议题 #23）。
- **登录自启**：本条当时只验了「现在这一次拉起」；「重启 / 断电后自己起来」后来补验了，见 §7.1.11。
- **真回话**：探针停在 🟡，缺的是**一个真实飞书应用**（出网已复测通过，见上）。

### 7.1.11 断电 / 重启之后，谁把它拉回来（2026-10-08，Linux 腿三层实测）

**问题**：`anc apply` 只保证「现在这一次拉起来」。服务器不断电是常态，但断电是**必须兜住**的那一次 ——
机器起来之后，没有人在旁边敲命令。

**三层保险，逐层实测（Ubuntu 24.04.5 / systemd 255 真 VM，全程无人登录）**：

| 层 | 怎么造出来的 | 结果 |
|---|---|---|
| 进程暴死 | `kill -9 <MainPID>` | 上游单元带 `Restart=on-failure` / `RestartSec=10`，systemd 10 秒后拉回（`NRestarts=1`、新 PID） |
| 机器重启 | `systemctl reboot` | 开机 **12 秒**后服务自己起来（01:51:01 开机 → 01:51:13 active），此时没有任何人登录 |
| **硬断电** | `vmrun stop <vmx> hard` 再上电 | 仍是 12 秒自己起来（01:52:18 → 01:52:30） |

**硬断电那一条才是真的**：dmesg 留下了非正常关机的痕迹 ——
`EXT4-fs (sda1): INFO: recovery required on readonly filesystem` → `recovery complete`，
以及 `systemd-journald: File …/system.journal corrupted or uncleanly shut down, renaming and replacing`。
也就是说：**日志回放过一轮之后，`anc apply` 装的那份东西照样自己站起来了** ——
残留的 `api.sock` 没挡住它，`cc-connect is running projects=3`。

**为什么它起得来**：上游单元是 `WantedBy=default.target` 且已 `enable`，加上 `loginctl` 的 `Linger=yes`
—— 用户级 systemd 管理器在开机时就起，不等人登录。**三条缺一条都不成立**，
所以这三条都得在（该由 `anc doctor` 查，归 #17）。

**2026-10-08 复验（换了 HEAD 的装载体再走一遍）**：把 HEAD 的 `anc_linux_amd64` 落到客机后，
`anc apply --apply` 全 7 步真做成功（退出码 0、`platform ready 3/3`、探针三个 🟡）。**关键是第 4 步
`daemon install --force` 会把单元整份重写成上游自己那份** —— 重写之后 `is-enabled=enabled`、
`Linger=yes` 都还在，`systemctl reboot` 后服务再自己起来（新 `MainPID`、`NRestarts=0`、无人登录）。
幂等：紧接着第二次 `--apply` 同样 exit 0、`fp 1104018a058f` 不变、`MainPID` 又换了一个、ready 仍 3/3。

**一处如实记（客机时钟）**：这台 VM 的 RTC 存的是**本地时间**，而系统声明 `RTC in local TZ: no`，
于是每次开机内核先把系统时钟读快 8 小时，几十秒后才被 NTP 拉回（实测：daemon 就在那个偏窗里启动，
它头几条日志的时间戳因此是错的）。真实服务器 RTC 走 UTC 不会这样，但值得记一句：
**断电后有一小段时间「时钟是错的」**，任何依赖时间的校验（TLS / token / TTL）都跑在那个窗口里。

**Windows 腿现在是缺口（明确标出）**：上游那份任务的触发器只有 `<LogonTrigger>`。
也就是说，**断电重启后如果没人登录这台机器，这个 daemon 就不会起来**；再叠上 §7.1.8 记的电源条件
（`DisallowStartIfOnBatteries` / `StopIfGoingOnBatteries`），笔记本上一拔电就直接停。
要补「开机就起」得给任务加启动触发器 —— 那要么以 SYSTEM 跑、要么存下账号口令，
是一条**替客户改上游默认**的决定，归议题 #42 拍。

**「这次兜底到底成没成」现在有人说话了（2026-10-08 实测）**：`anc doctor` 多了一组**只读**自检 ——
（**2026-10-16 补**：这一组后来又多了最硬的一格「任务是不是被停用」—— 见 §7.1.57。
本机当天实测就命中：这台开发机的 cc-connect 任务定义里是 `<Enabled>false</Enabled>`，
**一直是被停用的**，而在此之前这一组只看触发器与电源条件，从没报过它。）
Linux 查那三条前提（单元在不在 / `is-enabled` / `Linger`），Windows 查触发器里有没有 `BootTrigger`
与两条电源设置。**没装机就明确跳过**（不给刚 `init` 的机器平添红字），**不替客户改上游默认**，
也不设门禁（doctor 的结论行本来就写着「⚠️ 不一定是阻断项」）。实测：Linux 真 VM 三条全绿
（`单元 /home/anc/.config/systemd/user/cc-connect.service`、`enabled`、`Linger=yes`）；
未装机那条路（`HOME=/tmp/nohome`）如实报「单元不在（还没 apply）—— 跳过」；
Windows 真机如实报出「触发器只有 `LogonTrigger`」+「电池供电不启动 / 拔电就停」两条。
⚠️ **如实记**：`Linger=no` 与「单元 disabled」这两条**没真造出来跑过** —— 判据的解析有用例
（`parseLinger`），绿档那条也已经把「exec → 解析 → 分岔」整条路走通了，但要把「兜底坏掉」
那一刻真造出来，得先停 linger / 停单元，没动。系统单元（root 那份）同理没跑过。

**顺带改掉的一处误导**：`anc apply` 第 7 步原来在 `platform ready 0/N` 时一律把 FIX 指向凭据桥。
现在它先拨一次 socket 分岔（`startupHint`）：**进程根本没起**（拨不通）与**起来了但没到 ready**
是两种病，要查的地方完全不同。上面那台 Windows 机器今天就正好撞在第一种上：
同一台机器、同一时刻，输出从「凭据桥没生效」变成了「gateway 没在跑 + 看计划任务（含电源条件）」。

### 7.1.12 留痕的「决策与执行」层：`anc timeline`（2026-10-08，真沙箱 + 看板第七页）

**问题**：§7.1.6 记的是**机器做过什么**（事实层）、§7.1.7 记的是**该不该做**（判断层）。
还缺一层：**这件事卡在哪、谁拍的板、agent 怎么执行的、成没成、根因是什么** ——
人和 agent **自己写下来**的那条流水。它**不是审计**：谁**尝试**做了什么（含越权尝试）属于事件面，
还没实现（页内如实列缺，不假装答了）。

**为什么是 append-only JSONL，不是数据库**：vault 归 git 管，留存记录最需要的恰恰是
diff / review / 回滚 —— 二进制库这三样全丢。代价如实记：过滤与分页是**扫描**而非索引，
按「一条任务几条记录」的量级可忽略（`limit` 上限 200 只是防 `?limit=100000` 把一次请求变成全库扫描，
不是门禁）。

**三条口径**（都刻意留了「以后不用重构」的余地）：

| 口径 | 落法 |
|---|---|
| 真相源 = `timeline/<YYYY-MM>.<作者>.jsonl` | 按**作者**分片 —— 两个 agent 同时写也碰不到同一个文件，并发就地消掉，不靠锁 |
| 词表不锁死 | 配色只认 `done`(绿) / `running`(黄) / `blocked`·`failed`(红)；**认不出的词原样保留、画灰**（`unknown` 档）—— 以后加词只改数据 |
| 一个 case 多行 = 一次推进 | 折叠取最后一条当当前态，`recent` 带最近 5 条；**历史一行都不删** |

**不设门禁**：`add` 不校验「谁能写」（那是授权层 #32–#35 的事）；`kind` 是约定、不是白名单，
写别的也收。目录不存在 = 还没开始记 = 空时间线，**不是错**（刚 `init` 的机器不该因此报红）。

**实跑过什么（照实记）**：

- Go 侧：`go test -count=1 ./...` 十一包全绿（新增 `internal/timeline` 12 条、`internal/board` 时间线 6 条）。
  用例盯的是真踩过的坑 —— 分页游标里的 `+` 在 query 串里被解成空格（照抄 `next_before` 必踩）、
  空折叠结果必须是 `[]` 而不是 `null`（前端 `.length` / `.map` 会崩）、坏行只报 `文件名:行号`（不带本机路径）、
  参数错不当 500。
- CLI：临时 vault 里加 4 条 → 折成 3 个 case → 按作者落到两份文件 → 三档计数与
  `--json` / `--limit`（给出 `--before` 游标）/ `--band red` 全对。
- 看板第七页：新装 `dist/anc.exe` 起在 `127.0.0.1:8787`，在真沙箱 vault 里记 4 条（含一个**认不出的 status 词**），
  `--dump-dom "#/timeline"` 断言真数据真渲出来（`feishu-reply` / `waiting-upstream` / `灰 · 认不出的词`），
  并与 `/api/timeline` 的 `counts`（1 黄 / 1 红 / 1 灰 case）对了账。
- **没验的**：真机观感（字体 / 毛玻璃 / 屏宽折行）；控制台错误数（要 CDP 才测得到）；
  导航切换 / 浏览器后退 / 零控制台错误那三项仍是**六页时**的结论，加了第七页之后**没有重跑**。

**顺带改掉的一处自相矛盾**：总览页原来手抄了一份「两块还没接数据源」（数据流 / 原料），
而那两页早就是「已接入」了。现在从 `PAGES` 现算 —— 导航与总览只有一处口径。
### 7.1.13 接入面的信封：`anc envelope`（2026-10-08，单元 17 条 + 活体实测）

#44 的第一块落地：**与 harness 无关的那一层**。网关可替换（#30 已盘 CC / Codex / DSH / Hermes /
OpenClaw 五家怎么接），可换的是网关，**信封不变** —— 所以先把信封立住，再谈阶梯与入口。

**形状**（`internal/envelope`）：`id` / `ts` / `who` / `on_behalf_of` / `scope{domain,project}` /
`kind` / `body` / `refs` / `needs`。**只有 `id` / `ts` / `who` 必需** —— 其余留空不拦。

**两段报，刻意分开**：

| 段 | 判什么 | 怎么报 |
|---|---|---|
| 结构（`Parse`） | 读不读得懂 | 缺 `id` / `ts` 非 RFC3339 / 缺 `who` → **拒收**，退出码 1 |
| 绑定（`Bind`） | 指向的东西在真相源里**存不存在** | 一条发现，档位走生效规则表 |

分开的理由：结构错是「你格式写错了」，绑定错是「我们公司没有这个人」——
混在一起报，人分不清该改 JSON 还是该查人事。

**七条发现，整组默认 warn**：`envelope.who.unknown` / `on_behalf_of.missing` /
`on_behalf_of.unknown` / `scope.domain.unknown` / `scope.project.unknown` / `kind.missing` /
`kind.unknown`。这个档不是随口定的 —— **域表 / 项目表立的就是这个先例**：新能力先「看见」、
不先「拦」，一条红线都不设。要提红写 `company.md` 的 `policy:` 段（`= fatal`），要关掉写 `= off`。
七条全部登记在 `org.DefaultRules` 里，`anc org check --rules` 能看见；
**有测试盯着这件事** —— 规则 id 一旦拼错没登记，`Policy.Level` 的兜底是 `fatal`，等于偷偷上了门禁。

**身份不许自报**：`who` 必须能在 `members/` 里解出来；`on_behalf_of` 支持 `member:名字` /
`role:岗位` / `domain:slug` 三种前缀，也接受**裸名字**（按 成员 → 岗位 → 域 的固定顺序解，
不猜）。这是 #44 的验收判据之一。

**表为空就不报**：没有 `domains.md` / `projects.md` 的 vault，`scope` 一条都不出 ——
同 `LoadDomains` / `LoadProjects` 的谦让：**门禁不许长在别人的文档上**。

**判据摊开**：`check` 顺带把六问打出来（谁 / 代谁 / 哪块业务 / 要什么 / 证据在哪 / 要不要人拍）。
后两问**只列原文**：要不要人拍由信封自己说（`needs`），我们不从字符串里猜意图。

**实测**（2026-10-08，本机 `dist/anc.exe`，对着 `testdata/orgs/domains` 与真沙箱 vault）：

| 用例 | 结果 |
|---|---|
| 干净信封（`who=alice`、`on_behalf_of=member:alice`、`scope=trade/trade-q3`、`kind=ask`） | 六问全答出，**0 项发现**，exit 0 |
| 到处不对（`who=nobody`、`bot:alice`、`nope/nope-q9`、`escalate`） | **5 项 warn**，一条不落，exit 0（不拦） |
| 缺 `who` | 拒收：「缺 who —— 判据要求只靠信封就能答出谁」，exit 1 |
| 空域表 / 空项目表的 vault | `scope` 两条**不报**，其余 3 条照报 |
| `policy: envelope.who.unknown = fatal`（实测：把影响域拷进临时 vault 改 `company.md`） | 变 `🔴`，**exit 1** |
| `policy: envelope.kind.unknown = off` | 那条**整条不出** |
| `anc org check --rules` | 七条 `envelope.*` 都在表里，覆盖标记 `★` 也对 |

`gofmt` / `go vet` / `go test -count=1 ./...` **十一包全绿**（envelope 包 17 条）。
路上真踩到并修掉一处：`Bind` 一开始直接 append 裸切片，绕过了 `Report.Add` 的丢弃逻辑，
**`off` 档关不掉** —— 改成走 `*org.Report` 后才真的能关（有测试锁住）。

**仍未落**（#44 的其余三项，别当做了）：入口的抽象边界（收 / 发 / 会话归属 / usage / 拒答 五件事）、
能力降级阶梯（MCP → 插件 → skills → CLI → 只读 HTTP）在客户端侧怎么发、**入口唯一性**
（一机一网关 = 一份日志 / 计量 / 审计）怎么落代码。信封本身也**还没冻结** ——
等一个真实接入（#46）来回压一遍再定。
### 7.1.14 接入面真接入：MCP（2026-10-08，CC 2.1.291 + Codex 0.160.1 真调）

#44 的验收里有一条是「网关从 A 换成 B：信封与议题都不变，只有 adapter 换」。
这一条**不能靠纸面**，所以真接了两个 harness。

**服务端**：`anc envelope serve`（`internal/mcp` + `internal/envelope`）。
极小 MCP（streamable HTTP），只做 initialize / notifications / tools/list / tools/call；
对外**只暴露一个工具** `anc_send_envelope` —— 能力面收在一处，是「入口唯一性」的第一步。

**无状态**：不发 `Mcp-Session-Id`、不记会话、GET / DELETE 一律 405（不提供 SSE 流）。
这是 #45 那条待验主张（「新的 MCP 协议是无状态的，可能更适合」）在服务端的第一次践行：
一封封自带路由信息的信，**收到就能办**，本来就不需要先握手养出一段会话。

**实测（本机真调，不是模拟）**：

| harness | 客户端报的协议版本 | 链路 | 结果 |
|---|---|---|---|
| Claude Code 2.1.291 | `2025-11-25` | initialize → notifications/initialized → tools/list → tools/call | ✅ 信封落盘，六问六答回到 harness |
| Codex 0.160.1 | `2025-06-18` | initialize → tools/list → tools/call | ✅ 同上 |
| 裸 JSON-RPC（探针） | `2025-06-18` | 同上 | ✅ 同上 |

**两家的协议版本号不一样** —— 服务端**原样回客户端报的那个**才都通。写死一个版本，
报另一个版本的客户端就走了。这是实测教出来的，不是设计出来的。

**没动全局配置**：CC 走 `--mcp-config` + `--strict-mcp-config`（只在一次运行内接线）；
Codex 走 `-c mcp_servers.anc.url=...`。两个都是「用完就走」。

**Codex 侧的一个坑**：默认审批策略 `never` 会把 MCP 工具调用拦下
（`MCP tool call requires approval, but approval policy is never`）——
要 `-c 'mcp_servers.anc.tools.<工具>.approval_mode="approve"'`。这是 harness 侧的策略，
不是 ANC 侧的问题，但接的时候必须知道。

**身份对不上的那封也真跑了一遍**（经 CC）：`who=nobody` / `on_behalf_of=member:ghost` /
`kind=escalate` → 三项 🟡 **照收**（落盘、并把发现回给递信的人），与「默认 warn、不先拦」一致。
真沙箱的 `domains.md` / `projects.md` 都是空表，所以 `scope.domain=nope` **一条都不报** ——
「门禁不许长在别人的文档上」在真接入里也被验证了。

**日志落哪**：`<data>/envelope/<YYYY-MM>.<who>.jsonl`（按 who 分片、append-only）。
放 data 目录而不是 vault（git），两个理由：① 这是**流量**不是真相，每封信进 git 就是每天刷 diff
（同 §13 Q15 的理由）；② vault 顶层目录会被 `scanRouting` 当成「数据来源」扫进 persona 路由表 ——
一封信都不该改变谁的数据来源。

**门禁与「拒收」**：默认 warn = 照收；被 `company.md` 的 `policy:` 提成 `fatal` 的 = **拒收**
（不写日志、回 `isError=true`）。依据是 `fatal` 在这库里的定义就是「阻止落盘」（`org/rules.go`）。
`TestIngressFatalRejectsAndDoesNotLog` 锁住这条，也锁住「拒收的信不落盘」。

**实测覆盖**：`gofmt` / `go vet` / `go test -count=1 ./...` **十三包全绿**
（`internal/mcp` 8 条 + 合规对拍 7 条、包外对拍 1 条、`internal/envelope` 22 条、CLI 侧 8 条）。

**仍未落**：入口的抽象边界（收 / 发 / 会话归属 / usage / 拒答 五件事，现在只落了「收」）、
能力降级阶梯（MCP 已是主路；插件 / skills / CLI 未接）、入口唯一性**只做到「一个入口一个工具」**，
还**没有**做「一机一网关 = 一份日志 / 计量 / 审计」的强制。信封仍未冻结。

**合规对拍（2026-10-09，W3 第一格）**：MCP 是**手写的**（`internal/mcp` 零第三方依赖，
`go.mod` 只有 `module anc` + `go 1.24`），没引官方 SDK。为了让这份手写实现**能被信任**，
补了两份「照规范写用例」的对拍，而不是引一个 SDK 进来把体积和依赖一起背走：

| 文件 | 钉住什么 | 条数 |
|---|---|---|
| `internal/mcp/conformance_test.go` | 协议**形状**（用假工具）：initialize 三字段齐全 · `id` 原样回 · `tools/list` 的 `inputSchema` 是 JSON Schema 对象 · `tools/call` 的 `content[]` + `isError` · 缺 `arguments` 要传给工具**空 map** · 批量请求回 `-32700` · 响应 `Content-Type` 是 `application/json` | 7 |
| `mcp_conformance_test.go`（包外，走真 HTTP） | **真工具面**：恰两个工具且顺序固定（`anc_send_envelope` / `anc_read_context`）· description 非空 · `inputSchema.type=object` · 每个参数有 `type` + `description` · `required` 必须指向真实存在的参数 | 1 |

**对拍当场抓到一个真缺陷**：`handleCall` 原来是「先 `t.Call(args)`、后补空 map」——
缺 `arguments` 时工具收到 `nil` map，工具里只要写一次 `args[...]` 就是 **nil map panic**。
空 map 现在补在**调用之前**；`TestConformanceMissingArgumentsReachToolAsEmptyMap` 钉住这条，
**变异验证**过（把顺序改回去 → 该用例立刻变红，改回来 → 绿）。

**两条是有意不支持的，钉住防止被当 bug 修掉**：① 批量请求（JSON-RPC batch）一律 `-32700`；
② 不提供 SSE 流（GET / DELETE 405）。依据是这两条都不在「官方 spec 三版共同子集」里，
两条腿的真客户端（CC 2.1.292 / Codex 0.160.1）也都没用到。

**明确没做的**：没跑官方 conformance 跑分（没有这个工具）、没对拍 OAuth / resources /
prompts / sampling / 进度通知 —— 我们只实现「无状态 + 工具调用」这个子集。
**版本跟随策略见 `../SPEC.md` §13 Q22。**

### 7.1.15 授权表：`grants/`（2026-10-08，单元 12 条 + 沙箱真跑）

SPEC §6 的授权模型（谁 / 什么动作 / 什么客体 / 期限 / 署名）落到 `grants/`。
**到这一刻为止只有落点与结构校验** —— 执行层（怎么行使、到点怎么失效）一行代码都没有。

**形状：一个 grant 一个文件**（`internal/org/grants.go`）。为什么不是「一张表一行」：

| 选 | 好处 | 代价 |
|---|---|---|
| **一个文件一条**（采纳） | 落点天然可分层（给人快速定位）；改一条只碰一个文件，谁提的谁批的能在 git 里逐条追 | 条数一多文件就多（扫目录比扫表慢，现在这个量级无所谓） |
| 一张表一行 | 一眼看全 | 表会长成一坨，`slug` 得另起一列当主键；两个人同时改同一张表 = 天天冲突 |

**扫描递归、分层不管**：目录怎么分层是给人看的（按部门 / 按项目随你），换分层不用改代码；
顺序按路径定，快照 / diff 才有确定字节。

**判据：有 frontmatter 才算条目**。落点里的说明文件（`CLAUDE.md` / `README.md`）按名字跳过；
名字不在忽略名单、又没有 frontmatter 的**报出来**（`grant.file.unparsable`）—— 本包一贯的
「宁可拒绝，不做静默猜测」。同一理由多一条 `grant.file.multiple`：正文里还有第二段 `grant.`
frontmatter 就报（第二段压根不会被读，不报 = 那条授权静默消失）。判据收得很紧
（一行正好 `---` + 紧跟的第一行非空内容以 `grant.` 开头），正文里的分隔线不会误报 —— 有测试锁住。

**只做结构校验，语义只到 `from`**：四个必填字段一个不缺（`from` `to` `action` `object`）+ `from` 解得出成员或岗位。
`to` / `object` 的客体词表（信道 #33、看板分区 #7）**还没有定义处** —— 现在校验它们只能是猜，
所以**故意留白**。「这条权该不该给」也不归校验器：那是提案 + 人确认的事。
域不能当 `from`（`domain:trade` → `grant.from.unknown`）：域是**客体**不是主体。

**十条规则出厂档：九条 `warn` + 一条 `off`**（`grant.ttl.missing` —— 2026-10-10 改口径，见下），
一条红线都不设 —— 域表 / 项目表 / 接入面立的就是这个先例，理由一样：**授权是新能力，先「看见」不先「拦」**。
⚠️ 这是**加了十个可关的开关**，不是加了十个门禁：全部允许 `off`、也全部允许上调，
要提红得自己在 `company.md` 的 `policy:` 段写（如 `grant.action.unknown = fatal`）；
反过来，想让「授权必带期限」的口径回来，写 `grant.ttl.missing = warn`（或 `fatal`）—— 规则是数据，不是删掉。
`anc org check` 另外加了一行 `授权 N 条` —— 「先看见」得先看得见。

**看板投影全量、bot 侧零条**：投影（`anc.board/v1`）带 `grants`，十条字段原样搬、不筛选不脱敏；
bot 侧那个「只出与我相关的几条」的视图**还没做**。两个消费者、两个视图，**不共用一份产物** ——
共用迟早把看板的全量漏给 bot。

**期限（2026-10-10 改口径：默认永久）**：`ttl` 由「必填、统一 30 天」改为**可选、不写 = 永久**；
表里写多少执行层就按多少算，客户按自身需求配（`30d` / `90d` …）。代价见 SPEC §6「期限与撤回」：
没有期限，「授权腐烂」就只剩**级联失效 + 撤回即时生效**两道防线。授权表里**不放 token / 不放密钥** ——
它记的是「谁被允许做什么」，凭据怎么发是执行层的事（这也是「token 明文进上下文」那个担心的正面回答：
表里根本没有 token）。

**已落（2026-10-10，甲案）**：`grants.go` 的必填表去掉 `ttl`（五 → 四）；`rules.go` 里
`grant.ttl.missing` 的出厂档从 `warn` 改成 **`off`** —— **条目留着**（`grants.go` 照样发这条发现，
只是收集时被丢掉），所以客户想要「必带期限」时是改数据、不是改代码。
下面那张 2026-10-08 的实测表里「缺 `ttl` → warn」是**旧口径下的实测**，已被本次结果取代。

**本次实测（2026-10-10，沙箱 Linux + 真二进制 `sha256` 前16 `b258fe7a3605cdfa`；脚本 `<沙箱>/ttl-check.py`，
日志 `<沙箱>/logs/2026-10-10-期限口径实测.log`）**：拷一份沙箱 vault 到 `/tmp` 上跑（不动沙箱本体），
5 组 12 条断言**全 PASS** ——

| 用例 | 结果 |
|---|---|
| 缺 `ttl`（= 不写期限 = 永久） | **零发现**、`授权 1 条`、exit 0 |
| `ttl: 30d` | 零发现，投影里 `ttl` 原样带出来 |
| `policy: grant.ttl.missing = warn` | 报出来，且 **warn 不拦**（exit 0） |
| `policy: grant.ttl.missing = fatal` | 报出来且 **exit 1** |
| `anc org check --rules` | 这一条显示 **`off`**（其余九条 `warn`），无覆盖标记 |

`gofmt` 干净 / `go vet ./...` 干净 / `go test -count=1 ./...` **十六包全绿**（`internal/org` 新增两条用例：
「不写期限 = 干净」「出厂档 off 但能开回来」）。

**实测**（2026-10-08，本机 `dist/anc.exe`，对着沙箱 vault 的临时副本 —— 不动沙箱本体）：

| 用例 | 结果 |
|---|---|
| 两条干净 grant（`member:alice → role:manager`、`role:manager → member:alice`） | `授权 2 条`，0 项 grant 发现，exit 0 |
| 没有 `grants/` 目录 | `授权 0 条`，其余输出**逐字不变**，exit 0 |
| 缺 `ttl` | `grant.ttl.missing`，exit 0（不拦） |
| `from: domain:trade`（域当授权者） | `grant.from.unknown`，exit 0 |
| `action: bless` + 非 ASCII 文件名 + 没 frontmatter 的 md | 三条一起报（`action.unknown` / `slug.format` / `file.unparsable`），exit 0 |
| 一个文件里塞两段 `grant.` frontmatter | `grant.file.multiple`，exit 0 |
| `policy: grant.ttl.missing = fatal` | 变 🔴，**exit 1**（不落盘） |
| `policy: grant.action.unknown = off` | 那条**整条不出** |
| `anc org check --rules` | 十条 `grant.*` 全在表里，覆盖标记 `★` 也对 |
| `anc org export` | `grants` 段 2 条、十条字段齐全；`grants/` **不在** `routing` 里 |

`gofmt` / `go vet` / `go test -count=1 ./...` **十二包全绿**（`internal/org` 新增 12 条用例，
其中 `from` 那条跑 7 组子例）。

**仍未落**（别当做了）：
- **执行层只落了第一刀**（2026-10-10，W4：跨域信封「无匹配即拒」已接进接入面，见 §7.1.32）——
  **仍为零**：期限到点怎么失效（**已拍挂起**，见 §7.1.32）、怎么级联、能不能续期；
- bot 侧的按人过滤视图、看板上「谁能做什么」那个页（投影里已经有数据，页上还没画）。
  **2026-10-15 补**：看板那一页已画（§7.1.53「授权」页）—— 全量逐条搬 + 按主体换视角；
  **bot 侧那个「只出与我相关的几条」的视图仍未做**（两个消费者、两个视图）—— 已单开 **议题 #68** 钉住。
顺带记一条同日拍板：**信道的默认值是「不通」**（跨域默认不通，要通就写一条 grant）——
但信道本身是 #33，还没实现。

### 7.1.16 凭据键名的账 + 「名 → 键」的全函数派生（2026-10-08：单元用例 + 真二进制 dry-run + 真上游 cc-connect 各一次）

**症状（历史，审计报告 P0-4 的现场）**：成员名不是 ASCII 时 —— 比如 `张三` —— 渲染器照常写出
`app_secret = "${ANC_FEISHU_SECRET_张三}"`，但体检是**拿正则回读产物文本**算「要哪些键」，那把正则
只认 `[A-Za-z_][A-Za-z0-9_]*`，中文键它根本看不见。于是体检拿着「少数了一个键」的清单报 **✅ 齐**，
现场等来的是一个没有凭据、起不来的 bot。带 `-` 的名（`alice-2`）同样中招。

**修法一：键名当场记账**（`Plan.SecretKeys`，同日先落）。键名在写 `app_secret` 那一行当场记下，
体检吃这份账、不再回读文本。键名的写法只有一处：`render.FeishuSecretKey`。

**修法二：把「名 → 键」变成全函数**（同日改；人拍板「中文成员名是合法输入」）。判据只有一条：键名要
能写进环境（装载侧只认 `[A-Za-z_][A-Za-z0-9_]*`），而成员名是**人的名字** —— 中文、空格、`-`、`.`
都合法。所以：纯 ASCII 字母数字下划线 → 直接大写（**存量键一个字节都不变**）；出现别的字符 → 整个
名字按 rune 转义（能直通的直通并大写，其余写 `x` + 定长 6 位小写十六进制）。

两段**按构造不相交**：直通段只产大写 `[A-Z0-9_]`，转义段必含小写 `x` —— 不同的名撞不成同一个键。
定长转义还挡掉了「白 + 5」与单字 U+767D5 这类歧义。实测四个字面：

| 成员名 | 键名 |
|---|---|
| `alice` | `ANC_FEISHU_SECRET_ALICE`（存量不变） |
| `alice-2` | `ANC_FEISHU_SECRET_ALICEx00002d2` |
| `张三` | `ANC_FEISHU_SECRET_x005f20x004e09` |
| `王小明` | `ANC_FEISHU_SECRET_x00738bx005c0fx00660e` |

**规则 `member.name.format` 的判据跟着改**（同一个 id、红档、不锁死）：它不再量「名字是不是 ASCII」，
只量**事实** —— 这个名字会落成 `homes/<名>`、`members/<名>/` 与 project 名 `<公司 id>-<名>`，三平台
（Windows / Linux / macOS）都得建得出来。拦的是 `/`、`\`、Windows 建不出目录的字符（`: * ? " < > |`）、
控制字符、首尾空白、结尾 `.`，以及等于 `.` / `..`。中文 / 空格 / `-` / `.` 一律放行。
判据实现是 `org.MemberNameProblem`（导出的，规则与别处共用同一把尺子）。

装载侧那条「键名写不出来」的说明留着，但它在正常渲染里**已经不可达** —— 走到那里只可能是产物被手改过、
或派生逻辑坏了；所以那条 FIX 从「改名」改成「别手改产物，重跑 `anc render` / `anc apply --apply`」。

| 场景 | 命令 | 结果（2026-10-08 实测） |
|---|---|---|
| 全 ASCII 名（基线） | `anc apply <vault>`（dry-run） | `引用 2 个：…_ALICE, …_DEVBOT` + ✅ 齐，exit 0 |
| 派生字面（7 个名字，含中文 / `-` / `.`） | `internal/render` 用例 | 逐个字面比对；每个键都过装载侧那把尺子（`apply.LegalKey`）；9 个名字互不撞键 |
| 中文名拿得到键 | `internal/render` 用例 | 账与回读**都**看得见 `ANC_FEISHU_SECRET_x005f20x004e09`（回读看不见 = 当年那个静默形状） |
| 中文名 + 凭据文件里有那个键 | `anc apply <vault>`（dry-run） | [1/7] 校验过 → [2/7] `project 1 个：demo-王小明` → [3/7] ✅ 齐 → [7/7] exit 0 |
| 中文名 + 凭据文件里**没有**那个键 | 同上 | 停在 [3/7]：`🔴 缺键 / 空值键：ANC_FEISHU_SECRET_x00767d…` + 补键 FIX，exit 1（**不是**「写不出来」那一档） |
| 名字含 `/` `:` 或以 `.` 结尾 | `anc org check <vault>` | [1/7] 拦：`member.name.format … 含 / 或反斜杠 …`，exit 1 |
| 上游收不收中文 project 名 / 中文 work_dir | 真 `cc-connect v1.3.4 --config <anc 渲染产物>`（假 app_id，跑 12 s 后收掉） | `config loaded` → `platform ready project=demo-王小明` → **`engine started project=demo-王小明 agent=claudecode`**；`homes/王小明` 由 `anc render` 建出。唯一的错在授权那一跳：`app_id is invalid`（假 app，预期） |

`gofmt` / `go vet` / `go test -count=1 ./...` **十二包全绿**（`internal/render` 3 条、`internal/org` 3 条、
根包 2 条新用例）。

**残留（已知，没修）**：名只差大小写（`alice` / `ALICE`）会撞成同一个键 —— 存量键就是大写折叠出来的，
要修得动存量 vault；且 Windows 上这种重名连目录都建不出来。用例 `TestSecretKeyCaseFoldingResidual`
把它钉成已知事实：哪天改了，用例就红，逼着同步这一节。

**未验**：① 真凭据下（真飞书 app）中文名 bot 的对话闭环 —— 本机只有一个飞书应用，这一步要第二个 app；
② Linux 腿 systemd / Windows 腿计划任务**载体**里的中文路径（drop-in 与 schtasks 的参数编码）。
两条都不当「已实现」讲。

**没做**：成员名与 `display_name` 的彻底分离（成员 id 用 ASCII slug、中文只住 `display_name`）——
那是 schema 变更，要人拍板；本轮的取向是「中文名直接用，且不许静默」。
### 7.1.17 交付链路的门：`anc gate`（2026-10-08，单元 14 条 + 真二进制 + 真留痕）

**动机**：本项目的资产一直是两份、分居两处 —— 《ANC架构设计大框》§8 的落地链路
（诊断 → GO/HOLD/NO-GO → 装机 → 验证）与 SPEC §8 三个阶段门在**文档侧**，
`trail` / `judge` / `timeline` 三层的留痕在**代码侧**。judge 的设计（判据外置成数据、
来源 + 证据必填、不设门禁只出结论）本来就是那些门的宿主 —— 这次的活是**把两份接上**。

**为什么不是「给 judge 加几条规则」**：judge 原本的事实层是 `trail.Session`
（turns / denied / failed / failures / subagents / unreadable），**门要的证据不在会话记录里**：
「一线当场说这能帮到我」「员工不靠问人就能拿到上下文」是人记下来的，落在 `timeline/`。

**所以做的是一个引擎、两个作用域**：`session` / `turn` 的事实来自 harness 记录；
新增的 `delivery` 的事实来自**真相源仓库** —— `org`（人 / 岗 / 域 / 项目 / 授权）
+ `timeline`（留痕）。判据、schema、`--rules` 整份替换、「只出结论不设门禁」这套口径完全共用。

**门的名单与判据都在数据里**（`internal/judge/delivery.go` 的 `gates` + `BuiltinDelivery`）：

| slug | 门 | 判据（写进判据数据，也是该往 timeline 里记什么） | 出处 |
|---|---|---|---|
| `diagnosis` | 诊断门 | 企业 AI 全景图 + 用真实资料做的 Demo + 组织/路线建议，结论 GO / HOLD / NO-GO | 《大框》§8.2 |
| `proof` | 证明门 | 一线当场说『这能帮到我』+ 停止条件双方认可 | SPEC §8 一阶 |
| `deploy` | 装机门 | 把真实工作流正式接入（目标/资料/任务/权限/责任/决策/结果） | 《大框》§8.2 |
| `settle` | 沉淀门 | 员工不靠问人就能拿到完成任务所需的上下文、权限、工具 | SPEC §8 二阶 |
| `rebuild` | 重构门 | 在没增加中层的前提下，产能扩了 | SPEC §8 三阶 |

**一道门就是一个 timeline case**（`gate:<slug>`），所以「门过没过」与看板时间线页
说的是同一件事、用的同一套配色（绿 = 通过 / 红 = 卡点 / 黄 = 在跑 / 灰 = 认不出的词，形状口径见 §7.1.18）。
出厂判据 = 五门 × 两条（**没落痕** / **有卡点**）+ 一条全局缺勤 = 11 条。

**刻意只判这两类**：「一线那句话算不算数」「产能到底扩没扩」要人判 —— 判据只负责把
「该判的没落痕」摆到人面前，不替人下结论（同 SPEC §13 Q17）。加一道门 = 加两行数据。

**顺手修掉一处自家教条**：引擎里原本写死 `r.ID != "session-unreadable"` 才允许「无证据」——
违反「引擎不认识任何一条具体判据」。现在改成判据自己的字段 `NoEvidenceOk`（数据），
`session-unreadable` 与交付层那几条缺勤判据都靠它，引擎里一个具体 id 都不剩。

**口径校验（都是拒，不是忽略）**：写错作用域的指标、按门统计的指标不写 case、
全局指标写了 case、一条判据里出现两个 case、`say` 里用了按门统计的指标却没写 case —— 全部拒绝。
理由同原有的那条：**一条永远不会命中的判据，比一条写错的判据更难发现**。

| 场景 | 命令 | 结果（2026-10-08 实测） |
|---|---|---|
| 空 timeline | `anc gate <沙箱 vault>` | 6 条结论：全局缺勤 + 五门的「没落痕」各一条；exit 0 |
| 真留痕：`timeline add … -case gate:proof -status done` | `anc gate <vault>` | 证明门 `● 绿 1 / 黄 0 / 红 0`；该门「没落痕」不再报；其余四门照报 |
| 再加一条 `-status blocked` | 同上 | 证明门 `● 绿 1 / 黄 0 / 红 1`，报 `delivery-proof-stuck`，**证据只列那一条红档**（绿档不混进来）；exit 0 |
| `--show-rules` | 同上 | 11 条生效判据逐条列出（含 case 与豁免标记） |
| `--rules <坏文件>` / 跨作用域指标 | `internal/judge` 用例 | 一律拒（含「一条判据两个 case」） |

`gofmt` / `go vet` / `go test -count=1 ./...` **十二包全绿**（`internal/judge` 新增 9 条交付层用例）。

**仍未落**（别当做了）：看板上的「门」页（现在只有 CLI 与 JSON 出口）、
`gate` 的自动写入（谁在什么条件下往 timeline 记一条门留痕 —— 那是业务约定，不是代码门禁）、
以及 §8 那三个阶段门**更细的判据**（现在每门只有「没落痕 / 有卡点」两类）。

### 7.1.18 看板状态灯：形状 + 颜色双编码（2026-10-08，真浏览器逐档读计算样式 + 整页截图）

**动机**：时间线页的四档（绿 = 已完成 / 黄 = 运行中 / 红 = 卡点或失败 / 灰 = 认不出的词）原先一律是
8px 实心圆点，只靠色相区分。在浅色玻璃底上，绿与灰、黄与红挨着看**分不开**；而且「只靠颜色」
对色盲读者、灰度截图、小屏都不成立 —— 最要命的一种误读是**把灰看成绿**：「没人判过」被当成「过了」，
正好是这套配色存在的理由的反面。

**改法**（`internal/board/ui/index.html` 的 `.dot`，一处定义、全站生效）：

| 档 | 形状 | 颜色 | 读作 |
|---|---|---|---|
| `ok` | 实心圆 + 细白描边 + 2.5px 光环 | 绿 `#12a150` | 已完成 / 通过 |
| `warn` | 圆 + **4px 光环**（那圈光晕就是「在动」） | 琥珀 `#e07b0a` | 运行中 |
| `bad` | **菱形**（10px 方块转 45°，形状与另外三档最不一样） | 红 `#d92d20` | 卡点 / 失败 |
| `gray` | **空心虚线圈**（无填充） | 灰虚线 `#8b9aab` | 认不出的词，**没人判过** |

灰色刻意与绿色长得不像 —— 这是这一改唯一的硬约束。尺寸 8px → 10px，细白描边是为了在浅色玻璃底上
把形状咬住。形状与颜色的**含义仍只有一处口径**（`internal/timeline` 的 `Band`）：界面只是把
`green` / `yellow` / `red` / `unknown` 画出来，加词依旧只改数据。

时间线页的「三档」区顺手补了一条**图例**（`boardui/src/app.ts` 的 `bandLegend()`），并且四张统计卡片的
标签边上带上了**同一套**灯 —— 图例跟表格里读到的是同一个东西，不另画一套「好看的图例」。

**实测**（2026-10-08）：`npm run check`（tsc + esbuild）过；`gofmt` / `go vet` / `go test -count=1 ./...`
**十二包全绿**；新二进制起 `anc board serve` 后，真浏览器（Chromium，1280×900）打开 `#/timeline`：
`getComputedStyle` 逐档核对 —— ok / warn 为 10×10 圆配 2.5px / 4px 光环、bad 为 `matrix(0.707107, …)`
（45° 旋转）、gray 为 `dashed` 边框 + 透明填充，四者两两不同；整页截图确认**图例、统计卡片、表格行、
折叠头**四处形态一致。

### 7.1.19 行使的流水：`anc audit`（2026-10-08，真沙箱 + 看板审计页）

**问题**：§7.1.12 那一层是人和 agent **自己写下来的**（卡在哪、谁拍的板）。还缺一层：
**谁在什么时候、对谁、行使了什么** —— 含**被拦下的**和**失败的**。这层不能靠自述：
自己写的账簿答不了「有没有人试过越权」。所以在时间线页后面加了审计页（`#/audit`），答「谁碰了什么」。

**为什么不在行使路径上记账**：SPEC §6 的纪律是「能放在 harness / 平台的权限规则里拦，就不要放在业务代码里拦」。
于是行使路径上一个字都不加，流水从两条腿来：

- **事后归集**（`anc audit collect`）：从 harness **自己的记录**里抽行使（`tool_use` 与结果配对，
  流式中间态后到的覆盖先到的），默认 dry-run、`--write` 才落盘；
- **显式补记**（`anc audit add`）：归集抽不出来的那几类（没登记的工具、抽不出目标的工具、vault 外发生的事）
  由人或 agent 补一条。

**一条必须写在明面上的边界**：「**本可以行使但没行使**」**永远不可能自动归集** —— 它没有发生，就没有记录；
只有补记的那一类答得了。页内 `missing` 里如实列着这条。

**四条口径**：

| 口径 | 落法 |
|---|---|
| 真相源 = `audit/<YYYY-MM>.<行使者>.jsonl` | 按**行使者**分片 + **append-only**：同一秒两个 agent 写也碰不到同一个文件；一条记录＝一行，可 diff / review / 回滚 |
| 坏行不许静默跳过 | 逐行读，坏行进 `Bad`（只报 `文件名:行号`，不带本机路径）|
| 结果四档，词表不锁死 | `ok` / `denied` / `failed` / `unknown`；**`denied` 与 `failed` 是两件事**（「权没给」≠「做了没成」），配色与文案都分开 |
| **不存派生值** | 流水存当时的事实（`object` 原样）；「在哪个域、算不算跨域」按**当前**域表算 —— 域表改了，历史记录跟着重判，而不是留一堆变假的旧结论 |

**落点分四类**（`Zone`）：域内 / 跨域 / 域外 / vault 内非域。第 3、4 类分开是有实测理由的：
demo 会话的 27 次行使里，11 条落在「vault 内非域」、11 条落在「域外」（前者如 `vault/CLAUDE.md`、
`vault/.claude/…`，后者如 `.claude/settings.json`），那是两条不同的信号 —— 一条是「手伸到 vault 外面了」，一条是「在库里但不在任何业务域」。

**只记不拦**：不设门禁；不自己发明一套读权限（读权限归 #32/#34，留存期归 #27/#25）；
`add` 不校验「谁能写」，也不猜 `on_behalf_of`（那要渲染侧的 `inputs` 才算得准，没拍板就留空）。

**看板不摊本机布局**：`object` 归一（vault 内 → `<vault>/…`；vault 外 → 只留末两段），
`why` / `detail` **一个字段都不出门** —— 自由文本必然夹绝对路径（实测泄露 12 处），scrub 做不干净，
只留 `has_why` 标记。

**实跑过什么（照实记）**：

- Go 侧：`go test -count=1 ./...` **十三包全绿**（新增 `internal/audit`，外加 `internal/board` 的审计用例）。
  用例盯的是真踩过的坑：坏行不许静默跳过、自由文本不许出门、`add --at` 给错是**记录字段**（退出码 1）
  而 `--limit` 给错是查询参数（退出码 2）。`collect` 有「记不到的」时退 **1**（0 = 干净 / 1 = 有记录不全
  或未归集 / 2 = 参数错）—— 这是**报告口径，不是门禁**：该落盘的照样落盘。
- 真会话归集：`anc audit collect .\anc-demo\vault` → **4 分片 / 27 次行使**，四档 `ok 20 / denied 5 / failed 2`；
  落点「vault 内非域 11 · 域外 11 · 目标抽不出 5 · 跨域 0」；3 类「记不到的」（合计 5 次：
  `Agent` 1 / `Bash` 3 是记录里没有目标键，`Glob` 1 是有键但记录里空的）—— 抽不出来就如实报，不当成「没发生」。
- 验收判据 ①②（沙箱）：`audit add` 一条跨域读取（alice → `20-ops`，域 ops）+ 一条被拒的写入 →
  `audit log` 打出「行使者 / 动作 / 域 ⚠跨域 / 结果 / 原文」，`--cross` 两次筛出这两条。
- 验收判据 ③ 可 diff：`collect --write` 首次 27 条、第二次 **0 条**（幂等）；一条记录就是文件里的一行，
  旧行永不被改。
- 验收判据 ④：时间线页那句「不是审计」改写成指向审计页；审计页自己也列缺（vault 外的行使只报落点、
  真凭据下的归集没验、「本可以行使但没行使」永远归集不到）。
- 顺手抓到一个真泄露：`audit/` 会漏进 persona 的路由表 → 进 `skipDirs`
  （`anc org check` 改前 `路由 10-knowledge, 20-ops, audit`，改后 `…20-ops`）。
- 看板：`/api/audit`（`anc.audit/v1`）—— demo vault 归集出 **27 条**（`ok 20 / denied 5 / failed 2`，
  落点 `域内 0 / 跨域 0 / 域外 11 / vault 内非域 11 / 判不出 5`），沙箱里再加两条补记 = 29 条；
  `object` 形如 `<vault>/20-ops/CLAUDE.md`、`…/.claude/settings.json`（vault 外只留末两段）。
  `--dump-dom "#/audit"` 断言这些真数据都在页面上，且**无本机用户名泄露**（`dev` 一次都不出现）；
  同一页在真浏览器里也逐块核过（四档卡片 / 落点卡片 / 流水表 / 「被拦下的与失败的」/「这一页答不了什么」）。
- **没验的**：真凭据下的归集闭环；非 claude 的 harness（工具表是数据，加条目即可 —— **但没实测**）；
  `on_behalf_of` 的填充；审计的**归档 / 留存期**（#27 / #25）与**读权限**（#32 / #34）；
  多 bot 并发下的归集。

### 7.1.20 探针第四档：灰（2026-10-09，单元 6 条 + 真 VM 实测）

**要解决的问题**：SPEC 要求一个公司**恰好有一个启用中的 devbot**（不可豁免），而只有一个真飞书 app 的
客户必然有几个 bot 暂时没有真 app。这些 bot 没会话、探针判 🟡「没观测到任何会话」—— 于是
「**已接凭据的都绿了**」这个事实被一颗假黄盖住，整份报告都变得不值得信。

**口径（拍板 2026-10-09，口径 A）**：第四档 `unwired`（灰）。判据走**数据** ——
真相源里成员声明 `unwired: true`。**不按 `app_id` 的长相猜、不设白名单**（`cli_sandbox_*` 这种
占位命名今天像、明天不一定像；声明是客户能自己维护的那一层）。灰**不挡绿**：
`AllGreen()` 跳过灰档 —— 绿的口径是「已接凭据的都绿了」，不是「所有 bot 都绿了」。

**三条不肯省的设计**：

1. **灰先在「gateway 挂没挂」之前判**。没接凭据的 bot 本来就不该回话，拿它去凑红/黄都是噪声 ——
   网关不在跑时它照样是灰（单元用例 `TestRunUnwiredStaysGrayWhenGatewayDown` 钉住）。
2. **灰档自己的会话文件不算「残留」**。残留是「删过 bot 却留着记录」的签名，与「没接凭据」不是一回事；
   `Run` 把已判过的 project 从 `byProject` 里删掉，两类不会互相污染。
3. **空集不判绿**。一个绿的都没有时（全被声明未接、或 config 里压根没有 project），
   「已接凭据的都绿了」是**空真** —— 报绿就是最纯的那种假绿：什么都没验过。第一版 `AllGreen()` 在这里
   返回了 true，是**真机反向用例**（把三个 bot 全声明成 `unwired`）逼出来的，现在要求**至少一个 🟢**；
   CLI 会额外说明「这不是全绿」。同「没消息 = 没事」是反模式。
4. **四个消费方共用同一支算**（`render.UnwiredProjects`）。这是本轮**真机实测抓到的真 bug**：
   第一版只改了 `anc probe`，`anc apply` 第 7 步的回读仍是 🟡 —— 同一条链路上两个地方互相打脸。
   现在 `anc probe` / `anc apply` 回读 / 看板 `/api/runtime` / `anc notify` 全走那一支，
   停用的成员一律不算（停用的本来就不进 config，多报一个只是噪声）。

**回显**：CLI 与看板都用 ⚪；另有 `Tally()` 单独数一行 ——
「已接凭据的 N 个：x 绿 / y 黄 / z 红；另 w 个声明了未接凭据」。**分开数是有意的**：
一句「3/3 绿」会把「没接凭据」糊进绿里。看板 `RUN_STATE` 加第四档 `dot gray`（空心虚线圈，
与「判不出」同一形状语言，见 §7.1.18）。

**实测（真 VM，Ubuntu 24.04，2026-10-09）**：

| 验的 | 怎么验 | 结果 |
|---|---|---|
| 声明生效 | 给 `~/anc-vm2/vault` 的 bob / devbot persona 加 `unwired: true` | `anc org check` 零红（`unwired` 是**已知键**，不是被忽略的野字段） |
| 渲染不受影响 | `anc render --check` → `--apply` | `inputs` 变（v6 指纹把 `unwired` 算进去），project 仍是 3 个 |
| **灰不挡绿** | `anc probe <vault>` | alice 🟢 / bob ⚪ / devbot ⚪，**退出码 0** —— W1 门当场过 |
| 回读与 CLI 一致 | `anc apply --apply` 第 7 步 | 同一份三行 + 同一句话（修掉上面那个 bug 之后） |
| 告警不狼来了 | `anc notify`（dry-run） | 「没有要喊的（红 0 / 黄 0）」—— 未接凭据不该被喊 |
| 契约 | `anc probe --json` | `"state": "unwired"`（`anc.runtime/v1` 加了第四个取值） |
| **空集不判绿**（反向用例） | 把三个 bot **全**声明成 `unwired` 再探一次 | 三行全 ⚪ + 明确写出「一个绿的都没有 —— 这不是全绿」，**退出码 1**；还原 alice 的 persona 后回到 1 绿 / 2 灰 / **rc=0** |

**用例（7 条，全是新加的）**：`internal/probe` 4 条（第四档落档且不影响别人 / 灰档会话文件不是残留 /
计数口径 / 网关不在跑时仍是灰 / `AllGreen` 跳过灰 / **空集不判绿**）、`internal/render` 1 条（停用成员不算）、
`internal/board` 1 条（看板与 CLI 同口径）、`runtime` 端到端 1 条（fixture 声明 → `cmdProbe` 退出码 0）。
**变异验证**：把 `AllGreen` 里那行 `continue` 删掉 → `internal/probe` 与端到端两条**立刻变红**。

**没做（明确标出）**：探针盲区 ②「回的是错误的话」仍不算绿也仍抓不到 —— 本轮只做灰档，那条**未做**（不编一个不存在的事故档）。
**教训**：写命令行长路径的用例时，`t.TempDir()` 会带上测试函数名，Windows 的 AF_UNIX `sun_path`
上限 108 字节一顶爆，用例**静默变 skip**（看起来是绿的）—— 所以那两条用例改用短的 `os.MkdirTemp`
根目录，且 `listenSock` 失败时 `t.Fatalf` 而不是 `t.Skipf`。
### 7.1.21 桥的另一半：出站读出口（2026-10-09，W2）

**要解决的问题**：桥之前只有**入站** —— agent 可以把信封递给 ANC，但**要不到**东西。
而 North Star 第二问（「员工不必靠领导协调就能拿到 Context / 数据 / 工具」）缺的正是这一腿。
渲染期过滤管的是**推**（开工时那份已经是过滤好的），出站读出口管的是**主动要**。

**工具**：`anc_read_context(who, on_behalf_of?, domain?)` —— 挂在**同一个接入面**上
（`anc envelope serve` 的 MCP 服务端，工具名前缀同族）。**只读**：一个字都不写回真相源。

**四条复用的纪律（一件都没新造）**：

1. **可见范围 = `render.VisibleDomains`**。这是本轮最重要的一条：persona 段 8、域表体检
   （`lintDomainCells`）、出站读出口**三处共用同一把尺子** ——「各算各的迟早对不上号」是本仓库
   反复付过学费的（§7.1.20 那条真机 bug 就是同一类）。自己的域全行，别人的域只给名称 / 是什么 / 找谁。
2. **身份 = `envelope.Resolves`**（这一轮把它从 `resolves` 导出）。解不出**不是格式错，是这个人不存在** ——
   envelope 包注释第 1 条的原话。所以拒话里不许出现「参数」「格式」「schema」这类词：
   那会把人引去改参数，而问题是他不在这家公司。
3. **留痕 = `audit.Append`**。审计包自己写着「出站动作由人 / agent **显式补记**」——
   这里就是那条出站动作的自动落点：`Actor / Action=read / Object=要的域 / Result=ok|denied /
   Why=拒的理由 / Tool=anc_read_context / Source=serve`。**给了也记** —— 审计不是只记拒绝。
4. **回话是结构化 JSON，字段就是契约**。这不只是好看：它是「出站不泄漏本机路径与自由文本」的
   **结构性**保证 —— 只回契约字段，指针一律 **vault 相对**（`domains.md` + 行号）。

**三种拒，都是「没办成」而不是「工具坏了」**（`isError=true`，都留痕）：
无身份 / 解不出（→「我们公司没有这个人」）、`on_behalf_of` 解不出、要的是**别人的域**
（→「不在你的可见范围」+「找谁」）。跨域**拒但指路**是有意的：只拒不指路，agent 会转头去问人，
那正是这条腿要消掉的那一步。

**请求里的自由文本进痕之前先过 `safeToken`**（去控制字符、压成单行、封顶 64 字）——
痕不是给人塞私货的地方，更不是给注入留的一条通道。

**实测（真 VM，Ubuntu 24.04，2026-10-09 —— 计划格位是 10-16 → 10-22，门过在格位之前（看门不看日历）；二进制 sha256(前16)=`4cf69984d41aa4d9`）**：
在 `~/anc-vm2/vault-w2`（从活库拷的副本 + 一张两行的 `domains.md`：`knowledge`→`10-knowledge`（manager）、
`ops`→`20-ops`（devbot）；alice 划到 `knowledge`）上，用**部署上去的那个二进制**起接入面，再真调：

| 验的 | 怎么验 | 结果 |
|---|---|---|
| 两个工具都在 | `tools/list` | `anc_send_envelope` + `anc_read_context` |
| **一次调用答出四问** | `who=alice` | `what=把做过的活沉成可复用的资产` · `data=10-knowledge` · `who=经理（Alice、Bob）` · `evidence={domains.md,5}`；另附别域目录（`ops`，**没有 data**） |
| 跨域被拒 + 指路 | `who=alice, domain=ops` | `isError=true`；`refused.ask=开发运维（Devbot）`；回话里没有 `20-ops` |
| 拒的应答不带 `domains` 键 | 同上 | 契约形状干净（不是 `"domains":null`） |
| 无身份 = 这个人不存在 | `who=hacker` | 「我们公司没有这个人」—— 整段没有「参数 / 格式 / schema」字样 |
| 域表只有表头（存量 vault） | 在**活库** `~/anc-vm2/vault` 上再跑一遍 | `note=还没给你划域…`；**没有因为没划域而报错** |
| **留痕** | 查 `vault-w2/audit/*.jsonl` | 每条调用一行：`actor / action=read / object / result=ok·denied / why / tool / source=serve`；**给了的那次也记了**；无身份的记在 `hacker` / `unknown` 账上 |
| **只读** | 调之前之后各算一次「除 `audit/` 外全树 sha256」 | 两次**逐字节相同**（`8da76e95e6c5ba…`）—— 除了那条痕，vault 一个字节都没动 |
| **真 agent 调得到**（W2 10-21 那格） | Claude Code **2.1.292** 带 `--mcp-config`（http）+ `--strict-mcp-config` 真调 | `initialize`(2025-11-25) → `notifications/initialized` → `tools/list`(2 个) → `tools/call anc_read_context` → 用人话答出四件事（业务名 / 一句话 / 数据目录 / 找谁），**只调了一次** |

> **夹带发现（不属本轮，记下）**：那条 harness 的模型名 `deepseek-flash` 不被 Claude Code 2.1.292
> 的模型表认识，CLI 会打一条 `[claude-code:unrecognized_model]` 警告并**按 200k 上下文算 auto-compact**
> （与清单一里的 `CLAUDE_CODE_MAX_CONTEXT_TOKENS` 是同一件事，**待拍板才动**）。
**用例（10 条，`runtime/envelope_read_test.go`，全部走真 HTTP + 真 JSON-RPC，不是直接调函数）**：
`tools/list` 里两个工具都在 / 一次调用答出「是什么 · 数据在哪 · 找谁 · 证据指针」/ 只问自己那一行 /
跨域被拒且指路且**回话里没有别人的「数据在哪」**/ 域表里没有的域（不编「找谁」）/ 解不出的身份
（且**不许**出现「参数 / 格式 / schema」字样）/ 压根没给身份（痕记在 `unknown`）/
`on_behalf_of` 解得开与解不开 / 四种请求**都不泄漏 vault 绝对路径与盘符** / 自由文本被压成单行。
另有 `internal/render/domains_test.go` 3 条钉住那把尺子（自己域全行、没划域、没有域表）。

**变异验证**：① 把「跨域被拒」那行 `auditRead` 删掉 → 3 条用例变红；
② 把 `safeToken` 的两层清洗（控制字符 → 空格、`strings.Fields` 压行）**同时**去掉 →
`ScrubsFreeTextIntoTrail` 变红（单去掉任一层不红：两层互为兜底，这本身是个好性质）。

**没做（明确标出）**：可见范围来自**成员所属域**，**不是 grant** —— 跨域 grant（#32）的执行层
还没落，所以跨域一律拒。等执行层落下，这条出口是照 grant 开口子还是永远只给「找谁」，**待拍板**（SPEC §13 Q19）。
另：`anc probe` 之类的**运行态**观测还没接进出站读出口；看板也还没有「谁问过什么」的那一页
（痕已经在 audit 里了，看板审计页能看到，但没有按「出站」单列的视图）。
### 7.1.22 网关抽象第一刀：形态收成一张表（2026-10-09，W3 第二格）

SPEC §4.7 ① 说「换一个网关是加一张表 + 一个适配器」，但 2026-10-09 之前
**代码里没有那张表** —— 渲染器直接生成 cc-connect 形态，`apply` / `probe` / `trail` / `notify`
各自把 `cc-connect` 这个名字、这套路径、这几个枚举写在自己那一份里。这一刀就是把它们收到一处。

**落点**：`internal/gateway`（零第三方依赖，只 import `path/filepath`）。

**形状为什么是「表」不是「接口方法集」**：接口方法集要为两个实现服务，一个实现抽出来的
是**猜测**（这正是 SPEC §13 Q21 原本的顾虑）。而 §4.7 早就把「换网关」定义成
「加一张表 + 一个适配器」—— 所以表才是那个不变量。表里只装**形态**（怎么装、日志落哪、
会话落哪、认哪些枚举），不装行为（跑命令、读文件留在调用方）；这样表可以整份被测试比。

**它装了哪四条（= §4.7 ① 的四条不变量）**：

| 不变量 | 表里的字段 | 原来写死在哪 |
|---|---|---|
| G1 声明式全量配置 | `ConfigFile` / `DisplayModes` / `DefaultDisplay` / `RelaySection` | `render` 的产物名、`org` 的 `[display].mode` 清单 |
| G2 凭据只从进程环境解析 | `SecretKeyPrefix` | 凭据桥的键名空间 |
| G3 装 / 重启 / 日志落点 | `BinNames` / `BinSubdir` / `HomeDirName` / `DaemonManifestName` / `LogSubdir` / `LogFileName` / `ServiceName` / `InstallCmd` / `RestartCmd` | `apply.go` 的 8 处路径与两条子命令、`startup.go` 的计划任务名 |
| G4 会话落盘可读 | `SocketRel` / `SessionsRel` | `probe.go` 的 socket 与 sessions 两处、`trail.go` 的 sessions、`notify.go` 的 socket |

**消费方改造（六个，行为零变化）**：`apply.go`（10 处）/ `internal/probe`（2 处）/
`trail.go`（1 处）/ `notify.go`（1 处）/ `startup.go`（1 处）/ `internal/org`（呈现档清单与判据）。

**判据不是「看着对」，是钉子**：
- `internal/gateway/gateway_test.go`（6 条）：表里**没有空字段**（漏一个调用方就会退回自己拼一份 ——
  那正是这次要消灭的）/ 出厂默认档必须在合法清单里 / 六条落点路径拼对 / 候选二进制的平台顺序 /
  `{config}` 占位被替换且**返回的不是表里那片切片**（调用方改不到真相源）/ 枚举判据。
- `internal/org/display_test.go` 增 1 条：`org.DisplayModes` 必须逐项等于 `gateway.CCConnect.DisplayModes`，
  且 `DisplayQuiet == DefaultDisplay` —— 两边**不许各写一份**，漂了先红。

**实测**：`gofmt` 空 · `go vet` rc=0 · `go test -count=1 ./...` **十四包全绿**（新增 `internal/gateway`）。
**这一刀的性质**：纯搬运 + 钉子，**没有改任何行为**，也没加任何门禁 —— 表里没有「开关」，只有落点。

**没做的（第二刀，现在就写清免得变成隐形债）**：
- **G1 的产物正文没抽**：`config.toml` 的每个段怎么写（`[[projects]]` / `[relay]` / `[display]`）
  仍在 `internal/render/config.go` 的 `Build` 里。也就是说「形态表」有了，**「适配器」还没有** ——
  第二个网关要在 `render` 里加分支，而不是加一个 provider。这一步要动渲染产物、风险高，单开一刀。
- **没有配置面**：不存在「选哪家网关」的配置项或开关（那本身就是门禁 + 新配置面，
  没到真接第二家之前不加）。现在表里只有一行 `CCConnect`，由代码直接引用。
- **两家的 harness 腿仍各写一份**（§4.7 ② 的 H3 第二条腿未实测，见 §7 清单）。

### 7.1.23 工作上下文并进同一次调用（2026-10-09，W3）

**要解决的问题**：§7.1.21 的出站读出口一次调用答了四问（身份 / 业务域 / 数据源 / 可见范围），
但「我这活该怎么干、在跟哪个项目」这两问还得有人再讲一遍 —— 那正是 #53 要省掉的那一步。
所以把**工作上下文**并进**同一次** `anc_read_context`：不是新工具、不是新参数、也不是新文件格式。

**五个字段 = SPEC §4.7 ③ 那份清单**（身份 / 业务域 / 数据源 / 可见范围 / 技能来源），
这一轮把后两样从「只在 persona 里」提到**回话的契约字段**上：

| 字段 | 内容 | 真相源 |
|---|---|---|
| `skills[]` | `{name, where, missing}` —— 技能从哪来 | 清单 = `roles/<role>/persona.md` 的 `skills:`；正文 = `skills/<name>/SKILL.md`（`where` 是 **vault 相对**指针） |
| `projects[]` | `{slug, name, owner, period, source, evidence}` —— 在跟哪个项目（**第六问**） | `projects.md`；`owner` 走与域行同一个 `WhoLabel`（岗位 → 人名）；`evidence` 指回行号 |
| `gaps[]` | 这份上下文**答不了什么**（报缺，不编） | — |

**报缺的口径**（`gaps` 只在要「全部」时给 —— 单独要一域的人不该被一段通用说明淹掉）：
岗位没声明 `skills`（是「没登记」，不是「没有技能」）/ 声明了但正文还没落 / 可见域上没挂项目 /
**新鲜度没有真相源** / **版本只有 persona 指纹那一半**。不编，也不假装完整 ——
读的人据此知道该去问谁，而不是以为这就是全部。

**纪律照抄 §7.1.21，一条都没新造**：项目按**同一把尺子**（`render.VisibleDomains`）过滤 ——
域不在范围里连项目名都不给（与域行同一条纪律）；`period` / `source` **原样搬**（它们是指针，
真源在别处，ANC 不解析、也不规定格式）；指针一律 vault 相对，本机绝对路径一个字都不出现；
只读，一个字都不写回真相源。

**用例（新增 5 条，`runtime/envelope_read_test.go`，仍走真 HTTP + 真 JSON-RPC；该文件 10 → 15 条）**：
一次调用答出技能与项目（fixture 无 `skills/` → 两条都 `missing`；指针 vault 相对、无反斜杠；
项目恰一条、`owner` 是人名、`period`/`source` 原样、`evidence` 指 `projects.md`；`gaps` 含「新鲜度」与「技能正文还没落」）/
**负例**：真写一份 `skills/<name>/SKILL.md` 之后那条不许再报 `missing`（另一条仍报）/
`bob`（logistics 域）看不到 `trade-q3`（可见范围过滤）/
被拒的回话里 `projects` / `skills` 都为空（拒的时候不给数据）/
只要一域时不附通用报缺。

**实测**：`gofmt` 空 · `go vet` rc=0 · `go test -count=1 ./...` 十四包全绿 ·
`go test -run ReadContext -v .` 15 条全过。
**没做（明确标出）**：新鲜度 / 版本 / 跨 agent 共享上下文 / 蒸馏产物 ——
这四样都还没有真相源，所以它们只出现在 `gaps` 里，不假装有。

### 7.1.24 接入面扩容：hermes / openclaw 也真调通（2026-10-09，W3）

§7.1.14 接的是 CC / Codex 两家。这一轮又接两家，**服务端一个字没改** ——
#44 验收里的「网关从 A 换成 B」，从 2 家扩到 4 家。

| harness | 接线口径（客户端侧，各写各的） | 发现的工具 | 真调结果 |
|---|---|---|---|
| hermes | `mcp_servers.<name>.url` | 2 | ✅ 信封落盘 |
| openclaw | `mcp.servers.<name>.url` + `transport: streamable-http` | 2 | ✅ 信封落盘 |

**两家的口径完全不同**（一家是 `mcp_servers` 映射，一家是 `mcp.servers` + 显式 transport 名），
ANC 侧没有任何分支 —— 这正是「形态收成一张表」想要的结果。

**探针不等于通**：`tools/list` 报出 2 个工具只证明握手；这一轮两家都真跑了 `tools/call`，
判据是 `<data>/envelope/<YYYY-MM>.<who>.jsonl` 真长出一行、六问六答回到 harness。

**两条 harness 侧的坑（接的时候要知道，都不是 ANC 侧的问题）**：

1. **provider 名与内置 provider 重名会被遮蔽**：客户端把自定义端点取名叫某个内置名（如 `deepseek`），
   解析器会判成内置 provider（走内置的 key 环境变量）而不是那条自定义端点 —— 表现是「配置里明明有 key，
   却报没有 key」。给自定义端点取个不撞内置的名字即可。
2. **fake-IP 代理 × 客户端 SSRF 守卫**：客户端带 URL 守卫、而本机 DNS 被代理接管成 fake-IP
   （解析落在 `198.18.0.0/15`）时，**模型请求**会被判成「内网地址」拦下（MCP 那条走 `127.0.0.1`
   不受影响）。守卫放不放行取决于客户端有没有那条窄口径的 opt-in，接线前先确认。

**仍未落**：与 §7.1.14 同一张清单 —— 计量 / 拒答 / 会话归属。

### 7.1.25 留痕又两条腿：Codex / OpenClaw 记录读取器（2026-10-09，零依赖优先）

§7.1.22 说「换一家 harness = 加一行」——那一刀只到**接入口径**。这一轮把这句推到**记录读取**：
表里的 `reader` 字段指名用哪个读取器，`anc trail` 按名字取；表里 `reader` 为空的家，
明说**读不了**，不假装能读。

**零依赖优先**（2026-10-09 拍板）：两个读取器只用标准库（`bufio` / `encoding/json` / `os` /
`strings` / `time` / `io/fs`），一个第三方包都没引 —— 交付物里不为「跑 20 行逻辑带一整个运行时」。

**怎么定位一份记录：两种，都写在同一张表里**

| 方式 | 表里的字段 | 用在哪家 | 为什么 |
|---|---|---|---|
| 推目录 | `dir_rule` + `projects_dir` + `main_file` | claude / codebuddy / hermes / dsh | 目录名由 cwd 按规则压出来 |
| 按 id 递归找 | `search_dir` + `search_glob`（glob 必须含 `<id>`） | codex | 记录按 `年/月/日` 分层，cwd 推不出目录 |

`ResolveMain` 的两种结局都不许猜：**0 命中 = 读不到**（主记录留空，Problems 里写一句
「`<目录>` 底下没有句柄为 `<id>` 的主记录（这是**读不到**，不是 0 消耗）」）；**2 命中 = 报错** ——
句柄定不出唯一一份记录时宁可报错，也不挑一份出来充数。

**codex 六条口径**（2026-10-09 在本机真记录上逐字段对拍：一个 fork 出来的会话，5 轮 / 541 行）

1. 一轮的账 = 该轮各次调用的 `usage` 按 `response_id` 去重后求和 —— 实测与末条 `turn_token_usage`
   **逐字段相等**（5/5 轮），两条路互为对拍。
2. `thread_token_usage` 是**线程累计**：fork 出来的会话把前史一起带进来（本样本 **6.4 亿** vs
   本段 **1.04 千万**）—— 拿它当本会话的账就是严重多报。
3. 提问在 `event_msg/item_completed` 的 `UserMessage` 里；`response_item` 的 `message/role=user`
   是环境上下文那种**合成注入**，不算一轮（混起来轮次会翻倍）。
4. 失败判据在 `CommandExecution.status`（`completed` / `failed`），不在 `function_call_output` 上 ——
   后者只有输出文本；工具名靠 `call_id` 对回 `function_call`。
5. 记录里**没有金额**（实测：全文只有推理正文里出现过 cost 这个词）—— `cost_usd` 记 0 是
   **读不到**，不是花了 0 元。
6. **缓存读含在输入里**（实测 `total = input + output`，cached 是 input 的一部分）—— 不许相加。

活体：`turns 5` · `in 10446656` · `out 71875` · `cache_read 10302848` ·
`exec_command 64` / `write_stdin 5` · `失败 2` · `调用 69` —— 与独立写的一份 Python 对拍**逐数相等**。

**openclaw 六条口径**（2026-10-09 在本机真记录上读：一条 MCP 工具调用，提问 → toolCall →
toolResult → 收尾回答）

1. 一轮 = 一条 `role=user` 的消息；**用量与金额都挂在每条 assistant 消息上**，一轮可能有多条。
2. 同一条消息被重写时，**用量与金额都按 `responseId` 去重取大** —— 一个去重一个累加会自相矛盾。
   代价写清楚：取大不是取末值，重写时间线**早期**的成本会被保留（宁可略高，不略低）。
3. 失败判据是 `toolResult.isError`；**模型自己失败**的那一轮另有 `stopReason:"error"` +
   `errorMessage`（实测「Connection error.」），也记失败。
4. **金额记录里有**（`usage.cost.total`，美元）—— 这一家不用我们自己算。
5. **缓存读单列、不含在输入里**（实测 `input + output + cacheRead = totalTokens`）。
6. toolCall 的名字带 MCP 命名空间前缀（实测 `anc__anc_send_envelope`）—— 原样搬，不剥前缀
   （剥了就和记录对不上）。

活体两份：① 有 MCP 工具调用那份 `turns 1 in 20807 out 301 cache_read 49664 cost 0.00299726`，
与它自己 `.trajectory.jsonl` 里 `model.completed` 的 `usage`（input 20807 / output 301 /
cacheRead 49664 / total 70772）**完全一致**；② 另一份 `turns 4 in 54836 out 108 cache_read 12800
cost 0.00770728`，与逐条 assistant 的字段求和逐数相等。

**两家正好相反的一条（不许混用）**：**缓存读的归属**。codex 含在输入里，openclaw 单列。
两家各写一条 `usageOf`，混用会把输入算错一个量级。

**这一轮在真记录上抓到、都已修的三个读错**（各配用例）

| 读错 | 真记录里的样子 | 现在怎么读 |
|---|---|---|
| codex 提问被样板顶掉 | 整段是 `<in-app-browser-context …>` + 空行 + `## My request:` + 真人那句 —— 5 轮的 prompt 全是这段样板 | 先取 `## My request:` **后面**那句；没有记号就剥行首已知的块；**表里没有的记号不猜**、原样留着；剥完是空的就退回原文（宁可看到样板，也不能把提问抹成空白） |
| openclaw 模型自己失败**没记成失败** | 那一轮的 assistant 消息带 `stopReason:"error"`「Connection error.」，用量 0、也没有 `responseId` | 记 `failed`（`Tool` 留空 —— 是模型失败、不是某个工具失败），并在 Problems 里说明「这一轮到底烧没烧 token 记录里看不出来」。不改的话看板上是一排绿灯的空轮 |
| openclaw `content` 只认块数组 | 两种形状都出现过：块数组，也有整段就是字符串的（如 `[OpenClaw heartbeat poll]`） | 两种都收。只认块数组会把后者整行解析不了 —— 提问丢了，还被报成「记录格式可能变了」 |

**读不了的两家（印在表上，不是隐形债）**

- **hermes**：会话记录（`sessions/<id>.jsonl`）是 OpenAI 风格的消息，**里面没有用量字段** ——
  用量与金额只在它自己的 `state.db`（sqlite）的 `sessions` 表里。零依赖优先下读 sqlite 要引依赖，先不读。
- **dsh**：主记录是 **zstd 压缩**的 jsonl（`session.jsonl.zstd`），标准库没有 zstd 解压 ——
  要读得引一个解压实现，体积**未实测**，先登记不读。

**用例与判据**：`internal/trail/reader_codex_test.go` 9 条 + `reader_openclaw_test.go` 7 条 = **16 条**，
照真记录形状造（外层 type 分层 / `thread_token_usage` 故意写成 9.9 亿 / 末行时间戳乱序 / 失败判据两条 /
重写按 responseId 去重 / 环境上下文与未知记号 / 字符串 content）。`gofmt` 空 · `go vet` rc=0 ·
`go test -count=1 ./...` **十五包全绿**。

**仍未落（写清免得变成隐形债）**：读到的账怎么进看板（现在只到「按 reader 名字取到一份 Session」这一层）；
openclaw 同级那份 `.trajectory.jsonl` 事件流没读；hermes / dsh 的记录根只有 Windows 一条；
**macOS 记录根全部未验**；各家的子任务（subagent）记录落不落盘都还没验。

### 7.1.26 一次 trail 里读多家：认的是桥里写的类型（2026-10-09，端到端真跑）

§7.1.25 落了 codex / openclaw 两个**读取器**，但它们在当时**读不出来**：用哪一家口径读由命令行
一个旗标（`--harness`）说了算，一次只能给一家；而一段会话究竟是谁家的，桥（网关的会话落盘）
里写得清清楚楚。沙箱真跑的证据：桥里写着 `agent_type: openclaw` 的槽，被按 codex 口径去读 →
报「原生记录找不到」；同一份记录换成 `--harness openclaw` 就一字不差地读出来了。

**改法（表驱动，一处）**

- 表里每家多一列 `agent_types`：**别人怎么叫这一家**（实测 cc-connect 的 agent_type 写的是
  `claudecode` / `codex`，和表里的 id 不是一回事）。别名**跨家唯一** —— 撞了在**读表**时就拒：
  同一个类型指两家 = 一段会话没法认家，而这会随遍历顺序变（同一个现场两次结果不同）。
- `anc trail` 改为**逐段会话认家**：桥里写了 → 用那一家，**记录根也换成那一家自己的**
  （一次 trail 里根不止一个）；桥里没写（老槽、从没起来过）→ 退回 `--harness` 那一家；
  桥里写了、表里不认 → **明说读不到**。
- 头部与 `--json` 都逐家列出「用了哪家、从哪个根读的」——「哪段会话是从哪读的」正是
  这份账可不可信的前提。

**为什么「不认就退回默认口径」是错的**：按错的口径读出来的数字比读不到更糟 —— 读不到会被追，
读错了不会。用例把这个做成负例：那个 id 在兜底那家的根里**有**记录（999），一旦有人改回
「退回默认口径」，合计就会从 600 变成 1599（= 认错家的指纹）。

**实测（沙箱真跑；桥是手写的，记录是真的）**：一次 `anc trail`、**不给 `--harness`**，
三家一起读出来：

| 家 | 记录根 | 这一段读到的账 |
|---|---|---|
| Codex | `<家>/.codex` | 5 轮 · 输入 10446656 · 输出 71875 · 缓存读 10302848 · 工具 69（exec_command 64 / write_stdin 5）· 失败 2 · **无金额字段** |
| OpenClaw | `<家>/.openclaw` | 4 轮 · 输入 54836 · 输出 108 · 缓存读 12800 · 成本 $0.007707 · 失败 2 |
| OpenClaw | 同上 | 1 轮 · 输入 20807 · 输出 301 · 缓存读 49664 · 成本 $0.00299726 · 工具 `anc__anc_send_envelope` ×1 |

判断层跟着真出结论（不是空跑）：codex 那两轮各出一条 `turn-tool-failed`（失败原因照抄记录原话）；
openclaw 那两轮同样出结论。§7.1.25 里 codex 活体那份数（10446656 / 71875 / 10302848）走 CLI
再对上一次，**逐数相等**。

**用例**：`trail_cli_test.go` 2 条（四种认家情形一次跑完：认得的家 / 另一家换根 / 不认的家 /
没写类型退回旗标；文本模式逐家列根）+ `harness_test.go` 2 条（翻译表大小写与去空格、别名撞车与
空别名在读表时被拒）。

**仍未落 / 已知（写清免得变成隐形债）**

- **判据层把「模型自己失败」归到了「这一轮有工具执行失败」名下**：失败是真的，类别词是错的
  （证据里会出现「（工具名未知）」）。要么给失败加一类（如 `model-error`）并把指标拆开、
  要么改这条判据的措辞 —— 两条都要动判据的**数据契约**，所以留着拍板，没自己改。
- **openclaw 不是 cc-connect 的 agent 类型**（它认的是 claudecode / codex / gemini…，没有 openclaw），
  所以它的桥这次是**手写的**。要让 openclaw 的账真正进 ANC，得先定「它的会话 id 从哪来」——
  这仍是个开口（走 MCP / 入站信封是候选，未定）。
- 认家只决定**从哪读**，不决定放不放行 —— 这一刀没有加任何门禁。

### 7.1.27 业务 agent 表：`agents.md` 的落点（2026-10-10，SPEC §4.7 ③ 第一刀）

**为什么新开一张表，不塞进 `members/`**：SPEC §2.3 的两类 agent 单位不同 —— 成员 bot 代**人**
（一个经理一个 bot，bot 的 manager 天然是那个真人），业务 agent 代**岗位 / 流程**。失败代价也不同
（成员 bot 答错，人还能兜；业务 bot 处理错单要赔钱）。`members/` 的字段里有一半
（`display_name` / `open_id` / `allow_from`）对业务 agent 没有意义 —— 混在一张表里，「人」和
「岗位」就会被同一套必填项绑住。

**这一层只做一件事：声明「它存在、它挂哪块业务、它能用哪些工具」。** 它落在哪台机器、哪个 OS 账号、
哪个目录，全是**本机事实**，不进 git（SPEC §4.5：git = org 真相，`~/.anc` = 部署参数）。

| 列 | 必填 | 说明 |
|---|---|---|
| `slug` | 是 | ASCII 小写标识。进 project 名（`<公司 id>-<slug>`，`render.AgentProjectName` 一处派生），通常还要当 OS 账号名 |
| `name` | 是 | 中文显示名（看板上用的） |
| `domain` | 是 | 挂哪块业务（`domains.md` 的 slug）。**数据视野那把尺子（`render.VisibleDomains`）以域为单位** —— 没域等于没视野 |
| `harness` | 否 | 跑哪条腿（`harness` 表的 id）。空 = 默认那家（§4.7 H2 / H3 的消费方） |
| `tools` | 是 | `allowed_tools`。**非空是红线**，见下 |
| `model` | 否 | 空 = 走 company 默认 |

认得的列名收在 `knownAgentColumns`：不认得的列**忽略** —— 表可以多列备注，不影响解析（与域表 / 项目表同一套底座 `table.go`）。

**为什么 `tools` 空是红档（这条表里唯一一条 fatal）**：实测（上游 schema 790 行）—— `dontAsk` 下
**未预授权的工具一律自动拒绝**，于是空白名单的 bot「连得上、能回话，却干不了活」，而现场看上去
全是绿的。这与 `role.allowed_tools.empty` 是**同一条不变量**，所以同档：一条不变量只登记一条规则，
不因为换了张表就换档。**不锁死**：公司想先观察，在 `company.md` 写 `policy: agent.tools.empty = warn`。

其余六条一律 `warn`（slug 格式 / slug 重复 / 缺 name / 缺 domain / domain 不在域表里 / 表建歪），
理由与域表同一条：新表先观察，不设红线。跨表校验**只发现事实**，档次由规则表定 —— 实现里不许硬编码档次。

**指纹**：`InputsHash` v6 → **v7**。新增 `agent|<slug>|<name>|<domain>|<harness>|<tools>|<model>` 段
与 `host-agent-home|<slug>|<目录>` 段（后者是主机事实 —— 换了机器上的落点，渲染结果就该被判为漂移）。

**用例**：`internal/org/agents_test.go` 11 条 —— 读表 / 列序跟表头 / 跨表只告警且定位串指到 `agents.md` /
slug 重复 / 表缺席不报错 / 有文件无表 / **空 tools 拦下**（连降档那条一起钉住）/ 可上调 fatal /
改一处指纹必变 / `Host.AgentHome` 没配 = **如实报缺**（不许拿 `HomesRoot` 拼一个看着像的路径顶上）/
主机事实同样进指纹。

**仍未落（如实列缺，不是漏做）**

- **渲染没接**：`render.Build` 还不为业务 agent 产 `[[projects]]`。要接，得先定两件**没拍**的事：
  它的 persona 从哪来（表里没有 `role` 列）、它的平台绑定（`app_id` + `app_secret` 键名）落在哪。
  这两件是**真相源的表结构**，不该由实现顺手定 —— 记在 SPEC §13 **Q23**，定了才好接这一刀。
- **本机落点还没有入口**：`Host.AgentHomes`（agent slug → cwd）是有了，但 `anc render` / `anc apply`
  还没有把它填进去的地方（现在恒为 nil）—— 与上一条同一个前提，一起拍。
- **看板不认这张表**：业务 agent 还没有自己的分视图（那是 M2 第 6 项，也是 §4.7 ③ 的下一刀）。
  **2026-10-15 补**：**看得见了，但只见于一处**，三处口径分开记：①「授权」页（§7.1.53）把 `agents.md` 里的
  业务 agent 当**主体**列进视角名单（只看它有没有出现在某条 grant 的 `from` / `to` 上）；② 组织页仍然**不列**
  业务 agent（那一页读的是域 / 角色 / 成员 / 路由 —— 实测 `#/org` 里搜不到 agent 的 slug）；③ 「业务 agent
  控制面」剩的那半（待批 / 下发）仍未做。

### 7.1.28 业务 agent 进配置：一个 agent 一个 project（2026-10-10，Q23 按 (a) 拍完）

§7.1.27 只把业务 agent **声明**出来了（`agents.md`），渲染没接 —— 卡在 persona 从哪来、平台凭据落哪。
这一轮按 **(a) 表里加列** 拍完并接上：`role`（persona 复用 `roles/<role>/` 那一套）+ `app_id`（平台绑定）。

**与成员 bot 共用同一套机制，三处差别都是刻意的**

| | 成员 bot（代人） | 业务 agent（代岗位） |
|---|---|---|
| persona | `roles/<role>/` + `members/<名>/persona.md` + 域表 | `roles/<role>/` + 域表 —— **没有「人写的 persona」那一段**（FDE 的东西住在它的 cwd 里，ANC 不替它写第二份） |
| 收件人 | 本人 + `extra_allow_from` + admins | 这块业务 `who` 岗位下的成员 + admins（从 `members/` 现算，不另存名单） |
| 平台凭据键名 | `FeishuSecretKey(成员名)` | `AgentSecretKey(slug)` —— 同一支 `secretKeyBody`，例：`order-bot` → `ANC_FEISHU_SECRET_ORDERx00002dBOT` |

另有两条照 SPEC §4.7 四条禁令来的：**不发 `admin_from`**（业务 agent 不是给人下命令用的，少一个入口
少一类事故）；**空 `tools` 不写 `allowed_tools` 这个键**（与成员同一条写法 —— 写一个 `[]` 出去等于替上游
猜「空数组 = 全开还是全禁」，那次猜错的代价是现场）。

**cwd 是本机事实，所以走旗标**：`--agent-home <slug>=<目录>`（可重复，`anc render` / `anc apply` 都有）。
不进 org（SPEC §4.5：git = 真相，`~/.anc` = 部署参数），也不新开一种文件格式 —— 部署参数本来
就是旗标这一族。

**渲染期三种「这一台机器上先不渲染它」，都如实报缺（`plan.Warns`），不猜、不静默跳**

1. `role` 取不到（表里没写或在 `roles/` 里找不到）—— persona 的职责那一段取不到；
2. 本机没配 cwd —— **不猜目录**（拿 `HomesRoot` 拼一个看着像的，会在别人家目录里建夹子）；
3. 解不出收件人 —— 写出去就是一个按上游默认谁都能敲的 bot。

**没接 app_id = 照渲染，但进灰档**：与成员的 `unwired: true` 同一条口径，`UnwiredProjects` 一并收录
（四个消费方共用这一支算：探针 / apply 回读 / 看板运行态 / 告警）。**不新开一条规则** ——
「声明了还没接线」是状态，不是缺陷。

**指纹 v7 → v8**：`agent|` 段加上 `role` / `app_id` —— 换了岗位或换了平台绑定，重渲染判据必须变。

**用例**：`internal/org/agents_test.go` 13 条（新增 role 两种情形 + 与成员同名相撞）+ `internal/render/agents_test.go`
6 条（项目块逐字段 / 收件人按岗位取 / 没配 cwd 跳过 / role 取不到跳过 / 没接线进灰档 / `WorkDirs` 对得上 / 键名钉字面量）。`gofmt` / `go vet` 干净，**15 包全绿**。

**已实测（2026-10-10，§7.1.29）**：业务 agent 在**独立 OS 账号**里**真回话** —— 入站一问一答走完
（`message received` → `turn complete`，用量非零、`silent=false`），探针落绿。
同机同 app 只允许一条长连接，所以是**串行**验的：停掉占着那条连接的实例，验完起回来。

### 7.1.29 业务 agent 在独立 OS 账号里装载并真回话（2026-10-10，沙箱 Linux 真跑）

§7.1.28 的「仍未测」这一轮补齐：**换一台机器、换一个 OS 账号、非 root 跑通，并且真回话**。

**做法**：同一份公司真相源，业务 agent 落在**它自己的 OS 账号**下（与成员 bot 不共用账号），
`anc apply <vault> --agent-home <slug>=<它的 cwd> --apply`。它装**用户级** systemd 单元，
凭据桥照旧写 drop-in。

**实测（这台机器上逐条看过；`/proc/<pid>/environ` 与产物文本是判据）**

| 项 | 判据 | 结果 |
|---|---|---|
| 不需要提权 | 以业务 agent 自己的账号跑 `anc apply` | 单元装进它自己的用户级 systemd，`active (running)`，进程 uid = 该账号 |
| 凭据桥跟人走 | `/proc/<pid>/environ` | 全部 `ANTHROPIC_*` + 该 project 的 `ANC_FEISHU_SECRET_<slug>` 都在 |
| harness 在那个账号下可用 | 该账号下跑一次 headless 问答 | 出字、rc=0 —— 模型与 base 全来自 `secrets.env`，没依赖任何 GUI 登录态 |
| 平台连上 | 上游日志 | `feishu: bot identified open_id=…` + `platform ready`，且**没有** `app_id is invalid` |
| 产物只含它该有的 | `config.toml` | 只有一个业务 agent 的 project（+ 公司那个 devbot）；成员 bot 一条都不在里面 |
| 业务 agent 不发 `admin_from` | 同上 | 全份产物里只有 devbot 那条有 `admin_from` |
| 账号隔离 | 两边互读家目录 | 双向 `Permission denied`（家目录默认 700，不需要额外配置） |
| 真回话 | 一条真飞书消息 → 日志 → 探针 | `message received` → `turn complete`（`tools=17`、用量非零、`silent=false`）；探针随后报绿（「最近一次成功回复在 Ns 前」） |

**两条实测教训（都是这次踩出来的）**

1. **「隔离一个业务 agent」不能靠「把这个公司的成员全停用」**：`company.devbot.count` 是红档
   （SPEC §1：一个公司恰好一个启用中的 devbot）。正确做法是**只停用会和它抢同一个 app_id 的那些**，
   devbot 留着 —— 它本来就该是 `unwired` 灰档，不参与回话。
2. **上游说 `platform ready`，不代表这个 bot 能用。** 占位 / 无效 `app_id` 的 project 会先打
   `platform ready`、紧跟一条 `websocket error: app_id is invalid`（同一个进程里先绿后死）。
   所以「某 bot 到底能不能用」的判据只有真相源（`unwired` + 探针那四档），**不许拿上游那一行当证据**。

**回读判据修正（同一轮，已落盘）**：第 7 步的就绪判据原来是 `want = len(plan.Projects)` ——
把 `unwired` 的 project 也算进期望值，同一份部署**既会假绿也会假红**（上面那两种上游表现各对应一种）。
现在期望值只取**接了线的** project（`readyExpectation`，与灰档共用 `render.UnwiredProjects` 一支算），
并且**按每个 project 数**（认 logfmt 里那个 `project=`，带引号的也认）—— 不再按 `platform ready` 的行数数。
同族口径补一条：**一个可期待的都没有时不许打绿**（空集上的全绿是最纯的假绿，同探针那条）。
用例：`runtime/apply_cli_test.go` 5 条；把计数退回旧写法，其中一条当场红 —— 不是空跑。

**仍未测**：同机同 app 的**并发**（一条 app 只允许一条长连接，这次是串行验的），
以及第二条腿（codex / openclaw）在业务 agent 上回话。

### 7.1.30 审计归集双源：归集不再解析任何一家的记录（2026-10-10，W3「10-25 审计归集双源」）

**判据**：两个 harness 的记录都能被 `anc audit collect` 归集。

**起因**（2026-10-10 用沙箱 vault + config 实测，三条都不是推断）：

| 命令 | 结果 |
|---|---|
| `... --harness codex` | 扫到 **0 个分片**，报 `codex：dir_rule 是空的`，rc=1 |
| `... --harness openclaw` | 目录找对了（14 个分片），但 **0 次行使**，rc=0 |
| `... --harness claude` | 1 分片 / **0 行使**，报「工具 `mcp__anc__anc_read_context` 不在工具表里」 |

根因两条，都在同一处：`audit.Collect` **自己**按 claude 的 jsonl 形状解析（`message.content`
块数组 + `tool_use` / `tool_result`），口径表里的 `reader` 字段一次都没用上；找目录又用
`Family.TranscriptDir`（slug 规则），而 codex 的记录按 **年/月/日** 分层 —— 目录从工作目录**推不出来**。

**定形：把「行使」收进 reader（方案 A）**

- `trail.Session` 多一份 `Acts`：一次工具调用的**原始事实** —— `{CallID, At, Tool, Input, Result, Why}`。
  `Result` 只有四种：`ok` / `denied` / `failed` / `unknown`（没配到结果，**不许当成「没发生」**）。
  `Input` 是**各家原样**，不归一 —— 一份要被四家共用的结构不能带任何一家的语义。
- **四家读取器各自填 `Acts`**（claude / codex / openclaw / codebuddy）—— 格式知识一家只留一份。
- `audit.Collect` 改成：**枚举分片 → 该家 reader 读出 Acts → 工具表分类抽「对谁」→ 定结果**。
  自己那套 jsonl 解析删掉。**分工是硬的**：记录长什么样归 `internal/trail`，
  一次行使该怎么读归工具表（那是数据）。
- 新增 `trail.ReadShard(fam, path)`：按**路径**读一份分片，不自己推目录 —— 归集那一路
  已经把手上的分片列出来了。句柄也**不在那里定**（见下「幂等键」）。

**分片从哪来**：口径表新增一对数据 `shard_dir` + `shard_glob`（成对，半张表必须报错）。

- 声明了 = 这家把分片堆在同一层里、目录从项目名推不出来 → **全量枚举**，
  再按记录里自带的 `cwd` 认领到项目（`Session.Cwd`）。
- 没声明 = 按项目推目录（老路，`<root>/<projects_dir>/<slug(work_dir)>`）。
- 认不到 = 别的目录的会话（这台机器上不止我们这几个项目）→ **跳过，连它的口径说明都不带**。
  但**记录里没有 `cwd`** 是另一回事：那是认不出归属，必须报。
- 非 Windows 之外：`normPath` 比路径时统一分隔符，Windows 上不分大小写（配置写 `D:\Foo`、
  记录写 `d:\foo` 是同一处）。

**codex 的行使口径**（2026-10-10 实测本机 84 份 rollout，写代码直接照这个）：

| 项 | 实测 |
|---|---|
| 分片 | 全在 `<home>/sessions/` 下按 年/月/日 分层，文件名一律 `rollout-*.jsonl`（84 份，无例外） |
| 工作目录 | 只在 `session_meta.cwd` 里 —— 认领项目靠它 |
| 一次调用 | `response_item` 的 `function_call`：`{name, arguments, call_id}`，入参是**字符串**（不是对象） |
| 成败 | `event_msg/item_completed` 上：`CommandExecution.status`（实测 521 个命令里 41 个 `failed`）与 `McpToolCall.status`；`ImageView` 那种**没有 status** |
| 兜底 | 没有 status 的退到「有没有 `function_call_output`」（有输出 = 跑完了）；两样都没有 = `unknown` |
| 入参键 | `exec_command` 的键是 **`cmd`**（不是 `command`） |

**幂等键用文件名，不用记录自报的 `session_id`**：codex 一份分片是一个 **fork 出来的 rollout 文件**，
而记录里的 `session_id` 会和**母会话重名**（fork 把前史一起带了进来）。实测本机就有
`rollout-...01a11174-..._01a1214d-...` 这种一份：它自报的 `session_id` 正是母会话那个。
拿 `session_id` 当键，两份不同分片会撞成一条。用例 `TestCollectGlobalIDUsesFileNameNotSessionID` 盯着这一条。

**工具表补的条目（数据，不是代码）**：codex（`exec_command` / `shell_command` / `write_stdin` /
`js` / `view_image`→read(path) / `send_input` / `wait_agent` / `spawn_agent` / …）+ openclaw
（`read` / `memory_get` / `write` / `edit` / `exec` / `web_fetch`(url) / `message` / `gateway` / …）
+ ANC 自己接入口**三家前缀变体**（claude 是 `mcp__anc__anc_xxx`、openclaw 是 `anc__anc_xxx`、
codex 是光秃秃的 `anc_xxx`）。两个**刻意不登记目标键**的：`gateway`（它也有 `path`，但那是它
自己的配置路径，登记了会把「读了一个文件」错记到它头上）、`exec`/`exec_command` 这类自由文本。

**两条降噪，都是为了「真问题看得见」**

1. **读取器的口径说明去重**：每一家对**每一份**记录都会追加同样的说明（「codex 的记录里没有金额」），
   84 份会刷成 252 行。去重键是**读取器原话** —— 带文件名的那种（「N 行解析不了」）各是各的，不会被吃掉。
2. **别人的分片不留痕**：不是我们项目的记录，连它的口径说明都不混进我们的归集结果。

**判据（真数据实测：沙箱里的 vault + config）**

| 命令 | 结果 |
|---|---|
| `anc audit collect <v> --harness codex` | **84 分片 / 9 次行使 / 9 条记录**（ok 2 · failed 7 · 目标抽不出 5） |
| `anc audit collect <v> --harness claude` | **1 分片 / 1 次行使 / 1 条记录**（ok 1） |
| 两边 `--write` 之后重跑 | **0 条**（幂等：`Record.ID` 来自「分片句柄 + call id」） |

**用例**（新增 11 条，都不是空跑：把口径改回旧写法，对应那条当场红）

- `internal/trail/reader_acts_test.go` 5 条：四家的 Acts 形状（claude 配对 / codex 的 status 与
  「有输出算跑完」/ openclaw 的**对象**入参 / codebuddy 的**字符串**入参）+ `ReadShard` 按路径读。
- `internal/audit/collect_global_test.go` 2 条：全量枚举按 cwd 认领（别人的分片跳过且不报、
  没 cwd 的必须报）+ 幂等键用文件名。
- `internal/harness/shards_test.go` 4 条：递归枚举、没声明 shard_dir 的家不列全量、
  记录目录不在**不是错**、半边声明必须拒。

**仍未做**：openclaw 的分片枚举（那家同一层里还躺着 `<id>.trajectory.jsonl`，`*.jsonl` 会把它
一起吃进来 —— 要按前缀排除，所以这一家先留老路按目录枚举）；hermes / dsh 仍无读取器。

### 7.1.31 「进行中」的热层：达标落库、进行中落热层（2026-10-10，议题 C 的第一刀）

**拍板原话**：达标的事落库，还在做的事落缓存（热层）；一个项目周期一般不超过半年，够用；
TTL 是「按客户与业务状况配」的东西，先给默认值、改配置项即可。

**为什么不是文件**：任务状态是**当前值**，不是历史。拿行文本承载当前值，写一百次就留一百行 ——
最后没人说得出「现在谁在做、卡在哪」。历史归 `internal/timeline`（append-only、可 diff、可回滚），
当前值归热层（可覆写、可判新鲜度、可原子认领）。所以 `anc hot done` 的顺序是**先落库、再清热层**。

**两件容易被混在一起的事**（之前的说法把这两件混了，这里拆开）：

| | 副本式缓存 | 热层（本次落的） |
|---|---|---|
| 是什么 | 同一份数据的**第二份** | 进行中状态的**唯一权威** |
| 为什么不做 / 做 | 不做：共享状态只该有一份存储 | 做：它就是要被高频读、要被覆写的那一份 |
| 过期 | —— | 状态**不设 TTL**（`--ttl 0` 默认）；TTL 只给授权 / 租约 |

**落地的形状**：

| 件 | 落点 | 口径 |
|---|---|---|
| 状态编解码 | `internal/hot/hot.go`（`State` / `Fields` / `StateFromFields`） | **空字段也照写** —— 只写非空的，「上次有值、这次清空」会留下一个删不掉的旧值 |
| 新鲜度 | `State.Stale` + `--stale` | `as_of` + 阈值**打标记**，**不删、不拦**；时间读不懂按「不新鲜」算，不假装它新鲜 |
| 原子认领 | `Store.Claim`（`SET NX` + 可选 `PX`） | 抢不到报 `ErrOccupied` 并**说出现在是谁**；放开只认认领人自己 |
| 介质 | `Store` / `Open(Config)`（默认 Redis） | 地址 / 前缀 / TTL / 超时全在 `Config`；**进程连不上当场报错**（`Open` 里先 PING），不静默降级 |
| 客户端 | `internal/hot/resp.go`（手写 RESP2） | 只用十几条命令，所以不引第三方客户端：`go.mod` 仍**零第三方依赖** |
| CLI | `hot.go`（`anc hot ping·put·ls·claim·release·drop·done`） | `done` = 先 `timeline.Append` 成功，再 `Drop` 热层；落库失败则热层一个字不动 |

**体积账（实测，不是估）**：`anc hot` 让二进制从 12.85 MB 到 **12.96 MB（+0.11 MB，零新依赖）**；
对照：若引 `go-redis/v9`，二进制 +8.75 MB、`go.mod` 多 4 个第三方依赖。Redis 服务端最小交付集
3.4 MB 磁盘 / 空载 6.7 MB RSS（沙箱量过），客户端形态由客户侧决定（见「没做的」）。

**实测（沙箱 VM Ubuntu 24.04 + 真 Redis 7.0.15；二进制 sha256 前16 `299a78c72fcc0cb8`；
日志 `<沙箱>/logs/2026-10-10-热层redis实测.md`）**：16 步全过 ——
自检 · put 覆写 · ls · 直看 hash（`TYPE=hash`）· alice 认领 · bob 抢被拒且点名 alice ·
bob 替 alice 放开被拒 · alice 自己放开成 · `as_of` 改到 3 天前报「不新鲜」· 配 3 秒 TTL 后
**不带 TTL 覆写必须把过期时间清掉**（4 秒后仍 `EXISTS=1`）· `done` 落库 + 热层 `EXISTS=0` ·
**落库失败时热层原样还在** · 地址给错时说人话并给下一步 · `--prefix` 隔离 · 收尾清干净。
质量门：`gofmt -l .` 干净 · `go vet ./...` 干净 · `go test -count=1 ./...` **16 包全绿**（新增 `internal/hot` 一个包）。

**没做的（写清，别让它变成隐形债）**：

- **客户侧怎么装热层没定**：现在验的是「用户级、跑在 `/tmp`、不装系统包、不改全局配置」。
  随 ANC 一起装 / 客户自备 Redis / 将来换介质，**都没拍** —— 所以介质与地址是配置，不是硬编码。
- **期限口径已落**：2026-10-10 拍板「期限与 token 都默认永久（可配）」（SPEC §6 / Q25）后代码已改 —— `grants.go` 必填表去掉 `ttl`、`grant.ttl.missing` 出厂档 `off`（见 §7.1.15「已落」）。
- **热层还没接进接入面**：agent 自己 pull 待办（议题 A 的三件套）与 `anc_read_context` 都还没消费它 ——
  这一刀只是把介质与规矩立住。
- **`anc hot ls` 不联真相源**：热层里若留着一条真相源已终态的（漏落库），现在看不出来。

### 7.1.32 授权执行层第一刀：跨域信封进不进得来（2026-10-10，W4 第一格）

**这一刀补的是 §7.1.15 里那句「执行层一行代码都没有」。** 之前授权表只有落点与结构校验 ——
表能读、能投影，但**没有一处会拦**。这一刀在接入面的信封入口上立了第一个关卡：
`anc envelope check`（离线对拍）与 `anc envelope serve`（MCP 入口）**走同一支** `envelope.Gate`
＋ 同一份规则表，不存在「查得出、拦不住」那种两套判法。

**判什么（只判一件事）**：这封信的（主体，客体）有没有一条 grant 覆盖。

| 维度 | 取值 | 出处 |
|---|---|---|
| 主体 | `on_behalf_of` 优先，退回 `who`（bot 不自有权限，只是代理对象权限的投影） | SPEC §2.3；两处共用同一支 `o.Identity` |
| 客体 | `scope.domain` 的 slug；`object` 认裸 slug 与 `domain:` 前缀（走同一支 `org.SplitRef`） | — |
| 主体怎么算被覆盖 | `to` 认四条：与主体编号全等 / `member:` / `role:` / 裸名字（先成员后岗位，同 `Resolves`） | — |
| 动作 | **不判** —— 信封里没有 action 这一维，不在没有定义处的地方发明词表 | 同 `grants.go` 对 `to` / `object` 的留白 |

**只判跨域**：没写 `scope`、或写的是自己那一摊 → **不通 grant**（自己域内本来就不需要）。
这条与出站读出口 `refuseDomain` 同一个口径，不新造第二套。主体 / 客体**解不出来**不在这里报 ——
那是 `Bind` 的 `envelope.who.unknown` / `scope.domain.unknown` 的活，一条事实只报一处。

**档次：出厂 `fatal`、不锁死。** 这是本库第一条**出厂就拦**的规则 —— SPEC §6 不变量 1「默认拒」
是硬口径，所以它与「先看见不先拦」的先例**不同**。要松开：`company.md` 写
`policy: grant.match.missing = warn|off`（规则是数据，不是这里的 if）。

**留痕三条腿**（SPEC §6 不变量 4「行使必留痕」）：

- 跨域 + 没授权 → **拒收**（`fatal` 的定义 = 阻止落盘，所以不写信封日志）＋ **另落一条 `audit`**
  （`result=denied`，`Why` 是当时那句话的原文）；
- 跨域 + 有 grant → 收（落信封日志）＋ 一条 `audit`（`result=ok`，`Detail=grant <落点>`）；
- **域内 → 收、不进 audit**（它不是一次跨通道行使，信封日志已经记着它）。

跨域 + `warn` 档照收的那一次：`result=ok` 且 `Why` 写明「照收」—— `audit` 的四档
（ok / denied / failed / unknown）答不了「照收但未授权」，那件事由**信封日志的 findings** 记着
（两条腿互补）。

**类别先记 `invoke`**：信封的 action 这一维还没定，这里记的是「行使了**接入面这一个调用**」，
不是「这封信的业务动作」。等 kind → action 那张表拍出来，改的就是 `auditEnvelope` 那一个字段。

**本次实测（2026-10-10，沙箱 Linux + 真 MCP 调用）**：
二进制 `sha256` —— Linux `812ad166f6176418`、本机 Windows `498D7179FACAF75A`；脚本 `gate-check.py`，
日志 `logs/2026-10-10-w4-gate.md`。拷一份沙箱 vault 到 `/tmp` 上跑（不动沙箱本体），
起 `anc envelope serve` 走真 JSON-RPC / HTTP：

| 用例 | 结果 |
|---|---|
| CLI `envelope check`：跨域无 grant | exit 1、报 `grant.match.missing`、`--json` 里 `ok=false` 且 fatal 含该规则 |
| MCP：跨域无 grant | `isError=true`、回话点名规则；`audit` 新增 1 条 `denied`、**信封日志 delta 0**（拒收不落盘） |
| MCP：加一条 grant（`manager → member:alice`，`object: ops`）后跨域 | `isError=false`「收了」；`audit` 新增 1 条 `ok`、`Detail=grant grants/alice-ops.md`；信封日志 delta 1 |
| MCP：域内（`scope_domain=knowledge`） | 收、信封日志 delta 1、**audit delta 0** |

质量门：`gofmt -l .` 干净 · `go vet ./...` 干净 · `go test -count=1 ./...` **十六包全绿** ·
新增 **12 个用例**（`runtime/envelope_gate_test.go`，含一组 4 个子例）逐条 `--- PASS`。

**没做的（写清，别让它变成隐形债）**：

- **只判客体，不判动作**：信封里没有 action，`kind → action` 那张表还没拍。
  **2026-10-10 补**：`kind` 已进判据一处分 —— `proposal` 从「行使」里排除掉（见 §7.1.33）；动作维度仍未定。
- **不查 `from`、不做级联失效**：人一走整条授权链当刻断，是 W4 后面那格的事；这一刀只有「无匹配即拒」。
- **TTL 到期失效 = 挂起**（2026-10-10 拍板）：`ttl` 只是原样透出，**不判**。理由：默认永久这句拍板本身就说「现在没人要期限」，替不存在的需求定语法（`30d` 相对谁起算）= 过拟合；**等真有客户写下期限再做**。
- **`to` / `object` 的词表仍缺**：只认成员 / 岗位 / 域三种形态，bot / 信道 / 看板分区不算命中。
- **看板上「谁能做什么」那页还没画**（投影里已经有数据）。
  **2026-10-15 补**：已画 —— §7.1.53。

### 7.1.33 授权给予：手工对拍捞出的一刀 —— 「提案不是行使」（2026-10-10，W4 第二格）

**先对拍、再写代码。** 「一条权从 **提出 → 送批 → 点头 → 生效**」这条路先**手工**走了一遍
（ANC 侧由我扮演、点头方由人扮演，全程只用现有二进制 + `git` + `anc audit add`），
拿到三样东西：判据对账、**5 条发现**、以及「只该改一处」的结论。

**对拍结果**（脚本 `w5-grant-walk.sh`，日志 `logs/2026-10-10-授权给予对拍.md`）

| 判据 | 结果 |
|---|---|
| diff 干净 | 真相源改动**只有 `grants/` 那一个文件**；`audit/` 流水 +3 条（本来就该长） |
| 痕两条 | 提案（bob）+ 批准（alice）各一条，都读得出来 |
| 生效即真 | 同一封**行使信**：批前 exit 1（拒）→ 批后 exit 0；**MCP 真调用**自动留一条 `ok`（`Detail=grant grants/bob-ops.md`）—— 循环自己闭合，**不用补记** |

**捞出来的 5 条**

| # | 发现 | 处置 |
|---|---|---|
| 1 | **提案被自家门禁拒**：`kind` 没进判据，一封「跨域 + 没 grant」的提案被判成一次行使 | **这一刀就修**（见下） |
| 2 | 烂提案不会被拦（`envelope.who.unknown` 是 warn 档） | 写文档：想拦就在 `company.md` 写 `policy: envelope.who.unknown = fatal` —— **零新代码** |
| 3 | 域表的 `who` 是**岗位**（对接人），不是批准人本人 | 写文档：批准人 = **那个岗位上的人**（成员表里的成员）。域表只答「找谁」，不答「谁有权批」 |
| 4 | **审计算不出 `domain:<slug>` 的落点**（归成「vault 内·非域」、跨域 0） | **挂议题**：gate 的留痕口径与 audit 的读法不一致 |
| 5 | 沙箱 vault 本身没有 git | 无动作（真客户环境是 `anc org init --git` 出来的仓库） |

**这一刀改了什么：提案不是行使**

- `internal/envelope/gate.go` 加一张**数据表** `requestingKinds = {proposal}`：命中它的信封**不判授权、
  不报 `grant.match.missing`**，但 `CrossDomain` 仍然置位 —— 它**照样是一次跨域请求**，照样留痕
  （`audit` 一条 `ok`，`Why` 写明「提案：请求授权」）。
- 为什么单放「提案」这一类：它的定义就是「agent 只能提、不能直接行使」（#34）。**还没拿到权的人当然没有 grant**，
  拿 grant 当提案的门 = **提案永远提不出来**。门禁拦的是「把数据拿出去 / 写进去」，而提案里**没有任何业务数据**。
- **不给它配档位开关**：这不是「门的松紧」，是**门的适用范围**（提案不是行使）。松紧才是数据。
- `ask` **不在**这张表里 —— 它可能带着数据去问，算不算行使要单独判，**不搭便车**（用例里钉住了这条）。

**实测**：沙箱 Linux 真跑 —— 提案信 `exit 0`（收）／同一客体上的行使信 `exit 1`（拒）→ 写 grant →
行使信 `exit 0`、MCP 自动留 `ok` 痕；`gofmt` / `go vet` 干净、**16 包全绿**，新增 2 条用例
（`runtime/envelope_gate_test.go` ⑧）。

**没做的**：审批的落点（谁在哪儿批）仍然只有「送给人 + 人给一个可识别信号」这条口径，**没落代码**。

**提案的载荷不约定结构（2026-10-10 拍板：撤回「给提案定结构化字段」那一版）**：提案请什么权，
写在 `body` 里，**人话即可** —— 格式由**客户自己定**（同立项书：`charters/` 那边怎么规定是客户的事）。
**结构只出现在终点**：生效后的 `grants/` 那七个字段才是机器读的**契约**。起点是人话、终点是结构，
中间那次转换由**人和管理者 bot** 做，ANC 不碰 —— 所以它也就没有「结构化字段」这回事。
（错在哪：我把「终点需要的字段」当成了「起点也得有」。机器要的「谁 / 代谁 / 哪块业务 / 什么类别」
全在信封**铭文**里，不在 `body` 里 —— 那是判据与路由要用的，跟业务内容无关。）

### 7.1.34 审批的落点：待批队列 + 唯一信号入口（2026-10-10，W4 余项第一刀）

**落点定了**（2026-10-10 拍板）：**看板**与**飞书卡片**两处，走**同一个信号入口** ——
SPEC §7「写路径只有一条：CLI / 对话式 bot / 看板是同一个变更管道的三个前端」。
钉钉 / 企业微信**不做**（生态不熟，主推飞书 + 看板）。

| 做 | 不做 |
|---|---|
| 收提案（`kind: proposal` 入队） | 不写真相源 —— `grants/` 由**人侧的管理者 bot** 写 |
| 送给人（队列带「该谁批」= 域表 `who` 那一列，**岗位**） | 不校「点的人是不是该批的人」（先原样留痕，判断交给 agent） |
| 留痕（点头落两笔：`timeline` 一条 `decision` + `audit` 一条 `invoke`） | 不认语义 —— 只认 `approve` / `reject` / `hold` 三个字 |

**为什么是三个信号**：驳回与挂起是两件事 —— 驳回是「这件事不该做」，挂起是「先放着 / 信息不够」。
合成一个词，事后就分不出是哪种。`ask` **不在**这张表里（它可能带着数据去问，算不算行使要单独判）。

**待批放哪**：热层（`prop:<提案 id>`）—— 它是「进行中的事实」；点头是「达标」，
**先落库（timeline）→ 再落审计 → 再清热层**（反序就会丢东西，同 `hot` 的口径）。
队列的边界是**前缀**不是 `status`：热层里同时躺着「进行中的任务」，只靠 status 分不出来；
读不懂的态**照列**，静默丢掉的那一条在审批人眼里就是「压根没提过」。

**身份**：`open_id`（人 ↔ agent 绑定现算），**不新造身份体系**；看板侧不因此加一道校人 ——
先留痕，判断交给 agent。

**这一刀没做的**（**2026-10-10 第二刀已补掉一半**：看板那条已接，见 §7.1.35）：
两个嘴当时都没接 —— 看板（待批区 + 按钮）与飞书卡片（出站卡片 + 长连接收 `card.action.trigger`）。

**实测**（沙箱 Linux + 真 Redis 7.0.15，副本库 `vault-appr`）：

| 判据 | 结果 |
|---|---|
| 队列进得去出得来 | ✅ 入队 1 条 → 点头后 `ls` 空 |
| 痕两笔 | ✅ `timeline/2026-10.decision` + `audit/…invoke`，`case` = 提案 id 串起三段 |
| 非提案不进队 | ✅ `ask` 被拒（`exit 1`），队列仍只有那一条 |
| 重复点被拒且不再落痕 | ✅ `exit 1`；落库那行**字数没变** |

证据 `<沙箱>/logs/2026-10-10-审批落点第一刀.log`；质量门 `gofmt` / `go vet` / **17 包全绿**。


### 7.1.35 看板这条嘴：待批页 + 唯一写口（2026-10-10，W4 余项第二刀）

§7.1.34 定的两个落点，**看板这一处已接**：`#/approvals` 这一页（导航排在「总览」后面 ——
整块看板只有它等着人动手，而「有没有卡在等我」正是「现在有没有事」的下一句）。

**写口只有一处**：`POST /api/approvals/decide`。它不是「把看板改成可写」——
SPEC §7：CLI / 对话式 bot / 看板是**同一条变更管道的三个前端**；这条路走 `internal/approvals`
的同一个入口、同一份留痕（`timeline` 一条 + `audit` 一条），**不写真相源**。

| 做 | 为什么 |
|---|---|
| 只收 `POST` + `application/json` | 跨站表单发不出这个 Content-Type（浏览器先发预检，而看板不回 CORS 头）—— **没有 token 也挡得住「别的网页替你点头」** |
| 请求体限 64KB | 管道就该有口径，不是等出事再补 |
| 方法在**中间件**判，不留给处理函数 | `GET` 那个写口会落到处理函数上、报出「Content-Type 不对」的**假原因**（真原因是动词不对）—— 用例当场抓到的，见下 |
| 不校「点的人是不是该批的人」 | 同 §7.1.34：先留痕，判断交给 agent |
| `by` 由页面出（记在本机） | 看板没有免登；飞书卡片那条嘴不需要它 —— 长连接事件自带 `open_id` |

**契约**：`/api/approvals`（`anc.approvals/v1`）出 `wired` / `why` / `write_path` / `signals[]` /
`count` / `pending[]`。两个与别处不同的点：

- **`write_path` 由服务端给**，前端不另写一份常量 —— 否则改一次路由要两边各改一遍；
- 待批**转一层 `PendingView`**（snake_case），**不直接端领域类型** `approvals.Pending`：
  端出去就成了「改个字段名会静默改契约」的隐式契约。

**前端**（`boardui/src/app.ts`）：按钮**照 `signals[]` 画**，不写死那三个词（以后加词只改服务端）；
`by` 空**先在本地拒**（后端也这么拒，省一次往返）；点下去先按住按钮（连点两下，第二次必然
「不在队列里」）；成功后的回执把两个落点摆出来，并明说**真相源没动**。

**实测**（沙箱 Linux + 真 Redis 7.0.15，副本库 `vault-demo`，两封提案在队）：

| 判据 | 结果 |
|---|---|
| 只读那一路：`GET /api/approvals` | ✅ `wired=true`，两条待批，`write_path` / `signals` 都在 |
| 写口上 `GET` | ✅ 405 + `Allow: POST`（**修的就是这条** —— 原来是 415） |
| 跨站表单那一路 | ✅ 415（`application/x-www-form-urlencoded` 发不出去） |
| 认不出的信号（`yes`） | ✅ 400，`hint` 写「只认 approve / reject / hold（不猜）」 |
| 匿名点头（不带 `by`） | ✅ 400「匿名点头等于没点头」 |
| 真点头 | ✅ 200 + 回执；落点已摘本机路径（`<vault>/…`），`note` 说清真相源没动 |
| 再点一次 | ✅ 409（不许一次点头变两条痕） |
| 痕两笔 | ✅ `timeline` 一条 `decision`（`case` = 提案 id）+ `audit` 一条 `invoke` |
| CLI 与看板同一份队列 | ✅ 点完 `anc approvals ls` 空 —— 不是一个前端一套状态 |
| 前端真渲出来 | ✅ Edge 无头 `--dump-dom "#/approvals"`：两条待批、`manager` / `devbot`、`已等 2 小时`、三个按钮 |
| 前端真点得动（CDP） | ✅ 填 `by` → 点「同意」→ 回执出现（带两个落点 + 「真相源没动」）→ 队列 2 条变 1 条 |

证据 `<沙箱>/logs/2026-10-10-看板写口.log` + `…-待批页DOM.html`；质量门 `gofmt` / `go vet` / 全包用例全绿。

**这一刀没做的**：飞书卡片那条嘴（出站 interactive 卡片 + 长连接收 `card.action.trigger`）；
看板**没有免登**，所以 `by` 靠填 —— 身份那一层等 `panel` 客体（§6）落地，代码里不先钉死。

### 7.1.36 审计落盘前强制脱敏：抹成指针，不是抹成空白（2026-10-10，议题 #50）

**起因是真泄漏，不是假想**：`runtime/internal/audit/collect.go` 把工具结果头部塞进记录的 `why`
字段，全包 grep `redact|脱敏|mask|scrub|sk-` **全空** —— 没有任何脱敏。2026-10-07 那次真会话里，
bot 读了 `~/.claude/settings.json`，当时在用的 `ANTHROPIC_AUTH_TOKEN` 明文就跟着进了
`audit/<月>.<行使者>.jsonl`（append-only，删不干净）。这与本文 D4 那句「config / 备份 / diff
**全程无明文**」直接冲突 —— 当时那句话只管到了装载产物，没管到审计流水。

**落点选在 `Append`，因为它是全项目唯一的审计写口** —— 归集（`audit collect`）／补记（`audit add`）／
信封关卡（`envelope.go`）／读出口（`envelope_read.go`）／审批留痕（`internal/approvals`）
五条路都从这里过。「落盘前」这四个字才有唯一的意思；散在各命令里做，早晚漏一条。

| 口径 | 为什么 |
|---|---|
| **没有开关** | 脱敏不是门禁，是无条件的内容变换。留一个 off 开关等于给「明文落盘」留一个合法写法 —— 所以没有 policy 条目、没有 `--no-redact`。要改行为改的是**规则表** |
| **规则是数据** | 同 `ToolRule` 的理由：加键名 / 加形态改的是表，不是 if。第二处来源是凭据文件自己的键名（见下） |
| **抹成指针** | 写 `«已脱敏:<规则名>»`，**不是空字符串**：读的人必须还能看出「这里有过一个东西、它是哪一类」。静默抹平与假绿是同一类错误 |
| **只碰三格** | `object` / `why` / `detail` —— 只有它们可能夹带原话。`actor` / `action` / `result` / `tool` / `at` / `id` 是词表与标识，抹了审计就没法按人按类别筛 |

**三条腿**（`internal/audit/redact.go`，29 条出厂规则 ＋ 凭据文件带进来的那些）：

1. **已知值字面替换** —— 值来自 `~/.anc/secrets.env`（与 `anc apply --secrets` 同一个默认路径、同一个
   `apply.ParseSecrets` 解析器）。这是唯一挡得住「没有形态、没有键名」的那一腿。**值永不出口**：
   只报条数，不打印。
2. **键名档** —— 键名整词匹配，抹它后面的值（JSON 双引号串 / 单引号串 / 裸串三种写法）。`RulesForKeys`
   把凭据文件里**真实存在的键名**也变成规则：ANC 自己的键名是全函数派生的
   （`ANC_FEISHU_SECRET_<成员>`），出厂表写不全；而且**值是会换的** —— 靠字面值匹配只挡得住「读到的那一次」。
3. **形态档** —— 前缀原样保留、后面那串抹掉（`sk-` / `Bearer ` / `Basic ` / `ghp_` / `xoxb-` / `AKIA` / `eyJ` …）。

**两个刻意的设计点**：

- **整词匹配挡计量口径**：`token` 这条规则命中不了 `input_tokens` / `output_tokens` / `max_tokens`
  （那些词里 `token` 前面是词字符 `_`）—— 那些是用量口径，抹了审计就瞎了。实测第 5 例就是它。
- **跑到不动点**（上限四趟）：一次命中会「揭开」下一层。实测的例子是
  `Authorization: Bearer sk-xxx` —— 键名档先把 `Bearer` 当值抹了，形态档就再也够不到后面那串，
  于是密钥被「拆」在外面。形态档先跑 + 跑到不动点，两头都堵上。

**读口也过一遍**（兜底）：这份代码落地之前**已经躺着**的明文，保证它不再从 CLI / 看板漏出去；
同时把位置记进 `Doc.Tainted`，`anc audit log` 如实报「有 N 行**落盘时就是明文**」并说明
处置是轮换密钥 + 清理文件（**那是人做的事**，代码只指位置 —— 不能因为「显示干净」就当它不存在）。

**实测**（本机 Windows ＋ 真凭据文件 `~/.anc/secrets.env`，1 条已知密钥值）：

| 判据 | 结果 |
|---|---|
| `anc audit rules` | ✅ 30 条规则 ＋ 1 条已知值；`ANC_FEISHU_SECRET_ALICE` 由凭据文件自动带进来 |
| 真判据：`anc audit collect --write` 落盘后搜密钥值 | ✅ **0 命中**（50 条新记录） |
| 同一条泄漏原文走 `anc audit add --why/--detail` | ✅ **0 命中**；抹成 `"ANTHROPIC_AUTH_TOKEN": "sk-«已脱敏:sk»"`、`Authorization: «已脱敏:authorization» sk-«已脱敏:sk»` |
| 读口兜底：`anc audit log`（老的真污染文件） | ✅ 输出里 **0** 密钥值、有占位符；并报出 `2026-10.alice.jsonl:1` 落盘时是明文 |
| 不误伤：`input_tokens: 12345 output_tokens: 6789` | ✅ 原样不动 |
| 不误伤：`${ENV}` 引用（**含裸写的 `K=${VAR}`** —— 2026-10-10 补掉的一处漏伤） / 空值 / 短于 4 字的值 | ✅ 原样不动（引用本身就是指针） |
| 质量门 | ✅ `gofmt -l` 无输出 · `go vet ./...` 无输出 · `go test ./... -count=1` **17 包全绿** |

**要诚实说清的一件事**：拿今天的读取器跑那次「读 settings.json」的会话，`anc audit collect` 落到的
`why` **本来就是空的** —— claude 那条读取器现在只给**失败**留原话（成功结果不留），所以 #50 那条判据
在当前代码下会「巧过」。脱敏不是靠这个巧合成立的：为了证明它本身生效，同一条泄漏原文是**另走
`anc audit add --why`**（同一个 `Append`）打进去验的。

**老明文的清理（2026-10-10 收口）**：原先这里记的是「① 已落盘的老明文没有清理」—— 现在做了，口径
是**改名归档 ＋ 抹过的替代**（动 append-only 的流水是改历史，所以原件的字节一个不改，只是不让它再待在
活跃路径上）。范围＝全盘只读扫描命中的 4 个载体：

| 载体 | 处置 |
|---|---|
| `anc-demo/vault/audit/2026-10.alice.jsonl`（27 行，第 1 行） | 归档 ＋ 替代 |
| `ANC沙箱/trail-e2e/vault/audit/2026-10.alice.jsonl`（27 行，第 1 行） | 归档 ＋ 替代 |
| `_tmp/audit-sb1/vault/audit/2026-10.alice.jsonl`（29 行，第 3 行） | 归档 ＋ 替代 |
| `_tmp/collect.json`（482 行，第 20 行） | 归档 ＋ 替代 |

四处是**同一条痕**（2026-10-07 那次「读 `~/.claude/settings.json`」把整篇文件内容写进了 `why`）。
归档名 = `<原名>.tainted-2026-10-10`（**不以 `.jsonl` 结尾** —— 加载器只认这个后缀，归档件因此不会被
当成活的流水读进来）；替代件是把原文**过同一张脱敏表**重写的（走 `internal/audit` 的
`NewRedactor/Apply`，**不另写一份正则**），在这台机器上那一段变成 `"ANTHROPIC_AUTH_TOKEN": "sk-«已脱敏:sk»"`
（本机 `~/.anc/secrets.env` 里没有这条键，所以是**形态档 `sk-`** 吃掉的，不是键名档 —— 换成凭据桥里
有这条键的机器，抹出来的是 `«已脱敏:ANTHROPIC_AUTH_TOKEN»`；**占位符的名字取决于当地那张表**，这是设计）。
实测：活跃路径密钥值 **0 命中**、归档件仍留原件、四份都仍是合法 JSON、逐行 diff **只有那一行**、
`anc audit log` 的「落盘时就是明文」告警**归零**。

**别把归档件当成「干净了」**：归档件里**仍是明文**（那正是「归档原件」的意思），所以真正的解是
**轮换 `ANTHROPIC_AUTH_TOKEN`** —— 轮换之后归档件里那串就是死凭据。归档件要不要进一步抹／删，等轮换后再说。

**这一刀没做的**：① 已知值的来源只有 ANC 自己的凭据文件，客户别处的明文靠形态档与键名档兜；
② 脱敏原本只覆盖 `audit` 这一条流水 —— `timeline` / `envelope` 日志没有同类变换。**已经补上（2026-10-10，议题 #64）**，见 §7.1.40。

**踩到并记下的两个坑**（都是「重建时把原文拆成捕获组再拼回去」这一类）：

- 键名档最初**没把键名捕成组** → 重建时把键名吞掉，`"KEY": "值"` 变成 `"": "«已脱敏:KEY»"`；
- 形态档最初**没把前缀捕成组** → `sub[0]` 是整段命中，`sub[1]` 拿到的是 token 而不是前缀，
  于是**形态档一次都没命中过**（而 `go build` 与既有用例全绿）。

- **裸串那一支的字符集排掉了 `}`**（不排掉它，`{"k":"v"}` 的收尾大括号会被当成值的一部分吃掉），
  于是**裸写**的 `K=${VAR}` 被截成 `${VAR` —— `redactable` 认不出它已经是个指针、照抹，还留下一个
  孤零零的 `}`（实测：`token=${ANTHROPIC_AUTH_TOKEN}` → `token=«已脱敏:token»}`）。修法是给值那一支
  补一条「整条 `${NAME}` 指针」的写法，排在裸串支之前。**这一处是用例抓出来的** —— 之前只试过带引号的
  `${ENV}`（它走双引号串那一支，恰好是好的）。

前两次都是先被「拿真数据看一眼抹成了什么」抓住的，不是被用例抓住的；第三次（`${ENV}`）反过来，是用例抓出来的。

### 7.1.37 审计认不出的客体：引用形状先认下来（2026-10-10，议题 #63）

**起因是读出来的结论是假的**：`gate` 的留痕写的是**引用形状** —— `runtime/envelope.go` 的
`auditEnvelope` 写 `object = "domain:" + 域slug`；而出站读出口（`envelope_read.go`）与审批留痕
（`internal/approvals`）写的是**裸 slug**。但 `derive.go` 的落点分类**只认路径形状**，
两条都认不出。同一批流水，改前改后读出来是两个结论：

| `mcp-test` vault 的 9 条真流水 | 改前 | 改后 |
|---|---|---|
| 落点 | 域内 0 · vault 内非域 4 · 域外 0 · 目标抽不出 5 | 域内 4 · vault 内非域 0 · 域外 0 · 目标抽不出 5 |
| 其中跨域 | **0** | **3** |

「一条真跨域被读成 vault 内非域」正是本文反复点的那一类错误：把**判错 / 判不出**画成**没事**。

**`Owner` 三档，按「越明确越先」**：

1. **引用形状** `domain:<slug>` —— 词表的唯一定义处是 `org.SplitRef`（grants 的 `object` 认它，
   见 `internal/envelope/gate.go` 的 `objectIsDomain`）。审计**不另造一套引用语法、不自己解析冒号**
   （#43 第 4 点划下的边界）。**表里没宣过的 slug 也算命中这一档**：是不是域，由**写法**说了算。
2. **路径形状** —— vault 下的顶层目录 → `domains.md` 的 `data` 列（原来唯一认的那种）。
3. **裸 slug 兜底** —— 客体既不是路径、又不含 `/`、又正好是一个**已声明**的 slug。为什么要有这一档：
   出站读出口与审批留痕写的就是裸 slug，而域的 `data` 目录名**未必等于** slug
   （实测里 `slug=trade / data=projects`），只按路径认会把真跨域算成非域。

**「是不是域」与「这个域还在不在表上」分成两件事**（`Owner` / `Declared`）：域表是会改的，
老流水里会留下指着一个已经不在表上的域的行。那种行**仍然是「域」**（写口写的就是这个形状）、
`Actee` 留着让人看见它指的是谁，但 `Known=false` → 页面写「**跨域未判**」。
**不许**落进 `vault` / `域外` 那两格 —— 那等于替一个判不出来的东西说「没事」。
代价是「域内」这一格里会多算一条这种行；它的行面上明写着「跨域未判」，
比把它算进「vault 内非域」诚实。

**刻意不解**别的引用形状（`member:` / `role:` / `project:`）：四类落点里没有它们的位置，
落进「vault 内非域」是一条**已知的、如实留着的缺口**（今天也没有任何写口把这种形状写进 `object`），
等真出现再定 —— 先替它猜一个才是错。

**实测**（真 vault：`slug=trade/logistics`、`data=projects/shipments`，alice→trade / bob→logistics）：

| 判据 | 结果 |
|---|---|
| `--object domain:trade`（bob 在 logistics） | ✅ 域内 · **跨域** |
| `--object domain:logistics`（alice 在 trade） | ✅ 域内 · **跨域** |
| `--object domain:ops`（表里没有这个 slug） | ✅ **「ops（跨域未判）」** —— 不再冒充「vault 内非域」 |
| `--object logistics`（裸 slug，目录名是 `shipments`） | ✅ 域内 · **跨域** |
| 用例 | `TestDeriveZones` 补 4 条 ＋ `TestOwnerTakesRefShapeSeparatelyFromWhetherTheSlugIsDeclared` |
| 质量门 | ✅ `gofmt -l` 无输出 · `go vet ./...` 无输出 · `go test ./... -count=1` **17 包全绿** |

### 7.1.38 平台身份 → org 里的谁：三样各查一遍（2026-10-10）

**要解决什么**：平台的嘴（cc-connect）递过来的**不是编号**，是三样别的东西 ——
谁发的（`open_id`）、发到哪个 app（`app_id`）、落在哪台 bot 上（project 名）。
而审计流水 / 审批 / 授权里说的是人 / 岗位 / 域 / project。中间少这一层，
「这条审批是谁提的」「这个 agent 属于谁」就得各处自己拼。

**落点**：`internal/org/identity.go` —— 在已有的 `Identity(ref)`（按编号取主体）旁边补三个入口
＋ 一个归属判定。**只查表，不猜**：查不到就如实 false（同 `Identity(ref)` 的口径：
判不出来 = 这个主体不存在，调用方的拒话照这个口径写）。

| 入口 | 认什么 | 只能落在 |
|---|---|---|
| `ByOpenID` | 平台里唯一的「人」标识 | 成员 —— 业务 agent 不代表人，没有 open_id |
| `ByAppID` | 这条消息发到哪个 app | 成员 bot / 业务 agent 二选一（共用同一套接入，SPEC §4.7 ③） |
| `ByProject` | `<公司 id>-<名字>`；不带前缀的裸编号也收 | 成员 bot / 业务 agent |

**「属于谁」只答归属，不答收件**（`Owner`）：成员返回空 —— 人没有归属人（他属于他自己，
这是第一性的）；业务 agent 走 `域 → domains.md 的 who（岗位） → 该岗位上启用中的成员`，
与渲染器算**收件人**的第一段是同一支判据（`render.agentAllowFrom`）—— 两处各算各的，
迟早出现「消息发给甲、看板说归乙」。但域 who 岗位上没人时，渲染那边会落到公司 admins 兜底
（不兜它就没人叫得动这个 agent），那是**收件**口径；混进归属就等于宣称「没人的域归老板所有」。
所以这一格**空着并说明原因**，返回的第二个值就是那句判据原话。

**拼法只有一个定义处**：`<公司 id>-<名字>` 从 `render` 挪到 `org.ProjectName`
（它是 org 模型的事实，SPEC §4.7 ③），`render.ProjectName` 改成同一支的导出 ——
否则审计 / notify / render 三处各拼一遍，早晚出现「看板叫 alice、日志叫 demo-alice」。

**嘴**：`anc org who <vault> [--open-id …] [--app-id …] [--project …] [--json]`。
两头各自解、**互不代偿**：一头解不出就如实说（带原因），不拿另一头顶上。
`--app-id` 与 `--project` 同时给时以 app_id 为准（它更贴平台事实）。

**实测**（真 vault：`alice` = `ou_demo_alice` / `cli_demo_alice`，`bob` / `devbot` 三位成员，
外加一张 `agents.md` 里两个业务 agent）：

| 问 | 答 |
|---|---|
| `--open-id ou_demo_alice` | ✅ 人 alice（Alice Wang）· 岗位 manager · 域 trade · 这台 demo-alice |
| `--app-id cli_demo_bob` | ✅ 人 bob（Bob Li）· 岗位 ops · 域 logistics |
| `--app-id cli_demo_tradebot` | ✅ 业务 agent tradebot · 铭文「替 manager 岗盯进出口合同的签订与执行」· **属于 alice**（凭什么：域 trade 的 who 岗位 manager —— 经理（Alice Wang）） |
| `--app-id cli_demo_ghostbot`（domain 写成不存在的域） | ✅ **解不出归属**，并说明「域 nope 不在 domains.md 里」 |
| `--open-id ou_stranger` | ✅ **解不出**，并说明「在 members/ 里没人认领 —— 自报的身份不算」 |
| `--project demo-alice` / 裸 `alice` | ✅ 同上；`other-alice` 解不出（前缀要对齐本公司 id） |
| 用例 | `internal/org/identity_test.go` 5 条 —— 含两条反例：**归属不落到 admins**、**停用成员不算归属** |
| 质量门 | ✅ `gofmt -l` 无输出 · `go vet ./...` 无输出 · `go test ./... -count=1` 17 包全绿 |

**这一刀没做的**：① 飞书卡片那条路当时还没接（在飞书里点「批」→ 回调 → 同一个信号入口）——
**2026-10-10 已落**，见 §7.1.39（含「递出去那一步卡在上游没有那个口」的实测）；② `anc org who`
**只解身份、不做判断** —— 「谁能批、谁不能批」是授权层的事，这里一个 if 都不加。

**踩到的一个坑（值得单独记一笔）**：动手时**覆盖了一个已经存在的同名文件**
（`internal/org/identity.go` 早在 `29ab833` 就有了，里面就是「主体」那套）。
是 `git diff` 里出现 54 行删除才发现的 —— 编译器只能拦住「同名函数」，拦不住「整个文件被换掉」。
写新文件之前先查它在不在，这一步不能省。

### 7.1.39 飞书卡片：帧是数据、值是实时业务（2026-10-10，W4 余项第三刀）

**起因是 human 把方向顶住了**（原话）：「飞书卡片我们是**根据实时业务变更**的，所以你不能把它给写死了。
甚至出一个**粗糙框架**就可以，就是像按键之类的；但里面**的键、值都需要根据实时业务去填**，
不能写死 —— 要不然要我们的 ANC、要我们这么多 agent 都是废物吗？」

这句话否掉的是「把审批卡片画成一张固定页面」那条路（也就是 §7.1.35 看板那一刀的形状）：
看板那一页是**一个**前端、**一种**画法；卡片要出到 IM 里，收的人、要看的字段、能点的键
**每个客户都不一样**。所以卡片这一层分两块：

| 是什么 | 住在哪 | 谁改 |
|---|---|---|
| **帧**（长什么样：标题 / 几段字 / 哪几列 / 哪几个按键） | 一份 JSON（`--frame` 指过去；没有就用出厂帧） | **客户自己**，不用重编译 |
| **值**（此刻要显示的东西） | 由调用方**当场算**（热层那条提案 + 真相源 + 钟） | 业务自己动，代码不掺和 |

出厂帧在 `internal/card.Default()` —— 它是**数据**（一个 Frame 字面量），不是分支。
`anc approvals card --dump-frame` 把它打出来，拿走改、`--frame` 指回来就是新卡片。
**槽位缺了就喊**：帧里写 `{{x}}` 而值里没有 `x`，那一格渲染成「（没这个值：x）」并回一条 Note ——
空与「本来就没有」在卡片上分不出来，静默变空是最难查的那种错。

**槽位键是这一处的契约**（`approvals.Facts`）：`id / title / body / status / enqueued / waited / tone /
company / domain / project / target / to / submitter / submitter_kind / submitter_owner / approver`。
其中四个是**只有 ANC 答得出来**的（也是这一刀真正要的东西）：

| 键 | 答什么 | 判据 |
|---|---|---|
| `submitter` | 谁提的 | `org.Identity(who)` —— 名字 + 类别（成员 bot / 业务 agent）+ 铭文 |
| `submitter_owner` | **这个 agent 属于谁** | `org.Owner(id)`：成员「他属于他自己」；业务 agent 走域 who 岗位 → 启用中的人 |
| `approver` | 该谁批的是**哪个人** | `org.WhoLabel(域表 who)` —— 岗位是队列里的值，人名现算 |
| `tone` | 该什么颜色 | `timeline.Band(status)` → 飞书那套颜色名。待批 = 卡点需介入 = **红** |

**颜色只此一处说了算**：`pending` 落红是加在 `timeline.bandOf` 那一张表上的
（它也是 approvals 自己写进热层的词），不是卡片这一层另立一套判据。

**回程**（`anc approvals card-action`）：一次按键 = 一条**可识别信号**。
按键的 `value` 里只放两个键（`anc_id` / `anc_signal`，常量定义在 `internal/card`），
到这一侧才解析 —— **两个键都在才算数，缺一个不猜**，认不出的信号一律拒（同「只认三个字」）。
然后把 `open_id` 走 `org.ByOpenID` 定位「谁点的」，`--app-id`（网关那一侧知道这条回调落在哪个 app）
走 `org.ByAppID` 定位「哪台 bot、它属于谁」，最后走**与 CLI / 看板同一个信号入口**
（`approvals.New` + `approvals.Settle`）落两笔痕。承接口径一条没改：不校该不该他批、不写真相源、先落库再清热层。

**`by` 取编号身份，不取显示名**（实测当场抓到的）：拿显示名当 `by`，两份痕的文件名会变成
`2026-10.Alice_Wang.jsonl`（timeline 的 `safeName`）与 `2026-10.Alice Wang.jsonl`（audit 的 `ShardName`，
**带空格**）—— 两个写口两把尺子，而且换一次显示名历史就断成两截。改成 `alice` 之后两边同一条分片，
人话名字进 `Via`。

**新增 `Decision.Via`**：这一次点头**是怎么来的**（`飞书卡片 · 点的人 alice（Alice Wang · ou_demo_alice）
· 落在 tradebot（业务 agent），属于 alice`），缀在审计那一行的 `detail` 后面。空就不写 ——
不写「（无）」：一行没有来源说明的行，比一行写着「来源：无」的更容易看出是没记。

#### 上游那一侧：递不出去，是**没有这个面**（实测，不是没接）

对着 cc-connect v1.3.4 的源码逐条核过（`https://github.com/chenhg5/cc-connect`，克隆到本机核的）：

| 找什么 | 结论 | 证据 |
|---|---|---|
| 外部工具发卡片 | **没有这个口**：unix socket `POST /send` 的请求体只有 `message` / `images` / `files` / `audios` / `videos` / `at_*` | `core/api.go` 的 `SendRequest` 与 `handleSend` |
| 卡片是谁发出来的 | 引擎**进程内**给自带命令渲染的（`/status` `/help` `/model` `/dir` 那一类） | `core/engine.go` 里 `replyWithCard` / `sendWithCard` 的几十个调用点 |
| 飞书回调里有没有「谁点的」 | **有，但没往上游递**：`event.Event.Operator.OpenID` 取了，只用于 allow_from 校验；`cmd:` 前缀会把点击变成「该人发的一条消息」（这条 identity 是保住的） | `platform/feishu/feishu.go:799-812`、`onCardAction` 的分支 |
| 认不出的 `action` 值 | **直接丢掉**（`nav:` / `act:` 走它自己的 navHandler，认不出就 return nil） | 同上 |
| bridge 协议那条路 | 出站有 `card`、入站有 `card_action`，但那是**给平台适配器**的协议；而且 `card_action` 载荷**不含 operator**（谁点的丢在适配器那一侧） | `docs/bridge-protocol.md`、`core/bridge.go` 的 `bridgeCardAction` |

所以上游 PR 要的是两件小事（都不动它的架构）：① `/send` 加一个可选的 `card` 字段（把 ANC 渲染好的
卡片 JSON 原样递给平台）；② `onCardAction` 把**认不出的 action**（以及 `Operator`）转出来。
**我们不自己连飞书 API** —— 换平台是网关的事（SPEC §4.7 ①），绕过去就是第二条网关。

#### 顺带核到的一条（留给「ANC 不能主动下发任务」那个议题）

同一个二进制里有一个**反向下发**的口：`POST :9111/hook`
（`core/webhook.go`：`{project, session_key, prompt|exec, payload}` —— 往某台 bot 的会话里塞一条
prompt 或一条命令，`payload` 会作为上下文拼进 prompt）。它就是 human 说的
「通过对应的飞书应用机器人，把这条任务传到对应的人/agent 的会话里」那条路。
**这一刀没动它**（不在本次范围），但它的存在意味着「下发」不必等飞书卡片那一半 —— 记在这里免得重复找。

#### 实测（沙箱 Ubuntu + 真 Redis 7.0.15，判据床 `mcp-test` 那份 vault；`tradebot` 挂 trade 域、who=manager=alice）

| 判据 | 结果 |
|---|---|
| 换一份帧：只两列 + 三个按键，标题用 `{{company}}` | ✅ 渲染出来是 `Demo Trading Co（demo） · 有人要权`，按键值跟着帧走 |
| 帧里写了个不存在的信号 `maybe` | ✅ 帧照渲（按键照出）—— **拒在回程那一侧**：`认不出的信号 "maybe"`，`exit 1`。口径不散在两处 |
| 坏帧：写错槽位 + `kind: 表格` + 按键没写 `signal` | ✅ 三条 Note 各自指到哪一块；那一格写「（没这个值：没这个键）」；认不出的块与没信号的按键**不渲染**（不猜） |
| 帧版本不对（`anc.card/v9`） | ✅ 拒绝并说明只认 `anc.card/v1` |
| `card --json` | ✅ 真 interactive 卡片 JSON（`config` / `header.template=red` / `markdown` + `div.fields` + `action.actions`）；按钮 `value` 里是 `anc_id` + `anc_signal` |
| 业务 agent 提的（`tradebot`） | ✅ 「谁提的：贸易助手（业务 agent · trade）：替 manager 岗盯进出口合同的签订与执行」；**「属于谁：alice（域 trade 的 who 岗位 manager —— 经理（Alice Wang））」** |
| 成员提的（`devbot`） | ✅ 「属于谁：成员没有归属人 —— 他属于他自己」 |
| 该谁批 | ✅ `manager · 经理（Alice Wang）`（队列里存的是岗位，人名现算） |
| 颜色 | ✅ `red`，并写明「从状态 pending 现算」 |
| 回程：`anc_signal: yes` | ✅ 拒（`exit 1`），队列没动 |
| 回程：没带 `anc_id` | ✅ 拒：「两个键都在才算数，缺一个也不猜」 |
| 回程：`ou_stranger` | ✅ **照收**（先留痕，判断交给 agent）：`by` = `ou_stranger`，`via` 里写明「认不出（在 members/ 里没人认领 —— 自报的身份不算）」 |
| 回程：alice 点 approve（`--app-id cli_demo_tradebot`） | ✅ `谁点的 alice`；`via` = `飞书卡片 · 点的人 alice（Alice Wang · ou_demo_alice） · 落在 tradebot（业务 agent），属于 alice` |
| 痕两笔 | ✅ `timeline/2026-10.alice.jsonl` 一条 `decision`（`case` = 提案 id）+ `audit/2026-10.alice.jsonl` 一条 `invoke`（`detail` 带 `· 经 …`） |
| 重复点同一条 | ✅ 拒（`ErrNotQueued`），不再落痕 |
| 队列 | ✅ 点完 `anc approvals ls` 少一条 —— 与 CLI / 看板**同一份队列** |
| 质量门 | ✅ `gofmt -l` 无输出 · `go vet ./...` 无输出 · `go test ./... -count=1` **18 包全绿** |

证据：`<沙箱>/logs/2026-10-10-飞书卡片.log`、`…-飞书卡片-帧.log`；帧样本 `…/e2e/my-frame.json`、`…/broken-frame.json`。

**这一刀没做的**：① 递出去那一步（等上游那个口，见上）；② 卡片上**填理由**（`why`）的入口 ——
现在是按钮点了就走，`why` 留给后面（要看客户真怎么用）；③ 卡片里嵌「提案原文之外」的活数据
（比如让 agent 把当时的上下文塞进去）—— 那要等上下文资产那一块，现在不焊。
④ 用例：本轮**只跑了实测**，`internal/card` 还没有测试文件（加测试要先跟 human 定判据，见 AGENTS.md 第 2 条）。

### 7.1.40 脱敏横向铺开：两条流水吃同一张表（2026-10-10 夜，议题 #64）

**§7.1.36 留下的那一格**：脱敏当时只落在审计写口。可“夹带原话”不止一处 ——
时间线的 `title` / `detail` 写的就是“当时说了一句什么”，信封日志的 `body` 是投递正文、`refs` 是带查询串的链接。
同一台机器上，一条流水抹了、另一条留着，等于没抹。

**不另造一套正则**：`internal/audit` 露出 `Scrub(s string) (string, []string)` —— 薄薄一层，
走的就是 `Effective().Apply`。理由：三条流水必须吃**同一张表、同一个占位符形状**（`«已脱敏:<规则名>»`）。
占位符的名字取决于**当地那张表**（出厂表 ＋ `~/.anc/secrets.env` 的已知值 ＋ 凭据文件的键名），这是设计。

**抹哪几格（口径）**：

| 流水 | 抹 | 不抹（为什么） |
|---|---|---|
| `timeline` | `title` / `detail` / `refs` | `case` / `by` / `role` / `to` / `domain` / `project` / `kind` / `status` / `id` / `at` —— 标识与词表（按 case 折叠、按人按域筛全靠它们），抹了就看不出“谁在推进哪一格” |
| `envelope` 日志 | `body` / `refs` / `needs` / `findings[].msg` | `id` / `ts` / `who` / `on_behalf_of` / `scope` / `kind` —— 标识与词表。`findings[].msg` 之所以算“可能夹带内容”：它是**判据文案**，而判据文案会把字段值原样引一遍（`domain="nope" 不在 domains.md 里`） |

**落点都选在写口**（与审计同一条理由：写口是承诺的兑现处）：
`timeline.Append` 落盘前 `scrub(&e)`；`envelope.Append` 落盘前 `scrub(&rec)`。

**读口只兜底，不改历史**：`timeline.Load` 也过一遍，但**文件一个字节都不动**（append-only 是这条流水的性质），
并把“**磁盘上这行就是明文**”的行如实报出来（`Doc.Tainted`）—— `anc timeline list` 照 `anc audit log` 的文案打出来。
这是 §7.1.36 的同一条口径：判错要能看见，不许画成没事。

**实测**（沙箱 Ubuntu，真 Linux 二进制 `anc64`，vault 副本）：

| 判据 | 结果 |
|---|---|
| 时间线写口：title / detail / refs 带 `sk-ant-api03-…` | ✅ 落盘后磁盘 **0 命中**，抹成 `sk-«已脱敏:sk»` |
| 时间线 `list` 不误报 | ✅ 刚落盘的行**不算**“落盘时就是明文” |
| 时间线读口：手写一行老明文 | ✅ 读出来干净；如实报 `⚠️ 有 1 行落盘时就是明文：2026-10.alice.jsonl:4`；**文件 sha256 前后完全相同** |
| 信封写口（真接入面 `envelope serve` ＋ `curl POST /mcp anc_send_envelope`） | ✅ 磁盘 **0 命中**；`body` / `refs` / `needs` 全抹成占位符 |
| 用例 | `internal/timeline` 4 条 ＋ `internal/envelope` 3 条 |
| 质量门 | ✅ `gofmt -l` 无输出 · `go vet ./...` 无输出 · `go test ./... -count=1` **18 包全绿** |

脚本：`_tmp/vm_rd64.sh`、`_tmp/vm_rd64mcp.sh`。

**这一刀没做的**：信封日志**没有读口**（全项目只有 `envelope.Append` 一个入口，看板与 CLI 都不读它），
所以那一侧没有“读口兜底”可做；真正的缺口只有“这份代码之前写下的行”，而那批行的时间线侧已经靠 `Tainted` 指出位置了。
业务 agent 的会话原文（`trail` 几条腿读的上游记录）**不在这一刀的范围内** —— 那是“上下文资产”那一块的事。

---

### 7.1.41 按键回程：真事件的形状与我们以为的形状不是一份（2026-10-11 凌晨，实测抓到的）

§7.1.39 那一刀的实测，回程那几条**是拿手写事件喂进去的**（`e2e/evt.json`：`event.operator.open_id`、
`event.action.value`（对象）、`event.context.open_chat_id`）—— 那是照**事件订阅**那份文档写的形状。
2026-10-10 夜把**人真点出来的那一份**原样抓下来（`_tmp/card-event-2026-10-11.ndjson`），两份**不是一份**：

| 找什么 | 事件订阅那份（我们原来认的） | 长连接递进来那份（**人真点出来的**） |
|---|---|---|
| 谁点的 | `event.operator.open_id` | 顶层 `operator_id` |
| 点在哪 | `event.context.open_chat_id` / `open_message_id` | 顶层 `chat_id` / `message_id` |
| 按键带了什么 | `event.action.value` —— **对象** | 顶层 `action_value` —— **一串 JSON 文本**（`"{\"anc_id\":\"…\",\"anc_signal\":\"approve\"}"`） |
| 原始卡片 | — | `card_content`（也是**一串 JSON 文本**） |

所以 §7.1.39 的回程**实测根本没跑通**：`ParseClick` 认不出这一份，`anc approvals card-action`
第一句就「这份事件里没有 action.value」（exit 1）。**不是判据错，是收件口只开了一个方向。**

**改法：宽进严出。** 进来的形状**两处都收**（外面那层 `event` 有就进去；顶层与 `context` 各认各的、谁空补谁；
`action.value` 与扁平 `action_value` 都认，后者是文本就显式解一次 JSON）
—— 出去那一步**一步不让**：仍旧**只认 `anc_id` / `anc_signal` 两个键**
（`Point()`），别的键一律当没带，认不出的信号仍旧拒在 `approvals.ParseSignal`。判据里一个字都没加：
宽的是「同一件事的几种写法」，严的是「点的是什么」。

**实测**（沙箱 Ubuntu ＋ 真 Redis 127.0.0.1:6379 ＋ `~/anc-vm2/vault`）：

| 判据 | 结果 |
|---|---|
| 人真点出来的那一份（扁平、原样，`e2e/evt-real-flat.ndjson`） | ✅ 收：`谁点的 alice`（真 `ou_8759…abb0` 现查定位）、`落在 alice（成员 bot）`、提案 `prop-card-1`、`决定 同意` |
| 痕两笔 | ✅ `timeline/2026-10.alice.jsonl` 一条 `decision`（`case=prop-card-1`）＋ `audit/2026-10.alice.jsonl` 一条 `invoke`（`detail` 带 `· 经 飞书卡片 · …`） |
| 重复点同一条 | ✅ 拒（`exit 1`，`ErrNotQueued`），不再落痕 |
| 老那条路没被弄坏 | ✅ `e2e/evt.json`（嵌套形状）照收；`evt-badsig.json` 照旧拒在「认不出的信号 yes」 |
| 形状矩阵 | ✅ 扁平文本 / 嵌套对象 / `action_value` 已是对象 —— 三种都收；两个键不齐、串不是 JSON、不是 JSON —— 三种都拒 |
| 质量门 | ✅ `gofmt -l` 无输出 · `go vet ./...` 无输出 · `go test ./... -count=1` **18 包全绿** |

**顺带坐实的一条**（本来只是读源码的推断：认不出的 `action` 值被丢掉）—— 它仍旧是上游那个 PR 的立论根据：
`/send` 要加一个可选的 `card` 字段、`onCardAction` 要把认不出的 action 与 `Operator` 转出来（§7.1.39 已记）。
本轮 demo 是拿**我们自己**的长连接消费者（`lark-cli event consume card.action.trigger`）把这一节跑通的 ——
它是**第二条网关**，只在上游那个口没有的时候临时顶着用，正式态不回退到它。

**这一刀没做的**：点完把卡片**收口** —— 那一刀在 §7.1.42（ANC 只备数据与位置，不自己去 patch）。
**用例**（判据先跟 human 对过再写，AGENTS.md 第 2 条）：`internal/card/card_test.go` **7 条** —— 3 条收形状
（扁平文本 / 嵌套对象 / `action_value` 已是对象）、3 条认不出就拒（没 action / 串不是 JSON / 不是 JSON）、
1 条只缺 `anc_id`（解析收下、`Point()` 点不出）。**只钉解析这一圈** —— 判据本身（该不该他批、信号认不认）不在这里测。
样本住在 `runtime/testdata/card/`（**一处收着，不进正式文件**；真回调那份脱敏后原样保留键与值类型）。

### 7.1.42 收口那一帧：帧还是数据，收口那一步不由 ANC 干（2026-10-11）

human 原话：「这个同意的话…我点完之后它还是可以再点击的这就不行。」
所以一次点头之后那张卡要**收口**：换成一张终态帧、按钮撕掉、连点幂等。
这一刀只落 **ANC 这半边**：

| 那一半 | 谁干 | 落在哪 |
|---|---|---|
| 终态帧长什么样 | **数据**：出厂 `approval-done`（`--done-frame` 可以换成客户的） | `internal/card.DefaultDone()` |
| 终态帧的值 | 现算：提案那几格（与待批帧同一套键，帧可互换）＋ 这一次点头（`verdict` / `by` / `via` / `at`） | `approvals.DoneFacts()` |
| 颜色 | **跟着落库那一行**（`Decision.TimelineEntry` 的 status → `timeline.Band`）—— 收口卡不另编一套色 | `approvals.Tone` |
| 递出去 / 覆盖那条消息 | **不归 ANC** | `card-action --json` 里的 `done_card` ＋ `chat_id` / `message_id` |

**「没有按键」是帧说的，不是代码判的**：出厂收口帧里根本就没有 `buttons` 那一块。
（「不该他点」那类判据仍旧不在卡片这一层。）

**为什么 ANC 不自己 patch**：§7.1.39 那条口径没变 —— 我们不自己连飞书 API（换平台是网关的事）。
网关拿到这两样，两条路都能收口，而且都不用多一个凭据面：
① 把 `done_card` 当**这一次回调的应答**返回（平台侧就地换卡 —— 上游 cc-connect 的 `perm:` / `cmd:` 分支就是这么换的，
`platform/feishu/feishu.go:890-930`）；② 拿 `message_id` 覆盖同一条消息。
**连点幂等归递出去那一侧**（同一条消息覆盖就是幂等）；ANC 这一侧重复点本来就拒（`ErrNotQueued`）。

**实测**（沙箱 Ubuntu ＋ 真 Redis ＋ `vault`，**真回调原样**那份事件）：

| 判据 | 结果 |
|---|---|
| 人话 | ✅ 多两行：`收口帧    出厂帧（编译进去的那一份）` ＋ `收口      卡是谁递出去的，谁拿终态卡…覆盖` |
| `--json` 回执 | ✅ 多了 `chat_id` / `message_id` / `done_frame` / `done_card`（终态卡：`header.template=green`、标题 `同意 · member:alice 申请 trade 的权`、**没有 action 块**） |
| 用例 | ✅ `internal/card` ＋3 条（收口帧没按键 · 出厂两份 · 按名字取得到）；`internal/approvals/card_test.go` **3 条**（颜色跟着落库那一行 / 结论进标题且没按键 / 槽位缺了要喊） |
| 质量门 | ✅ `gofmt -l` 空 · `go vet` 空 · `go test ./... -count=1` **18 包全绿** |

证据：`~/anc-vm2/bin/anc-done`（沙箱那份二进制）、`e2e/evt-real-flat.ndjson`。

**这一刀没做的**：网关那一侧（把 `done_card` 当应答返回 / 覆盖消息）——
上游 cc-connect 现在既不发卡也不领未知 `action`（§7.1.39 那个 PR 记着），
demo 里这条路是靠我们自己的长连接消费者跑通的，所以卡片真正「点完就灭」要等网关那个口。
判据一条没改：卡片这一层从不读写真相源，也不判该不该他点。

### 7.1.43 收口的样子是「变暗」，不是「撕掉」（2026-10-11，收 §7.1.42 的尾）

§7.1.42 把收口落成「终态帧里没有 `buttons` 那一块」—— 按键**直接消失**。human 在飞书上真点之后这条被否了，
原话：

> 「它没有变暗，我点击选项框之后，这应该是一次性点击的东西。那么三个选项框都应该变暗才对。
> 之后再再也点击不了才是正确的方式。但是这里我们没有实现，就会造成信息错误失真。」

**为什么「撕掉」是错的**：收口卡本来就是给人回看的（谁点的 / 怎么来的 / 什么时候），
把按键撕掉之后，卡面上再也看不出**原来有哪几个选项** —— 等于从证据里抹掉一层。
**变暗**把这一层留住，同时把「还能再点」关掉。

**改了什么**（只加语法与出厂数据，判据一个字没加）：

| 那一格 | 改成 |
|---|---|
| 帧语法 | `card.Button` 加 `Disabled bool`（`json:"disabled"`）—— **帧说的一句**，不是判据；`CardButton` 同步带上 |
| 出口 | `FeishuJSON` 把 `disabled: true` 写进那份 JSON（飞书 v1 的按钮认这个字段：变暗 ＋ 平台不再回传点击） |
| 出厂收口帧 | `DefaultDone()` 加回 `buttons` 那一块：三键**都在**、都 `Disabled: true` |

**回不了头仍有两处，都不在卡片这一层**：① 平台侧 `disabled` 之后不再回传点击；② ANC 侧那条提案已经不在队列里
（重复点本来就拒，`ErrNotQueued`）。§7.1.42 那条口径没变：**递出去 / 覆盖那条消息不归 ANC**（那个口在议题 #65）。

**实测**（沙箱 Ubuntu，真事件 `e2e/evt-real-flat.ndjson`）：

| 判据 | 结果 |
|---|---|
| `--json` 回执 | ✅ `done_card` 里三个 `actions[]` 都带 `"disabled": true`（`header.template` 仍是绿、标题带结论） |
| 平台上真变暗 | ✅ 拿这份 `done_card` 手工 `PATCH /open-apis/im/v1/messages/:message_id` 就地更新那条测试卡，平台回 `ok:true` —— **手工一次、验渲染**，不是产品代码自己连飞书 API |
| 用例 | ✅ `internal/card`：原「出厂收口帧没有按键」换成「三键都在但全禁用」；新增「禁用按键带 `disabled`」（钉出口）；`internal/approvals/card_test.go`：原「没有按键」换成「三键全禁用」 |
| 质量门 | ✅ `gofmt -l` 空 · `go vet` 空 · `go test ./... -count=1` **18 包全绿** |

**这一刀没做的**：「生成终态卡 → 覆盖消息」这一串还没**自动**跑起来 —— 网关那一侧见议题 #65。

### 7.1.44 第二条网关：relay 长在 ANC 这一侧（2026-10-11，收 §7.1.39 / 议题 #65 的尾）

human 拍了「先做第二条网关（仓库正式件），上游 PR 并行推」。第一件事是把**上游那一侧到底有没有这个口**核死 ——
不是读文档，是对着 `chenhg5/cc-connect` 的 `origin/main`（`dfad1941`）源码逐条看：

| 找什么 | 结论 | 证据 |
|---|---|---|
| 把**卡片**递出去 | 没有这个口 | `core/api.go:50` 的 `SendRequest` 只有 `message` / `images` / `files` / `audios` / `videos` / `at_*` |
| 把**卡片事件**收进来 | 没有这个口 | `core/webhook.go` 的 `POST :9111/hook` 是**反向下发**：`{project, session_key, prompt\|exec, payload}` 往某个会话里塞一条 prompt 或一条命令 —— 能下发，**换不了卡** |
| bridge 那条协议 | 入站 `card_action` **不带 operator** | `core/bridge.go:122` 的 `bridgeCardAction`：`{type, session_key, action, reply_ctx, project}` |
| 上游有没有人在做 | 有，但没合 | open PR #1828（`card_hook_dir`：按键带 `hook`，点击时**同步跑一个本地脚本**，stdout 当替卡）—— 它缺的正是 operator |

**所以 relay 长在 ANC 这一侧**：事件从 stdin 进、收口从 stdout 出，**两端都是适配器**，
这一层不碰平台 API（SPEC §4.7 ① / §7.1.39 口径没变）。形状：

| 住哪 | 是什么 |
|---|---|
| `internal/gateway/relay.go` | 判断与顺序：五个结局（`settled` / `duplicate` / `rejected` / `ignored` / `error`）、两个 Sink 方法（`Settle` / `Patch`）、一个 `Config`（`NoPatch` / `Capacity` / `TTL`）。碰外面那两步是接口，所以核心能整份被测试对着比 |
| `runtime/gateway.go` | CLI：`anc gateway relay <vault> [--app-id <id>] [--patch print\|off\|cmd:<命令>]` |

**判断只有一份**：relay 的 `Settle` 调的是 `settleCardEvent` —— 与 `anc approvals card-action` 同一个函数，
判据一个字没加（两个键都在才算数 · 只认三个字 · 没带 open_id 不算点头 · 先落库再清热层 · 真相源一动不动）。
它是从 `cmdApprovalsCardAction` 里**原样抽出来**的，不是新写的一份。

**幂等怎么算账**（这条得说清楚，不然容易被当成假保证）：
键优先 `event_id`，没有就退到一枚指纹（哪条消息 + 谁点的 + 点的什么）；
但那张表是**进程内**的 —— 换个进程就没了。跨进程的真闸从来不是它，是**队列本身**：
同一条点第二次，`approvals.Settle` 报 `ErrNotQueued`。所以幂等表是**减噪器**（同进程内少喊一声），
不是正确性的闸。实测就是这么表现的：第二次跑是新进程，答的是 `rejected` 不是 `duplicate`。

**收口三档**（都是数据，配置里给）：`print`（默认）把「该盖什么」按 NDJSON 打到 stdout 交给部署侧；
`off` 只落痕（调用方自己就是应答方时用得上，比如上游那个 hook）；`cmd:<命令>` 跑一条配置里的命令去收。

#### 实测（沙箱 Ubuntu + 真 Redis 7.0.15，`~/anc-vm2`，vault `vault`）

| 判据 | 结果 |
|---|---|
| 已经结过账的那一条（`e2e/evt-real-flat.ndjson`，`prop-card-1`） | ✅ `rejected` `e:e041e2cb…`，理由是判据原话「待批队列里没有这条提案」——**不是** error |
| 新提案 + 合成真事件，`--patch print` | ✅ `settled` `e:gw-selftest-e1` → stdout 一行 `anc.gateway.patch/v1`：`om_selftest_1` / `oc_1837…` / `done_card`（三键 `disabled:true`、`header.template=green`） |
| 同一条再递一次（新进程） | ✅ `rejected`（跨进程的闸是队列，见上） |
| 队列 | ✅ 2 条 → 1 条（只有那一条自测提案被结掉） |
| 一样本坏一行（不是按键 / 缺键） | ✅ 用例钉着 `ignored` 与 `rejected` 分得开；流本身不断 |
| 用例 | ✅ `internal/gateway` 13 条（钥匙 / 幂等 / 五种结局 / 收口失败不回退 / 容量与保鲜期 / 那一行话） |
| 质量门 | ✅ `gofmt -l` 无输出 · `go vet ./...` 无输出 · `go test ./... -count=1` **18 包全绿** |

#### 这一刀没做的

① 长连接**本体**还不在这一条腿里（现在由 `lark-cli event consume` 那条腿把 NDJSON 喂进来；
将来换飞书官方 SDK 那条腿，写的是适配器，relay 一个字不用改）；
② `print` 出来那一行**谁来盖** —— demo 里是 `lark-cli` 打 `PATCH`，产品里是部署侧那个适配器；
③ 上游 PR（站在 #1828 上补 operator）—— 要 fork 授权，还没提。

### 7.1.45 看板外壳：视觉换血、导航分组、长列表翻页（2026-10-11）

human：「主页面可能是一个总的看板，分页面要看一些更精细化的东西」、
「动画特效可以，可以开始动工了，分页那些都开始做」。

这一刀**只动外壳**：`internal/board/ui/index.html` 的样式层 ＋ `boardui/src/app.ts`
的呈现层。后端（`internal/board/*.go` / `boardserve.go`）**一行没改** ——
`app.js` 认的是类名，类名一个都没动，所以改样子没改语义。

**一、长列表翻页是视图，不是门禁。**

「一屏铺多少行」和「能不能看」是两件事。所以 `PER_PAGE` 只决定这一屏铺哪一段，
接口语义**一个字没改**（`/api/timeline?limit=` / `/api/audit?limit=` 还是原样）：

| 做 | 为什么 |
|---|---|
| 翻页在**视图里**做（`sliced()` 切当前屏、`pager()` 画那条） | 每翻一页重取一次 = 一次点头变两次往返 |
| 只有一页就**不出那条** | 少一行噪音 |
| 越界自动夹回 | 不报错、不假装有第 9 页 |
| `?p=` 走 hash 查询串，翻页是 `<a>` 不是 `<button>` | 每一屏都能分享、能前进后退 |
| 第一页的「上一页」是 `aria-disabled`（不是撕掉） | 同 §7.1.43：撕掉就看不出原来有哪几个选项 |

**二、没取到的照实说。** 两个接口都带 `total`（不受 limit 影响），一次取回
`timeline?limit=200` / `audit?limit=500`，然后在视图里翻。真实总数 > 取回条数时，页尾补一节「还有更早的」
并把**下一步命令**摆出来（`anc timeline list <vault> --before <cursor>` /
`anc audit log <vault> --limit 200`） —— **不静默截断**。

（顺手堵了一个静默：`audit` 默认 limit 是 50，不显式取就只拿得到 50 条而安然无事。）

**三、导航按「问题」分组。** 组的标题是问题，页名才是答案：
`现在`（总览／待批）、`组织`（组织／项目）、`记录`（时间线／审计）、
`运行`（运行态／数据流）、`原料`（沉淀）。`Page` 多一格**必填**的 `group`，
而 `PAGES` 数组的顺序就是分组顺序 —— 加页只要 `group` 写对，导航自己会开新组。

**四、动效分三层。** 硬约束：同视口同时循环的 ≤ 2 个 · 氛围件周期 ≥ 3.6s · 不做快闪。

- **可动（氛围层）**：导航、空态、加载骨架、状态灯呼吸（仅 `warn` / `bad`；`ok` 与 `gray` 一律静）；
- **只进一次（进场层）**：换页时每节 blur-fade 进一次（错峰 ≤ 0.55s）；
  **刷新不算换页** —— 天天看的工具每次全屏重播是噪音；
- **一律静（数据层）**：表格 · 时间线 · 队列 · 审批列表 —— 只有「状态变了」那一下才动；
- `prefers-reduced-motion: reduce` 全关。

**实测**（本机 Chrome，`board serve` 真跑，vault 副本 39 条流水 · 37 个 case · 审计 27 条）：

| 判据 | 结果 |
|---|---|
| 翻页条本体 | ✅ 时间线第 1/2 与 2/2 页、审计第 1/2 与 2/2 页均渲染；第 1 页时「上一页」`aria-disabled`、第 2 页时「下一页」 |
| 第二屏不重叠 | ✅ `#/timeline?p=2` 跨到另一批 case，表头写「共 37 个 · 这一屏第 2 / 2 页」 |
| 「被拦下的与失败的」横切视图 | ✅ **不分页**（它不是一个记录流，只列前 20 条并把 `anc audit log <vault> --result denied` 写在当场） |
| 质量门 | ✅ `go build ./...` · `go test ./... -count=1` **18 包全绿** · `npx tsc --noEmit` 无输出 · `npm run build` 产物 43.6kb（连续两次构建哈希一致） |

#### 这一刀没做的

① 动效本体（呼吸 / 光跑边 / 换页进场）只做了**静态核对** —— 截图看得出「状态对」，看不出「好看」，手感要人眼在真机确认；
② 翻页只落在**会无限变长的两个记录流**（时间线 / 审计）。其余页面现在的列表都是**有边的**，
不预先给它们套上翻页 —— 等真长到需要再说（不预先设门）；
③ macOS 那一条腿本轮未验（没机器）。

### 7.1.46 常驻侧断了会自己接回来：坏掉的那条命令不重发（2026-10-14，议题 #66）

**症状**（2026-10-11 记下）：`Store` 在 `Open` 时拨一次号，此后的命令全走那一条。
热层重启一次、网络抖一下，从那以后**每条命令都报错、没有任何重拨** —— 看板那一格永远「读不到」，
只有重启看板才会好。CLI 侧没这毛病（每次自己拨号），洞只在**常驻侧**。

**判据是人的**：「重发安不安全」按命令而异 ——
读（`HGETALL` / `GET` / `SCAN`）重发无害，但 `SET NX` 在「服务端执行了、回包没回来」这个窗口里
重发会看到 key 已存在 ——「被别人抢了」和「自己上次其实成功了」长得一模一样。
那个判据要人拍，所以这一刀**只治「断了不会接」：坏掉的那条命令照实报错、不重发**。

立了两条不变量：

| 不变量 | 意思 |
|---|---|
| 断了就换一条 | 换不上（热层还没回来）就留着旧的、如实报错，**下一次调用再试**；热层自己回来之后，谁都不用重启进程 |
| 标了死的流不再写 | 上一次读失败时，那条命令的回复可能还半截在路上；这时候再往上写，两条命令的答复会串在一起 —— **读回一份错的值，比报个错坏得多** |

改动清单：

| 住哪 | 改了什么 |
|---|---|
| `internal/hot/resp.go` | `respConn` 加 `dead`；新增 `replyError` —— 热层**自己回**的 `-ERR` 与传输层错误分开，别把「一条命令被拒」当成「连接坏了」；只有写不出去、或读到非 `-ERR` 的错，才标 `dead` |
| `internal/hot/redis.go` | `Store` 加 `mu` / `dial` / `timeout`；新增 `do()`，成为本包所有命令的**唯一出口**（原 12 处 `st.conn.do(` 全换成 `st.do(`）；`replaceConn` 回「换完之后手里这条」；`Open` 挂上 `dial`，Ping 失败改走 `st.Close()`（Ping 失败时手里那条可能已经换过了 —— 顺手修掉的真 bug）；`Close()` 走锁 |
| `internal/hot/reconnect_test.go` | 新增 3 条用例：断了会换一条且坏的那条**不重发** · 热层自己回的 `-ERR` 不算断线 · 换不上新连接时**不许往死流上写** |

#### 实测（Windows 11 + go1.27.0；热层 = WSL2 Ubuntu 里的真 `redis-server`）

看板用 `anc board serve <vault> --hot-addr 127.0.0.1:6379` 起在 **8790**（用户那个 8787 没碰）。
A/B 对照跑的是**同一条脚本**、两个二进制：基线 = 从 `git archive HEAD` 解出来重编的（不含本刀）。

| 判据 | 修复前（基线） | 修复后 |
|---|---|---|
| ① 正常 | `total=1`，无错 | `total=1`，无错 |
| ② `redis-cli shutdown nosave` 之后 | `wired=true` + `error`（**读得到「读不了」**，不是拿空列表冒充） | 同左 |
| ③ 同端口同地址起回 redis（`PING → PONG`）后连打 | **错 · 错 · 错 · 错**（永不恢复，只能重启看板） | **通**；3 轮 × 3 次全通 |
| ④ 自愈后读真数据 | — | 落一条 → `total=1`，标题对得上 |

**一个环境留下的疤**：WSL2 的 localhost 转发在 redis 重启那一瞬会抖 —— 偶尔**新拨的那条连接**第一笔就被掐
（`wsarecv ... An established connection was aborted`），那一次照实报错，但**下一次调用就好了**，
不像以前那样永久坏掉。热层与看板同机（真服务器）没有这层转发，不该有这一抖。

质量门：`gofmt -l internal/hot` 无输出 · `go vet ./...` 无输出 · `go test ./... -count=1` **18 包全绿**。

#### 这一刀没做的

① 「坏掉的那条命令要不要重发」——判据要人拍（读可以、`SET NX` 不行），没拍之前一律不重发；
② 重拨只在**下一次调用**发生，不做后台探活；
③ 只在 Windows + WSL2 redis 上实测过，macOS 侧未测（没机器）。

### 7.1.47 工作上下文包：一个装配点、两个出口（2026-10-14，议题 #53）

议题 #53 有两半，**人已经拍了前一半**（2026-10-09 的评论）：真相源就是 vault 本身（不新增文件格式）·
把「项目」与「技能正文」补进上下文 · 组装点**只做一个**（并进 `anc_read_context` 的同一次调用）·
没有真相源的（新鲜度 / 版本）**报缺，不编**。后来代码把这四条都落了
（`envelope_read.go`：`identity` / `domains` / `directory` / `skills` / `projects` / `gaps`），
**唯一还悬着的是「这份上下文落到哪种形态」** —— 分析结论也写在同一条评论里：**按需生成，不落盘常贮**。

这一刀就收这个尾：给它一个**载体**，且不新开第二个装配点。

| 住哪 | 是什么 |
|---|---|
| `envelope_read.go` 的 `assembleContext(o, vault, who, onBehalf, ask, now)` | **唯一一个装配点**：解身份 → 按可见范围取 域 / 目录 / 技能 / 项目 → 报缺。回 `(readContextOut, 拒没拒)` |
| `envelope_read.go` 的 `readContext` | MCP 那个出口：调装配 + 真相源读不出来时的报错口径 + 每次调用一条 `audit` 痕（**行为一个字没变**，抽函数时逐条对齐过） |
| `context.go` | CLI 那个出口：`anc context pack <vault> --who <编号> [--domain <slug>]` |

**两条不变量，各有用例钉着**：

| 不变量 | 为什么 |
|---|---|
| 两个出口**除回答时刻外逐字段相同**（`TestContextPackAndMCPAreOneAssembly`，成功与拒绝两条路都盖） | 两份装配迟早对不上号，而「对不上号的工作上下文」比没有更坏 —— 读的人以为看到的就是全的 |
| 投影**只读**：真相源一个字节不动（`TestContextPackWritesNothing`，全树指纹前后相同） | 纯函数产物只要存一次、改一次，就成了第三处会漂移的源 |

**为什么 CLI 不留痕**：`audit` 记的是「一次**行使**」—— 谁向平台要了什么；那条账挂在 MCP 那个入口上。
CLI 是操作者**在自己机器上**打一份投影（同 `anc org export`，它也不留痕），不是 agent 在平台上行使。
所以**写入面一个口子都没多**：多的是一个只读出口，不是一条新的写路径。

**实测（本机 Windows 11，真起二进制跑真 fixture；非单元测试）**：

| 判据 | 结果 |
|---|---|
| 五问 + 项目 | ✅ `--who alice`：`identity`（member / trade / manager）· `domains`（含 `data` / `who` / `terms` / `sources`，指针 `domains.md:8`）· `directory`（别域只给「找谁」）· `skills`（`where` + `missing:true`）· `projects`（`projects.md:7`）· `gaps` 4 条 |
| 越域 | ✅ `--domain logistics` → 退出码 1 + 拒话「不在你的可见范围（数据默认不通）」+ 指路 `ask`「运营（Bob Li）」；stdout 里**没有**别人的 `data` |
| 不存在的人 | ✅ 退出码 1；拒话里没有「参数 / 格式 / schema」这类词（他是**不在这家公司**，不是写错了参数） |
| stdout 纯度 | ✅ 恒为合法 JSON，且搜不到 vault 绝对路径与盘符 |
| 用法错 | ✅ 不给 `--who` → 退出码 2（与「这个人不存在」分开报） |
| 用例 | ✅ `runtime/context_cli_test.go` 9 条 |
| 质量门 | ✅ `gofmt -l .` 无输出 · `go vet ./...` 无输出 · `go test ./... -count=1` 18 包全绿 |

#### 这一刀没做的

① **形态本身仍待拍**：这份包现在只有「stdout 上的 JSON」一种落法。真要落成 vault 里的一份文件、
或看板上的一页（W4 的「看板分视图」），那是另一刀 —— 本刀不动真相源，所以两种都接得上；
② `on_behalf_of`（署名）只在 MCP 那条路上开，CLI 没开 —— 等真有人要用再加，不预设；
③ 看板还没有这一页（`/api/context`）。

**①②③ 的答案在同一天下来了，见 §7.1.48**：人拍了 ① + ②（冻结副本落 vault · 看板加一页）。

### 7.1.48 上下文包的形态落地：看板一页 + 冻结副本（2026-10-14，议题 #53）

§7.1.47 悬着的 ①②③，人当天拍了两条：**① 冻结副本落 vault 一份文件**、**② 看板加一页**；
③（`on_behalf_of` 在 CLI 上开）没拍，仍只在 MCP 那条路上。

这一刀既要加一个出口、又要加一个落点，前提是装配点得先能被三处共用 —— 原来的装配点住在
`package main`（`envelope_read.go`），看板在 `internal/board`，**够不着**。所以第一步是把装配点
搬进 `internal/ctxpack`：

| 住哪 | 是什么 |
|---|---|
| `internal/ctxpack/pack.go` | **唯一装配点**：`Assemble(o, vault, who, onBehalf, ask, now) (Out, 拒没拒)` + `IdentityOf` + `SafeToken`。从 `package main` 整段搬来，结构改名（`Out` / `IdentityOut` / …），**json tag 一字未动**，行为逐字不变 |
| `envelope_read.go` | 只剩 `readContext`（MCP 出口）+ `auditRead` / `readActor` / `mustJSON`，改调 `ctxpack.Assemble` |
| `context.go` | CLI 出口 + 新增 `freeze` |
| `internal/board/context.go` | 看板出口（`/api/context`，`anc.context/v1`） |

**为什么搬**：不是为了好看。CLI / MCP / 看板要打的是**同一份**东西，而「同一份」只能靠同一个函数
保证；让看板去 import `package main` 不行，让看板抄一份更不行 —— 那就成了第二个装配点，
§7.1.47 那条不变量当场作废。

**看板那一页（`/api/context`）**

| 口径 | 定法 | 为什么 |
|---|---|---|
| 不给 `who` | 回 **roster**（`org.Members` + `org.Agents`，逐条走 `o.Identity` + `IdentityOf`） | 看的人得先知道「可以看谁」，这是挑人的那一步 |
| 给 `who` | 回 **Pack**（那一份） | 同 CLI，同一份 |
| 拒了（不存在 / 越域） | **照回 200**，拒话在 `pack.refused` 里 | 拒是一次**正常回答**，不是服务坏了 —— 回 500 会让人以为平台躺了 |
| 加载失败 | 走 `error` 字段 | 它没有「背后服务」这一层，不套 wired / why |

前端：新增「上下文」页（分组 `组织`，问答「一个 agent 的工作上下文是什么？」），插在「项目」之后；
`fetchData` 多取一份 `/api/context`，`hashchange` 里这一页**只重取那一份**（点 chip 换人时不整页重刷）。
手写 CSS 加了 `a.chip` 三个态 + `.nw`（标识符不被列宽切两半）。

**冻结副本（`anc context freeze`）**

```
anc context freeze <vault> --who <编号> [--domain <slug>] [--to <目录>]
```

- 落点默认 `<vault>/context/`，`--to` 可换；文件名 = **解出来的 `out.Who`**（给了 `domain` 再跟 `.scope`）。
- **拒了就不冻**：查无此人 / 越域 → 退出 1，**不建目录、不留空壳** —— 先算，算出来才写。
- 它是**冻结副本**，不是常贮：再冻一次覆盖上一次；落点是快照，真相源仍是 vault 里那些 markdown。
- **落点不进路由表**：`org.ContextDir` 进了 `skipDirs` —— 它比 `charters/` 更该挡：charters 好歹是
  「谁放进来的资料」，而 `context/` 是**投影冻住的一刻**，当数据源读就是把一份旧投影当成事实。
  这一条 `TestContextDirIsNotRouted` 钉着，`anc org export` 的 routing 里也确实没有它。
- `org init` 的骨架多写一份 `context/CLAUDE.md`（落点约定：不是真相源 / 是快照不是常贮 / 再冻覆盖 /
  看板看 · MCP 要 · 这目录只给交付与审计取证）。

**两条新的不变量**：

| 不变量 | 为什么 | 用例 |
|---|---|---|
| 冻结副本 == stdout + 换行 | 落盘那份与打出来那份**必须逐字节是同一份**，否则「冻住的」与「看到的」又是两个东西 | `TestContextFreezeWritesWhatPackPrints` |
| 拒了不冻、且不动真相源 | 拒过的请求不该在 vault 里留痕迹（空目录 / 半份文件都是痕迹） | `TestContextFreezeRefusalWritesNothing` · `TestContextFreezeWritesExactlyOneFile`（移走落点后全树指纹逐字节回到前） |

**为什么落 vault（进 git）**：正因为进 git 才叫**冻结副本** —— 可 diff、可回看、可作交付与审计取证。
它不因此变成真相源：`context/CLAUDE.md` 明写「改这里不影响别人」，路由表也把它挡在外面。

**为什么 freeze 也 CLI 不留痕**：同 §7.1.47 —— 留痕挂的是「一次**行使**」（谁向平台要了什么），
那条账在 MCP 入口上；CLI 是操作者在自己机器上打投影 / 冻存档，性质同 `anc org export`。

**实测（本机 Windows 11，真起二进制；非单元测试）**：

| 判据 | 结果 |
|---|---|
| 看板 roster | ✅ `/api/context` 回 3 人（alice / bob / devbot） |
| 看板那一份 | ✅ `?who=alice` 回五问 + 项目 + 4 条 gaps，**无本机路径** |
| 看板拒话 | ✅ `?who=alice&domain=logistics` → 黄横幅拒话 + 指路「运营（Bob Li）」，**不带别人的数据** |
| 看板换人 | ✅ `?who=alice` 点 Bob → hashchange 重取，页变成 Bob Li / logistics（**不整页重刷**） |
| 看板没选人 | ✅ 只有「可以看谁」+「挑一个」 |
| freeze 落盘 | ✅ `context freeze <vault> --who alice` → 退出 0，`context/alice.json` 1258 字节；`pack` stdout 959 字节 + 换行 = 文件内容 |
| freeze 覆盖 | ✅ 再冻一次 → 「覆盖了原来那一份」 |
| freeze 拒了 | ✅ 越域 / 查无此人 → 退出 1、落点里没有新文件、目录都没建 |
| 落点不进路由 | ✅ `anc org export` 的 routing 里**没有** `context`；`anc org check` 通过 |
| 用例 | ✅ `internal/board/context_test.go` 5 条 + `context_cli_test.go` freeze 4 条 |
| 质量门 | ✅ `gofmt -l .` 无输出 · `go vet ./...` 无输出 · `go test ./... -count=1` 18 包全绿 |

### 7.1.49 反向下发第一刀：读出口捎上「派给我的待办」（2026-10-14/15，议题 #58）

议题 #58 的形态人已经拍过：**ANC 不能主动往 agent 的会话里插话**（插了就是打断 —— 上一条还没跑完，
第二条已经在改前提了），所以「派活」拆成两件**各自可替换**的事：

| 那件事 | 谁的活 | 落在哪 |
|---|---|---|
| 决定派给谁 / 派什么 | **ANC**（人提、被派的域 owner 点头） | 热层（任务本体，带保鲜期） |
| 让对方开始动 | **对方自己**（它在自己会话里 pull，pull 到了才开始） | 会话开头要一次上下文 —— 这一刀让它 pull 得到 |

所以这一刀**不做**：不新增协议面、不动 MCP 那个无状态子集、不往任何会话里推东西。
只做**「pull 得到」**这一半：工作上下文里多两格。

**契约多两格（`ctxpack.Out`）**

| 那一格 | 是什么 | 判据 |
|---|---|---|
| `todo` | 派给我的待办 | 热层 `to` == **解出来的主体编号**（`subj.Ref`） |
| `doing` | 我正在做的 | 热层 `by` == `subj.Ref` |

`TaskOut` 的字段就是热层那条记录本身（`id/title/status/by/to/domain/project/note/as_of`），
**不加工**。四条口径，逐条都有理由：

| 口径 | 定法 | 为什么 |
|---|---|---|
| 匹配用 `subj.Ref`，不用请求里那个拼法 | 同「身份不许自报」那条 | 请求可以写别名，编号只有一个 |
| 状态**一条都不筛** | 热层里还留着一条 `done` 说明它还没清走 | 「达标 = 先落库、再清热层」（§7.1.31 那条：`anc hot done` 的顺序是先落库、再清热层），那不归装配这层判 |
| 顺序**定死**（编号升序） | `sort.Slice` | 热层 `List()` 走 SCAN，顺序不保证；三处出口要「除回答时刻外逐字段相同」，顺序就不能飘 |
| 单域问（`--domain`）**不捎** | `if ask == ""` 才填 | 同「找谁」目录那条纪律：要单独一块的人不该被别的东西淹掉 |

**为什么输入只带一个 `Wired`、不带原因**（`ctxpack.TasksIn`）：原因是**环境的**（没配 / 连不上 / 读错），
一进契约，三个出口的报缺文案就会各飘各的，「三处逐字段相同」当场作废。原因说给人听，去处是各出口
自己的台面：CLI / MCP 打 stderr，看板在回话里单给一格（下面）。

**热层没接上 = 报缺，不许空着**（这一刀最要紧的一条）：`gaps` 最前面多一条
「派给我的待办：热层没接上 —— 这一段答不了，**不是**「没人派给你」」。
拿一份空清单冒充「没人派给你」正是这个系统最怕的那种假绿 —— 空清单在任何一份回话里都长一样。

**三个出口各自的接线**

| 出口 | 怎么接上热层 | 接不上时 |
|---|---|---|
| MCP（`anc envelope serve`） | 新增 `--hot-addr` / `--hot-prefix`（也认 `ANC_HOT_ADDR` / `ANC_HOT_PREFIX`），起服务时开一次 | **不拦接入面**（递信封、要上下文照样能用）：stderr 报一遍，读出口把报缺如实写进 `gaps` |
| CLI（`anc context pack` / `freeze`） | 同两个 flag；走一道**可替换的包级缝** `readHotTasks`（用例换掉它，不必起 redis） | 同 MCP：stderr 报一遍 + `gaps` 报缺 |
| 看板（`/api/context`） | 起 `anc board serve` 时那个 `--hot-addr` 复用 | 回话里多两格：`tasks_wired` + `tasks_why` —— 这一页要自己答完，不逼前端去别的端点替它解释（同 roster 那条口径） |

**看板多出来的第三格 `task_bands`**：任务状态 → 配色档（`green/yellow/red/unknown`）。
放**视图层**不放契约里，两条理由：agent 那份契约一个字不该多（那是结构性保证）；而「这个词画什么颜色」
本来就是视图层的事，且**词表只有一处说了算** —— `timeline.Band`（热层里的 status、时间线里的 status、
这里摆的 status 是同一套词），前端不另抄一份。前端据此摆两段（「派给我的待办」/「我在做的」）+
四档图例（不看颜色的人也有一条路）；`internal/board/ui/app.js` 是构建产物，入库。

**e2e 当场抓到一个真缺陷（已修）**：热层没接上时，MCP 读出口**整条路断连**（`http: panic`）。
根因是接线用了 **typed nil**：`var st *hot.Store` 直接塞进 `hotLister` 接口 —— 接口不是 nil，
`g.Hot == nil` 判不出来，于是走进 `(*hot.Store)(nil).List()` 当场 nil deref。
两处收口：

| 收口 | 内容 |
|---|---|
| 接线 | 只在**开成功**时才往接口里塞（没接上就让它一直是 nil 接口） |
| 介质层 | `(*Store).List` 对 nil 接收者回一句人话（「热层没开：Store 是 nil —— 调用方把「没接上」写成了「接上了」」），不 panic —— 交付出去的东西，坏法该是「报缺」不是「断连」。用例：`TestListOnNilStoreSaysSo` |

**新增/改到的用例**：`TestContextPackAndMCPAreOneAssembly`（逐字段相同那条，现在把两摞也钉住）、
`TestContextPackSaysWhenHotIsNotWired`、`TestContextPackScopedAskCarriesNoTasks`、
`internal/board/context_test.go` 三条（没接上 / 接上了两摞与配色档 / 接上了但读不动）。

**实测（本机 Windows 11，真起二进制 + 真 redis；非单元测试。vault 用的是 fixture 的临时副本 ——
读出口会往 `audit/` 落痕，直接打在工作区里的 fixture 上会把用例数据弄脏，这一步踩过）**：

| 判据 | 结果 |
|---|---|
| CLI 接上了 | ✅ `anc context pack <vault> --who alice` → `todo=case-2`（to=alice）· `doing=case-1`（by=alice）· bob 那件**一个字段都没漏进来** |
| CLI 没接上 | ✅ `--hot-addr 127.0.0.1:6399` → stderr 报一遍；回话里 `todo`/`doing` **两个键都不出现**，`gaps[0]` 是那句「答不了」 |
| MCP 接上了 | ✅ 真 HTTP + 真 JSON-RPC 调 `anc_read_context`：`todo=case-2 doing=case-1`，`gaps` 里没有「热层」那一条 |
| MCP 没接上 | ✅ 起服务时 stderr 报一遍；回话 `keys=who,identity,domains,directory,skills,projects,gaps,at`（**没有** todo/doing），`gaps[0]` 是那句「答不了」；**不再断连** |
| 看板 | ✅ `/api/context?who=alice` → `tasks_wired=true`、`task_bands={"case-1":"red","case-2":"yellow"}`；页面两段 + 图例都出来了（无头 Edge 截图核过） |
| 看板单域 | ✅ `?who=alice&domain=trade` → 那一份里**没有** `todo` 键，`task_bands` 也不出 |
| 用例 | ✅ CLI 侧 4 条 + 看板侧 3 条 + hot 侧 1 条 |
| 质量门 | ✅ `gofmt -l .` 无输出 · `go vet ./...` 无输出 · `go test ./... -count=1` 19 包全绿 · `npm run check`（tsc + esbuild）过 |

**这一刀之后仍悬着的**（不在本刀范围）：飞书只送指针那一半（发卡出去 / 收 action，网关口在议题 #65）、
多 agent 互派的队列与版本号（议题 #60）、以及「派活这件事的裁判权在人」那条审批链路。
### 7.1.50 外部材料进真相源的唯一机器入口 `anc ingest`（2026-10-15，议题 #47）

真相源的写者**只许有两类**：① 人手改 markdown ② 这条管道。第三方系统（含客户自己的知识库）
**不许直接写** —— 那就是第二个真相源，两边迟早各说各话，而「哪边是真的」到那天没人答得出来。
#47 要的那条「第三方系统写不进去」，不靠约定，靠**没有那个入口**。

```
anc ingest <vault> --from <提案.json|-> [--dry-run] [--json]
```

**提案（JSON，三个字段，多一个字段一律报错）**

| 字段 | 是什么 |
|---|---|
| `by` | 谁提的。**必填** —— 与信封「缺 `who` = 拒收」同一条口径：身份认错，后面每一条留痕都记在错的人头上，比不记还坏 |
| `why` | 一句附注（可选），照原样进审计 |
| `writes[]` | `{path, content}` —— **整份文件**的全文，不是补丁 |

`DisallowUnknownFields`：拼错的字段名不许静默成「没写」。这条管道干的是改写真相源，
静默少写一条比拒收坏得多。

**为什么整份替换、不打补丁**：补丁要匹配上下文，人写错一行整条都落不下去；
而整份替换的判据是**换完之后这棵树合不合法** —— 与人手改 markdown 是同一把尺子。
git 那边照样只看得到一行 diff（可 diff / 可 review / 可回滚，与时间线同一条理由）。

**落点白名单（就这四处）**

```
domains.md · projects.md · agents.md · members/<成员名>/<文件>
```

- 别处**没有那个入口**。`members/<名>/` 下面再分层是允许的（白名单管落点，不管深度）。
- 成员名那一段走 **`org.MemberNameProblem`** —— 与 org 层**同一把尺子**：它会落成目录名、
  也会进 `homes/<名>` 与 project 名，两处各立一套判据迟早会漂。
- 路径必须相对、必须 `/` 分隔、不许 `.` / `..` / 空段 / 盘符 / 反斜杠。

**四步**（第 1–3 步真 vault 一行没动）

| 步 | 做什么 | 不过的时候 |
|---|---|---|
| 1 | 提案形状 + 落点白名单 | 拒收（退出码 1）。**读不懂提案也算拒收**，不是用法错 —— 它和「落点不在白名单里」是同一件事：这份东西不该落进去；两个出口（文本 / `--json`）说的是同一份理由 |
| 2 | 改写应用在 vault 的**临时副本**上，`org.Load` 跑 `anc org check` 同一张规则表 | `LoadError` → 逐条列出**规则 id**（只说「没过」等于让人去猜），非红档也一并回显 |
| 3 | 有红档 → **不落盘** | 退出码 1，真 vault 一行没动、临时副本删掉 |
| 4 | 通过 → 原子写回真 vault + `audit.Append` 一条 `action=write` | 文件落了审计没落 → 说清并让人先补审计，别重跑提案 |

**为什么是「改副本 → 校验」而不是「先落盘、不对再回滚」**：回滚在进程被打断时会留下一棵半改的树，
而这条管道写的正是真相源。临时副本是几 KB 的 markdown，代价可以忽略。

**幂等**：一条都没变（逐字相同）→ **不落盘、也不记审计**。记了就等于「同一个改动留两条痕」，
后面的账会跟着失真。

**它不 git commit**：历史归人，机器不写历史。落完提示一句「自己 `git diff` 看一眼再提交」。
（`#47` 里写的是「校验 → 原子写 → 进 git」，这一刀把最后一步改成**人**的 —— 决定权在 2026-10-15
与 human 对过。）

**实测（本机 Windows 11，真起二进制）**

| 判据 | 结果 |
|---|---|
| 落真 | ✅ 一份提案（新增 `members/赵四/persona.md` + 改写 `projects.md`）→ 退出 0，两个文件按提案落盘，审计多一行 `{actor:张三, action:write, source:ingest, object:"projects.md, members/赵四/persona.md"}`；vault 里**没有** `.git` |
| 白名单 | ✅ `company/company.md` / `roles/…` / `members/alice` → 退出 1，理由里带白名单全文，真 vault 树逐字节没动 |
| 红档拦住 | ✅ 把角色的 `role:` 改成一个不存在的岗位 → 退出 1，回显 `[member.role.missing]`，`alice/persona.md` 一字未改 |
| `--dry-run` | ✅ 退出 0、打印会动哪个文件、树逐字节没动 |
| 幂等 | ✅ 同一份提案第二次 → 「一条都没变」，审计没多一行 |
| 形状 | ✅ 缺 `by` / `writes` 空 / 认不出的字段 / 不是 JSON / 同一路径两次 → 各自退出 1 并说清；缺 `--from` 是用法错（退出码 2） |
| 用例 | ✅ `runtime/ingest_cli_test.go` 7 条（含 members 下再分层那条正例） |
| 质量门 | ✅ `gofmt -l .` 无输出 · `go vet ./...` 无输出 · `go test ./... -count=1` 19 包全绿 |

**这一刀之后仍悬着的**：智能那半（harness 出提案）**只提案、不落盘** —— 它出 patch，人 review 后由
本管道落（与「agent 只能提案、不能行使」同一条边界）。外部数据源（#36）落地时复用这条，别各写一套。

### 7.1.51 判真：第四档「等人给一句话」+ 灰分两种符号（2026-10-15，议题 #59）

判真是三问：谁判 / 拿什么判 / **判不出来落哪一档**。前两问现状已答（数据判、人不自动处置；
外部依据三重交叉）。这一刀修的是第三问 —— 它原来有两处说不清。

**一、灰原来叠了两种意思，现在分符号**

| 那一格 | 现在 | 为什么 |
|---|---|---|
| 认不出的词 | **空心虚线圈**（`dot gray`） | 「没人判过」—— 该去判它 |
| 真相源声明 `unwired` | **灰实心**（`dot gray-solid`） | 「本来就不该期待」—— 别去催它 |

共用一个符号时，读的人分不清「没判过」和「没轮到它」—— 一个要人动手，一个不要。
（议题里列的第三处「导航未接入」是 `.res` 标签**不是状态灯**，不与这两个撞符号，所以不动。）

**二、多一档「等人给一句话」**

A2A 的任务状态机把 `input-required` / `auth-required` 当**一等中断态**，不是失败。
对着我们那根「要不要人动手」的轴：塞进黄（系统自己在走）是错的，塞进红（坏了）也不准 ——
**没坏，只是缺一个决定或一次授权**。所以：

| 那一档 | 符号 | 数据源 |
|---|---|---|
| 等人给一句话 | **靛蓝 + 暂停形**（`dot wait`） | **只有待批队列**（热层 `prop:` 那摞 = `anc approvals ls`）：在队列里 = 有人在等你点头 |

三条口径，都是「两路数据不许混」这一条推出来的：

- **词住 `internal/board.BandInputRequired`（`input-required`），不进 `timeline.Band` 的词表**：
  那根轴分的是**真相源里 status 那些词**，这一档不来自 status —— 来自「这条正躺在队列里」这个事实。
  用例把这条钉住了（`timeline.Band(BandInputRequired) == "unknown"`）。
- **档由服务端给（`PendingView.band`），前端照画**：与 `task_bands` 同一条纪律 ——
  词表只有一处说了算，前端不另抄一份。
- **status 不许被 band 盖掉**：提案自己写的词照原样端出来，两回事。

**三、视觉重量按「要不要人动手」排**（议题 3.4）

红是唯一要求人行动的档，原来视觉面积反而最小；灰最淡，几乎看不见。这一刀：

| 那一档 | 改法 |
|---|---|
| 红 | 13px → 16px（数字卡片上 20px），仍是菱形（四个形状里最不一样的那个） |
| 灰（虚线圈） | 描边 1.6px → 2px + 一层淡底，站得住 |
| 灰（实心）/靛蓝 | 新加的，实心档都带一圈同色淡光环（暖纸吃色） |
| 数字卡片上的点 | 17px（红 20px）—— 它们站在 42px 的大数字旁边，同一个尺寸会显得没重量 |
| 暂停形怎么画 | 纯 CSS（`linear-gradient` 两段实色 + 中间空档），不加元素：「加一张表别加一层 if」同样管视觉 |

**顺手修掉一个真缺陷（截图核出来的）**：`.dot` 原来**没有 `display`** —— 固定在宽高的 `span`
若还是 inline，宽高一律被忽略。行内那些点靠 `.st`（inline-flex）撑着看不出来，但**运行态那张表**
用的是光杆 `.dot`：它塌成一根 4px 细条，**绿 / 黄 / 红三档直接是空白**（形状与颜色一起丢）。
补 `display: inline-block` 之后四档在那张表里都立住了 —— 这正好是议题 3.4 说的「缩到 100% 屏宽看不出来」
的根因之一。

**实测（本机 Windows 11，真起二进制 + 无头 Edge 截图；另起一台 8790 指向一份 vault 副本，
不动 human 正在看的那份）**

| 判据 | 结果 |
|---|---|
| 第四档在数据里 | ✅ `/api/approvals` 那一行带 `"band":"input-required"`，`status` 仍是 `pending`（没被盖掉） |
| 第四档在画面上 | ✅ 待批卡片上靛蓝暂停符号 + 「靛·等你给一句话」，队列标题里也说明了这一档是什么 |
| 灰实心 | ✅ 副本里把 alice 标 `unwired: true` → 运行态那张表出「灰实心·未接凭据（本来就不该期待）」 |
| 红在那张表里 | ✅ 去掉 `unwired` → 同一格出「红·卡点，需介入」菱形（**修 `display` 之前这一格是空的**） |
| 四档图例 | ✅ 时间线页：实心圆 / 带光环的圆 / 菱形 / 空心虚线圈，四形状并排各自分得开 |
| 用例 | ✅ `internal/board/approvals_test.go` 新增 `TestApprovalsCarryInputRequiredBand`（同时钉住「不进 timeline 词表」） |
| 质量门 | ✅ `gofmt -l .` 无输出 · `go vet ./...` 无输出 · `go test ./... -count=1` 19 包全绿 · `npm run check` 过（`app.js` 入库） |

**仍悬着的**：热层任务里真写了 `input-required` 这个词时，那根轴仍把它当「认不出的词」（画灰）——
按拍板口径，这一档只挂在待批队列这个真数据源上；等它在别处也有真数据源时再议（不许从字符串猜）。

### 7.1.52 落盘前那一眼：提案的 diff 与出厂形状（2026-10-15，收议题 #47 的另一半）

#47 的验收三条里，第二条（校验不过就拒收）与第三条（第三方写不进去）第一刀已落（§7.1.50）。
剩下第一条：**「一段立项材料，经一个 harness（或人手写）变成 vault 里的一行 / 一个目录，
全过程一条 diff 看得见」** —— 而第一刀留的口径是「模型只提案、**人 review 后**再由管道落」。
这一刀补的就是 review 那一步缺的东西。

**问题**：提案里装的是**整份文件全文**（口径：补丁要匹配上下文，人写错一行整条都落不下去）。
代价是：人拿到一份 JSON，看不出「相对库里现在这一份，改了哪几行」。
**没东西可看，review 就成了点头** —— 那这条管道与「让模型直接写」的差别就只剩形式了。

**两件**：

| 是什么 | 给谁 | 形态 |
|---|---|---|
| `anc ingest --patch` | 人（review 那一步） | 落盘前的 unified diff，与 git 那份同形状（顶格那份可以直接喂 `git apply -p1`） |
| `anc ingest --dump-shape` | harness 那一侧（出提案的） | 出厂提案形状：字段 / 白名单 / 规矩 / 边界 —— **数据**，不是分支 |

**三条口径**：

- **拒收不配 diff**：被红档拦住的提案一条都落不下去，给它配一份 diff 只会让人以为「就差点头了」。
- **不给 `--patch` 就一个字都不变**：文本与 JSON 两个出口都逐字照旧（两条用例钉着）。
- **白名单只有一处**：形状里的白名单就是 `ingestWhitelist` 那个常量 ——
  与 `anc approvals card --dump-frame` 同一条纪律（形状住代码里，拿走改就是了，不另抄一份）。

**diff 是自己算的**（`ingest_diff.go`，纯函数）。理由不是偏好：这一层零依赖
（`go.mod` 一个 require 都没有），而**真相源不要求是 git 仓库** —— 用例里那棵 vault 就没有 `.git`，
`copyTree` 也刻意不搬它，所以借不了 `git diff`。两处要紧的细节：

1. 切行**留着行尾那个换行** —— `a\nb` 与 `a\nb\n` 不是同一份文件；按「去尾换行」切会把这条差异抹平，
   而这份 diff 的用途正是给人看差异（末行没有换行时按 git 的写法补一句）。
2. **规模闸**：行数乘积超过 `diffLineCap` 就如实写「太大，不逐行比」，不硬算 —— 不给一次内存炸弹。

**智能那半落到哪**：一件能力件 `skill/anc-ingest-propose/`（方法与边界，**不是代码**）。
「材料里哪句话该进哪个字段」没有稳定规则，写进代码就是过拟合；能固化的那部分
（形状、白名单、校验、原子写、留痕）全在 `anc ingest` 里，且是数据。
能力件照 `--dump-shape` 说话，不抄文档 —— 抄的那份迟早与代码不一样。

**实测（本机 Windows 11，真起二进制；vault 是 `anc org init --git` 建的一次性副本，不动工作区里任何 vault）**

| 判据 | 结果 |
|---|---|
| 一段材料 → 提案 → `--dry-run --patch` | ✅ 两个 hunk：`projects.md` 里那三行注释换成一行（`@@ -15,6 +15,4 @@`）、`members/赵四/persona.md` 新文件（`@@ -0,0 +1,15 @@`，左头 `/dev/null`） |
| 空跑一行没落 | ✅ 树逐字节没动（用例 `TestIngestPatchShowsTheDiff` 同款断言） |
| 拒收时不配 diff | ✅ 骨架那条 `role.allowed_tools.empty` 拦住时，输出里没有 `@@ -`；拒收理由仍逐条指到规则 id |
| 落盘 | ✅ 退出 0 + 审计一条（`actor=张三` · `source=ingest` · `detail=2 个文件，新增 1 / 改写 1` · `ref` 指回那份提案）；`git diff --stat` 就是**一条 diff**（1 insertion / 3 deletions）+ 新文件与 `audit/` 未跟踪 |
| 幂等 | ✅ 同一份提案第二次：「一条都没变」，审计仍是 1 行 |
| `--dump-shape` | ✅ 不要 vault 也能问；白名单与 `ingestWhitelist` 逐字相同（用例钉住） |
| 用例 | ✅ `ingest_diff_test.go` 7 条（行号 / hunk 合并 / 新文件 / 末行无换行 / 相同不许出 diff）· `ingest_cli_test.go` 新增 5 条 |
| 质量门 | ✅ `gofmt -l .` 无输出 · `go vet ./...` 无输出 · `go test ./... -count=1` **18 包全绿**（`internal/ctxpack` 没有用例文件） |

**这一刀之后仍悬着的**：#36（外部数据源）落地时复用这条 —— 现在它多了一个「照得着的形状」；
外部那张表 / 那份会议纪要怎么变成提案，仍归 #36 自己那一刀（别各写一套）。

### 7.1.53 看板「授权」页：一页一个问题（2026-10-15，W4 的「看板分视图」）

§7.1.15 末尾留的那句（「看板上「谁能做什么」那个页」）到今天才画。这一刀**不动真相源**：
`grants/` 一个字节没变，加的是一个**读出口**。

**先定分工，再画页**。看板上「谁能做什么」这句话本来被两页同时认领 —— 数据流页的问题句
写着「谁可以做什么、通道开着吗？」。两页答同一句话，读的人就不知道该信哪一份。按仓库既有的
那条（`assetsPage` 上的注释：问题句按**现在真答得了**的写）拆开：

| 页 | 问题句 | 数据源 | 变化节奏 |
|---|---|---|---|
| 数据流 | **通道开着吗、每个 bot 手里有什么？**（这一刀改的） | 执行面 `config.toml`（gateway 真吃着的那份）+ 投影 | 动 config / 动组织 |
| 授权（新） | 谁被允许对谁做什么？ | 真相源 `grants/`（逐条搬，一个字不改） | 动授权表 |

**一条新契约**：`anc.board.grants/v1`（`internal/board/grants.go`）。字段
`schema / wired / total / shown / scope / rows / subjects / findings / missing / note / error`。
路由加在 `/api/context` 后面（`server.go` 一行）。

**四条口径**（都写进注释了）：

1. **全量、不筛选、不脱敏** —— 与 `anc.board/v1` 的 `grants` 同一纪律（看板是观测面，不是凭据面）。
   授权表本来就**没有凭据**（SPEC §6：它记的是「谁被允许做什么」），所以这一页不出
   `app_id` / `app_secret`。「脱敏」在这里**无事可做** —— 不是漏做，是表里没有可脱的东西。
2. **视角只筛、不改写**：`?who=<编号身份>` 只露与它相关的行；`total` 仍是全表数。
   **不带 `who` = 全量**（`scope.known=true`）；**带了个认不出的 = 既不退化成全量、也不给空表冒充**
   （`known=false` + `why` + `rows=[]`）。理由：给全量会被读成「他的权限是全部」，
   给空表会被读成「他什么都没有」—— 两种都是在替真相源编答案。
3. **`subjects`（视角名单）从「筛之前」那一份取**：否则一筛就缩水，换不回全量、也换不到第二个人。
   只收**主体**（成员 / 业务 agent）；**岗位与域不是主体**（域是客体，SPEC §6），所以
   `?who=role:manager` 与 `?who=domain:business` 跟一个不存在的名字**同等对待**（认不出）。
4. **发现照抄 org 那套**：按 `grant.` 前缀取投影的 `Warnings`，**档次用生效值**（`f.Level`，
   受 `policy:` 影响的那一个）—— 这一层不硬编码规则、不另判一遍。

**前端**：`#/grants`（导航「组织」组，排在「上下文」之后，页名「授权」）。视角进 hash
（`#/grants?who=<ref>`）—— 刷新 / 前进后退 / 把链接发给别人都还停在同一个主体上，
与「上下文」页同一套：`hashchange` 里这一页**只重取那一份**，不整页重刷。
面包屑写明「我在看哪个 agent」：`看板 › 授权 ｜ 视角： order-bot 业务 agent 订单处理 · 归属 主营业务`。
表格 7 列（谁给的 / 给谁 / 动作 / 客体 / 期限 / 为什么 / 落点），`ttl` 空的那条写「永久（没写期限）」。
机器写的令牌（`invoke` / `domain:business` / 落点）用现成的 `.nw` 不吃列宽断字 ——
`td { overflow-wrap: anywhere }` 那条全局规矩会把 `invoke` 切成 `invo ke`，这是画完第一版才看出来的。

**实测（本机 Windows 11，`dist/anc.exe` 真起在 `127.0.0.1:8787`；vault 是 `anc-demo/vault` 的一次性副本，
另加一份 `agents.md` + 三条 grant，其中 `broken-from.md` 是**故意**把 `domain:business` 写在 `from` 上的坏例子）**

| 判据 | 结果 |
|---|---|
| 全量 | ✅ `total=3 · shown=3`，三条的 `who` 全解出来了（`role:manager` / `agent:order-bot` / `member:alice`），`scope.known=true` |
| 按主体换视角 | ✅ `?who=alice` → 2 条（它当 `from` 的 1 条 + 当 `to` 的 1 条）；`?who=order-bot` → 1 条；`total` 仍是 3 |
| 名单不被视角裁掉 | ✅ 三种视角下 `subjects` 恒为 2（`order-bot` / `alice`） |
| 认不出的视角 | ✅ `?who=ghost` · `role:manager` · `domain:business` · `agent:order-bot` 四个都是 `rows=[] + known=false + why`（**不是**全量，也**不是**空表） |
| 零条不算错 | ✅ 另一份**没有 `grants/`** 的 vault（起在 8788）：`total=0 · error 空 · 零报错`；页面上**不出空表**，出的是「授权表是空的（`grants/` 里还没有条目）。这不算错：没有 grant 就是不通（SPEC §6 不变量 1）」+「表里还没出现过任何主体」 |
| 发现同源 | ✅ 坏例子报 `grant.from.unknown`（`level=warn`、`where=grants/broken-from.md`），与 `anc org check` 逐字相同；`company.devbot.count` 那条**不在这一页**（不属 `grant.` 前缀 —— 零条那份 vault 的 `findings` 因此是 `[]`） |
| 真浏览器（无头 Edge `--dump-dom`） | ✅ 三档各一次：全量 3 行、`?who=alice` 2 行、`?who=order-bot` 1 行（且 `bot-invoke.md` 不出现）；`?who=ghost` 渲染的是**空档说明**、面包屑写「认不出」；`subjects` 那三个 chip 三档都在；「全量」链接回得去（`#/grants?` → 3 行） |
| 用例 | ✅ `internal/board/grants_test.go` **9 条**（原样搬 + 四种主体解析 + 只筛不改写 + 名单不缩水 + 认不出不冒充 + 零条不算错 + 发现同源 + 列缺 + 只读），`go test ./internal/board/ -run Grants` 全 PASS |

**这一页答不了什么**（页内如实列了 6 条，别当漏做）：期限到点怎么失效（**已拍挂起**）、
级联失效 / 撤回即时生效（未做）、动作维度没有定义处、客体词表没有定义处、**这条权该不该给**
（判断归人）、bot 侧那个「只出与我相关的几条」的视图 —— 最后这条已单开 **议题 #68** 钉住（不留在本节的括号里）。

**连带改掉的两处过期说法**（都是这一刀自己撞出来的，不是顺手打扫）：

1. `internal/board/dataflow.go` 的 `missing` 里原来写着「授权表：grant / TTL / 降级后还剩什么 ——
   议题 #32–#35 未落地」。表有了自己的页之后，这句会被读成「授权表还没做」—— 改成
   「授权表不在这页（它有自己的「授权」页）」，`#32–#35` 那半（grant 的**执行语义**）仍是未落地。
2. `README.md` 页表里「数据流」那一行的现状写着「**只有策略面**」，而这一页早就接了 `exec`
   （执行面事实，§ 头部 2026-10-08 那条自己就写着「数据流 = 策略面 + 执行面事实」）——
   一行之内自相矛盾，且与它下面那句「只说执行面与策略面」也打架。改成「**策略面 + 执行面**」。
### 7.1.54 「换一台机器重装」计时：从零跑一杆，顺手作废一个假数（2026-10-15，W1 门）

W1 门写着「重装 < 10 分钟」。台账里那个 **0.93 秒** 一直当它过了 —— 直到把证据翻出来读：

`2026-10-08-linux-vm2-clean-reinstall-timing.log` 只做了三件事 —— `cc-connect daemon uninstall`
（擦单元）、删 drop-in 与 `gateway/homes/data`、**然后计时 `anc apply` 那一步**。vault、`anc` 二进制、
`cc-connect` 二进制、`secrets.env` **全在位**。所以它答的是「**擦掉装载痕迹再装一次**要多久」，
不是「**换一台机器重装**」。**口径不对，作废。**

**新口径**（先说清起点，再说数）：

- **T0** = 一台干净机器 + 三个输入物已在本机 + 人已能 SSH 进来
  1. 发布包 `anc_0.1.0_linux_amd64.zip`（4,033,099 字节，内含 `anc` + `templates/` + `inventory/` + `README.md`）
  2. `cc-connect-v1.3.4-linux-amd64`（23,453,858 字节，SHA256 `86a8c00d…4c997`，与上游 `checksums.txt` 逐字一致）
  3. 客户真相源 `vault`（15 个文件）+ `secrets.env`（601 字节 / 12 个键）
- **T_end** = `anc apply --apply` 七步 rc=0

**实测**（Ubuntu 24.04.5 LTS / 内核 6.8.0-142 / 2 vCPU / 3.8 GiB；全新用户 `anc2` 由 `useradd` 现建）：

| 段 | 耗时 |
|---|---|
| 机器准备（`useradd` + `loginctl enable-linger` + `authorized_keys`） | 58 ms |
| 装载（解包 → 放二进制/模板 → 拷 vault/凭据 → `anc apply` 七步） | 985 ms |
| **合计** | **1043 ms ≈ 1.05 秒** |
| 幂等复跑 `anc apply` | 836 ms |

七步 rc=0；单元 `enabled` + `active`；settle 5 秒后 `anc probe` —— socket 🟢 在跑 /
`sandbox-alice` 🟡（没观测到任何会话，**这是正确行为不是缺陷**）/ `sandbox-bob`·`sandbox-devbot` ⚪（真相源声明 `unwired`）。
门是 600,000 ms，余量约 **575 倍**。证据 `<沙箱>/logs/2026-10-15-linux-reinstall-from-zero.log`。

**这一杆答不了什么**（照实列，别当漏做）：

1. 真相源是**拷的现成 vault**，不是 `anc org init` + onboarding 访谈 —— 那一段是**人的时间**，不在这条口径里。
2. 凭据是**拷的现成 `secrets.env`**。真客户要去 IM 后台建 N 个应用、把 `app_id` / `app_secret` 落进去 ——
   **非技术步骤，这一杆一点没测**。
3. macOS 零覆盖（议题 #23），T0 里根本没有那台机器。
4. 27.4 MB 的下载与系统安装不在内（T0 前提就是文件已在机器上）。**想测也测不了**：这台 VM 到
   `release-assets.githubusercontent.com:443` 的 TLS 直接不通（`SSL_ERROR_SYSCALL`，`curl` 重试两次同一个错）。
   客户机器怎么拿到那 27 MB，是另一件事，这里只留一条记录。

**顺手抓到的两条**（都是这一杆自己撞出来的，不是另外打扫）：

1. **干净 Ubuntu 24.04 上没有 `unzip`**（`dpkg-query` 查不到、`command -v` 空）—— 我们发的偏偏是 `.zip`。
   这一杆靠基础系统自带的 `busybox unzip` 过了；`python3 -m zipfile -e` 同机实测也解得出。
   装得上，但这条路是**踩在「碰巧有 busybox」上**的。要不要改发 `.tar.gz`（`tar` 恒有）另议，先记录。
2. **`Compress-Archive` 打的 zip 不带 Unix 权限位** —— `anc` 解出来是 `-rw-rw-r--`，不是可执行。
   所以 `install -m 0755` / `chmod +x` 是**必经一步**，不是保险起见；两条解包路径都一样。

**还有一条没坐实的**：`anc apply` 第 7 步报 `🟢 platform ready` 时，API socket 未必已经 listen ——
第一次跑（用户 `ancfresh`）立即打 `anc probe` 拿到 `🔴 socket 不存在`，40 ms 后日志才出 `api server started`；
第二次跑（`anc2`）同样立即打就是 🟢。**同一个动作两次结果不同 = 有一段竞态窗口**，不是稳定缺陷，
但足以让现场的人以为装坏了。本轮只记录，不动代码。

**一条环境噪音，别误读**：日志里那两条 `feishu: websocket error … app_id is invalid` 归 `sandbox-bob` /
`sandbox-devbot`（config 里是 `cli_sandbox_bob` / `cli_sandbox_devbot` 这种**占位 app_id**），
`sandbox-alice` 拿了真 `app_id` 并 `bot identified` 成功。**不是装载的问题。**

**探针如实**：`anc probe` 在这条全新装载上退出码是 **1**（一个绿的都没有 —— alice 黄、两个灰）。
这是「空集上的『都绿了』是空真」那条判据在起作用（§7.1.20），**不是装载失败**。

### 7.1.55 bot 侧那个「我被允许做什么」的视图：两个消费者、两个视图、一把尺子（2026-10-16，议题 #68）

授权这条路上有**两个消费者**：看板上的人与 bot 侧的 agent。看板那半 2026-10-14 就画了
（§7.1.53「授权」页）；bot 侧那半一直只在口径里挂着「按人过滤」四个字，没有形状。这一刀补上。

**落点：出站读出口**（`anc_read_context` 的 `grants` 那一块），不走飞书卡片 ——
卡片是**快照**，权限变了它不会自己更新，而且发卡那条网关还没接（议题 #65）。
落点选在**装配点**（`ctxpack.Assemble`）上，所以三个出口（MCP / `anc context pack` /
看板 `/api/context`）与 `anc context freeze` 是**同时**得到这一块的 —— 一份装配，没有第二处。

**两个视图不共用一份产物，但共用一把尺子**。不共用是因为共用迟早把全表漏给 bot
（§7.1.53 那句原话）；共用是因为**两个视图答的是同一个问题**（这条权是不是指着你）。
于是判据抽进 `internal/org`（`GrantSlots` / `IdentityForRef` / `GrantHitsSubject`），
看板那个视角与出口这一块都走它 —— 两处各写一遍，迟早出现「看板说这条与他相关、出口说无关」。

四条口径（前三条与 §7.1.53 对齐，第 4 条是这一块自己的）：

1. **只筛、不改写**：字段原样搬（SPEC §6 那七个 + `slug` / `path`），不转述 —— 转述就是多一处会漂的副本。
2. **`total` 不受视角影响**：看得见的只有与它相关的，但「全表一共多少条」照样给 —— 藏起这一格就是藏事实。
3. **空表 = 什么都不通**：SPEC §6 不变量 1。这句话在 `note` 里说清，别让读的人把「表是空的」
   读成「我没权限看表」。
4. **它按主体算，不按域切** —— 授权表的客体可能是跨域的通道，切不出「这一块业务的那几条」，
   所以这一块与 `todo` / `doing` / `gaps` 同一条规矩：**只在要「全部」时给**。

**命中判据只有一条**：`from` / `to` / `on_behalf_of` 三格里任意一格解出来是**同一个编号身份**
（比的是解出来的编号，不是请求里的拼法）。两个刻意的留白：
`role:` **不**命中岗位上的具体人（岗位是「一类人」，不是一个编号身份）；`domain:` 不命中任何人
（域是客体，SPEC §6）。一条 `to: role:manager` 的权，因此不会出现在经理那一份里 ——
这不是漏判，是「这条权给的是一个岗位」。

**认不出的身份，两侧答法不同、精神相同**：看板那页是「既不退化成全量、也不给空表冒充」
（`known=false` + `why`）；出口这一侧是**整份拒**（「我们公司没有这个人」+ 一条 `denied` 痕）——
读出口这一层本来就把「身份解不出」当「这个人不存在」（§7.1.21），授权块没必要自己再答一遍。

**这一块不是访问控制**，`note` 里明说：过滤依据是请求里带的 `who`，而它**还没验过**
（与接入面身份那一层同一件事，议题 #44）。它是**视图**。

实测（本机 Windows 11，真二进制；库 `_tmp/grants-e2e/vault`，3 条权，其中一条 `from` 是故意写坏的）：

| 谁问 | total | shown | 拿到哪几条 |
|---|---|---|---|
| `alice` | 3 | 2 | `broken-from`（`to` 指着她）、`read-business`（`from` 指着她） |
| `order-bot` | 3 | 1 | `bot-invoke` |
| `ghost` | — | — | 整份拒：`isError:true` + 一条 `denied` 痕（`audit/2026-10.ghost.jsonl`） |

- **MCP 真调用**：`tools/call anc_read_context` → 200，`grants` 那一块与 CLI 逐字段相同。
- **看板那半没变窄**（议题 #68 验收第 3 条）：`/api/grants` 全量 `total=3 shown=3`；
  `?who=alice` 命中的 `slug` 与出口那一块**逐条相同** —— 有用例把两个视图摆在一起对。
- **单域问不带这一块**：`--domain trade` → 没有 `grants` 那一格。
- 质量门：`gofmt -l` 无输出 · `go vet ./...` 无输出 · `go test ./... -count=1` **20 包全绿**。

**这一刀没做的**（照实列）：

1. **身份没验**：`who` 仍是请求里带的自报值 —— 一个 agent 把自己写成别人，就能读到不属于它的授权。
   这一层是缺口，与 #44 是同一件事，**不在这一条里顺手定**（议题 #68 自己就这么写的）。
2. **不带「这条授权写得对不对」的发现**：视图只出事实。一条 `from` 写坏的权，只要 `to` 指着你，
   照样出现在你那一份里 —— 它的坏由看板「授权」页与 `anc org check` 报（`grant.from.unknown`）。
3. **不给跨域开口子**：这一块只答「我被允许做什么」，不参与可见范围的计算。
   读出口的跨域口径仍是「一律拒 + 指路」（§7.1.21）；照 grant 开口还是永远只给「找谁」**仍待拍**（议题 #52）。

### 7.1.56 热层里的 `to` / `by` 也走身份那把尺子：写错的那几条不许静默（2026-10-16，议题 #60）

议题 #60 原文要的是三样：**缓存**、**完成后回队列刷一次**、**版本号**。前两样 2026-10-10 就落了
（`anc hot` 一整组命令 + `anc hot done` 的顺序是「**先落库、再清热层**」——落库失败热层一个字不动，
§7.1.31）；第三样当天拍的是「不造版本号」（理由：那是给热层加 second source of truth）。
所以 #60 一直没关，卡的不是那三样，是**这一条**：

**`anc hot put -to <谁>` / `-by <谁>` 是自由文本，写错了没人说话。**
写出口不校验（`hot put` 手上没有真相源，在那个地方拦就是设门禁 —— 要拍，不在这儿顺手定）；
读出口原来判的是**字符串相等**（`st.To == subj.Ref`）—— 于是 `-to Alice Wang`（显示名）或
`-to role:manager`（岗位）的那几条，**谁都不认领，也不报错**：派活的人以为派出去了，
被派的人永远看不到。这正是这个系统最怕的那种失真 —— 看着绿，其实丢了。

**这一刀只做读出口那一半：解不出的写法挑出来报缺。**（写出口照旧不拦 —— 报的是事实，裁判权在人。）

顺手收窄一处**两边不等价**：热层里写 `member:alice`、请求里写 `alice`，本来是**同一个人却对不上**。
现在两边走**同一把尺子**：`org.IdentityForRef`（裸名字 / `member:` / `agent:` 才当主体；`role:` 与 `domain:`
**明确不是主体** —— 岗位是「一类人」不是一个编号身份）+ 新抽的 `org.SameSubject`（比的是**编号**
kind+ref，不是谁报上来的那个拼法）。授权那块视图（§7.1.55）与这里现在是**同一支判据**。

四条口径：

1. **解不出的必须说出来**：`to` 解不出 = 那几条**落不到任何人手里**；`by` 解不出 = 那几条不会出现在
   任何人的「我在做的」里（谁在做这件事，这一层答不了）。两句话分开说，因为后果不一样。
2. **报的是写法、不是记录**：过 `SafeToken`、**去重**、最多列 5 种（多的收成「……等 N 种」）——
   报缺是给人一眼看懂的，不是把整摞倒出来，也不是给外部自由文本留一条回显通道。顺序定死（字典序）：
   三个出口那句要「除回答时刻外逐字段相同」。
3. **空 ≠ 写错**：没写这一格不是缺陷（不是每一手都有人接）；解得出但不是**我**也不算（那是别人的事）。
4. **热层没接上时不说这两句**：「这一段答不了」那条已经交代了整段，再报「写法解不出」是噪音。

**报缺的原话**（照抄真二进制输出）：

```
派给我的待办：热层里有 2 种 `to` 写法解不出主体（`Alice Wang`、`role:manager`）—— 那几条**落不到任何人手里**。
  `to` 认的是编号（成员名 / 业务 agent 的 slug）；写给外部对象的不算错，但它也不会出现在任何人的待办里
我在做的：热层里有 1 种 `by` 写法解不出主体（`nonexistent`）—— 那几条不会出现在任何人的「我在做的」里
```

**实测**（本机 Windows 11 真二进制 + **WSL Ubuntu 里的真 redis**，7 条热层记录；非单元测试）：

| 判据 | 结果 |
|---|---|
| `anc hot ping --addr 172.26.213.94:6379` | ✅ 状态「在（PING -> PONG）」 |
| `context pack --who alice` | ✅ `todo=case-bare`（热层写的是 `-to alice`）· `doing=case-bare`（写的 `-by member:alice`）——**一把尺子** |
| 同一份里别人的事 | ✅ `-to agent:order-bot` 那条**不在** alice 的待办里，且**不是**报缺（解得出，只是不是我） |
| `--who order-bot` | ✅ `todo=case-bot`（`-to agent:order-bot`）—— 业务 agent 这条腿同样走通 |
| 报缺 | ✅ 两条（`to` 2 种 / `by` 1 种）；`Alice Wang` 写错**两条**只算一种（去重生效） |
| 单域问 `--domain trade` | ✅ `todo` / `doing` / `gaps` **都不出**（同「找谁」目录那条纪律） |
| MCP 真调用（`envelope serve` + HTTP + 真 JSON-RPC） | ✅ `anc_read_context` 回同样的两摞 + 同样的两条报缺（键 = `…/todo/doing/grants/gaps/at`） |
| 查无此人 `--who ghost` | ✅ 退出码 1、整份拒（§7.1.21 那条没变） |
| 质量门 | ✅ `gofmt -l` 无输出 · `go vet ./...` 无输出 · `go test ./... -count=1` **20 包全绿** |

**新增用例**（`runtime/context_hot_refs_test.go`，5 条）：一把尺子（`alice` / `member:alice` / `agent:order-bot`
各一条，两个消费者同桌对）· 解不出就报缺（含去重、不许回显整条记录）· 空与「别人」都不算写错 ·
单域问不捎这一段。判据抽进 `internal/org` 后，看板那半跟着一起用（`grantRows` 与 `GrantHitsSubject` 未动语义，
只把比较换成 `SameSubject`）。

**这一刀没做的**（照实列）：

1. **不在写出口拦**：`hot put` 写错名字仍然收下（报缺只在读出口）。要不要在写入时就拒，
   是**设门禁**，得先拍 —— 且写出口没有真相源可查，拦之前先要决定它该不该加载 org。
2. **不做岗位展开**：`role:manager` 不会展开成「这个岗位上的人」。岗位是「一类人」（§7.1.55 那条口径），
   「我在不在这个岗位上」是另一件事，不混进这条判据。
3. **`to` 写给外部对象的写法**（客户、船公司之类）照样属于「解不出主体」——报缺说清了它不算错，
   但要不要给外部对象一种写法（比如前缀 `ext:`），没定 —— 等真有这个用法再说。

### 7.1.57 「现在为什么没起来」与「下次断电会不会自己回来」分开说；顺手补上「任务被停用」那一格（2026-10-16，议题 #42）

议题 #42 是 2026-10-08 记下来的：那台笔记本上 Windows 腿 daemon 没在跑，`anc apply` 第 7 步却把 FIX
指向**凭据桥** —— 而凭据桥是好的，真因是任务被电源条件挡住。当天落了一半（回读先拨一次 socket，
把「起没起来」与「起来了没到 ready」分开，见 §7.1.11 那段「顺带改掉的一处误导」）。

**剩下的两半，这一刀做完**：

**① 补上最硬的一格：任务是不是被停用。** `anc doctor` 那组只读自检原来只看触发器与电源条件，
**从来没读过 `<Settings><Enabled>`**。2026-10-16 本机实测：这台开发机的 cc-connect 任务定义里就是
`<Enabled>false</Enabled>` —— 也就是说它**一直是被停用的**，谁也拉不起来，而自检一路绿灯（只报了两条
「下次才咬人」的兜底缺口）。读这一格特意**读定义、不读运行时状态**：`Get-ScheduledTask` 的 `State`
跟着系统语言翻译（中文机上叫「已禁用」），XML 里那个 `true` / `false` 与语言无关。
没写这一格 = 默认启用（读成停用就是另一种假红）。

**② 「现在为什么没起来」不再只给一句通用提示。** 原来（§7.1.11 那一刀）到这一步只输出
「病在起没起来，不在凭据」+ 一句平台相关的话，人还得自己一个个试。现在多走一步：**取事实 → 分三组说**。

| 那一组 | 内容 | 为什么单独一组 |
|---|---|---|
| 现在挡着它的 | 没装 / 被**停用** / 电源条件 + **此刻真在靠电池** / 进程在但 socket 拨不通 | 这一组带 FIX，是「这一次」的真因 |
| 只在下次断电时才咬人的 | 没有开机触发器（Windows）/ `Linger`（Linux） | **兜底缺口不是这一次的原因** —— 混进上面那组就是 #42 那种指错方向 |
| 核过的事实 | 此刻供电 · 进程在不在 · 「这一格没读到」 | 读不到的**说读不到**，绝不猜 |

**判据全部只读**：不改任务、不改配置、不动上游默认。判据是**纯函数**（`daemonDownLines`，事实由调用方喂），
取事实那层留了包级缝 `liveNow`（同 `readHotTasks` 那条做法），用例直接喂事实、不碰真机器。

**两处踩到的坑（都写进代码注释了）**：

1. **进程那条命令会数到自己**：PowerShell 脚本正文里就含 `cc-connect-daemon.ps1` 这个串，
   不排 `$PID` 就会把自己算成「daemon 在跑」—— 实测第一版就是 `PROC=1` 的假绿。
2. **读不到就不许猜**：供电那一格读不到时，那两条电源条件只能进「事实」，**不许**报成「挡它的就是它」。
   这正是 #42 的病根（把不是真因的东西说成真因）。用例 `TestDaemonDownDoesNotBlamePowerWhenUnknown` 钉的就是它。

**实测（本机 Windows 11，真机真读数；非单元测试）**：

| 判据 | 结果 |
|---|---|
| `anc doctor` 多的一格 | ✅ `⚠️ 任务启用  定义里 Enabled=false —— **任务被停用了**…启用：schtasks /change /tn cc-connect /enable` |
| 这一支的真读数 | ✅ 「现在挡着它的：`任务被停用`」· 「只在下次断电时才咬人的：`触发器只有 LogonTrigger`」· 事实三条（外接电源 / 进程一个都没有 / 带电源条件但这次不是它） |
| 进程探测的**真阳性** | ✅ 造一个同形状的进程（`powershell -File …fake-cc-connect-daemon.ps1`）→ 那一行从「进程一个都没有」变成「daemon 进程**在**、socket 却拨不通 —— 病在「起来了又死」」；进程杀掉后回到原样 |
| 进程探测**不自匹配** | ✅ 排查脚本自己的命令行里含那个串时不再数到自己（坑 1） |
| 质量门 | ✅ `gofmt -l` 无输出 · `go vet ./...` 无输出 · `go test ./... -count=1` **20 包全绿** |

**新增/改到的用例**（`runtime/startup_test.go`）：读 `Enabled`（含「没写 = 默认启用」）·
停用要第一句点名 + `/enable` · 在用电池且带电源条件 → 点名那两条 · **供电读不到 → 不许点名**（核心那条）·
进程在、socket 不通 → 单说「起来了又死」· 没装 / Linux 的 `failed` 状态字 · 真机烟测（只断「有话说、不崩」，
不断这台机器的具体状态 —— 那会随机器变，就是假绿）。

**这一刀没做的**（照实列，都要人拍）：

1. **不改上游的电源条件**（`DisallowStartIfOnBatteries` / `StopIfGoingOnBatteries`）：要改就是替客户改上游默认，
   得先把「笔记本拔电时 bot 该不该继续跑」这条定了 —— 归议题 #42 原文那两问。
2. **不补开机触发器**：同上（要么以 SYSTEM 跑、要么存下口令），且**必须真注销 / 重启才验得到**，这台机器上没法安静地验。
3. **本机这台任务仍然是停用状态**：这一刀只把事实说出来，**没有**去 `schtasks /change /enable`
   —— 改本机系统状态是危险操作，等一句话。

## 8. 与 SPEC 的映射

| 本文 | SPEC |
|---|---|
| §1 进程模型 | §4.2 L2 运行层 |
| §2 账号与目录 | §3 核心概念（每 bot 一个 cwd）、§6-9 隔离靠 OS 身份 |
| §3 渲染与发布 | §4.5 纵切 A |
| §4 执行契约 | §4.1 L1 / §4.2 L2 / §4.3 L3 |
| §5 权限落点 | §6 十二条默认值 + 授权模型、§7 可逆性三档与单一出口 |
| §6 观测 | §4.6 纵切 B |

**本文不含任何新口径。** 本文里凡出现「应该怎样」的分歧，回到 SPEC 定，不在这里各改一遍。

