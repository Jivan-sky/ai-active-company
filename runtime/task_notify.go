package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"anc/internal/gateway"
	"anc/internal/notify"
	"anc/internal/org"
	"anc/internal/outbox"
)

// taskNotice 是「一条记录 + 送它去哪」的视图。路由失败不是吞掉，
// 而是把原因端出来 —— 通知发不出去，本身就是需要人看的事。
type taskNotice struct {
	outbox.Record
	Project string `json:"project,omitempty"`
	Route   string `json:"route_error,omitempty"`
}

// routeTaskNotice 把收件人解成 gateway project。业务 agent 用 slug、成员用 name，
// 与 render 那边同一个拼法（org.ProjectName），不另造一套。
func routeTaskNotice(o *org.Org, who string) (string, string) {
	who = strings.TrimSpace(who)
	if who == "" {
		return "", "没有收件人（to 为空）"
	}
	if a, why := findAgent(o, who); why == "" {
		if strings.TrimSpace(a.AppID) == "" {
			return "", "业务 agent 没登记 app_id（还没接线）"
		}
		return org.ProjectName(o.Company.ID, a.Slug), ""
	}
	for _, m := range o.Members {
		if m.Name == who || (m.DisplayName != "" && m.DisplayName == who) {
			if m.Disabled {
				return "", "成员已停用"
			}
			if m.Unwired {
				return "", "成员 bot 声明未接凭据"
			}
			return org.ProjectName(o.Company.ID, m.Name), ""
		}
	}
	return "", "收件人不在 agents.md / members 里（不猜）"
}

func mergeRecords(old, add []outbox.Record) []outbox.Record {
	seen := make(map[string]bool, len(old)+len(add))
	out := make([]outbox.Record, 0, len(old)+len(add))
	for _, r := range append(append([]outbox.Record{}, old...), add...) {
		if seen[r.ID] {
			continue
		}
		seen[r.ID] = true
		out = append(out, r)
	}
	return out
}

func cmdTaskNotify(args []string) int {
	fs := flag.NewFlagSet("task notify", flag.ContinueOnError)
	o := addTaskOpts(fs)
	data := fs.String("data", "", "gateway data_dir（默认 <vault>/../data）")
	send := fs.Bool("send", false, "真发；不加就只打印")
	limit := fs.Int("limit", 50, "这一轮最多送几条")
	asJSON := fs.Bool("json", false, "出 JSON")
	flagArgs, pos := splitArgs(args, map[string]bool{"send": true, "json": true})
	if err := fs.Parse(flagArgs); err != nil {
		return 2
	}
	if len(pos) != 1 {
		fmt.Fprint(os.Stderr, taskUsage)
		return 2
	}
	vault, err := filepath.Abs(pos[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "错误：%v\n", err)
		return 2
	}
	oa, err := org.Load(vault)
	if err != nil {
		fmt.Fprintf(os.Stderr, "真相源加载失败：%v\n", err)
		return 1
	}
	st, err := o.open()
	if err != nil {
		return hotUnreachable(err)
	}
	defer st.Close()
	tasks, err := st.List()
	if err != nil {
		fmt.Fprintf(os.Stderr, "任务读不了：%v\n", err)
		return 1
	}
	dataDir := *data
	if strings.TrimSpace(dataDir) == "" {
		dataDir = filepath.Join(filepath.Dir(vault), "data")
	}
	now := time.Now()
	planned := outbox.Plan(tasks, now)
	boxPath := outbox.Path(dataDir)
	curPath := outbox.CursorPath(dataDir)
	existing, err := outbox.Load(boxPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "发件箱读不了：%v\n", err)
		return 1
	}
	cur, err := outbox.LoadCursor(curPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "发件箱游标读不了：%v\n", err)
		return 1
	}

	appended := 0
	if *send {
		appended, err = outbox.Append(boxPath, planned)
		if err != nil {
			fmt.Fprintf(os.Stderr, "发件箱写不了：%v\n", err)
			return 1
		}
		existing, err = outbox.Load(boxPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "发件箱读不了：%v\n", err)
			return 1
		}
	}
	pending := outbox.Unsent(mergeRecords(existing, planned), cur)

	rows := make([]taskNotice, 0, len(pending))
	for _, r := range pending {
		project, why := routeTaskNotice(oa, r.To)
		rows = append(rows, taskNotice{Record: r, Project: project, Route: why})
	}

	sent, failed := 0, 0
	if *send {
		sock := gateway.CCConnect.SocketPath(dataDir)
		for i := range rows {
			if sent+failed >= *limit {
				break
			}
			r := rows[i]
			if r.Route != "" {
				failed++
				continue
			}
			if err := notify.Push(sock, r.Project, outbox.Text(r.Record)); err != nil {
				failed++
				fmt.Fprintf(os.Stderr, "⚠️  %s → %s 没送出去：%v（留在发件箱，下一轮重放）\n", r.ID, r.Project, err)
				continue
			}
			cur.Mark(r.ID, now)
			if err := cur.Save(curPath); err != nil {
				fmt.Fprintf(os.Stderr, "⚠️  已送出但游标没存上：%v（下一轮可能重发这一条）\n", err)
				failed++
				continue
			}
			sent++
		}
	}

	left := len(rows) - sent
	if *asJSON {
		printJSON(map[string]any{
			"ok": failed == 0, "send": *send, "planned": len(planned), "appended": appended,
			"unsent": len(rows), "sent": sent, "failed": failed, "left": left,
			"notices": rows,
		})
	} else {
		if !*send {
			fmt.Println("任务通知（dry-run，不发；加 --send 真发）")
		} else {
			fmt.Println("任务通知")
		}
		if len(rows) == 0 {
			fmt.Println("  没有要送的通知。")
		}
		for _, r := range rows {
			to := dashIfEmpty(r.To)
			where := r.Project
			if r.Route != "" {
				where = "路由不到：" + r.Route
			}
			fmt.Printf("  [%s] %-16s → %-14s %s\n    %s\n", r.Kind, r.Task, to, where, r.Body)
		}
		if *send {
			fmt.Printf("  这轮送出 %d 条，失败 %d 条，还剩 %d 条。\n", sent, failed, left)
		} else {
			fmt.Printf("  待发 %d 条；发件箱 %s\n", len(rows), boxPath)
		}
	}
	if failed > 0 {
		return 1
	}
	return 0
}
