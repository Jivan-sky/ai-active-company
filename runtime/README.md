# ANC 运行层装配器（runtime）

> **一句话**：把一台空机器（或一个空目录）在几分钟内变成「客户的 ANC 运行层」——树干目录 + 路由表 + 判层卡 + 计划书骨架 + 七个连接件 + 操作日志与可逆性分级。
> 目的：**驻场时不再从零开始**。用已经做好的东西（FDE skill、技能工厂、知识库 ANC 三件套）快速装配，面对不同客户只换模板与分层，不换骨架。

## 技术选型交代

- **语言：Go 单 exe**。理由：装配器要在客户的 Mac mini / Windows / Linux 上跑，同一份源码三平台；交付物不许自带运行时（判据：跨平台 + 一次性起进程 → Go）。
- **代价**：现场改**逻辑**要重编（约 10 秒）。所以**模板全部外置成纯文件**（`templates/`），现场改模板不用重编译；
  同一份模板**又用 `go:embed` 编进 exe**（议题 #19）——三处磁盘都找不到时落到内嵌那一份，代价是 exe 大 **38.5 KB**（实测，见 `DESIGN.md` §7.1.58）。
- 只用标准库，无第三方依赖 → 交叉编译零障碍。

## 用法

```bash
# 1. 到现场先自检（网络能不能连 IM、git 在不在、模板全不全、有没有全局代理）
anc doctor

# 2. 生成客户运行层
anc init ./climax --client 某车队 --tier 1 --form F1 --git

# 3. 看我们已有的可复用资产（别重造）
anc assets

# 4. 生成 org 真相源骨架（vault）—— 公司 / 角色 / 成员 / 数据目录路由
anc org init ./climax-vault --client 某车队 --id climax --members alice,bob

# 5. 校验骨架（--rules 打印生效规则表），然后渲染 gateway 配置
anc org check ./climax-vault --rules
anc render ./climax-vault --apply

# 6. 起只读看板（本地；前端默认**已嵌在二进制里** —— 不需要 node、不需要外网）
anc board serve ./climax-vault          # http://127.0.0.1:8787
# 现场要换一份前端（改样式、临时补一页、拿真机产物核界面）就指过去，不必重编译：
# anc board serve ./climax-vault --ui-dir ./我的前端

# 7. 装载：渲染 + 校验 + 装上游 daemon + 凭据桥 + 重启 + 回读（默认 dry-run，--apply 才真做）
anc apply ./climax-vault
anc apply ./climax-vault --apply

# 8. 哪些事已经闭环、技能还缺哪段正文（只读；不写 skills/）
anc distill ls ./climax-vault
```

`--tier` 就是九宫格判层：`1` 降本增效型（约 30 人，要省钱）/ `2` 进步型（约 200 人，要进步）/ `3` 战略型（国央企，要战略变革）。**答错层＝后面全错。**
`--form` 是这次交什么：`F1` 48h 薄切片 / `F2` 一页纸真伪结论 / `F3` 现场工具 / `F4` 报告。

## 生成出来的客户库长什么样

```
<客户>/
├─ README.md            这是什么、目录说明、怎么用、三条红线
├─ CONTRIBUTING.md      入库规范（frontmatter schema、命名、目录约定）
├─ CLAUDE.md / AGENTS.md  Agent 路由表 + 五条铁律（双 harness）
├─ 00-inbox/            待整理；_originals/ 放原件，只追加不改写
├─ 10-knowledge/        已整理的客户知识（一条易变事实只在一处维护）
├─ 20-ops/              00-七个连接件.md · 01-操作日志与可逆性分级.md
├─ 30-delivery/         00-判层卡.md · 01-计划书.md
├─ roles/               角色 persona（`anc org init` 往里填；人设只此一处）
├─ 50-skills/           客户专属 skill
└─ company/             公司自描述（onboarding 产物）
```

## org 真相源 → gateway config（`anc render`）

真相源是 git 里的 markdown + frontmatter，**只存岗位与职责**。`anc render <vault>` 把它编译成
cc-connect 的 `config.toml`；**默认 dry-run**，落盘必须显式 `--apply`（时间戳备份 + 原子写）。

```
vault/
	├─ company/company.md         公司自描述 + defaults + policy（规则覆盖）
	├─ roles/<role>/persona.md    角色层：职责 / 风格 / 术语表 + model / mode / allowed_tools
	├─ members/<name>/persona.md  成员层：display_name / role / model / feishu.*
	├─ domains.md                 业务域表（罗盘）：slug / name / what / who / data / sources / terms
	├─ projects.md                项目表（立项书汇总）：slug / name / domain / owner / period / source
	├─ charters/<项目 slug>/      立项书**副本**落点（真源在客户侧；里面放什么 ANC 不解析）
	├─ grants/**/<slug>.md       授权表：一个 grant 一个文件（谁 → 什么动作 → 什么客体 → 多久）
	└─ <数据目录>/CLAUDE.md        顶层数据目录 → persona 段 3 的路由表（首句即说明）
```

两道差分门，防的都是「不可逆」而不是「你不听话」：

- `--adopt`：目标配置没有 anc 指纹 → 拒绝覆盖（那是别人的文件，不是我们的产物）。
- `--allow-scale`：本次会新增 / 删除 project → 拒绝落盘（防一次误操作静默把 bot 上下线）。

两道门互相独立：接管他源配置时 project 集合必然全变，所以 `--adopt` 与 `--allow-scale` 都要显式过。

### 规则表：校验住在数据里，不埋在实现里

`company.md` 的 `policy:` 段可以逐条改规则的处置档：

```yaml
policy:
  member.feishu.open_id.format: off      # 平台是可变层，格式不该拦人
  persona.slot.unreplaced: fatal         # 这个客户要求模板必须零未替换槽位
```

- 三档：`fatal` 拦落盘 / `warn` 照走但回显 / `off` 不报。默认档只有一条判据 ——
  **拦不可逆的**（bot 下线、对外答错、覆盖人的手写文件）；可逆的小事只告警。
- 标 `[红线]` 的不许降级（SPEC §6-5 无例外红线、§171 生产事故）。
- 规则 id 拼错、级别值非法 → 直接拦：拼错等于「你以为关了其实没关」。
- 全表随时可查：`anc org check <vault> --rules`。
- 非红档发现**必须回显**（`anc render` / `anc org check` 都会打），门禁不许静默。

### 业务域表：桥上的罗盘（`domains.md`）

一块业务一行。它是**「域标签」客体的唯一定义处** —— persona 上下文、未来授权的「域」作用域、
看板的分区都从这一张表长出来，所以只有一处维护。

```
| slug | name | what | who | data | sources | terms |
|---|---|---|---|---|---|---|
| trade | 大宗贸易 | 进出口合同的签订、执行与结算 | manager | projects | 邮件、合同扫描件 | 保留英文：pipeline |
```

- `slug` 机器用（进文件名与代码），必须小写 ASCII；`name` 给人看。
- `who` 填**岗位**：渲染时从 `members/` 现算成人名（`Alice、Bob（经理）`），
  所以人事变动不用改表，也不会「表里张三、实际李四」。同一张表既给管理层看「谁在做哪块」，
  也给业务人员查「这件事该找谁」。
- `data` 是**指针**（vault 顶层目录名），不是说明 —— 目录的说明归该目录的 `CLAUDE.md`。
- **列可以多，认不得的列忽略**；列序随便排，按表头认，不按位置猜。
- 表可以空着（只留表头）= 还没划域，不算错。

渲染边界：**自己所属的域整行进 persona（段 8）**；别人的域只给一行目录
（域 / 名称 / 是什么 / 找谁），**不给它的 `data` 与 `terms`**。要跨域就按「找谁」接头、走授权。
`who` 那一列填的是**岗位**（对接人）—— 它同时是**提案该送给谁批**的那个岗：**批准人是那个岗位上的人**
（成员表里的成员，人名由渲染现算）。域表只答「这块业务找谁」，**不答「谁有权批」**。
**没有 `domains.md` 的 vault，persona 一个字节都不变**（存量零影响）。

域表里的字同样要过 persona 的体检（三引号 / `${}` / 绝对路径 …）—— 它也是要进上下文的文本。

规则默认整组 `warn`：域表是新能力，先观察；想卡死在 `policy:` 段写 `domain.slug.format: fatal`。

### 项目表：立项书汇总（`projects.md`）

一个项目一行 —— 它是 **「谁在做哪个项目」的唯一机器落点**：看板按业务分区块、业务人员查
「这件事该找谁」，都从这张表长出来。

```
| slug | name | domain | owner | period | source |
|---|---|---|---|---|---|
| trade-q3 | Q3 结算改造 | trade | manager | 2026-07-01 → 2026-09-30 | https://example.feishu.cn/docx/xxxx |
```

- **ANC 不定义立项书**：格式随公司 / 行业 / PMO 而变，而且它在各人的 agent 那边生成，
  到 ANC 时就已经存在。这里只做两件事：约定**落点**、把立项书**汇总**成这张表（agent 抽取）。
- 四个锚点 —— 哪个项目 / 挂哪块业务 / 卡住找谁（填**岗位**）/ 周期多久 —— **一个都不设必填**：
  抽不到就先空着 = 待确认。**门禁不许长在别人的文档上**（缺字段不报，只报「连 slug 都没有」
  这种「这一行会被忽略」的事实，且默认 `warn`）。
- `domain` 填域表里已定义的 slug；`owner` 填**岗位**（渲染 / 投影时现算人名，人事变动不用改表）。
- `period` 是**自由文本**，原文照搬 —— **不解析成日期**。这是「不定义别人的文档」的第一条验收。
- `source` 是**指针**（飞书 URL / doc id / 路径），不是说明。
- 跨表只告警：挂的域不存在、owner 不是真岗位 —— 两条都默认 `warn`，与域表同一档，
  可在 `policy:` 段上调成 `fatal`。
- 表可以空着（只留表头）= 还没抽，不算错；看板投影里就是 `[]`。

**没有 `projects.md` 的 vault，行为一个字节都不变**（存量零影响）。

### 立项书副本落点（`charters/`）

**接入契约里最小的那一半**：ANC 只约定**放哪、名字怎么对得上**，不规定里面是什么。

```
vault/charters/<slug>/    一个项目一个子目录，目录名 = projects.md 的 slug
                          （里面放什么、叫什么名字，由交付的人定）
```

- **真源在客户侧**（飞书 Wiki / 多维表格 / 客户自己的库）：`projects.md` 的 `source` 列指向它。
  这里是**同步副本** —— 用途是断网 / 换人 / 客户改权限之后还查得到。
- **副本可以没有**：真源才是权威版本，副本是保障 —— 缺了不算错，也不拦任何东西。
- 目录名对不上任何项目行 → `charter.entry.unmatched`，**只告警**：副本比表行**早到**是正常的
  （材料先丢进来、抽取还没做），那是进度问题不是错。
- 落点自己的说明文件（`CLAUDE.md` / `README.md`）不算项目条目：**扫描只认子目录**。
- 它**不进** persona 段 3 的路由表（`skipDirs`）—— 那行「数据来源」回答的是「业务资料去哪查」，
  副本落点不是资料；而且资产页已经单独统计它，再进路由表会重复计一次。
- **谁在什么时候把副本放进来**（触发器 / 同步节奏 / 冲突判定）属于运行态，**还没定**
  （阶段 C + 议题 #36）—— 这一节只约定落点。

### 授权表（`grants/`）

**控制面，不是业务资料** —— SPEC §6 的授权模型落在这里。**一个 grant 一个文件**：

```
vault/grants/<怎么分层随你>/<slug>.md
```

