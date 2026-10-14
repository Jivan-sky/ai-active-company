package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"

	"anc/internal/ctxpack"
	"anc/internal/org"
)

// ---------- 工作上下文包（议题 #53）的用例 ----------
//
// 盯的是一个不变量：**装配点只有一个**。
// MCP 的 anc_read_context 与 `anc context pack` 打出来的必须是同一份（除回答时刻外逐字段相同）——
// 两份装配迟早会对不上号，而「对不上号的工作上下文」比没有更坏：读的人以为看到的就是全的。

// ctxPack 跑一次 CLI，回 (stdout, 退出码)。
func ctxPack(t *testing.T, vault string, extra ...string) (string, int) {
	t.Helper()
	args := append(append([]string{}, extra...), vault)
	return captureStdout(t, func() int { return cmdContextPack(args) })
}

// `context pack` 的 stdout 必须是**纯 JSON**：给人看的话只能走 stderr，
// 否则接着它的东西（管道 / 交付包）会拿到一份坏的。
func TestContextPackStdoutIsPureJSON(t *testing.T) {
	out, code := ctxPack(t, copyFixture(t, "domains"), "--who", "alice")
	if code != 0 {
		t.Fatalf("退出码 %d，期望 0", code)
	}
	if !json.Valid([]byte(out)) {
		t.Fatalf("stdout 不是合法 JSON：\n%s", out)
	}
}

// 五问 + 项目：身份 / 业务域 / 数据源 / 可见范围 / 技能来源 / 在跟哪个项目。
// 缺项报缺（gaps），不编 —— 这条与出站读出口是同一条口径。
func TestContextPackAnswersTheFiveQuestions(t *testing.T) {
	out, code := ctxPack(t, copyFixture(t, "domains"), "--who", "alice")
	if code != 0 {
		t.Fatalf("退出码 %d，期望 0：%s", code, out)
	}
	var got ctxpack.Out
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatal(err)
	}
	if got.Identity == nil || got.Identity.Ref != "alice" || got.Identity.Role == "" {
		t.Fatalf("身份那格不对：%+v", got.Identity)
	}
	if len(got.Domains) == 0 || got.Domains[0].Data == "" {
		t.Fatalf("业务域 / 数据源没答出来：%+v", got.Domains)
	}
	if len(got.Directory) == 0 {
		t.Fatal("可见范围：别域的「找谁」目录没给")
	}
	if len(got.Skills) == 0 {
		t.Fatal("技能来源没答出来")
	}
	if len(got.Projects) == 0 {
		t.Fatal("在跟的项目没答出来")
	}
	if len(got.Gaps) == 0 {
		t.Fatal("没有真相源的那几样该进 gaps（报缺，不编）")
	}
}

// 核心不变量：CLI 打的那一份与 MCP 回的那一份**逐字段相同**（回答时刻除外）。
// 一个装配点、两个出口 —— 哪天有人给 CLI 单开一条近路，这条用例就该红。
func TestContextPackAndMCPAreOneAssembly(t *testing.T) {
	h, _ := readHarness(t)
	for _, ask := range []string{"", "trade"} {
		t.Run("domain="+ask, func(t *testing.T) {
			args := map[string]any{"who": "alice"}
			if ask != "" {
				args["domain"] = ask
			}
			text, _ := callRead(t, h, args)
			mcp := parseRead(t, text)

			cliArgs := []string{"--who", "alice"}
			if ask != "" {
				cliArgs = append(cliArgs, "--domain", ask)
			}
			out, code := ctxPack(t, copyFixture(t, "domains"), cliArgs...)
			if code != 0 {
				t.Fatalf("退出码 %d：%s", code, out)
			}
			var cli ctxpack.Out
			if err := json.Unmarshal([]byte(out), &cli); err != nil {
				t.Fatal(err)
			}

			// 两份都在「现在」回答的，时刻本来就不同 —— 比的是内容，不是时间戳。
			mcp.At, cli.At = "", ""
			if !reflect.DeepEqual(mcp, cli) {
				t.Fatalf("两个出口打出来的不是同一份：\nMCP: %+v\nCLI: %+v", mcp, cli)
			}
		})
	}
}

// 拒也一样是同一份：越域时两边都得给同一句拒话与同一个「找谁」。
func TestContextPackRefusalMatchesMCP(t *testing.T) {
	h, _ := readHarness(t)
	text, isErr := callRead(t, h, map[string]any{"who": "alice", "domain": "logistics"})
	if !isErr {
		t.Fatalf("越域该被拒：%s", text)
	}
	mcp := parseRead(t, text)

	out, code := ctxPack(t, copyFixture(t, "domains"), "--who", "alice", "--domain", "logistics")
	if code != 1 {
		t.Fatalf("拒的时候退出码该是 1（脚本得看得见），实际 %d", code)
	}
	var cli ctxpack.Out
	if err := json.Unmarshal([]byte(out), &cli); err != nil {
		t.Fatal(err)
	}
	mcp.At, cli.At = "", ""
	if !reflect.DeepEqual(mcp, cli) {
		t.Fatalf("拒话两边不是同一份：\nMCP: %+v\nCLI: %+v", mcp, cli)
	}
	if cli.Refused == nil || cli.Refused.Ask == "" {
		t.Fatalf("拒了就该指路「找谁」：%+v", cli.Refused)
	}
	// 拒的时候不许夹带数据 —— 别人的域连 data 都不能出现。
	if strings.Contains(out, "订舱") || strings.Contains(out, `"data":`) {
		t.Fatalf("拒话里夹带了别人的数据：%s", out)
	}
}

