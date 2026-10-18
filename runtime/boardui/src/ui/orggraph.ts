import { card } from "../core/dom";
import type { Data } from "../core/types";
import { agentsOf, botProject, botStateOf } from "../org/view";
import { openDrawer } from "./drawer";
import { currentTheme } from "./shell";
import { runState } from "./status";

export type OrgNode = {
  x: number; y: number; r: number; color: string; kind: string;
  label: string; sub: string; facts: Array<[string, string]>;
};
export type OrgEdge = { a: number; b: number; color: string; seed: number };

export let orgRaf = 0;
export let orgResize: ResizeObserver | null = null;

export function stopOrgGraph(): void {
  if (orgRaf) cancelAnimationFrame(orgRaf);
  orgRaf = 0;
  orgResize?.disconnect();
  orgResize = null;
}

export function mountOrgGraph(scope: HTMLElement, d: Data): void {
  stopOrgGraph();
  const canvas = scope.querySelector<HTMLCanvasElement>(".org-canvas");
  if (!canvas) return;
  const ctx = canvas.getContext("2d");
  if (!ctx) return;
  const light = currentTheme() === "light";
  const b = d.board;
  if (!b) return;
  const bots = d.runtime && d.runtime.wired ? d.runtime.bots : [];
  const reduced = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
  const nodes: OrgNode[] = [];
  const edges: OrgEdge[] = [];
  const transform = { x: 0, y: 0, k: 1 };
  let W = 0;
  let H = 0;
  let dpr = 1;
  let hover = -1;
  let dragging = false;
  let moved = 0;
  let lastX = 0;
  let lastY = 0;

  const build = (): void => {
    W = Math.max(360, canvas.clientWidth);
    H = Math.max(360, canvas.clientHeight);
    dpr = Math.min(window.devicePixelRatio || 1, 2);
    canvas.width = Math.round(W * dpr);
    canvas.height = Math.round(H * dpr);
    transform.x = W / 2;
    transform.y = H / 2;
    transform.k = 1;
    nodes.length = 0;
    edges.length = 0;
    nodes.push({
      x: 0, y: 0, r: 9, color: light ? "36,38,34" : "236,236,232", kind: "core", label: b.company.name || "ANC",
      sub: b.company.platform + " · " + b.company.language,
      facts: [["公司", b.company.name], ["平台", b.company.platform], ["业务域", String(b.domains.length)], ["项目", String(b.projects.length)]],
    });
    const domainIndex = new Map<string, number>();
    b.domains.forEach((x, i) => {
      const a = -Math.PI / 2 + (i / Math.max(1, b.domains.length)) * Math.PI * 2;
      const idx = nodes.length;
      nodes.push({
        x: Math.cos(a) * W * 0.24, y: Math.sin(a) * H * 0.26, r: 6.5, color: light ? "158,118,38" : "230,207,152", kind: "domain",
        label: x.name, sub: x.slug,
        facts: [["是什么", x.what], ["谁在做", x.who_label], ["数据在哪", x.data], ["口径", x.terms]],
      });
      domainIndex.set(x.slug, idx);
      edges.push({ a: 0, b: idx, color: light ? "158,118,38" : "230,207,152", seed: (i * 0.17) % 1 });
    });
    const projectIndex = new Map<string, number>();
    b.projects.forEach((x, i) => {
      const host = nodes[domainIndex.get(x.domain) ?? 0]!;
      const a = Math.atan2(host.y, host.x) + ((i % 5) - 2) * 0.10;
      const rad = 0.34 + (i % 4) * 0.028;
      const idx = nodes.length;
      nodes.push({
        x: Math.cos(a) * W * rad, y: Math.sin(a) * H * rad, r: 5.2, color: light ? "104,96,178" : "195,188,242", kind: "project",
        label: x.name, sub: x.slug,
        facts: [["挂哪块业务", x.domain], ["卡住找谁", x.owner_label], ["周期", x.period], ["真源", x.source]],
      });
      projectIndex.set(x.slug, idx);
      edges.push({ a: domainIndex.get(x.domain) ?? 0, b: idx, color: light ? "104,96,178" : "195,188,242", seed: (i * 0.13 + 0.3) % 1 });
    });
    b.members.forEach((x, i) => {
      const dom = x.domains.find((s) => domainIndex.has(s));
      const host = nodes[domainIndex.get(dom ?? "") ?? 0]!;
      const a = Math.atan2(host.y, host.x) + ((i % 7) - 3) * 0.075;
      const rad = 0.40 + (i % 3) * 0.02;
      const idx = nodes.length;
      nodes.push({
        x: Math.cos(a) * W * rad, y: Math.sin(a) * H * rad, r: 3.8, color: light ? "42,132,108" : "172,224,207", kind: "member",
        label: x.display_name || x.name, sub: x.role + (x.disabled ? " · 已停用" : ""),
        facts: [["角色", x.role], ["模型", x.model], ["业务域", x.domains.join("、") || "—"], ["状态", x.disabled ? "已停用" : x.admin ? "启用 · 管理员" : "启用"]],
      });
      edges.push({ a: domainIndex.get(dom ?? "") ?? 0, b: idx, color: light ? "42,132,108" : "172,224,207", seed: (i * 0.11 + 0.6) % 1 });
    });
    agentsOf(b).forEach((x, i) => {
      const dom = domainIndex.has(x.domain) ? x.domain : "";
      const host = nodes[domainIndex.get(dom) ?? 0]!;
      const a = Math.atan2(host.y, host.x) + 0.55 + ((i % 5) - 2) * 0.09;
      const rad = 0.38 + (i % 3) * 0.02;
      const idx = nodes.length;
      const st = botStateOf(d, botProject(b, x.slug));
      nodes.push({
        x: Math.cos(a) * W * rad, y: Math.sin(a) * H * rad, r: 4.6,
        color: light ? "168,67,99" : "227,179,196", kind: "agent",
        label: x.name || x.slug, sub: x.role,
        facts: [["slug", x.slug], ["铭文", x.description || "—"], ["业务域", x.domain || "—"],
          ["岗位", x.role || "—"], ["运行态", st.label], ["harness", x.harness || "默认"]],
      });
      edges.push({ a: domainIndex.get(dom) ?? 0, b: idx, color: light ? "168,67,99" : "227,179,196", seed: (i * 0.23 + 0.2) % 1 });
    });
    bots.forEach((x, i) => {
      const hit = projectIndex.get(x.project);
      const host = nodes[hit ?? 0]!;
      const a = Math.atan2(host.y, host.x) + ((i % 3) - 1) * 0.13;
      const rad = 46 + (i % 2) * 18;
      const idx = nodes.length;
      nodes.push({
        x: host.x + Math.cos(a) * rad, y: host.y + Math.sin(a) * rad, r: 4.2, color: light ? "52,122,152" : "172,214,232", kind: "bot",
        label: x.project, sub: x.state,
        facts: [["project", x.project], ["状态", runState(x.state).label], ["为什么", x.why]],
      });
      edges.push({ a: hit ?? 0, b: idx, color: light ? "52,122,152" : "172,214,232", seed: (i * 0.19 + 0.4) % 1 });
    });
    let maxX = 0;
    let maxY = 0;
    for (const n of nodes) {
      const labelW = n.kind === "domain" || n.kind === "project" || n.kind === "core" ? 160 : 0;
      maxX = Math.max(maxX, Math.abs(n.x) + n.r + labelW);
      maxY = Math.max(maxY, Math.abs(n.y) + n.r + 16);
    }
    const fit = Math.min((W / 2 - 24) / Math.max(1, maxX), (H / 2 - 24) / Math.max(1, maxY));
    transform.k = Math.max(0.55, Math.min(1, fit));
  };

  const project = (n: OrgNode): { x: number; y: number } => ({
    x: transform.x + n.x * transform.k, y: transform.y + n.y * transform.k,
  });

  const hitAt = (sx: number, sy: number): number => {
    let best = -1;
    let bestD = 18;
    for (let i = 0; i < nodes.length; i++) {
      const p = project(nodes[i]!);
      const dd = Math.hypot(p.x - sx, p.y - sy);
      if (dd < Math.max(bestD, nodes[i]!.r * transform.k + 6)) { best = i; bestD = dd; }
    }
    return best;
  };

  const draw = (now: number): void => {
    const t = now / 1000;
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    ctx.clearRect(0, 0, W, H);
    const core = project(nodes[0]!);
    const halo = ctx.createRadialGradient(core.x, core.y, 0, core.x, core.y, 150 * transform.k);
    halo.addColorStop(0, light ? "rgba(168,130,50,0.045)" : "rgba(230,207,152,0.10)");
    halo.addColorStop(1, light ? "rgba(168,130,50,0)" : "rgba(230,207,152,0)");
    ctx.fillStyle = halo;
    ctx.beginPath();
    ctx.arc(core.x, core.y, 150 * transform.k, 0, Math.PI * 2);
    ctx.fill();

    for (const e of edges) {
      const a = project(nodes[e.a]!);
      const bb = project(nodes[e.b]!);
      ctx.strokeStyle = "rgba(" + e.color + "," + (light ? "0.24" : "0.15") + ")";
      ctx.lineWidth = 0.9;
      ctx.beginPath();
      ctx.moveTo(a.x, a.y);
      ctx.lineTo(bb.x, bb.y);
      ctx.stroke();
      if (!reduced) {
        const u = (t * 0.12 + e.seed) % 1;
        const px = a.x + (bb.x - a.x) * u;
        const py = a.y + (bb.y - a.y) * u;
        const g = ctx.createRadialGradient(px, py, 0, px, py, 5);
        g.addColorStop(0, "rgba(" + e.color + ",0.9)");
        g.addColorStop(1, "rgba(" + e.color + ",0)");
        ctx.fillStyle = g;
        ctx.beginPath();
        ctx.arc(px, py, 5, 0, Math.PI * 2);
        ctx.fill();
      }
    }
    nodes.forEach((n, i) => {
      const p = project(n);
      const r = Math.max(2.4, n.r * transform.k);
      const on = i === hover;
      const glow = ctx.createRadialGradient(p.x, p.y, 0, p.x, p.y, r * (on ? 8 : 4.5));
      glow.addColorStop(0, "rgba(" + n.color + "," + (light ? (on ? 0.20 : 0.12) : (on ? 0.55 : 0.34)) + ")");
      glow.addColorStop(1, "rgba(" + n.color + ",0)");
      ctx.fillStyle = glow;
      ctx.beginPath();
      ctx.arc(p.x, p.y, r * (on ? 8 : 4.5), 0, Math.PI * 2);
      ctx.fill();
      ctx.fillStyle = "rgba(" + n.color + ",0.97)";
      ctx.beginPath();
      ctx.arc(p.x, p.y, r, 0, Math.PI * 2);
      ctx.fill();
      if (on) {
        ctx.strokeStyle = light ? "rgba(20,22,18,0.55)" : "rgba(255,255,255,0.5)";
        ctx.lineWidth = 1;
        ctx.beginPath();
        ctx.arc(p.x, p.y, r + 6, 0, Math.PI * 2);
        ctx.stroke();
      }
      if ((n.kind === "domain" || n.kind === "project" || n.kind === "core") && n.label !== "") {
        const size = n.kind === "core" ? 12 : 10.5;
        ctx.font = "500 " + size + "px -apple-system, 'PingFang SC', sans-serif";
        ctx.fillStyle = light
          ? "rgba(26,28,24," + (n.kind === "core" ? 0.9 : 0.72) + ")"
          : "rgba(236,236,233," + (n.kind === "core" ? 0.86 : 0.62) + ")";
        const label = n.label.length > 14 ? n.label.slice(0, 14) + "…" : n.label;
        ctx.fillText(label, p.x + r + 7, p.y + 3.5);
      }
    });
    if (!reduced) orgRaf = requestAnimationFrame(draw);
  };

  const openNode = (i: number): void => {
    const n = nodes[i];
    if (!n) return;
    openDrawer({
      eyebrow: n.kind.toUpperCase() + " / 关系图谱", title: n.label, sub: n.sub,
      body: [card(n.label, n.sub, n.facts)],
    });
  };

  const onMove = (ev: MouseEvent): void => {
    const rect = canvas.getBoundingClientRect();
    const sx = ev.clientX - rect.left;
    const sy = ev.clientY - rect.top;
    if (dragging) {
      const dx = ev.clientX - lastX;
      const dy = ev.clientY - lastY;
      moved += Math.abs(dx) + Math.abs(dy);
      transform.x += dx;
      transform.y += dy;
      lastX = ev.clientX;
      lastY = ev.clientY;
      return;
    }
    hover = hitAt(sx, sy);
    canvas.style.cursor = hover >= 0 ? "pointer" : "grab";
  };
  const onDown = (ev: MouseEvent): void => {
    dragging = true;
    moved = 0;
    lastX = ev.clientX;
    lastY = ev.clientY;
    canvas.style.cursor = "grabbing";
  };
  const onUp = (ev: MouseEvent): void => {
    if (dragging && moved < 5) {
      const rect = canvas.getBoundingClientRect();
      const i = hitAt(ev.clientX - rect.left, ev.clientY - rect.top);
      if (i >= 0) openNode(i);
    }
    dragging = false;
    canvas.style.cursor = "grab";
  };
  const onWheel = (ev: WheelEvent): void => {
    ev.preventDefault();
    const rect = canvas.getBoundingClientRect();
    const sx = ev.clientX - rect.left;
    const sy = ev.clientY - rect.top;
    const wx = (sx - transform.x) / transform.k;
    const wy = (sy - transform.y) / transform.k;
    const k = Math.max(0.55, Math.min(2.6, transform.k * (ev.deltaY < 0 ? 1.12 : 0.89)));
    transform.k = k;
    transform.x = sx - wx * k;
    transform.y = sy - wy * k;
  };
  canvas.addEventListener("mousemove", onMove);
  canvas.addEventListener("mousedown", onDown);
  canvas.addEventListener("mouseup", onUp);
  canvas.addEventListener("mouseleave", () => { hover = -1; dragging = false; });
  canvas.addEventListener("wheel", onWheel, { passive: false });
  canvas.addEventListener("dblclick", () => { build(); });

  build();
  orgResize = new ResizeObserver(() => { build(); if (reduced) draw(performance.now()); });
  orgResize.observe(canvas);
  if (reduced) draw(performance.now());
  else orgRaf = requestAnimationFrame(draw);
}