```markdown
---
grant:
  from: member:alice          # 谁给的：member:名字 | role:岗位 | 裸名字（按 成员 → 岗位 顺序解）
  to: role:manager            # 给谁
  action: read                # read | write | invoke（认不出的照收，只报）
  object: 10-knowledge        # 客体标识
  ttl: forever                # 期限：可选；不写 = 永久（2026-10-10 改口径）。要期限就配，如 30d
  on_behalf_of: member:alice  # 署名（可省）：bot 发出的必须带
  reason: 试点期先给一个人看知识库   # 一句话：为什么给（可省）
---

正文随便写，ANC 不解析它。
```

- **扫描是递归的**：目录怎么分层是给人**快速定位**用的（按部门 / 按项目 / 按客户都行），
  换分层不用改代码。顺序按路径定 —— 快照 / diff 才有确定字节。
- **判据：有 frontmatter 才算条目**。落点里的 `CLAUDE.md` / `README.md` 按名字跳过（说明文件不是条目）；
  名字不在忽略名单、又没有 frontmatter 的**报出来**（`grant.file.unparsable`）——
  静默丢掉一条授权，比报一条错危险得多。
- **一个文件里塞两条也报**（`grant.file.multiple`）：第二段压根不会被读，不报就等于那条授权悄悄没了。
- **表默认零条**：没有 `grants/` 目录 = 还没有任何授权，**不算错**（存量 vault 一个字节都不变）。
  「没有 grant 就是不通」是口径，不是缺陷（SPEC §6 不变量 1）——所以它不是「没配置」，是「明确不通」。
- **这一层只做结构校验**：四个必填字段一个不缺（`from` `to` `action` `object`）+ `from` 解得出成员或岗位。
  `to` / `object` 的客体词表（信道 #33、看板分区 #7）**还没有定义处**，现在校验它们只能是猜 ——
  宁可留白，不做「看着像校验」的猜测。「这条权该不该给」更不是校验器的事：那是人拍板的事。
- **域不能当 `from`**（`domain:trade` → `grant.from.unknown`）：域是**客体**不是主体 ——
  给权的人自己得先有这个能力。
- **不进 persona 段 3 的路由表**（`skipDirs`）：那行「数据来源」回答的是「业务资料去哪查」，
  授权表是控制面。也**不进任何 bot 的工作目录** —— SPEC §3：真相源仓库 ≠ bot 的工作区。
- **不放 token、不放密钥**：这张表记的是「谁被允许做什么」。凭据怎么发、发给谁，是执行层的事。
- **期限可选，不写 = 永久**（2026-10-10 改口径；原口径是「必填、统一 30 天」）。客户按自身需求配
  （`30d` / `90d` …）；写了多少，执行层就按多少算。**到点怎么失效、怎么级联、能不能续期**归执行层 ——
  这一节只约定**形状**。出厂档：`grant.ttl.missing` 是 **`off`**（不写期限不报）—— 想要「授权必带期限」，
  在 `company.md` 里写 `policy: grant.ttl.missing = warn`（规则是数据，不是删掉），见 `DESIGN.md` §7.1.15。
- **结构那十条**出厂档 **九条 `warn` + 一条 `off`**（`grant.ttl.missing`）：与域表 / 项目表 / 接入面
  同一条先例 —— 新能力先「看见」、不先「拦」，一条红线都不设。要提红 / 关掉，在 `company.md` 的
  `policy:` 段写 `grant.action.unknown = fatal` / `grant.ttl.missing = warn`（实测见 `DESIGN.md` §7.1.15）。
- **执行那一条（`grant.match.missing`）出厂就是 `fatal`**：跨域信封没有被任何一条 grant 覆盖 →
  **拒收**（SPEC §6 不变量 1「默认拒」）。它与上面十条不同 —— 那十条是结构校验（先看见），
  这一条是**执行层**（出厂就拦）。要松开写 `policy: grant.match.missing = warn|off`，规则是数据。
  见 `DESIGN.md` §7.1.32。
- **看板投影里带 `grants`（全量、不筛选）**，这张表也有了自己的页（`#/grants`，全量 + 按主体换视角）。
  注意看板是**全量**、bot 侧该看的是**只属于它那几条** —— 两个消费者、两个视图，不共用一份产物
  （bot 侧那个视图**仍未做**）。
### 平台身份 → 谁（`anc org who`）

```
anc org who <vault> [--open-id <ou_…>] [--app-id <cli_…>] [--project <名>] [--json]
```

平台的嘴递过来的**不是编号**，是三样别的东西 —— 谁发的（`open_id`）、发到哪个 app（`app_id`）、
落在哪台 bot 上（project 名）；而流水 / 审批 / 授权里说的是人 / 岗位 / 域 / project。
这一条就是把那三样各查一遍，**只查表、不猜**：查不到就说查不到（带原因）。

- **三样各有一个入口**，也能一次问两头：`open_id` → 成员（业务 agent 不代表人，没有 open_id）；
  `app_id` → 成员 bot 或业务 agent；`project` 名（`<公司 id>-<名字>`）→ 同一批；
  裸编号（直接写 `alice` / `tradebot`）也收 —— 前缀必须对齐本公司 id，`other-alice` 不算。
- **两头各自解、互不代偿**：一头解不出就如实报，不拿另一头顶上。`--app-id` 与 `--project`
  一起给时以 app_id 为准（它更贴平台事实）。
- **「属于谁」只在业务 agent 上答**：域 → `domains.md` 的 `who`（岗位）→ 该岗位上**启用中**的成员，
  并回一句**判据原话**（哪块业务、哪个岗位、岗位上是谁）。人没有归属人 —— 他属于他自己。
- **归属 ≠ 收件**：域 who 岗位上没人时，渲染那边会落到公司 admins 兜底（不兜它就叫不动这个 agent），
  那是**收件**口径；归属这一格**空着并说明原因**，不写「归老板」。
- **它不做判断**：谁能批、谁不能批是授权层的事 —— 这条命令一个 if 都不加。
  设计记录见 `DESIGN.md` §7.1.38。

### 进门判定（`anc org access`）

```
anc org access <vault> --open-id <ou_…> [--domain <slug>] [--json]
```

回答「这个 `open_id` 在这家公司里是谁、能看哪些域」。**白名单制**：对不上
`members/*/feishu.open_id` 一律拒绝，`disabled` 的成员同样拒绝 —— 没有游客档。

- **拒绝是判定结果，不是命令失败**：退出码仍是 0，脚本读 `--json` 的 `allowed`。
- **管理员两条路都认**：`company.md` 的 `admins`，或该成员写 `admin: true`。
  两处都认，是为了「随时把某人提成最高权限」既能在公司层集中做，也能在人身上单独做。
- **跨域可见性是客户口径**：`company.md` 的 `access.cross_domain`，`hidden` = 只看自己域，
  不写 = `visible`（2026-10-19 拍板）。写错档是红档 `company.access.cross_domain.invalid`。
- **不属于任何域的公司级资料**（slug 为空）对登录进来的人都可见：它本来就渲染在每个人的 persona 里。
- **它只答「看」**：派任务 / 审批 / 改配置的口径还没定全（R-12 / #62），这里不替它们表态。

### 只读投影出口（`anc org export`）

```
anc org export <vault>            # stdout 出一份 JSON，消费方自己重定向
```

给**看板 / 前端**吃的只读视图：公司 / 角色 / 成员 / 业务域 / 项目（含副本落点）/ 数据路由 / **授权表**，
带 `schema`（`anc.board/v2`）—— 字段增删一律改版本号，前端据此判断能不能吃。

- **纯函数**：同一份真相源 + 同一时刻 → 逐字节相同，可以按字节 diff。
- **只读**：不写真相源、不碰机器、不生成任何能给 gateway 吃的东西。
  这也是它和 `internal/render` **分两个包**的原因 —— 混在一起，迟早有人拿「生成看板」的路径去生成配置。
- **刻意不导出** `feishu.app_id` / `open_id` / `extra_allow_from` / `allow_chat`：
  看板是**观测面，不是凭据面**（SPEC §2.3 / §6-3）。「这人绑没绑好飞书」属于阶段 C 的探针。
- `who` 是岗位，投影里派生成 `who_label`（`Alice Wang（经理）`），
  与 persona 用的是**同一处口径**（`org.WhoLabel`）—— 免得看板和 persona 各存一份名单。
- 项目的 `charter` 是副本在 vault 里的**相对路径**（`charters/trade-q3`），空 = 还没进副本 ——
  看板据此显示「这个项目的材料进来了没」。
- `grants` 是授权表的**全量**投影（十条字段一条不筛）—— 看板回答「谁被允许做什么」的全貌。
  **bot 侧要的是另一个视图**（只出与它相关的那几条），两个视图不共用一份产物：
  共用迟早把看板的全量漏给 bot。
- 非红档发现走 **stderr**，stdout 恒为纯 JSON（有测试盯着这条）。

### 外部材料进真相源（`anc ingest`）

```
anc ingest <vault> --from <提案.json|-> [--dry-run] [--patch] [--json]
anc ingest --dump-shape            出厂提案形状（数据）；不需要 vault
```

真相源的写者**只许有两类**：人手改 markdown，或者这条管道。第三方系统（含客户自己的知识库）
**不许直接写** —— 那就是第二个真相源。这一条不靠约定：**别处根本没有那个入口**。

提案是这个形状（**三个字段，多一个字段一律报错**）：

```json
{ "by": "张三", "why": "立项会那份材料",
  "writes": [ { "path": "projects.md", "content": "……整份文件……" } ] }
```

- `content` 是**整份文件**，不是补丁：补丁要匹配上下文，人写错一行整条都落不下去。
  整份替换的判据是「换完之后这棵树合不合法」—— 与人手改 markdown 是同一把尺子，
  git 那边照样只看得到一行 diff。
- `by` 必填：与信封「缺 `who` = 拒收」同一条口径（身份认错，留痕全记在错的人头上）。
- **落点白名单四处**：`domains.md` · `projects.md` · `agents.md` · `members/<成员名>/<文件>`。
  成员名那段走与 org 同一把尺子（它会落成目录名、也进 project 名）。
- **四步**：形状与落点 → 改在 **vault 的临时副本**上用 `anc org check` 同一张规则表校验 →
  有红档**不落盘**（逐条说清哪一条不过）→ 通过才原子写回真 vault，并记一条审计。
  第 1–3 步真 vault 一行没动。
- **一条都没变**（逐字相同）→ 不落盘、也不记审计：同一个改动留两条痕，账会失真。
- **它不 git commit**：历史归人 —— 落完自己 `git diff` 看一眼再提交。
- **落盘前那一眼**：`--patch` 把「这份提案会改成什么样」打成 unified diff（与 git 那份同形状，
  顶格那份可以直接喂 `git apply -p1`）。提案里装的是整份文件全文，光看 JSON 看不出改了哪几行 ——
  这一步就是给 review 用的；**被红档拦住时不打 diff**（那种提案一条都落不下去，配 diff 只会让人以为就差点头了）。
  不给 `--patch` 时两个出口与从前逐字一样（有用例钉着）。
- **形状是数据**：`anc ingest --dump-shape` 把出厂提案形状打出来（字段 / 白名单 / 规矩 / 边界），
  给 harness 那一面照它说话，不抄文档（抄一份迟早漂）。白名单也来自同一处常量。
- 智能那半（harness 出提案）**只提案、不落盘**：给它的能力件在 `../skill/anc-ingest-propose/`
  —— 它读形状、读现状、出提案，自检走 `--dry-run --patch`，**落盘仍由人点头后跑这条管道**。

设计记录见 `DESIGN.md` §7.1.50。
### 看板（`anc board serve`）

```
anc board serve <vault> [--addr 127.0.0.1:8787] [--data <data 目录>] [--hot-addr 127.0.0.1:6379]
                        [--ui-dir <前端目录>] [--insecure-bind]
```

一个**几乎只读**的本地看板（写只有两处：「待批」页的点头、热层终态条目的归档，见下）。**分页，不挤一页** —— 一页只回答一个问题，那句问题就写在页头：