// 要单独一域时只给那一行：不写 domain 才给「找谁」目录，也不塞通用报缺。
func TestContextPackScopesToOneDomain(t *testing.T) {
	out, code := ctxPack(t, copyFixture(t, "domains"), "--who", "alice", "--domain", "trade")
	if code != 0 {
		t.Fatalf("退出码 %d：%s", code, out)
	}
	var got ctxpack.Out
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatal(err)
	}
	if got.Scope != "trade" {
		t.Fatalf("scope = %q，期望 trade", got.Scope)
	}
	if len(got.Domains) != 1 || got.Domains[0].Slug != "trade" {
		t.Fatalf("要一域却回了 %+v", got.Domains)
	}
	if len(got.Directory) != 0 {
		t.Fatalf("要单独一域的人不该被目录淹掉：%+v", got.Directory)
	}
}

// 解不出的人 = 这个人不存在，不是「参数写错了」—— 拒话里不许出现参数 / 格式 / schema 这类词，
// 那会把读的人引去改参数，而问题是他不在这家公司。
func TestContextPackUnknownWhoIsNotAParamError(t *testing.T) {
	out, code := ctxPack(t, copyFixture(t, "domains"), "--who", "查无此人")
	if code != 1 {
		t.Fatalf("退出码 %d，期望 1", code)
	}
	var got ctxpack.Out
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatal(err)
	}
	if got.Refused == nil {
		t.Fatalf("该被拒：%s", out)
	}
	for _, bad := range []string{"参数", "格式", "schema", "invalid"} {
		if strings.Contains(got.Refused.Reason, bad) {
			t.Fatalf("拒话里出现了 %q —— 那会把读的人引去改参数：%s", bad, got.Refused.Reason)
		}
	}
}

// stdout 是指针一律 vault 相对：本机绝对路径一个字都不许出现。
func TestContextPackLeaksNoLocalPaths(t *testing.T) {
	vault := copyFixture(t, "domains")
	out, _ := ctxPack(t, vault, "--who", "alice")
	if strings.Contains(out, vault) {
		t.Fatalf("回话里出现了本机路径：%s", out)
	}
	if re := regexp.MustCompile(`[A-Za-z]:\\\\`); re.MatchString(out) {
		t.Fatalf("回话里有盘符：%s", out)
	}
}

// 只读：一个字不写回真相源（也不落 audit —— 留痕是 MCP 那个入口的账）。
func TestContextPackWritesNothing(t *testing.T) {
	vault := copyFixture(t, "domains")
	before := vaultFingerprint(t, vault)
	if _, code := ctxPack(t, vault, "--who", "alice"); code != 0 {
		t.Fatalf("退出码 %d", code)
	}
	if _, code := ctxPack(t, vault, "--who", "查无此人"); code != 1 {
		t.Fatalf("退出码 %d，期望 1", code)
	}
	if after := vaultFingerprint(t, vault); after != before {
		t.Fatal("真相源被动过了 —— 投影必须是纯只读")
	}
}

// 没给 --who 是用法错（退出码 2），不是「这个人不存在」—— 两件事分开报。
func TestContextPackMissingWhoIsUsageError(t *testing.T) {
	if code := quiet(t, func() int { return cmdContextPack([]string{copyFixture(t, "domains")}) }); code != 2 {
		t.Fatalf("没给 --who 该是用法错（2），实际 %d", code)
	}
}

// ---------- 冻结副本（`anc context freeze`）----------
//
// 这一刀把「形态」那一半也补上了：pack 只打 stdout，freeze 另冻一份进 vault 的 context/。
// 两条不变量各有用例钉着 ——
//  1 冻下来那一份与 pack 的 stdout **除末尾换行外逐字节相同**（对不上号的副本比没有更坏）；
//  2 拒了**就不冻**（没有可交付的上下文，就不该在落点留个空壳）。

// ctxFreeze 跑一次 CLI，回 (stdout, 退出码)。
func ctxFreeze(t *testing.T, vault string, extra ...string) (string, int) {
	t.Helper()
	args := append(append([]string{}, extra...), vault)
	return captureStdout(t, func() int { return cmdContextFreeze(args) })
}

