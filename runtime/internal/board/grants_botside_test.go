package board

import (
	"testing"
	"time"

	"anc/internal/ctxpack"
	"anc/internal/org"
)

// 议题 #68：授权这条路上两个消费者、**两个视图**、**一把尺子**。
//
// 看板那一页答「谁能做什么」（全量 + 视角），出站读出口答「我被允许做什么」（只出与我相关的）。
// 这条用例把两个视图摆在一起对同一个库、同一个主体：
//
//  1. 看板那一份加了这条出口之后**一条都没变窄**（全量仍是全量）—— 验收第 3 条；
//  2. 出口那一份是它的**真子集**，且 `total` 两边相同（不藏事实）；
//  3. 两边对「与我相关」的判断**逐条相同** —— 尺子只有一把，不是各挑各的。
func TestBotSideViewAndBoardViewShareOneRuler(t *testing.T) {
	vault := grantsFixture(t)
	o, err := org.Load(vault)
	if err != nil {
		t.Fatal(err)
	}
	full, _ := grantsOf(t, &Server{Vault: vault})
	if full.Total != 3 || full.Shown != 3 {
		t.Fatalf("全量那一份不该被这条出口改窄：total=%d shown=%d", full.Total, full.Shown)
	}

	for _, who := range []string{"alice", "bob", "order-bot"} {
		t.Run(who, func(t *testing.T) {
			if _, ok := o.Identity(who); !ok {
				t.Fatalf("%s 应该在真相源里", who)
			}
			// 出口那一份：只出与我相关的
			pack, refused := ctxpack.Assemble(o, vault, who, "", "", time.Now(), ctxpack.TasksIn{})
			if refused || pack.Grants == nil {
				t.Fatalf("出口那一份没给授权：refused=%v %+v", refused, pack)
			}
			// 视角那一份：同一把尺子筛出来的
			scoped, _ := grantsAt(t, &Server{Vault: vault}, "/api/grants?who="+who)

			if pack.Grants.Total != scoped.Total || pack.Grants.Total != full.Total {
				t.Fatalf("total 三边该相同：pack=%d scoped=%d full=%d",
					pack.Grants.Total, scoped.Total, full.Total)
			}
			got := make([]string, 0, len(pack.Grants.Rows))
			for _, r := range pack.Grants.Rows {
				got = append(got, r.Slug)
			}
			want := make([]string, 0, len(scoped.Rows))
			for _, r := range scoped.Rows {
				want = append(want, r.Slug)
			}
			if len(got) != len(want) {
				t.Fatalf("两个视图命中条数不同：出口 %v / 看板 %v", got, want)
			}
			in := map[string]bool{}
			for _, r := range full.Rows {
				in[r.Slug] = true
			}
			for i := range got {
				if got[i] != want[i] {
					t.Fatalf("同一把尺子筛出来的行不一样：出口 %v / 看板 %v", got, want)
				}
				if !in[got[i]] {
					t.Fatalf("出口给出的 %q 不在全量表里 —— 视图不许自己造行", got[i])
				}
			}
		})
	}
}