前端**默认**用嵌在二进制里的那份（一个 exe 就起来）；`--ui-dir` 指到磁盘上的一份就**覆盖**它。
覆盖是替换不是叠加，且只换「前端文件从哪来」—— 路由、只读口径、端点一个都不动。目录写错直接退。
默认只绑本机；绑到非回环地址必须显式加 `--insecure-bind`，否则拒绝启动（要给别人看请走 ssh 端口转发）。

| 页 | 回答什么 | 读者 | 数据源 | 现状 |
|---|---|---|---|---|
| 总览 | 现在有没有事？ | 所有人 / 老板 | 投影 + 校验发现 | 已接入 |
| 待批 | 哪几件事在等人点头、该谁点？ | 该点头的人 | 待批队列（热层 `prop:` 前缀）+ 三个信号 | 已接入（**能写**；点头只留痕；每条带靛蓝「等你给一句话」那一档） |
| 组织 | 公司长什么样、边界在哪？ | 管理者 / FDE | 投影（域 / 角色 / 成员 / 路由） | 已接入 |
| 项目 | 谁在做什么、到几号、卡住找谁？ | 跟进的人 / 老板 | 投影（`projects.md`） | 已接入 |
| 上下文 | 一个 agent 的工作上下文是什么？ | 接手的人 / FDE | 装配点（`internal/ctxpack`，与 `anc context pack` 同一处）+ 真相源 | 已接入（**一个装配点、两个出口**：看板一页 + 冻结副本） |
| 授权 | 谁被允许对谁做什么？ | 管理者 / 想知道「谁被允许做什么」的人 | 授权表（`grants/**`，逐条搬、一个字不改） | 已接入（**全量** + 按主体换视角；判断归人，机器只校验与留痕） |
| 时间线 | 卡在哪、谁在跟、下一步是谁的决定？ | 跟进的人 / 老板 | 决策与执行的留存（`timeline/*.jsonl`） | 已接入（**留存**：人自己写下来的） |
| 审计 | 谁在什么时候、对谁、行使了什么（含被拒的）？ | 管理者 / 关心「谁碰了什么」的人 | 行使的流水（`audit/<YYYY-MM>.<行使者>.jsonl`） | 已接入（**事后归集 + 显式补记**） |
| 运行态 | 每个 bot 活着吗、真的能回话吗？ | 运维 / FDE | 防假绿探针（socket 真拨 + 会话事实 + 真相源里的 `unwired` 声明） | 已接入（**四档**：绿 / 黄 / 红 / 灰实心「未接凭据」） |
| 数据流 | 通道开着吗、每个 bot 手里有什么？ | 管理者 / 老板 | 策略面（真相源）+ 执行面（gateway 真吃的那份 config） | 已接入（**策略面 + 执行面**；事件面在审计页，授权表在「授权」页） |
| 原料 | 原料有哪些、多久没动了？ | 所有人 | 真相源的数据目录 + 目录里现在有什么 | 已接入（**原料 ≠ 沉淀**，沉淀层见 #26 / #27） |
- **数据源是十一份各管各的契约**：投影（`anc.board/v2`，改组织才动）、授权表（`anc.board.grants/v1`，
  按视角筛、但不改写）、工作上下文（`anc.context/v1`，装配点的只读出口）、校验发现（与 `anc org check`
  同一张规则表）、运行态（`anc.runtime/v1`，读 gateway 的 data 目录）、数据流（`anc.dataflow/v1`）、
  原料（`anc.assets/v1`）、决策与执行留存（`anc.timeline/v1`）、行使的流水（`anc.audit/v1`）、
  待批队列（`anc.approvals/v1`，在读的是**热层**、不是真相源）、热层探针（`anc.hot/v2`）。
  **各判各的**：一份坏了不该让别的页跟着空着。看板不自己算口径，也不另存一份 —— 免得看板和 persona 各说各话。
- **数据流页只说执行面与策略面**：通道口径读的是 gateway **真吃着**的那份 `config.toml`
   （「我们打算让它吃什么」不算数）；有没有越权尝试属于**事件面** —— 那在**审计页**，本页只答策略面与执行面。
- **授权表有两个出口**：投影里的 `grants`（`/api/board`，全量）+ 它自己的页（`/api/grants`，`#/grants`，
  全量 + 按主体换视角）。那一页只答授权表本身；「通道开着吗」仍在数据流页（读的是执行面 `config.toml`
  与投影）—— 两个数据源、两个变化节奏，别互相当替身（见 `DESIGN.md` §7.1.53）。
- **原料页不叫沉淀**：数得出目录里有几个文件、多久没动，但「有文件」离「有资产」差着一整层，
  所以标题、口径、页脚全写「原料」。页名保留「沉淀」是因为那才是这一块最终要回答的问题。
- **时间线页是「留存」，不是「审计」**：那一行行是人和 agent 用 `anc timeline add` **自己写下来的**（谁**尝试**做了什么、含被拒的，在**审计页**），
  看板只折叠、只呈现 —— 不替谁总结，也不编一条。三档**只按 status 配色、形状跟着档走**（`done` /
  `running` / `blocked`·`failed`），**认不出的词原样保留、画灰**：以后加词只改数据、不改代码。
  **灰不许长得像绿** —— 实心圆 / 带光环的圆 / 菱形 / 空心虚线圈，色盲、灰度截图、小屏也分得开
  （见 `DESIGN.md` §7.1.18）。
  一个 case 多行 = 一次推进，表里取最后一条当当前态，历史一行不删。
- **一个意思一个符号**（五个，见 `DESIGN.md` §7.1.51）：绿实心圆 = 已完成 / 黄带光环的圆 = 运行中 /
  红菱形 = 卡点·失败（**最大最重** —— 它是唯一要求人动手的那一档）/ 靛蓝暂停形 = **等人给一句话**
  （只在「待批」页出现：在待批队列里 = 有人在等你点头，**不从 status 推**）/ 空心虚线圈 = **没人判过**
  （认不出的词）/ 灰实心 = **本来就不该期待**（真相源声明 `unwired`）。两种灰必须分开：
  一个要人去判它，一个不要 —— 共用一个符号就没人分得清「没判过」和「没轮到它」。
- **只读是默认；写只有两处，都与 CLI 同源**：其余一律只放行 GET / HEAD，别的方法 405（带 `Allow` 头）——
  没有多出来的写入口，就不存在多出来的越权面。两处写口：
  - `POST /api/approvals/decide`（「待批」页那次点头）：与 `anc approvals decide` **同一个入口、
    同一份留痕**（`timeline` 一条 + `audit` 一条），**只留痕、不写真相源**；
  - `POST /api/tasks/settle`（热层终态条目的归档）：与 `anc task settle` / MCP `anc_task settle`
    **同一个领域入口**（`internal/tasksettle`）—— 先落 `timeline`、再清热层，幂等；
    queued / running 默认拒结，要终止必须显式给 `status`。
  两条口都只收 `POST` + `application/json`（跨站表单发不出这个 Content-Type，浏览器会先发预检
  而看板不回 CORS 头）、请求体限 64KB，**不校「点的人是不是该批的人」**（`by` 原样留痕，
  判断交给 agent；归档时中文显示名会尽量对到已登记成员的 slug）。
- **「待批」页要热层**：起服务时给 `--hot-addr`（或 `ANC_HOT_ADDR`，默认 `127.0.0.1:6379`）。
  **接不上不拦看板**：那一页如实报「没接上」，不拿一份空队列冒充「没人提」。
- **默认只绑 `127.0.0.1`**：看板里有组织与项目信息。要给别人看请走 ssh 端口转发；
  绑到别处会打警告（不拦你，但你得知道自己在干什么）。
- **不含凭据面字段**：`feishu.app_id` / `open_id` 一律不进投影；连「文件缺失」这类
  到不了报告的报错，也会把本机绝对路径摘成 `<vault>` 再出门。
- **红档不隐藏**：`/api/issues` 如实端出校验发现，界面把它摆在最上面 ——
  vault 坏了的时候，看板正是用来看「哪里坏了」的。
- **十一页都接上了，不代表每页都答得全**：答不了的部分在页内单独一块如实列（沉淀的**沉淀层**；审计页也自己列缺 —— vault 外的行使只报落点、真凭据下的归集没验、「本可以行使但没行使」永远归集不到），导航上也直接标「已接入 / 未接入」，不靠人点进去才发现。
  **不编数据填界面** —— 宁可承认不知道，也不把「真相源里定义了角色」说成「bot 在跑」
  （与「防假绿」同一条纪律）。
- **总览页的「还有哪几页没接数据源」是从 `PAGES` 现算的**（以前手抄一份，写着「两块还没接」，
  而导航早把那两页标成「已接入」了 —— 手抄的清单迟早跟导航说不一样）。
- **路由是浏览器原生 hash**（`#/org`）：加一页＝加一条 `Page`（`boardui/src/app.ts`），
  后端零改动 —— 已接入的页吃的是同一份 `/api/board` + `/api/issues`，切页不重新打网络。

前端源码是 TS（`runtime/boardui/`），**只在构建期存在**：`npm run build` 打成
`internal/board/ui/app.js`，再由 `go:embed` 打进二进制。交付物里没有 node、没有 node_modules，
现场编译也不需要 npm —— **产物要一起提交**，否则二进制里还是旧界面（有测试盯着这条）。

### 决策与执行的留存（`anc timeline`）

```
anc timeline add <vault> -by <谁> -title <一句话> [选项]
anc timeline list <vault> [--limit N] [--before <RFC3339>] [--band <档>] [--status <词>] [--json]
```

「卡在哪、谁拍的板、agent 怎么执行的、成没成、根因是什么」—— 落成**行文本**，
真相源是 `timeline/<YYYY-MM>.<作者>.jsonl`（append-only）。看板的「时间线」页就是它的只读出口。

- **为什么不是数据库**：vault 归 git 管，留存记录最需要的恰恰是 diff / review / 回滚 ——
  二进制库这三样全丢。
- **为什么按作者分片**：两个 agent 同时写也碰不到同一个文件 —— 并发就地消掉，不靠锁。
- **词表不锁死**：配色只认 `done`(实心圆·绿) / `running`(带光环的圆·黄) / `blocked`·`failed`(菱形·红)；
  认不出的词**原样保留**、画空心虚线圈（`--band unknown` 能把它们挑出来）。加词只改数据。
- **不设门禁**：`add` 不校验「谁能写」—— 那是授权层的事（#32–#35）；`kind` 是约定、不是白名单，
  写别的也收。
- **一个 case 多行 = 一次推进**：折叠取最后一条当当前态，`recent` 带最近 5 条，历史一行不删。
- **目录不存在 = 还没开始记 = 空时间线，不是错**（刚 `init` 的机器不该因此报红）。
- 坏行只报 `文件名:行号`，不带本机路径（看板是观测面，不是凭据面）。

### 行使的流水（`anc audit`）

```
anc audit log <vault> [--actor X] [--result ok|denied|failed|unknown] [--action read|write|invoke]
                       [--cross] [--limit N] [--json]
anc audit add <vault> -actor <谁> -action <read|write|invoke> -result <ok|denied|failed|unknown> [-object <对谁>]
                       [-why <原话>] [-at <RFC3339>] [-on-behalf-of X] [-tool X] [-source X] [-ref X] [-detail X]
anc audit collect <vault> [--dir <会话记录目录>] [--since <RFC3339>] [--write] [--json]
anc audit tools [--json]
anc audit rules [--json]           打印生效的脱敏表（不打印已知密钥值，只报条数）
```

「谁在什么时候、对谁、行使了什么 —— 含**被拦下的**和**失败的**」。真相源是
`audit/<YYYY-MM>.<行使者>.jsonl`（append-only，一行一条，旧行永不改写）。看板的「审计」页就是它的只读出口。