// 冻下来的那一份 = pack 打的那一份（除末尾换行）：同一个装配点、同一次序列化。
func TestContextFreezeWritesWhatPackPrints(t *testing.T) {
	vault := copyFixture(t, "domains")
	who := "alice"

	packed, code := ctxPack(t, vault, "--who", who)
	if code != 0 {
		t.Fatalf("pack 退出码 %d", code)
	}
	_, code = ctxFreeze(t, vault, "--who", who)
	if code != 0 {
		t.Fatalf("freeze 退出码 %d", code)
	}

	// 默认落点 = <vault>/context/<编号>.json，且文件名用**解出来的编号**（不是 --who 的原样）。
	path := filepath.Join(vault, "context", who+".json")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("默认落点里没有冻结副本：%v", err)
	}
	if string(b) != packed+"\n" {
		t.Fatalf("冻下来的那份与 stdout 上那份对不上：\n文件: %q\nstdout: %q", string(b), packed)
	}
}

// 只写落点那一份，别的一个字节都不动 —— 「冻副本」不是「改真相源」的入口。
func TestContextFreezeWritesExactlyOneFile(t *testing.T) {
	vault := copyFixture(t, "domains")
	before := vaultFingerprint(t, vault)
	if _, code := ctxFreeze(t, vault, "--who", "alice"); code != 0 {
		t.Fatalf("退出码 %d", code)
	}
	if after := vaultFingerprint(t, vault); after == before {
		t.Fatal("冻了一份副本，指纹却一点没变 —— 这份没真落下去")
	}
	// 挪走落点，指纹应当逐字节回到冻结之前：换句话说，动过的**只有**落点。
	frozen := filepath.Join(vault, "context", "alice.json")
	body, err := os.ReadFile(frozen)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(frozen); err != nil {
		t.Fatal(err)
	}
	if after := vaultFingerprint(t, vault); after != before {
		t.Fatalf("除了落点那一份，别的地方也被动过了：\n前: %s\n后: %s", before, after)
	}
	if len(body) == 0 {
		t.Fatal("冻出来的是个空文件")
	}
}

// 拒了就不冻：越域、查无此人，两种都不该在落点留个空壳。
func TestContextFreezeRefusalWritesNothing(t *testing.T) {
	for _, c := range []struct {
		name string
		args []string
	}{
		{"越域", []string{"--who", "alice", "--domain", "logistics"}},
		{"查无此人", []string{"--who", "查无此人"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			vault := copyFixture(t, "domains")
			before := vaultFingerprint(t, vault)
			if _, code := ctxFreeze(t, vault, c.args...); code != 1 {
				t.Fatalf("拒的时候退出码该是 1（脚本得看得见），实际 %d", code)
			}
			if after := vaultFingerprint(t, vault); after != before {
				t.Fatal("拒了却写了东西 —— 落点里不该有半份上下文")
			}
			if _, err := os.Stat(filepath.Join(vault, "context")); err == nil {
				t.Fatal("拒了却把落点目录建出来了")
			}
		})
	}
}

// 要单独一域时文件名带上那一段，免得把「全部」那一份悄悄盖掉。
func TestContextFreezeScopedNameAndOverrideDir(t *testing.T) {
	vault := copyFixture(t, "domains")
	if _, code := ctxFreeze(t, vault, "--who", "alice", "--domain", "trade"); code != 0 {
		t.Fatalf("退出码 %d", code)
	}
	if _, err := os.Stat(filepath.Join(vault, "context", "alice.trade.json")); err != nil {
		t.Fatalf("单独一域的那份该叫 alice.trade.json：%v", err)
	}
	// --to 换个落点：交给调用方定（交付包里常要落到别处）。
	elsewhere := t.TempDir()
	if _, code := ctxFreeze(t, vault, "--who", "alice", "--to", elsewhere); code != 0 {
		t.Fatalf("退出码 %d", code)
	}
	if _, err := os.Stat(filepath.Join(elsewhere, "alice.json")); err != nil {
		t.Fatalf("--to 指定的落点里没有那份：%v", err)
	}
}

// 落点不进路由表：它是算出来的快照，不是「人把资料放进来、agent 去查」的地方 ——
// 进了 persona 段 3 的数据来源，等于给 agent 指错路。
func TestContextDirIsNotRouted(t *testing.T) {
	vault := copyFixture(t, "domains")
	if _, code := ctxFreeze(t, vault, "--who", "alice"); code != 0 {
		t.Fatalf("退出码 %d", code)
	}
	o, err := org.Load(vault)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range o.Routing {
		if r.Dir == org.ContextDir {
			t.Fatalf("落点进了数据路由表：%+v", o.Routing)
		}
	}
}

// vaultFingerprint 把整棵 vault 折成一个指纹（相对路径 + 内容）—— 用来钉「只读」。
func vaultFingerprint(t *testing.T, root string) string {
	t.Helper()
	var rels []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		rels = append(rels, rel)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(rels)
	h := sha256.New()
	for _, rel := range rels {
		b, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatal(err)
		}
		fmt.Fprintf(h, "%s\\x00%d\\x00", filepath.ToSlash(rel), len(b))
		h.Write(b)
	}
	return hex.EncodeToString(h.Sum(nil))
}
