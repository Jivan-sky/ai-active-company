package org

import "testing"

// 议题 #68：「这条授权与我相关吗」是一把**共用**的尺子 —— 看板上那个视角与 bot 侧那个
// 「只出与我相关的几条」的视图问的是同一个问题。这组用例把它钉死成一条：
//
//   - 裸名字 / `member:` / `agent:` 才算主体，且比的是**解出来的编号身份**，不是请求里的拼法；
//   - `role:` 与 `domain:` **明确不是主体** —— 一条 `to: role:manager` 的权**不**命中任何一个
//     具体的人（哪怕他就在那个岗位上）。岗位是「一类人」而不是一个编号身份；域是客体
//     （SPEC §6：域不会授权）。要判「我在不在这个岗位上」是另一件事，不混进这条判据。
func TestGrantHitsSubject(t *testing.T) {
	v := copyVault(t, "domains")
	writeAt(t, v+"/agents.md",
		"| slug | name | domain | harness | tools | model | role | app_id |\n"+
			"|---|---|---|---|---|---|---|---|\n"+
			"| order-bot | 订单处理 | trade | codex | read,bash | claude-sonnet-5 | manager | cli_demo_order |\n")
	o, err := Load(v)
	if err != nil {
		t.Fatal(err)
	}
	alice, ok := o.Identity("alice")
	if !ok {
		t.Fatal("alice 应该在 members/ 里")
	}
	bob, ok := o.Identity("bob")
	if !ok {
		t.Fatal("bob 应该在 members/ 里")
	}
	bot, ok := o.Identity("order-bot")
	if !ok {
		t.Fatal("order-bot 应该在 agents.md 里")
	}

	cases := []struct {
		name string
		g    Grant
		idn  Identity
		want bool
	}{
		{"from 指着我", Grant{From: "member:alice", To: "role:ops"}, alice, true},
		{"to 指着我（裸名字）", Grant{From: "role:manager", To: "bob"}, bob, true},
		{"on_behalf_of 指着我", Grant{From: "role:manager", To: "domain:trade", OnBehalfOf: "alice"}, alice, true},
		{"业务 agent 也是主体", Grant{From: "role:manager", To: "agent:order-bot"}, bot, true},
		{"别人的行", Grant{From: "member:bob", To: "member:carol"}, alice, false},
		{"to 是岗位：不命中岗位上的具体人", Grant{From: "member:bob", To: "role:manager"}, alice, false},
		{"to 是域：域是客体", Grant{From: "member:bob", To: "domain:trade"}, alice, false},
		{"认不出的前缀：不许猜", Grant{From: "team:alice", To: "domain:trade"}, alice, false},
		{"没写人的格子不算", Grant{From: "role:manager", To: "domain:trade"}, alice, false},
	}
	for _, c := range cases {
		if got := GrantHitsSubject(o, c.g, c.idn); got != c.want {
			t.Errorf("%s：命中=%v，期望 %v（%+v）", c.name, got, c.want, c.g)
		}
	}

	// 空主体不命中任何东西 —— 「没这个人」与「这个人什么都没被给」是两件事，
	// 后者由调用方去说（total 摆在那儿），不许在这里混成一个 true。
	if GrantHitsSubject(o, Grant{From: "", To: "", OnBehalfOf: ""}, Identity{Kind: KindMember, Ref: ""}) {
		t.Error("空编号身份不该命中")
	}
}

// GrantSlots 只摊**写了的**格子，顺序固定（from → to → on_behalf_of）--
// 两个消费者都照这份顺序走，顺序飘了，「同一把尺子」也就只剩一句话。
func TestGrantSlotsSkipsEmptyAndKeepsOrder(t *testing.T) {
	g := Grant{From: "member:alice", OnBehalfOf: "member:alice"}
	got := GrantSlots(g)
	if len(got) != 2 {
		t.Fatalf("空格子不该出现，实际 %+v", got)
	}
	if got[0].Slot != "from" || got[1].Slot != "on_behalf_of" {
		t.Fatalf("顺序不对：%+v", got)
	}
	if all := GrantSlots(Grant{From: "a", To: "b", OnBehalfOf: "c"}); len(all) != 3 {
		t.Fatalf("三格都写了就三格都要在：%+v", all)
	}
}