- **不在行使路径上记账**：SPEC §6 的纪律是「能放在 harness / 平台的权限规则里拦，就不要放在业务代码里拦」。
  所以流水从两条腿来：**事后归集**（`collect` 从 harness 自己的记录里抽 `tool_use` + 结果配对，
  默认 dry-run、`--write` 才落盘）+ **显式补记**（`add`）。
- **一条必须写在明面上的边界**：「**本可以行使但没行使**」**永远不可能自动归集** —— 它没有发生，就没有记录；
  只有人补记的那一类才答得了。
- **只记不拦**：不设门禁、不自己发明一套读权限（读权限归 #32/#34，留存期归 #27/#25）；`add` 不校验「谁能写」。
- **落盘前强制脱敏**：`object` / `why` / `detail` 里夹带的密钥值，一律抹成
  `«已脱敏:<规则名>»`（**不是抹成空白**）。落点是全项目唯一的写口，
  所以归集 / 补记 / 信封关卡 / 读出口 / 审批留痕五条路都过它。规则是**数据**
  （`anc audit rules`），来自三处：出厂表 + 凭据文件里的已知值（`~/.anc/secrets.env`）
  + 凭据文件自己的键名。**这一层没有开关** —— 脱敏不是门禁，是无条件的内容变换。
  读口也过一遍（兜底），并把「**落盘时就是明文**」的行如实报出来。
  设计记录见 `DESIGN.md` §7.1.36。
- **结果四档，`denied` 与 `failed` 是两件事**：「被拦下」去查授权，「做了没成」去查环境；词表不锁死。
- **不存派生值**：流水存当时的事实（`object` 原样）；「在哪个域、算不算跨域」按**当前**域表算 ——
  域表改了，历史记录跟着重判，而不是留一堆变假的旧结论。落点四类：域内 / 跨域 / **域外** / **vault 内非域**
  （后两类分开：一条是「手伸到 vault 外面」，一条是「在库里但不在任何业务域」）。
- **认客体三种形状，按「越明确越先」**：① 引用形状 `domain:<slug>`（授权层的客体写法，
  词表只有一处定义）② vault 下的路径 → `domains.md` 的 `data` 列 ③ 裸 slug 兜底
  （出站读出口与审批留痕写的就是裸 slug，而域的 `data` 目录名未必等于 slug）。
  **域表里没有的 slug 不算「没事」**：它落「**跨域未判**」—— 归在域内、但跨域判不出来；
  不落「vault 内非域」，那会把一次域上的行使读成没事。别的引用形状（`member:` / `role:` / `project:`）
  不解：四类落点里没有它们的位置，如实留在「vault 内非域」这个已知缺口里。
- **看板不带自由文本**：`object` 归一（vault 外只留末两段），`why` / `detail` 一个字段都不出门
  （自由文本必然夹本机路径），页上只留一个「原话见 CLI」的标记。
- **坏行不许静默跳过**：逐行读，坏行进报告，只报 `文件名:行号`。
- **退出码**：`collect` 有「记不到的」时退 1（报告口径，不是门禁 —— 该落盘的照样落盘）；参数错退 2。

### 交付链路的门（`anc gate`）

```
anc gate <vault> [--rules <文件>] [--show-rules] [--json]
```

留痕有了，下一个问题是「**门过了没有**」。出厂五道 —— 诊断 / 证明 / 装机 / 沉淀 / 重构，
判据分别来自《ANC 架构设计大框》§8.2 的落地链路与 SPEC §8 三个阶段门（不是这里现编的）。

- **事实只有两个来源，都是真相源**：`org`（人 / 岗 / 域 / 项目 / 授权）+ `timeline` 留痕。
  判据不自己造事实 —— 它只决定「这几个数放在一起算不算一件事」。
- **一道门就是一个 case**，命名 `gate:<slug>`（例：`gate:proof`）。记法：

  ```
  anc timeline add <vault> -by fde -case gate:proof -status done -title "一线班长当场说这能帮到我，停止条件双方认可"
  ```

- **门名与判据都是数据**：出厂五门 × 两条（没落痕 / 有卡点）+ 一条全局缺勤 = 11 条。
  `--show-rules` 看生效表，`--rules` 整份换掉；加一道门 = 加两行数据，不改代码。
- **只判它看得出来的那两类**：「一线那句话算不算数」「产能到底扩没扩」要人判 ——
  判据只负责把「该判的没落痕」摆到人面前，不替人下结论（同 SPEC §13 Q17：裁判权留给人工）。
- **不设门禁**：只出结论，判据不影响退出码。（读不到 vault 这类真错误仍然非 0 —— 那不是判据的意见。）
- 配色与看板同一套：`done` = 实心圆·绿（通过）/ `blocked`·`failed` = 菱形·红（卡点）/ `running` = 带光环的圆·黄；
  认不出的词画**空心虚线圈·灰**。灰不算绿也不算红 —— 把灰当绿，就会把「没人判过」看成「过了」
  （形状口径见 `DESIGN.md` §7.1.18）。

### 进行中的状态（`anc hot`）

```
anc hot ping [--addr 127.0.0.1:6379] [--prefix anc] [--ttl 0]
anc hot put <id> [-status <词>] [-by <谁>] [-to <谁>] [-domain <域>] [-project <项目>] [-title <一句话>] [-note <备注>]
anc hot ls [--stale 24h] [--json]
anc hot claim <id> -by <谁> [--lease 30m]
anc hot release <id> [-by <谁>]
anc hot drop <id>
anc hot done <vault> <id> -by <谁> [-title <一句话>] [--kind execution] [--status done]
```

**达标的事落库，还在做的事实放热层。** 状态是「当前值」，不是「历史」：拿行文本承载当前值，
写一百次就留一百行，最后没人说得出「现在谁在做、卡在哪」。历史与结论仍然归 `timeline/`（append-only）。

- **介质是配置**：默认 Redis（一个独立小进程）；地址 / 前缀 / TTL / 超时全走 flag 或环境变量
  （`ANC_HOT_ADDR` / `ANC_HOT_PREFIX`），代码里不留死值。
- **TTL 不是新鲜度**：`--ttl` 是**删数据**，只给「授权 / 租约」那种「到点就该失效」的东西用；
  新鲜度靠 `as_of` + `--stale` **打标记**、**过期不删**（`--ttl 0` = 不失效，是默认）。
- **认领是原子的**（`SET NX`）：谁在做这件事只能有一个；`--lease` 到期自动放开，人 / agent 断了
  不会把事情永远锁住；放开只认认领人自己（替别人放会被拒）。
- **`done` 的顺序是「先落库、再清热层」**：往真相源写失败时热层一个字都不动 —— 反序就丢东西。
- **客户端是手写的**：只用得到十几条命令，所以不引第三方 Redis 客户端（零依赖、单 exe、
  `CGO_ENABLED=0`）。换回标准客户端只动 `internal/hot/resp.go` 一个文件。

### 任务队列（`anc task`）

```
anc task add <vault> --to <agent> --title <一句话> [--id <id>] [--by <谁>]
                     [--note <正文>] [--domain <域>] [--project <项目>]
                     [--session <会话>] [--intent queue|interrupt|new_session]
                     [--root <根 case>] [--parent <父工作项>] [--accept <验收口径>]
                     [--due <RFC3339>] [--priority <词>]
                     [--refs a,b] [--max-attempts <n>] [--max-hop <n>]
anc task pull <vault> --who <agent> [--limit 1] [--lease 10m] [--json]
anc task ack <vault> <id> --who <agent> --status done [--fence <n>] [--note <回执>]
anc task handoff <vault> <id> --who <agent> --to <agent> --fence <n> --note <为什么> [--refs a,b]
anc task submit <vault> <id> --who <agent> --reviewer <agent|成员> --fence <n>
                              [--accept <验收口径>] [--note <说明>] [--refs a,b]
anc task verify <vault> <id> --who <验收人> --verdict accepted|rejected [--note <意见>]
anc task escalate <vault> <id> --who <agent> --to <agent|成员> [--fence <n>] --note <卡点>
anc task notify <vault> [--data <目录>] [--send] [--limit 50] [--json]
anc task reap <vault> [--json]
anc task ls <vault> [--who <agent>] [--stage queued|running|review|blocked|done]
                    [--status <业务词>] [--overdue] [--limit 50] [--json]
```

**ANC 只入队、观测、记录；不 push、不杀会话、不替 agent 决定执行策略。** 业务 agent 下次
`pull` 才看到任务；`intent=interrupt` 只是请求，打不打断当前回合由 agent 自己决定。

- **`stage` 与 `status` 是两根轴**：`stage` 是协议阶段（queued / running / review / blocked /
  done / cancelled），`status` 是业务词。业务可以加词，机制不跟着猜；认不出的词不会挪动阶段。
- **认领发围栏号**：每次 `claim` / `reap` 都 `INCR` 一个 fence；回执带上当前 fence，旧持有者
  拿着旧号回来写会被拒。租约过期不等于任务结束，只说明「这个人可能断了」。
- **写状态走 CAS**：任务路径的每次写入都带 `version` 比对（Redis EVAL 原子执行），
  `version` 由存储层推进；两个写者撞上时晚到的一刀拿 `ErrVersion`，不会互相覆盖。
  `anc hot put` 是显式人工覆盖，仍允许无条件写。
- **回收在 `pull` 之前跑**：过期租约回队列、`attempt+1`、`fence+1`；超过 `max_attempts`
  才进 `blocked` 等人接手 —— 是升级，不是删除，也不是判失败。
- **回执只有被派的人能写**：`ack` 必须给有效租约；`done` 先留在热层，等 `settle` 落库后才清走。
- **handoff 不是重新派单**：只有当前持有者能交，必须带 `fence`，`note` 写清为什么；
  `root / from / refs` 保留，只换 `to`，`hop+1` 回队列。超过 `max_hop` 进 `blocked`。
- **跨域交接默认放行但留痕**：调用方按 `agents.md` 的 `domain` 算跨域，热层不据此拒；
  看板标红、`audit/` 记一条。要硬拦，是业务自己的门禁，不是这里偷偷加。
- **submit / verify 是验收那一段**：`submit` 必须有验收口径（任务上已有可沿用），
  进 `review` 并放开租约；`verify` 只认提交时写的验收人，`accepted → done`，
  `rejected → 回作者队列`且理由必填。自验允许，但会标 `self` 并告警。
- **escalate 一定带着下一手**：`running` 中要租约 + `fence`，`review` 中要是参与方；
  结果进 `blocked`，`to` 写明交给谁 —— 卡住但不知道找谁，等于没升级。
- **截止与优先级只描述事实**：`--due` 是 RFC3339，超期由 `as_of` 现算成 `overdue` / `overdue_secs`，不改 `stage`、不自动升级；`--priority` 是自由文本，不参与 pull 排序。`task ls --overdue` 只是筛选，不是门禁。
- **通知先落箱、再发送**：`anc task notify` 默认 dry-run，不写发件箱；`--send` 落 `data/state/task-outbox.jsonl` 并按 `org.ProjectName` 路由，发一条推一格游标（`task-outbox.cursor.json`），只发 `overdue / blocked / review_pending`。语义是 at-least-once：宁可重发，也不要「以为送了、其实没送」。
- **配置不写死**：地址 / 前缀 / 租约 / 尝试上限都走 flag 或环境变量
  （`ANC_HOT_ADDR` / `ANC_HOT_PREFIX` / `ANC_TASK_LEASE`）。

### 日报导入（`anc import reports`）

```
anc import reports <vault> <日报目录> [--to <agent>] [--since YYYY-MM-DD]
                                   [--by <谁>] [--limit N]
                                   [--no-timeline] [--no-tasks] [--apply] [--json]
```

**只把日报里的既成事实搬进来，不替 agent 改执行策略。** 默认 dry-run，`--apply` 才落盘。

