package main

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"anc/internal/audit"
	"anc/internal/ctxpack"
	"anc/internal/org"
)

// ---------- 出站读出口（W2）：桥的另一半 ----------
//
// 入站是 agent **递**给 ANC（anc_send_envelope）；出站是 agent 向 ANC **要**上下文
// （anc_read_context）—— **只读**：一个字都不写回真相源。
//
// 这个文件只做出口那三件事，**装配**那一半在 internal/ctxpack（唯一一个装配点，
// MCP / CLI / 看板三个出口共用同一个）：
//
//  1 **可见范围**：自己的域全行 / 别人的域只给「找谁」—— 尺子是 render.VisibleDomainsOf，
//     persona 段 8 用的是同一把。
//  2 **身份**：与信封同一条口径（envelope.Resolves）—— 解不出**不是格式错，是这个人不存在**。
//  3 **留痕**：audit.Append 一条（谁 / 要什么 / 给没给 / 为什么拒）。审计包自己写着
//     「出站动作由人 / agent 显式补记」—— 这里就是那条出站动作的自动落点。
//
// 回话是 **JSON**，字段就是契约，多一个都不给 —— 契约的形状在 ctxpack.Out。

// readContext 是工具实现：解身份 → 按可见范围组一份 JSON → 落一条痕。
//
// 装配那一半在 ctxpack.Assemble（**唯一一个装配点** —— `anc context pack` 与看板那一页
// 走的是同一个函数）；这里只多做两件事：真相源读不出来时的报错口径，和每一次出站调用的留痕。
//
// isErr=true 的三种情形都是「没办成」而不是「工具坏了」：这个人不存在 / on_behalf_of 解不出 /
// 要的是别人的域。三种都留痕 —— 被拒的那次同样是「一次想行使」。
func (g *envelopeIngress) readContext(args map[string]any) (string, bool) {
	now := g.now()
	who := strArg(args, "who")
	onBehalf := strArg(args, "on_behalf_of")
	ask := ctxpack.SafeToken(strArg(args, "domain"))

	o, err := org.Load(g.Vault)
	if err != nil {
		// 真相源本身是红的：读不了就不编一份绿灯（同 envelope check 的口径）。
		return "真相源加载失败，读不了：" + err.Error() + "\n（先修 org：anc org check <vault>）", true
	}

	out, refused := ctxpack.Assemble(o, g.Vault, who, onBehalf, ask, now, g.tasks())

	// 留痕：谁 / 要什么 / 给没给 / 为什么拒。被拒的那次同样是「一次想行使」。
	// 身份都没解出来那一次，账记在「报上来的那个名字」（清洗后）或 unknown 上 —— 一次都不许静默不记。
	actor := out.Who
	if actor == "" {
		actor = readActor(who)
	}
	if refused {
		g.auditRead(actor, ask, audit.ResultDenied, out.Refused.Reason, now)
	} else {
		g.auditRead(actor, ask, audit.ResultOK, "", now)
	}
	return mustJSON(out), refused
}

// tasks 读热层里那一摞当前值 —— 「派给我的待办」这一块的输入。
//
// 读不出来**不编**：Wired=false，装配点据此报缺（「这一段答不了」），
// 绝不拿一份空清单冒充「没人派给你」。**原因是环境的**，不进契约（三个出口的报缺文案
// 必须一模一样）—— 它打 stderr，说给人听。
func (g *envelopeIngress) tasks() ctxpack.TasksIn {
	if g.Hot == nil {
		// 没配这一层：起服务那一步已经报过一次了（「注意：热层没接上（…）」），不在这儿重复刷。
		return ctxpack.TasksIn{}
	}
	states, err := g.Hot.List()
	if err != nil {
		// 读不出来是**运行中**发生的事（起服务时是好的），得喊一声 —— 静默就会变成假绿。
		fmt.Fprintf(os.Stderr, "注意：热层读不出来（%v）—— 「派给我的待办」这一段会如实报缺。\n", err)
		return ctxpack.TasksIn{}
	}
	return ctxpack.TasksIn{Rows: states, Wired: true}
}

// auditRead 给一次出站调用留一条痕：谁 / 要什么 / 给没给 / 为什么拒。
//
// 留痕失败**不改回话**：读出口的职责是答问题，不是把审计故障变成对 harness 的报错。
// 但要在 stderr 吼一声 —— 静默丢一条痕，等于审计少一次行使。
func (g *envelopeIngress) auditRead(actor, domain, result, why string, at time.Time) {
	g.auditTool("anc_read_context", actor, domain, result, why, at)
}

// auditTool 给任意出站调用留一条痕（读上下文 / 检索共用这一份落点）。
// 工具名是 data 的一部分 —— 审计要能分开「他读了上下文」和「他搜了知识」。
func (g *envelopeIngress) auditTool(tool, actor, object, result, why string, at time.Time) {
	if actor == "" {
		actor = "unknown"
	}
	rec := audit.Record{
		ID:     fmt.Sprintf("read-%d-%s", at.UnixNano(), audit.ShardName(actor)),
		At:     at.Format(time.RFC3339),
		Actor:  actor,
		Action: audit.ActionRead,
		Object: object,
		Result: result,
		Why:    why,
		Tool:   tool,
		Source: "serve",
	}
	if _, err := audit.Append(g.Vault, rec); err != nil {
		fmt.Fprintf(os.Stderr, "⚠️  出站调用没留上痕（audit）：%v\n", err)
	}
}

// readActor 决定「解不出的那个 who」往谁的账上记：用请求里报上来的名字（清洗后），空则 unknown。
// 一次都不许静默不记 —— 「没有身份」不是「没有发生」。
func readActor(claimed string) string {
	if s := ctxpack.SafeToken(claimed); s != "" {
		return s
	}
	return "unknown"
}

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		// 结构是我们自己定的，序列化不了只可能是编程错误 —— 别把 panic 扔给 harness。
		return `{"refused":{"reason":"内部错误：回话序列化失败"}}`
	}
	return string(b)
}
