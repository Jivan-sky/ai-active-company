package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ---------- 合规对拍（真实工具面）----------
//
// mcp 包里的那份钉**协议形状**（用假工具）；这份钉**我们自己挂上去的那两个工具**的 schema
// 是不是一张合法的 JSON Schema —— 因为它是给模型看的**唯一**接口描述：
// type 写错、properties 缺了、required 指到一个不存在的参数，harness 侧就是「给不出参数」
// 或「调用必被拒」，而且不会报错，只会答不准。
//
// 走真 HTTP + 真 JSON-RPC（同 envelope_read_test.go）：与 harness 走同一条路。

type toolsOut struct {
	Result struct {
		Tools []struct {
			Name        string         `json:"name"`
			Description string         `json:"description"`
			InputSchema map[string]any `json:"inputSchema"`
		} `json:"tools"`
	} `json:"result"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func toolsRoundTrip(t *testing.T, h http.Handler) toolsOut {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var out toolsOut
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("tools/list 回话不是 JSON：%v\n%s", err, rec.Body.String())
	}
	return out
}

func TestConformanceRealToolsSchemaIsValidJSONSchema(t *testing.T) {
	h, _ := readHarness(t)
	out := toolsRoundTrip(t, h)
	if out.Error != nil {
		t.Fatalf("tools/list 不应报错：%+v", out.Error)
	}

	// 桥的两半 + 检索 / 取正文 + 心跳 + 任务 / 会话挂在**同一个入口**上（#44 入口唯一性）：七个工具都在，且顺序稳定。
	var names []string
	for _, tool := range out.Result.Tools {
		names = append(names, tool.Name)
	}
	want := []string{"anc_send_envelope", "anc_read_context", "anc_search", "anc_get", "anc_agent_heartbeat", "anc_task", "anc_session"}
	if len(names) != len(want) {
		t.Fatalf("工具个数应为 %d，实际 %d：%v", len(want), len(names), names)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Errorf("第 %d 个工具应叫 %s，实际 %s", i+1, want[i], names[i])
		}
	}

	for _, tool := range out.Result.Tools {
		if tool.Description == "" {
			t.Errorf("工具 %s 没有 description —— 模型只能靠它决定用不用", tool.Name)
		}
		schema := tool.InputSchema
		if schema == nil {
			t.Fatalf("工具 %s 没有 inputSchema", tool.Name)
		}
		if schema["type"] != "object" {
			t.Errorf("工具 %s 的 inputSchema.type 必须是 object，实际 %v", tool.Name, schema["type"])
		}
		props, ok := schema["properties"].(map[string]any)
		if !ok || len(props) == 0 {
			t.Fatalf("工具 %s 的 properties 必须非空对象，实际 %v", tool.Name, schema["properties"])
		}
		for key, raw := range props {
			spec, ok := raw.(map[string]any)
			if !ok {
				t.Errorf("工具 %s 的参数 %s 的 schema 必须是对象", tool.Name, key)
				continue
			}
			if _, ok := spec["type"].(string); !ok {
				t.Errorf("工具 %s 的参数 %s 缺 type —— 客户端拿不到类型就没法校验", tool.Name, key)
			}
			if desc, _ := spec["description"].(string); desc == "" {
				t.Errorf("工具 %s 的参数 %s 缺 description —— 模型只能靠它理解这个参数", tool.Name, key)
			}
		}
		// required 必须指向真实存在的参数：指一个不存在的，等于宣告「这个调用必被拒」。
		if raw, ok := schema["required"]; ok {
			list, ok := raw.([]any)
			if !ok {
				t.Fatalf("工具 %s 的 required 必须是数组，实际 %T", tool.Name, raw)
			}
			for _, item := range list {
				key, ok := item.(string)
				if !ok {
					t.Errorf("工具 %s 的 required 里有非字符串项：%v", tool.Name, item)
					continue
				}
				if _, exists := props[key]; !exists {
					t.Errorf("工具 %s 的 required 指到了不存在的参数 %q —— 这个调用永远过不了", tool.Name, key)
				}
			}
		}
	}
}