- **读结构，不猜语义**：`## 完成事项` / `## 关键决策` 的 bullet → timeline；
  `## 遗留待办` / `## 今日待办` 的 checkbox 与表格（按「任务 / 状态」列） → 待办。
  `###` 子标题下的说明性 bullet 不当待办；`---` 之后回到顶层。
- **待办以最后一次出现为准**：同一标题在多份日报里反复出现，只留最后一次；
  最后一次是 `[ ]` 才入队，`[x]` 只进 timeline，`~~划线~~` 跳过。
- **稳定 id**：`rpt-<日期>-<段>-<hash>`。重导不会重复写 timeline，也不会重复入队。
- **导入不关闭任务**：日报说做完了只写 timeline；任务状态仍由回执 / 验收决定。
- **落点只有两处**：`timeline/<YYYY-MM>.<by>.jsonl` 与热层任务队列；不写 vault 的 markdown。

没写 checkbox 的顶层 bullet 按未完成收并给出 warning；解析不了的文件逐份列出
（`没有可识别的章节` / `章节在，但没有可导入的 bullet / 表格`），不静默跳过。
### 审批的落点（`anc approvals`）

```
anc approvals add <vault> <信封.json> [--at RFC3339]
anc approvals ls [--json] [--stale 24h]
anc approvals decide <vault> <提案 id> -by <谁> -signal approve|reject|hold [-why <一句话>]
```

**只管「等人点头」那一段。** 提案（`kind: proposal`）进来 → 进待批队列（热层，`prop:` 前缀）→
人给一个**可识别信号** → 落痕两笔（`timeline` 一条 `decision` + `audit` 一条 `invoke`）→ 清队列。

- **ANC 只做三件**：收提案、送给该批的人、留痕。**不写真相源** —— `grants/` 那个文件由
  **人侧的管理者 bot** 写（提案是人话，结构只出现在终点）。
- **该谁批**：域表 `who` 那一列 —— **岗位**，不是人名（人名由展示层现算）。解不出只提示、不拦 ——
  队列里宁可带一句「不知道该谁批」，也不要一个空的 `to` 让人以为只是没填。
- **只认三个字**：`approve` / `reject` / `hold`（同意 / 驳回 / 挂起）。别的一律拒 ——
  把「他这句算不算批准」交给模型猜，就是把门交给一个能被说服的东西。`ask` 不在这张表里。
- **不校「点的人是不是该批的人」**：先原样留痕，判断交给 agent。要校，是后面的事。
- **顺序**：先落库（`timeline`）→ 再落审计 → 再清热层。库没落成，队列不动 —— 反序会把一次点头弄丢。
- **幂等**：重复入队不冲时间戳；不在队里再点头 = 报错（`exit 1`），不静默成功。
- **两个嘴**：**看板这条已接**（「待批」页 + 写口 `POST /api/approvals/decide`，见 README 的看板一节）；
  **飞书卡片这条 ANC 侧已接**（`anc approvals card` 出卡片、`anc approvals card-action` 收回程，
  两处都走**同一个信号入口** —— CLI / 对话式 bot / 看板是同一个变更管道的三个前端，SPEC §7）。
  卡片长什么样是**数据**（帧住在外面的 JSON 里），键与值全是**现算**的业务事实：
  谁提的 / 这个 agent 属于谁 / 该谁批的是哪个人 / 等了多久 / 该什么颜色。改帧不用重编译。
  **缺的只有网关那一侧的口**：cc-connect v1.3.4 没有「外部工具发卡片」的 API 面
  （unix socket `POST /send` 只有文本与附件），飞书回调把点击递给我们也没有现成通道 ——
  这一段见 `DESIGN.md` §7.1.39，上游 PR 记在那里。

### 接入面的信封（`anc envelope`）

```
anc envelope check <vault> <信封.json> [--json]
```

不管对面是 Claude Code、Codex、DSH、Hermes、OpenClaw，还是一个插件、一条 MCP ——
进了 ANC 就只剩一种东西：**信封**。网关可替换（A 换 B 是 adapter 的事），**信封不变**（#44）。

- **判据**：只靠信封就能答出六件事 —— 谁 / 代谁 / 哪块业务 / 要什么 / 证据在哪 / 要不要人拍。
  `check` 就把这六问答出来。后两问**只列原文、不做判断**：要不要人拍由信封自己说（`needs`），
  我们不从字符串里猜意图。
- **身份不靠自报**：`who` 与 `on_behalf_of` 必须能在真相源（`members/` / `roles/` / `domains.md`）里
  解出来；解不出不是「格式错」，是**这个人不存在**。所以分两段报：**结构**（读不读得懂 ——
  缺 `id` / `ts` 不是 RFC3339 / 缺 `who` → 拒收，退出码 1）与**绑定**（指向真不真 —— 一条发现）。
- **只有三样必需**：`id` / `ts`(RFC3339) / `who`。其余留空不拦 ——
  现场先发一句，比逼人填全字段然后干脆不发要好。
- **词表不锁死**：`kind` 只认 `ask` / `report` / `notify` / `ingest` / `proposal` 去归类，
  认不出的**原样保留**（落一条 warn）；`needs` 是自由文本，一个词都不认。加词只改数据。
- **不设门禁**：七条发现整组默认 `warn`（只回显，不拦）。想提红或想关掉，写进
  `company.md` 的 `policy:` 段（`envelope.who.unknown = fatal`，`= off` = 整条不出）——
  **这就是「开关」：门禁是数据，不是代码里的 if**。生效档位看 `anc org check <vault> --rules`。
- **授权那一组出厂就拦**：`scope_domain` 写的是**别人的**域（跨域）而在 `grants/` 里没有一条覆盖
  → `grant.match.missing`，出厂 `fatal` = **拒收**（退出码 1）。这是 SPEC §6 不变量 1「默认拒」。
  想先看见不先拦，写 `policy: grant.match.missing = warn|off`。见 `DESIGN.md` §7.1.32。
- **提案（`kind: proposal`）不是行使**：它**不判授权** —— 还没拿到权的人当然没有 grant，拿 grant 当提案的门
  就等于**提案永远提不出来**。提案里没有任何业务数据，只是一句请求；它**照样算一次跨域请求**，所以照样留痕
  （`audit` 一条 `ok`，理由写明「这是请求」）。见 `DESIGN.md` §7.1.33。
- **想拦烂提案**（`who` 不是真人那种）：不用新代码 —— 在 `company.md` 写
  `policy: envelope.who.unknown = fatal`（绑定那一组默认 `warn` = 只回显、不拦）。
- **域 / 项目表为空就不报**：没有 `domains.md` / `projects.md` 的存量 vault，
  不该因为「有人发了封信」被提醒 —— 门禁不许长在别人的文档上。
- 真相源本身是红的（`org.Load` 红档）→ 绑不了、直接退出 1，不编一份绿灯。

### 接入面（`anc envelope serve`）

```
anc envelope serve <vault 目录> [--addr 127.0.0.1:8791] [--data <data 目录>]
                                 [--hot-addr 127.0.0.1:6379] [--hot-prefix anc]
                                 [--insecure-bind]
```

harness 通过 **MCP（streamable HTTP）** 调工具。**桥的两半挂在同一个入口上**：
默认只绑本机；绑到非回环地址必须显式加 `--insecure-bind`，否则拒绝启动。

```
anc_send_envelope(who, on_behalf_of?, kind?, body, scope_domain?, scope_project?, refs?, needs?)   # 递进来
anc_read_context(who, on_behalf_of?, domain?)                                                      # 要出去（只读）
```

`anc_read_context` 是 **W2 的出站读出口**（2026-10-09）：问「这块业务是什么 / 数据在哪 / 卡住找谁 /
证据在哪」，回一份 **JSON**（字段就是契约）。

- **可见范围 = `render.VisibleDomains`，与 persona 段 8 同一把尺子**：自己的域全行，
  别人的域只给名称 / 是什么 / 找谁；显式要**别人的域 → 拒**，但**指路**（告诉你要找谁）。
- **身份不许自报**：`who` 必须能在 `members/` 里解出来 —— 解不出**不是格式错，是这个人不存在**。
- **每次调用留一条痕进 `audit/`**（谁 / 要什么 / 给没给 / 为什么拒）；**给了也记**。
- **不泄漏本机路径**：指针一律 vault 相对（`domains.md` + 行号），回话里没有任何绝对路径。
- **只读**：一个字都不写回真相源。
- **一个入口**：两个工具都开在同一个服务端、都在「一次调用 = 一条痕」之内 ——
  多的不是入口，是同一个入口上的第二个动作。
- **无状态**：不发 `Mcp-Session-Id`、不记会话、不提供 SSE 流。信封自带全部路由所需信息，
  所以收到就能办；连接断了也不用重连 —— 下一封信自己会到。
- **服务端补 `id` 与 `ts`**：harness 不该为「这封信叫什么」操心。
- **捎上「派给我的待办」**（议题 #58 的第一刀）：读出口多答两格 —— `todo`（热层 `to` = 你）、
  `doing`（`by` = 你），按**编号升序**、状态**一条都不筛**（热层里还留着一条 `done`，说明它还没清走 ——
  达标是「先落库、再清热层」，那不归这一层判）。热层由 `--hot-addr` / `--hot-prefix` 指
  （也认 `ANC_HOT_ADDR` / `ANC_HOT_PREFIX`，默认 `127.0.0.1:6379` / `anc`）；
  接不上**不拦接入面** —— 递信封、要上下文照样能用，只是那两格**报缺**
  （`gaps` 最前面那条「这一段答不了 —— **不是**「没人派给你」」），绝不拿空清单冒充「没人派给你」。
  `to` / `by` 走**同一把身份尺子**（裸名字 / `member:` / `agent:` 都认；显示名、岗位解不出）——
  解不出的写法**报缺**（「那几条落不到任何人手里」），绝不静默（§7.1.56）。
  这是**反向下发**那一半的落点：ANC 不往会话里插话（插了就是打断），agent 在自己会话开头 pull。
- **多答一格「我被允许做什么」**（议题 #68，2026-10-16）：`grants` —— 授权表里**与这个主体相关**的
  那几条（`from` / `to` / `on_behalf_of` 三格解出来是同一个编号身份）。四条口径：**只筛不改写**
  （字段原样搬）· **`total` 给全表数**，不受这一份视图影响（不藏事实）· **空表 = 什么都不通**
  （SPEC §6 不变量 1，在 `note` 里说清）· **只在要「全部」时给**（授权按主体算，切不出「这一块业务的那几条」）。
  命中判据与看板「授权」页那个视角**同一支**（`org.GrantHitsSubject`）—— 两个视图、两处落点、一把尺子。
  **它不是访问控制**：过滤依据是请求里带的 `who`，而 `who` 还没验过（议题 #44），`note` 里明说。
  一条 `to: role:manager` 的权**不**出现在经理那一份里（岗位是「一类人」，不是一个编号身份）。- **门禁跟 `check` 完全一致**（走同一份规则表）：默认 warn = **照收**（落盘 + 回话）；
  被 `policy:` 提成 `fatal` 的 = **拒收**（不写日志、回 `isError`）——
  因为 `fatal` 在这个库里的定义就是「阻止落盘」。
- **授权那一组出厂 `fatal`**：跨域信封没有 grant 覆盖 → 拒收。**拒也留痕** —— 被拒的信封进不了
  信封日志，所以另落一条 `audit`（`result=denied`）；跨域且被授权的那次同样留痕（`result=ok`，
  `Detail=grant <落点>`）。**域内信封不通 grant、不进 audit**（信封日志已经记着它）。见 §7.1.32。
- 日志落 **data 目录**（`<data>/envelope/<YYYY-MM>.<who>.jsonl`，按 who 分片、append-only）：
  这是**流量**、不是真相 —— 真相在 git 里，每封信进 git 就是每天刷 diff；
  也避免 vault 顶层多一个目录被 org 当成「数据来源」扫进路由表。
