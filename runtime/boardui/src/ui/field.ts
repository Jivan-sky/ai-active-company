import type { Data } from "../core/types";
import { currentTheme } from "./shell";

export type FieldNode = { x: number; y: number; r: number; color: string; label: string; kind: string };
export type FieldLink = { a: number; b: number; color: string; seed: number; bend: number };

export let fieldRaf = 0;
export let fieldResize: ResizeObserver | null = null;

export function stopField(): void {
  if (fieldRaf) cancelAnimationFrame(fieldRaf);
  fieldRaf = 0;
  fieldResize?.disconnect();
  fieldResize = null;
}

export function mountField(scope: HTMLElement, d: Data): void {
  stopField();
  const canvas = scope.querySelector<HTMLCanvasElement>(".field-canvas");
  if (!canvas) return;
  const ctx = canvas.getContext("2d");
  if (!ctx) return;
  const light = currentTheme() === "light";
  const reduced = window.matchMedia("(prefers-reduced-motion: reduce)").matches;
  const nodes: FieldNode[] = [];
  const links: FieldLink[] = [];
  // 尘埃与轨道只是**气氛**，不带标签、不参与命中 —— 有数据的节点永远比它们亮。
  const dust: Array<{ x: number; y: number; r: number; a: number; ph: number }> = [];
  let W = 0;
  let H = 0;
  let dpr = 1;
  let span = 0;
  const TAU = Math.PI * 2;

  const build = (): void => {
    W = Math.max(320, canvas.clientWidth);
    H = Math.max(260, canvas.clientHeight);
    dpr = Math.min(window.devicePixelRatio || 1, 2);
    canvas.width = Math.round(W * dpr);
    canvas.height = Math.round(H * dpr);
    nodes.length = 0;
    links.length = 0;
    const cx = W * 0.52;
    const cy = H * 0.43;
    span = Math.min(W, H);
    dust.length = 0;
    for (let i = 0; i < 56; i++) {
      const a = i * 2.39996;
      const rr = Math.sqrt((i + 0.5) / 56);
      dust.push({
        x: cx + Math.cos(a) * rr * W * 0.46,
        y: cy + Math.sin(a) * rr * H * 0.42,
        r: 0.5 + (i % 3) * 0.28,
        a: 0.05 + (i % 5) * 0.012,
        ph: i * 0.7,
      });
    }
    nodes.push({ x: cx, y: cy, r: 5.5, color: light ? "36,38,34" : "236,236,232", label: "ANC", kind: "core" });

    const domainPos = new Map<string, number>();
    const domains = d.board ? d.board.domains : [];
    const dn = Math.min(domains.length, 14);
    for (let i = 0; i < dn; i++) {
      const x = domains[i]!;
      const a = 0.2 + (i / Math.max(1, dn)) * TAU;
      const idx = nodes.length;
      nodes.push({ x: cx + Math.cos(a) * span * 0.27, y: cy + Math.sin(a) * span * 0.23, r: 4.2, color: light ? "158,118,38" : "230,207,152", label: x.name, kind: "domain" });
      domainPos.set(x.slug, idx);
      links.push({ a: 0, b: idx, color: light ? "158,118,38" : "230,207,152", seed: (i * 0.17) % 1, bend: 0.10 });
    }

    const projectPos = new Map<string, number>();
    const projects = d.board ? d.board.projects : [];
    const pn = Math.min(projects.length, 22);
    for (let i = 0; i < pn; i++) {
      const x = projects[i]!;
      const a = 1.25 + (i / Math.max(1, pn)) * TAU;
      const idx = nodes.length;
      nodes.push({ x: cx + Math.cos(a) * span * 0.42, y: cy + Math.sin(a) * span * 0.34, r: 3.4, color: light ? "104,96,178" : "195,188,242", label: x.name, kind: "project" });
      projectPos.set(x.slug, idx);
      links.push({ a: domainPos.get(x.domain) ?? 0, b: idx, color: light ? "104,96,178" : "195,188,242", seed: (i * 0.13 + 0.3) % 1, bend: 0.14 });
    }

    const members = d.board ? d.board.members : [];
    const mn = Math.min(members.length, 36);
    for (let i = 0; i < mn; i++) {
      const x = members[i]!;
      const a = 2.4 + (i / Math.max(1, mn)) * TAU;
      const idx = nodes.length;
      nodes.push({ x: cx + Math.cos(a) * span * 0.56, y: cy + Math.sin(a) * span * 0.44, r: 2.4, color: light ? "42,132,108" : "172,224,207", label: x.display_name || x.name, kind: "member" });
      let di = 0;
      for (const s of x.domains) {
        const hit = domainPos.get(s);
        if (hit !== undefined) { di = hit; break; }
      }
      links.push({ a: di, b: idx, color: light ? "42,132,108" : "172,224,207", seed: (i * 0.11 + 0.6) % 1, bend: 0.08 });
    }

    const bots = d.runtime && d.runtime.wired ? d.runtime.bots : [];
    const bn = Math.min(bots.length, 20);
    for (let i = 0; i < bn; i++) {
      const x = bots[i]!;
      const a = 0.75 + (i / Math.max(1, bn)) * TAU;
      const idx = nodes.length;
      nodes.push({ x: cx + Math.cos(a) * span * 0.36, y: cy + Math.sin(a) * span * 0.30, r: 3.0, color: light ? "52,122,152" : "172,214,232", label: x.project, kind: "bot" });
      links.push({ a: projectPos.get(x.project) ?? 0, b: idx, color: light ? "52,122,152" : "172,214,232", seed: (i * 0.19 + 0.4) % 1, bend: 0.12 });
    }
  };

  const qp = (a: FieldNode, b: FieldNode, mx: number, my: number, u: number): { x: number; y: number } => {
    const v = 1 - u;
    return { x: v * v * a.x + 2 * v * u * mx + u * u * b.x, y: v * v * a.y + 2 * v * u * my + u * u * b.y };
  };

  const draw = (now: number): void => {
    const t = now / 1000;
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    ctx.clearRect(0, 0, W, H);
    const core = nodes[0]!;
    const halo = ctx.createRadialGradient(core.x, core.y, 0, core.x, core.y, 92);
    halo.addColorStop(0, light ? "rgba(168,130,50,0.05)" : "rgba(230,207,152,0.18)");
    halo.addColorStop(1, light ? "rgba(168,130,50,0)" : "rgba(230,207,152,0)");
    ctx.fillStyle = halo;
    ctx.beginPath();
    ctx.arc(core.x, core.y, 92, 0, TAU);
    ctx.fill();

    // 同心轨道：只是尺度参照，不表示任何层级或流向。
    ctx.save();
    ctx.setLineDash([2, 8]);
    for (let k = 1; k <= 3; k++) {
      ctx.strokeStyle = light
        ? "rgba(30,32,28," + (0.075 - k * 0.012) + ")"
        : "rgba(236,236,232," + (0.042 - k * 0.008) + ")";
      ctx.lineWidth = 0.6;
      ctx.beginPath();
      ctx.arc(core.x, core.y, span * (0.16 + k * 0.12), 0, TAU);
      ctx.stroke();
    }
    ctx.restore();

    for (const p of dust) {
      const tw = 0.5 + 0.5 * Math.sin(t * 0.45 + p.ph);
      ctx.fillStyle = light
        ? "rgba(58,62,54," + (p.a * (0.55 + 0.45 * tw) * 1.8).toFixed(3) + ")"
        : "rgba(240,236,224," + (p.a * (0.35 + 0.65 * tw)).toFixed(3) + ")";
      ctx.beginPath();
      ctx.arc(p.x, p.y + Math.sin(t * 0.16 + p.ph) * 5, p.r, 0, TAU);
      ctx.fill();
    }

    for (const l of links) {
      const a = nodes[l.a];
      const b = nodes[l.b];
      if (!a || !b) continue;
      const mx = (a.x + b.x) / 2 + (b.y - a.y) * l.bend;
      const my = (a.y + b.y) / 2 - (b.x - a.x) * l.bend;
      ctx.strokeStyle = "rgba(" + l.color + ",0.13)";
      ctx.lineWidth = 0.8;
      ctx.beginPath();
      ctx.moveTo(a.x, a.y);
      ctx.quadraticCurveTo(mx, my, b.x, b.y);
      ctx.stroke();
      if (reduced) continue;
      const u = (t * 0.10 + l.seed) % 1;
      const p = qp(a, b, mx, my, u);
      const pulse = ctx.createRadialGradient(p.x, p.y, 0, p.x, p.y, 6);
      pulse.addColorStop(0, "rgba(" + l.color + ",0.85)");
      pulse.addColorStop(1, "rgba(" + l.color + ",0)");
      ctx.fillStyle = pulse;
      ctx.beginPath();
      ctx.arc(p.x, p.y, 6, 0, TAU);
      ctx.fill();
    }

    for (const n of nodes) {
      const glow = ctx.createRadialGradient(n.x, n.y, 0, n.x, n.y, n.r * 5);
      glow.addColorStop(0, "rgba(" + n.color + "," + (light ? (n.kind === "core" ? 0.16 : 0.13) : 0.55) + ")");
      glow.addColorStop(1, "rgba(" + n.color + ",0)");
      ctx.fillStyle = glow;
      ctx.beginPath();
      ctx.arc(n.x, n.y, n.r * 5, 0, TAU);
      ctx.fill();
      ctx.fillStyle = "rgba(" + n.color + ",0.95)";
      ctx.beginPath();
      ctx.arc(n.x, n.y, n.r, 0, TAU);
      ctx.fill();
      if ((n.kind === "domain" || n.kind === "project") && n.label !== "") {
        ctx.fillStyle = light ? "rgba(28,30,26,0.78)" : "rgba(236,236,233,0.68)";
        ctx.font = "10.5px -apple-system, 'PingFang SC', sans-serif";
        ctx.fillText(n.label.slice(0, 12), n.x + n.r + 5, n.y + 3);
      }
    }
    if (!reduced) fieldRaf = requestAnimationFrame(draw);
  };

  const start = (): void => {
    build();
    if (fieldRaf) cancelAnimationFrame(fieldRaf);
    if (reduced) draw(performance.now());
    else fieldRaf = requestAnimationFrame(draw);
  };
  fieldResize = new ResizeObserver(start);
  fieldResize.observe(canvas);
  start();
}