- 默认只绑本机。**它不构成「开了一个入站端口」**：绑 `127.0.0.1`，卖点「免公网 IP」不受影响。

**实测（2026-10-08，本机真调）**：Claude Code 与 Codex 各走 MCP HTTP 真调通，
**同一份信封、同一台 ANC、零改动** —— 换 harness 只换接线：

| harness | 实测链路 | 备注 |
|---|---|---|
| Claude Code 2.1.291 | `initialize`(报 **2025-11-25**) → `notifications/initialized` → `tools/list` → `tools/call` ✅ | 用 `--mcp-config` + `--strict-mcp-config` **只在一次运行内接线**，不动全局配置 |
| Codex 0.160.1 | `initialize`(报 **2025-06-18**) → `tools/list` → `tools/call` ✅ | 需 `-c 'mcp_servers.anc.tools.<工具>.approval_mode="approve"'`：默认审批策略 `never` 会把 MCP 工具调用拦下 |

**「回版本号」这件事是实测教出来的**：两家报的 MCP 协议版本**不一样**，服务端**原样回客户端报的那个**
才都通 —— 写死一个版本，报另一个版本的客户端就走了。

**出站读出口的本地实测（2026-10-09，本机）**：真 HTTP + 真 JSON-RPC 往返（`runtime/envelope_read_test.go`，11 条）
—— `tools/list` 两个工具都在；`who=alice, domain=logistics`（跨域）回 `refused` 且 `audit/` 里落一条 `denied`；
`who=nobody` 回「我们公司没有这个人」；四种请求的回话里都搜不到 vault 绝对路径与盘符。

### 工作上下文包（`anc context pack` / `anc context freeze`）

```
anc context pack   <vault> --who <编号> [--domain <slug>]
anc context freeze <vault> --who <编号> [--domain <slug>] [--to <目录>]
```

一个 agent 的**工作上下文** —— 身份 / 业务域 / 数据源 / 可见范围 / 技能来源 / 在跟的项目 ——
**按需算出来的一份只读投影**（议题 #53）。

- **不新增真相源**：真相源仍是 vault 里的 markdown（persona 各段 / `domains.md` / `projects.md` /
  `roles.skills` 清单 + `skills/*/SKILL.md` 正文）。这份包是**纯函数产物**，**默认不落盘常贮** ——
  存下来就成了第三处会漂移的源。要一份**冻住的副本**就走 `freeze`：写进 `<vault>/context/`
  （`--to` 可换），是**快照不是常贮** —— 再冻一次就覆盖上一次。它也不是真相源：改它不影响别人，
  `context/` 已在数据路由表外（`anc org export` 的 routing 里没有它）。
- **一个装配点、三个出口**：它与 harness 走 MCP 调的 `anc_read_context`、看板那页 `/api/context`
  **共用同一个函数**（`internal/ctxpack`）—— 三处打出来的那一份**除回答时刻外逐字段相同**
  （有用例盯着）。给 agent 走 MCP，给人 / 给交付走 CLI，给人当场看走看板。
- **缺项报缺**：`gaps` 明说这份上下文答不了什么（技能正文没落 / 没挂项目 / 新鲜度 / 版本），**不编**。
- **只读真相源**：一个字不写回真相源，也不落 `audit/` —— 留痕是「一次行使」的账，挂在 MCP 那个入口上；
  CLI 这条是操作者在自己机器上打一份投影 / 冻一份存档，性质同 `anc org export`（要一次带留痕的行使，走 MCP 那个口）；
  `freeze` 只写自己那一个落点（`<vault>/context/`），不碰别处。
- **拒了就不冻**：查无此人 / 越域，`freeze` 退出 1，**不建目录、不留空壳** —— 冻结副本只在真答出来时存在。
- **要单独一域**（`--domain`）就只给那一行：不塞「找谁」目录，也不塞通用报缺。
- **捎上「派给我的待办」**（议题 #58 的第一刀）：`todo`（热层 `to` = 你）/ `doing`（`by` = 你），
  除这两个键之外契约一格没动。热层由 `--hot-addr` / `--hot-prefix` 指
  （也认 `ANC_HOT_ADDR` / `ANC_HOT_PREFIX`，默认 `127.0.0.1:6379` / `anc`）——
  读不出来**报缺**（`gaps` 里那句「这一段答不了」，与「没人派给你」分开说），不拿空清单冒充后者。
  `to` / `by` 走同一把身份尺子、解不出的写法报缺（§7.1.56）。要单独一域时不捎这两摞（同「找谁」目录那条纪律）。
- 退出码：`0` 答出来了 / 冻上了 · `1` 拒了（这个人不存在 / 越域 / 署名解不出）或真相源读不了 · `2` 用法错。
- 设计记录见 `DESIGN.md` §7.1.47（包）· §7.1.48（看板的「上下文」页 + 冻结副本）· §7.1.49（捎上「派给我的待办」）·
  §7.1.55（bot 侧的授权视图）· §7.1.56（`to` / `by` 的身份尺子与报缺）。

**接法（不变量）已定形**（2026-10-09）：`../SPEC.md` **§4.7** —— 网关 **G1–G4** + 四条禁令、
harness **H1–H4**、业务 agent 独立账号的五件套。判据一句话：**换一个网关、加一条 harness 腿，
是「加一张表 + 一个适配器」，不是重写一套。**
上面那张 harness 实测表就是 H1 / H2 的现场证据；**H3（原生记录 + 用量三口径）与 H4（工具名映射）
的第二条腿仍未实测** —— 见 `DESIGN.md` §7 清单最后两行。
形状与口径见 `internal/envelope/`（解析与绑定）与 `internal/mcp/`（协议层）。
可见范围那把尺子在 `internal/render/domains.go`（`VisibleDomains`）—— persona 与出站读出口共用。
**协议层补了「合规对拍」（2026-10-09）**：手写的 MCP 不靠「两家客户端通了」这一条孤证 ——
`internal/mcp/conformance_test.go`（7 条）钉协议形状，`mcp_conformance_test.go`（1 条，真 HTTP）
钉真工具面。它当场抓到一个真缺陷：缺 `arguments` 时工具收到 `nil` map（一写 `args[...]` 就 panic），
已修并**变异验证**过。批量请求 `-32700`、不提供 SSE 是**有意不支持**，一并钉住防止被当 bug 修掉。
细节见 `DESIGN.md` §7.1.14。### 验证到了哪一步（别把「一致」当成「已验证」）

- `--check` 比的是「现在这份 == anc 上一轮生成的」。它能抓人的手改、能抓 org 的变化，
  **抓不到渲染器自己的 bug**，也**不代表 gateway 已经吃下这份配置**。
- 真实拉起 + 功能探针（kickstart + 90s 窗口）在阶段 B 的服务单元里落地。
- 测试三层（`go test ./...`）：性质测试（确定性 / 往返读回 / 时间只影响指纹头）、
  负面断言（**只断规则 id，不断文案**）、一个结构快照 `testdata/golden/one.snapshot`（`-update` 重写）。
  不存全文 golden —— 它的失败模式是「改一个字就红」，然后人就会习惯性 `-update`，测试就死了。
- 看板已验证：端点（首页 / app.js / favicon / 投影 / 发现）、只读口径（写方法一律 405）、
  内嵌产物完备（挡「忘了 build」）、不外泄（越权路径 404、本机路径被摘）、真监听 + 优雅关闭。
- 看板**浏览器侧**也已实跑验证（无头 Chrome + CDP）：六个 hash 路由各自渲染、点导航切换、
  浏览器后退回到上一页、页签标题跟着页走、**零控制台错误**；页面实际加载的四个资源
  （`/app.js`、`/favicon.svg`、`/api/board`、`/api/issues`）全 200；420px 窄屏实测
  无横向溢出（`scrollWidth == clientWidth`），导航自动折成横向可滚 tab。
  **仍未验证**：真机观感（字体、毛玻璃、你的屏宽下的折行）—— 自己跑一次
  `anc board serve <vault>` 打开看才算数。
- 数据流 / 原料两页（2026-10-08）：端点、只读口径、不外泄（标识符与绝对路径都不出门）、
  空数组不是 null，都有测试。**浏览器侧也重验了**（无头 Chrome `--dump-dom`，六个 hash 路由
  各渲一遍，对着真沙箱断言页面上确实出现了真数据：`demo-alice` / `通道关着` / `dontAsk` /
  `10-knowledge` / `最近动的文件` 等）。**没验的**：控制台错误数（要 CDP 才测得到 —— 2026-10-08 实测：`--enable-logging=stderr`（含 `--v=1` / `--vmodule=console=2`）这条路**抓不到 JS 控制台**，拿一个故意 `console.error` + `throw` 的对照页跑，也是 0 命中。所以「0 条」不等于干净，别拿它当证据；`--dump-dom` 只证明页面把真数据渲出来了）、
  真机观感（字体、毛玻璃、屏宽折行）。
- 时间线页（2026-10-08）：端点（空目录不是错、折叠、坏行不泄本机路径、分页不重不漏、按档 / 按状态筛、
  参数错不当 500）与 CLI（`add` 真写进沙箱 vault、按作者分片、`list --json` / `--limit` 给出 `--before`
  游标 / `--band red`）都有测试。**浏览器侧也真跑过**：新装 `dist/anc.exe` 起在 `127.0.0.1:8787`，
  在真沙箱 vault 里用 `anc timeline add` 记了 4 条（两个 case + 一条独立、含一个**认不出的 status 词**），
  `--dump-dom "#/timeline"` 断言页面上确实出现了真数据与真原词（`feishu-reply`、`waiting-upstream`、
  `灰 · 认不出的词`、`demo-bob`），并对着 `/api/timeline` 核过 `counts`（1 黄 / 1 红 / 1 灰）。
  **没验的**：真机观感（字体、毛玻璃、屏宽折行）；控制台错误数（理由同上，要 CDP 才测得到）。
  上面那三条 CDP 级断言（导航点击切换 / 浏览器后退 / 零控制台错误）仍是**六页时**跑的结论，
  加了第七页之后**没有重跑**；这一页的 DOM 级证据只到「真数据真渲出来了」。
- 审计页（2026-10-08）：端点与 CLI 都有测试（四档计数、落点四类、坏行不泄本机路径、自由文本不出门、
  `collect` 幂等、`add --at` 给错退出码 1 而 `--limit` 给错 2）。**浏览器侧也真跑过**：`--dump-dom "#/audit"`
  断言页面上出现了真数据（`⚠ 跨域` / `⚠ 域外` / `判不出（目标抽不出）` / `原话见 CLI`），且无本机路径泄露。
  **没验的**：真机观感；控制台错误数（要 CDP 才测得到）。上面那三条 CDP 级断言（导航点击切换 / 浏览器后退 /
  零控制台错误）现在是**六页时**的结论 —— 现在十一页，**仍未重跑**（「重跑」指 CDP 那三条：
  导航点击切换 / 浏览器后退 / 零控制台错误。**DOM 级**跑过两轮：九页时逐路由 `--dump-dom` 页页命中真数据、
  无红块；2026-10-15 补跑「授权」页三档（全量 / `?who=alice` / `?who=ghost`），三档都命中，
  且「认不出」那一档渲染的是**说明**、不是空表）。
- 待批页（2026-10-10，写口之一）：端点与拒法都有测试（`GET` 写口 → 405 + `Allow: POST`、
  跨站表单 → 415、匿名/认不出的信号/坏 JSON → 400、不在队列 → 409、热层没接上 → 503；契约是 snake_case，
  另有一条专门断言**不泄漏 Go 字段名**）。**真 VM 实测**（Ubuntu 24.04 + 真 Redis 7.0.15 + 真 HTTP）：
  只读那一路拿到两条待批；点头 200 且回执把两个落点（已摘本机路径）+「真相源没动」都摆出来；
  再点一次 409；痕两笔落进 `timeline` / `audit`；`anc approvals ls` 跟着空 —— **一个前端一套状态的坑**没踩。
  **浏览器侧也真跑过**：Edge 无头 `--dump-dom "#/approvals"` 断言页面上出现真数据（两条待批、`manager` /
  `devbot`、`已等 2 小时`、三个按钮），再用 **CDP 真点了一下「同意」** —— 填 `by` → 点 → 回执出现 →
  队列 2 条变 1 条。**没验的**：真机观感（字体、屏宽折行）；飞书卡片在**真飞书**里的观感
 （卡片 JSON 与回程都是真的，见下一段的实测 —— 但「点一下真按钮」这一步过不了网关那一侧，缺上游的口）。
- 热层归档（2026-10-18，看板第二个写口）：`POST /api/tasks/settle` 与 CLI / MCP 走同一个
  `tasksettle.Settle`。测试覆盖：GET 写口 → 405 + `Allow: POST`、匿名 → 400、queued / running → 409、
  已沉淀 → 200 `already=true` 且不追加第二行、中文显示名 → 成员 slug。**真 Redis + 真 HTTP 实测**：
  done 任务从看板归档后热层归零、`/api/timeline` 出现绿色 case；重复归档只回「早已沉淀」。
- 可沉淀候选（2026-10-18，只读）：`internal/distill` 纯函数 + `anc distill ls` + `/api/distill` +
  看板「沉淀」页下半部分。测试覆盖：绿档才算候选（先 done 后 running 不算）、折叠取最后一条、
  signals 逐项、同名统计、技能缺正文 / 孤儿技能、空 timeline 不是错、`limit` 截断如实报
  `truncated` 且 `candidate_total` 不变。**真 vault 实测**（沙箱 vault）：
  CLI 回 2 条候选 + `im-send` 缺正文；新二进制起在独立端口，`/api/distill` 回
  `schema=anc.distill/v1`、`wired=true`、`skills_wired=true`，内嵌 `app.js` 含新页面代码。
  **浏览器侧也真跑过**：无头 Edge `--dump-dom "#/sediment"` 断言页面上出现了真候选
  （`ANC 当前落地的 top3 硬卡点`）、`im-send` 与`缺正文`、以及「这一块只读」。
  **草稿链路**：单测覆盖绿档筛选 / 事实-only Markdown / 名字规则 / 不碰正式技能 / 重复拒绝 /
  `--force` / 未闭环拒绝 / 非法名退出 2；真 vault 跑 `distill draft` 生成 1231 字节草稿。
  **发布前校验**：单测覆盖 fail / warn / ok 三档与「明文不回显」；真 vault 副本上未补四段
  `4 fail · 2 warn`（退出 1），补完四段 `0 fail · 2 warn`（退出 0）。
  **没验的**：真机观感与浏览器控制台（要 CDP / 人眼）。
- 探针第四档「灰」（2026-10-09）：单元 6 条（`internal/probe` / `internal/render` / `internal/board` /
  端到端各一条，共 7 条），**变异验证**过（删掉 `AllGreen` 里那行 `continue` → 两条立刻变红）。
  **真 VM 实测**：`unwired: true` 声明 → `anc probe` 退出码 **0**、`anc apply` 回读一致、
  `anc notify` 不喊、`--json` 里 `"state": "unwired"`；反向用例（三个 bot 全声明未接）**退出码 1** ——
  **空集不判绿**（细节见 `DESIGN.md` §7.1.20）。
- **出站读出口（W2，2026-10-09）**：单元 **10 条**（`runtime/envelope_read_test.go`，全部走真 HTTP + 真 JSON-RPC）
  + 尺子 3 条（`internal/render/domains_test.go`）；**变异验证**过（删掉「跨域被拒」那行留痕 → 3 条变红；
  把 `safeToken` 两层清洗同时去掉 → 单行断言变红）。**本机实测**：`tools/list` 两个工具都在；
  `who=alice, domain=logistics` 跨域 → `refused` + `audit/` 落一条 `denied`；`who=nobody` →
  「我们公司没有这个人」；四种请求的回话里**搜不到 vault 绝对路径与盘符**。
  **真 VM 实测（2026-10-09，Ubuntu 24.04，部署上去的那个二进制）**：四问一次答出（what / data / who / 证据指针）·
  跨域 `isError` + 指路且回话里没有别人的「数据在哪」· 无身份 → 「我们公司没有这个人」·
  域表只有表头的存量 vault 不报错（回 `note=还没给你划域`）· `audit/` 里每条调用一行（**给了也记**）·
  **除 `audit/` 外全树 sha256 调前调后逐字节相同**（只读是实测的，不是声称的）·
  **Claude Code 2.1.292 真调通**（`initialize` → `tools/list` → `tools/call`，一次调用答出四件事）。
  **没验的**：Codex（只在本机走了 JSON-RPC 往返）· 那条 harness 的 `deepseek-flash` 模型名不被
  Claude Code 认识（会打 `[claude-code:unrecognized_model]` 警告、按 200k 算 auto-compact）—— 待拍板才动。
  细节见 `DESIGN.md` §7.1.21。
- **工作上下文并进同一次调用（W3，2026-10-09）**：`anc_read_context` 在原来的四问上多答两样 ——
  `skills[]`（技能清单 + 正文在不在，逐条 `missing`）与 `projects[]`（在跟哪个项目，按可见范围过滤），
  外加 `gaps[]` **报缺**（没登记 skills / 声明了但正文没落 / 可见域上没项目 / 新鲜度与版本**没有真相源**）。
  **同一把尺子**（`render.VisibleDomains`）、`period` / `source` 原样搬、指针 vault 相对、只读。
  单元 **5 条**（`runtime/envelope_read_test.go` 10 → 15 条）· `gofmt` 空 · `go vet` rc=0 · 十四包全绿。
  **没做的**：新鲜度 / 版本 / 跨 agent 共享上下文 / 蒸馏产物 —— 只出现在 `gaps` 里，不假装有。
  细节见 `DESIGN.md` §7.1.23。

## 装载（`anc apply`）

一条命令把 org 变成**一台机器上真在跑的东西**。默认 dry-run，`--apply` 才真做。

```powershell
anc apply ./climax-vault                    # 打印这七步要看的东西，什么都不动
anc apply ./climax-vault --apply            # 真做
```

七步，顺序固定，失败即停：

| # | 这一步 | 做什么 |
|---|---|---|
| 1 | 校验 | org 真相源；红档即停（一个文件都不用动） |
| 2 | 渲染 | 与 `anc render` **同一份实现**（同一条 `commitConfig`、同一套门禁 `--adopt` / `--allow-scale`） |
| 3 | 体检 | 产物**要求的凭据键**（渲染时记账，不回读文本），`~/.anc/secrets.env` 里齐不齐（缺键 = 装载前就停；键名本身写不出来时另行说清，那是改名的事，不是补键的事） |
| 4 | 装载 | `cc-connect daemon install --config <cfg> --no-capture-secrets --force` |
| 5 | 凭据桥 | 把 `secrets.env` 读进 daemon 的进程环境（上游没有 dotenv，这一步不能省） |
| 6 | 重启 | `cc-connect daemon restart --force` |
| 7 | 回读 | 日志里 `platform ready` 到齐没有 + 只读探针（探针只回显，不参与退出码）；没到齐时先拨 socket 分岔：**进程没起**与**起来了但没到 ready** 是两种病 |

**为什么第 5 步必须存在**：上游 `${ENV}` 只从**进程环境**解析（实测 v1.3.4 没有 dotenv），
而 `daemon install` 默认会把 `${ENV}` **捕获成明文**写进服务文件 —— 所以装载必须带
`--no-capture-secrets`，凭据只能靠这一步在拉起 gateway 之前注入。
注入的落点按平台不同：Windows 上是**上游生成**的装载体（`~/.cc-connect/cc-connect-daemon.ps1`，
注进去的托管区带边界标记与指纹：幂等、且能一眼看出「被上游重装冲掉了」）；Linux 上是我们**自己**的
systemd drop-in（`<单元名>.d/anc-secrets.conf`，上游不碰它，天然幂等），写完必须自己跑一次
`systemctl --user daemon-reload` —— 实测上游 `daemon restart` 不认新写的 drop-in。

**平台**：Windows 与 Linux 两腿都落了（Linux 腿实测见 `DESIGN.md` §7.1.10）。macOS 零覆盖（议题 #23）：
launchd 包装脚本**未实现即报错** —— 不许「装一半、留个读不到凭据的 daemon」。

**断电之后靠谁起来**：上游单元是 `WantedBy=default.target` 且已 `enable`，配上 `Linger=yes`，
Linux 上开机就自己拉起（进程暴死 / 软重启 / **硬断电**三层都实测过，见 `DESIGN.md` §7.1.11）。
Windows 那份任务只有登录触发器 —— **断电重启后没人登录就不会起**（归议题 #42）。
`anc doctor` 会把这几条前提自检一遍（Linux：单元存在 / `is-enabled` / `Linger`；Windows：**任务有没有被停用** /
触发器里有没有 `BootTrigger` / 两条电源设置）—— 只读，不改上游任务；**没装机就跳过**，不给刚 `init` 的机器平添红字。
`anc apply` 第 7 步在 gateway 拨不通时，还会把「**现在**为什么没起来」与「**下次断电**会不会自己回来」
分开说（没装 / 被停用 / 正在用电池且带电源条件 / 起来了又死），读不到的那一格照实说读不到 —— 见 `DESIGN.md` §7.1.57。

### 可沉淀候选（`anc distill ls`）

```
anc distill ls <vault> [--limit 50] [--json]
anc distill draft <vault> <case> [--name <技能名>] [--force]
anc distill check <vault> <技能名> [--json]
```

把「已经闭环的事」与「技能还缺哪段正文」摆在一张表上 —— **只读，一个字节都不写**。

- **候选口径只有一条**：timeline 里**最后一条是绿档**（`done`）的 case。同一 case 先 done 后 running，
  就按 running 算（没闭环）；要终止的事得先显式结账成终态。一个 case 多行时取最后一条当当前态。
- **signals 只陈述证据**：几条推进 / `detail` 多少字 / 几个 `refs` / 同名重复几次 / 同域几条 ——
  **没有价值分、没有优先级**。值不值得写成技能是人的判断，这一层不替谁排。
- **技能覆盖**：清单来自 `roles/<role>/persona.md` 的 `skills:`，正文看 `skills/<名>/SKILL.md` 在不在；
  声明了没正文 = `missing`（并写进 `gaps`）。**有正文、没有任何岗位声明**的技能单列为孤儿 ——
  它对 agent 不可见。
- **候选不是技能**：写入点是 `skills/<名>/SKILL.md`，那是另一刀（人确认之后）。这条命令不改
  `timeline`、不碰热层、不写 `skills/`。
- 看板「沉淀」页下半部分就是这一份（`/api/distill`，schema = `anc.distill/v1`）。
- **草稿**（`draft`）只对绿档闭环的 case 生成：落到 `skills/_drafts/<名>.md`，
  **不是** `skills/<名>/SKILL.md`。草稿只装 timeline 里的归属 / 证据 / 原始记录 / 引用，
  **不编「怎么做」**；发布步骤写在文末（补四段 → 写到正式落点 → 在 `roles` 登记 → 跑 `distill ls` 核对）。
  名字默认从 case 推（小写 ASCII）；推不出来、或同名正式技能已存在，就用 `--name` 另起。
- **发布前校验**（`check`）只读，三档结论：
  - `fail`（拦发布）：名字不合法 / 草稿不在 / frontmatter 的 `name` 与文件名不一致 /
    「何时用 / 输入 / 步骤 / 验证」任一缺内容 / 占位符（`TODO` 等）/ 明文命中脱敏规则 /
    正式落点 `skills/<名>/SKILL.md` 已被占用。
  - `warn`（可以发，但会出问题）：没有岗位声明（发布后 agent 取不到）/ 来源 case 对不上（不是绿档或已删）/
    正文有本机绝对路径（换台机器失效）/ roles 或 timeline 读不动。
  - 退出码：`0` = 没有 fail（warn 仍可发布）· `1` = 有 fail · `2` = 用法错。
  校验结果里**不回显命中的明文**，只报规则名。
- 设计记录见 `DESIGN.md` §7.1.68（候选）/ §7.1.69（草稿）/ §7.1.70（发布前校验）。

### 退役：`anc service`（2026-10-07）

阶段 B 那套「一个 bot 一套用户级服务单元」已经删掉：它生成的单元跑的是 `anc serve --bot`，
而 D4 之后 serve 永不存在 —— 装上去只会得到一个反复退出的服务。常驻交给上游
`cc-connect daemon`，装载走 `anc apply`（上一节）。留下的教训记在 `DESIGN.md` §7.1.9。

## `anc org init` 生成的骨架

```
<vault>/
├─ company/company.md         公司自描述 + defaults + policy（规则覆盖）
├─ roles/<role>/persona.md    角色层：职责 / 风格 / 术语表 + allowed_tools 等
├─ members/<name>/persona.md  成员层：display_name / role / feishu.app_id / feishu.open_id / unwired（选填）
├─ domains.md                 业务域表（罗盘）—— 空表，由 agent 访谈后填
├─ projects.md                项目表（立项书汇总）—— 空表，由 agent 抽取后填
├─ charters/CLAUDE.md         立项书副本落点 + 约定（接入契约；副本自己按 slug 建子目录）
└─ <数据目录>/CLAUDE.md        首句即 persona 路由表里的说明
```

**骨架不是成品**：`app_id` / `open_id` 是占位符，`allowed_tools` 故意留空 —— 空着就是红档，
`anc render` 会被拦住（实测：`dontAsk` 下没预授权的工具会被自动拒绝，空着 = bot 连得上却干不了活）。
`init` 会明确告诉你哪几条红档是「故意留的」，填完才会放行。

## 构建

```powershell
pwsh -File build.ps1              # 六个目标（win/darwin/linux × amd64/arm64）
pwsh -File build.ps1 -Only local  # 只编本机 windows/amd64
```

产出 `dist/anc_<version>_<os>_<arch>.zip`，每个 zip **自包含**（exe + `templates/` + `inventory/` + `README.md`）。实测体积（2026-10-16，含内嵌模板）：单平台 exe **8.36–9.33 MB**、zip **3.55–4.01 MB**（`CGO_ENABLED=0` + `-trimpath -ldflags "-s -w"`，纯静态无 libc 依赖）。exe 找模板的次序是 `--templates` → **自身同级** `templates/` → `./templates` → **编在 exe 里那一份**：所以 zip 解到哪都能直接跑，而**只有一个 exe** 也能跑（议题 #19）。

## 缺省目标平台（对应客户形态）

| 目标 | 对应 |
|---|---|
| `darwin/arm64` | **Mac mini M 系列 —— 主力交付形态**（客户自购，脚本交付） |
| `darwin/amd64` | 老 Intel Mac |
| `windows/amd64` | 常见客户 PC / 开发机 |
| `windows/arm64` | Windows on ARM |
| `linux/amd64` | 租的云主机 / 信创 x86 |
| `linux/arm64` | ARM 服务器 / 麒麟 ARM |

## 48 小时 → 48 天怎么走

- **48h（薄切片）**：判层 → 只做一条工作流 → 输出接到客户已经用的界面（IM / 工单）→ 一线当场验证。**48h 做出来的东西应该是可以丢的**，它的任务是换取 48 天的授权和现场。
- **48 天（沉淀）**：把长在人脑里的判断写成 Context / Skill / Evals；这时才谈知识库、检索层、观测面板。
- **不做什么**：不重构 ERP。ERP 只读当权威数据源；动作写在旁边这一层（有审计），要改数据走接口回写 + 审批。

## 目录

| 路径 | 说明 |
|---|---|
| `DESIGN.md` | 运行层落地设计 v0.1（草案；D4/D5/D6 已拍板，D1/D2 未拍板） |
| `main.go` | 装配器（`init` / `doctor` / `assets` / `render` / `apply` / `probe` / `notify` / `trail` / `timeline` / `hot` / `envelope` / `gate` / `gateway` / `context` / `board` / `org` / `version`） |
| `render.go` | `anc render` / `anc org check` 的 CLI（dry-run 默认、原子写、两道差分门） |
| `board.go` | `anc org export` 的 CLI（只读投影，无写操作） |
| `context.go` | `anc context` 的 CLI：`pack`（给一份只读投影）/ `freeze`（把这一份冻进 `vault/context/`）（与 MCP / 看板共用同一个装配点） |
| `internal/ctxpack/` | 工作上下文包的唯一装配点（纯函数；MCP / CLI / 看板三处共用，除回答时刻外逐字段相同） |
| `org.go` | `anc org who` / `anc org access` 的 CLI（平台身份 → 谁；这个 open_id 能看哪些域） |
| `internal/org/access.go` | 进门判定：open_id 白名单 → `Viewer`（管理员 / 所属域 / 跨域可见） |
| `internal/board/` | org 真相源 → 看板消费的只读 JSON（纯函数，不含凭据面字段） |
| `internal/board/server.go` | 看板的只读 HTTP 出口（GET / HEAD；投影 + 校验发现） |
| `internal/board/ui/` | 看板前端**产物**（手写 `index.html` + 构建出的 `app.js`）；`go:embed` 进二进制 |
| `boardui/` | 看板前端**源码**（TS + esbuild；`npm run build` 出到 `internal/board/ui/`） |
| `boardserve.go` | `anc board serve` 的 CLI（默认只绑本机、优雅退出；`--data` 接运行态、`--hot-addr` 接待批） |
| `timeline.go` | `anc timeline add` / `list` 的 CLI（append-only，不改旧行） |
| `hot.go` | `anc hot ping` / `put` / `ls` / `claim` / `release` / `drop` / `done` 的 CLI（进行中的状态在热层，达标落库） |
| `internal/hot/` | 「进行中」的热层：状态编解码 + 新鲜度判据 + 最小 RESP2 客户端（纯 Go 标准库，零第三方依赖） |
| `approvals.go` | `anc approvals add` / `ls` / `decide` 的 CLI（提案 → 待批 → 点头 → 留痕） |
| `internal/approvals/` | 待批队列与结账：热层进出 + 「先落库、再清热层」（口径写在包注释里） |
| `internal/timeline/` | 决策与执行留存的读写与折叠（纯 Go 标准库；按作者分片、词表不锁死） |
| `internal/board/timeline.go` | 时间线的只读出口（`/api/timeline`，`anc.timeline/v1`） |
| `internal/board/approvals.go` | 待批的只读出口（`/api/approvals`）+ 审批写口（`/api/approvals/decide`） |
| `internal/board/tasks.go` | 任务归档写口（`/api/tasks/settle`）：与 CLI / MCP 共用 `internal/tasksettle` |
| `internal/board/context.go` | 上下文的只读出口（`/api/context`，`anc.context/v1`）：不给 who 回名单，给 who 回那一份 |
| `audit.go` | `anc audit` 的 CLI：`log` / `add` / `collect` / `tools` / `rules`（事后归集 + 显式补记） |
| `internal/audit/redact.go` | 审计落盘前的强制脱敏（规则是数据；三条腿；抹成指针；没有开关） |
| `internal/audit/` | 行使的流水的读写 + 派生（落点 / 跨域）+ 归集（工具表是数据，不是代码；纯 Go 标准库） |
| `internal/board/audit.go` | 审计的只读出口（`/api/audit`，`anc.audit/v1`；自由文本不出门） |
| `envelope.go` | `anc envelope check` / `serve` 的 CLI（解析 + 绑真相源；接入面走 MCP） |
| `internal/envelope/` | 接入面的信封：类型 / 解析 / 与真相源绑定（与 harness 无关的那一层，#44） |
| `internal/mcp/` | 极小 MCP 服务端（streamable HTTP，无状态）：initialize / tools/list / tools/call |
| `apply.go` | `anc apply` 的 CLI：七步装载（render → 校验 → daemon install → 凭据桥 → 重启 → 回读） |
| `gate.go` | `anc gate` 的 CLI：交付链路的门 —— org + timeline 两个真相源算事实，跑交付层判据（`internal/judge/delivery.go`） |
| `internal/judge/` | 判据层（**判据是数据，不是代码**）：会话层（`anc trail`）+ 交付层（`anc gate`）一个引擎两个作用域；只出结论、不设门禁 |
| `gateway.go` | `anc gateway relay` 的 CLI：第二条网关的收口环（吃一行一条的事件流 → 落痕 → 收口；收口三档 `print` / `off` / `cmd:`） |
| `internal/gateway/` | 可替代网关的**形态表**（`gateway.CCConnect`：装 / 重启 / 日志落点 / 会话落点 / 凭据键名空间 / 认哪些枚举）+ relay 核心（认事件 / 幂等 / 五个结局；碰外面那两步是 `Sink`） |
| `orginit.go` | `anc org init`：生成 org 真相源骨架（vault 模板） |
| `internal/org/` | org 真相源的解析与校验：frontmatter、org 模型、域表 / 项目表、规则表（`rules.go`） |
| `internal/org/charters.go` | 立项书副本落点（`charters/<slug>/`）的扫描与跨表校验 |
| `internal/org/grants.go` | 授权表（`grants/**/*.md`，一个 grant 一个文件）的扫描与跨表校验 |
| `internal/render/` | 纯函数渲染：persona 七段叠加 + lint、config 全量生成 + 往返回读、凭据键记账（`Plan.SecretKeys` / `FeishuSecretKey`，装载前体检吃它）+ `${ENV}` 引用回读（与账交叉校验用） |
| `internal/apply/` | 装载的纯逻辑：secrets.env 解析 / 键名合法性（`LegalKey`）/ 缺键判定 + 凭据落点注入（Windows：上游装载体托管区；Linux：systemd drop-in。都幂等、带指纹） |
| `templates/org/` | vault 骨架模板（纯文件，现场可直接改；同时 `go:embed` 编进 exe） |
| `testdata/orgs/` | 校验正/负例 vault（one / six / disabled / broken-validate / broken-section） |
| `testdata/golden/` | 结构快照（只锁语义） |
| `templates/` | 客户库模板，**纯文件，现场可直接改**；磁盘三处都没有时用 exe 里内嵌的那一份（议题 #19） |
| `inventory/assets.md` | 已有资产台账 |
| `build.ps1` | 本机编译 + 交叉编译 + 打包 |
| `dist/` | 产出（见 `build.ps1`） |

## 边界

- 装配器**只写目标目录**，不动系统、不改全局配置（`--git` 用的是局部身份 `anc/anc@local`，不碰你的 git 全局配置）。
- 凭证一律不进仓库。
- **两个家，各管各的**（议题 #24）：`~/.anc/`（700）放**这一台机**上与客户无关的东西 —— `bin/` · `secrets.env` · `host.json`；**这一个客户库**的部署产物按旗标走、默认落在 **vault 的兄弟目录** —— `--config` = `<vault>/../gateway/config.toml` · `--homes` = `<vault>/../homes` · `--data` = `<vault>/../data`（**绝不写进 vault**，git 只放 org 真相）。平台约定位置（macOS `~/Library/LaunchAgents/` / Linux `~/.config/systemd/user/` / Windows 计划任务库）由上游 `cc-connect daemon install` 自己写。落点全表见 `docs/M1-DESIGN.md` §主机局部状态。
- 本模块是**工程实现**；ANC 的完整理论口径见 `../SPEC.md`。
