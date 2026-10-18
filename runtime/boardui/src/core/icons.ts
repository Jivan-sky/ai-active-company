import { el } from "./dom";

// ---------- 图标：静态内联 SVG，不引外部图标库 ----------
// 只画线条，不加载字体或图片；颜色跟随 currentColor，放在哪个状态里就继承哪个颜色。
export const ICONS: Record<string, string> = {
  overview: "M12 3v3M12 18v3M3 12h3M18 12h3M5.6 5.6l2.1 2.1M16.3 16.3l2.1 2.1M18.4 5.6l-2.1 2.1M7.7 16.3l-2.1 2.1M12 8a4 4 0 100 8 4 4 0 000-8",
  check: "M4 12l5 5L20 6",
  pulse: "M3 12h4l2-6 4 12 2-6h6",
  flow: "M4 7h9a4 4 0 010 8H4M4 7l3-3M4 7l3 3M20 17h-9a4 4 0 010-8",
  flame: "M12 3c3 4 5 6 5 9a5 5 0 01-10 0c0-2 1-3 2-5 0 2 1 3 2 3 0-3 1-5 1-7z",
  building: "M4 21V8l8-5 8 5v13M9 21v-6h6v6M8 10h.01M12 10h.01M16 10h.01",
  grid: "M4 4h6v6H4zM14 4h6v6h-6zM4 14h6v6H4zM14 14h6v6h-6z",
  users: "M9 11a4 4 0 100-8 4 4 0 000 8zm-7 9a7 7 0 0114 0M17 11a3 3 0 100-6M18 21a6 6 0 00-3-5.2",
  layers: "M12 3l9 5-9 5-9-5 9-5zm-9 9l9 5 9-5M3 16l9 5 9-5",
  context: "M4 5h16v10H4zM8 19h8M12 15v4",
  key: "M14.5 4a5.5 5.5 0 00-3.9 9.4L4 20v-3h3v-3h3l.6-.6A5.5 5.5 0 1014.5 4z",
  clock: "M12 3a9 9 0 100 18 9 9 0 000-18zm0 4v5l3 2",
  alert: "M12 3l9 17H3L12 3zm0 6v5m0 3v.5",
  shield: "M12 3l8 3v6c0 5-3.5 8-8 9-4.5-1-8-4-8-9V6l8-3z",
  box: "M12 3l8 4.5v9L12 21l-8-4.5v-9L12 3zm0 0v9m8-4.5l-8 4.5-8-4.5M12 21v-9",
  strata: "M3 6h18M6 12h15M9 18h12",
  graph: "M5.5 6.5a2.5 2.5 0 105 0 2.5 2.5 0 00-5 0zM13.5 17.5a2.5 2.5 0 105 0 2.5 2.5 0 00-5 0zM13.5 6.5a2.5 2.5 0 105 0 2.5 2.5 0 00-5 0zM8 8.6l5.5 6.8M16 9v6",
  gateway: "M3 9h18M3 15h18M8 3v6M16 15v6M12 9v6",
  spark: "M12 3l1.7 5.3L19 10l-5.3 1.7L12 17l-1.7-5.3L5 10l5.3-1.7zM18.5 17.5l.7 2.1 2.1.7-2.1.7-.7 2.1-.7-2.1-2.1-.7 2.1-.7z",
  chevronL: "M15 6l-6 6 6 6",
  link: "M10 13a5 5 0 007.1 0l1.4-1.4a5 5 0 00-7.1-7.1L10.5 5.4M14 11a5 5 0 00-7.1 0l-1.4 1.4a5 5 0 007.1 7.1l.9-.9",
  search: "M10.5 4a6.5 6.5 0 104.4 11.3L20 21M10.5 4a6.5 6.5 0 014.4 11.3",
  menu: "M4 7h16M4 12h16M4 17h16",
  close: "M6 6l12 12M18 6L6 18",
  refresh: "M20 12a8 8 0 11-2.3-5.7M20 4v5h-5",
  copy: "M9 9h10v11H9zM5 15V4h10",
  sun: "M12 8a4 4 0 100 8 4 4 0 000-8M12 2v2M12 20v2M2 12h2M20 12h2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M19.1 4.9l-1.4 1.4M6.3 17.7l-1.4 1.4",
  moon: "M20.2 14.6A8.6 8.6 0 019.4 3.8a7 7 0 108.8 10.8z",
  arrow: "M5 12h14M13 6l6 6-6 6",
  chevron: "M6 9l6 6 6-6",
  dot: "M12 8a4 4 0 100 8 4 4 0 000-8",
};

export function icon(name: string, size = 16): SVGSVGElement {
  const ns = "http://www.w3.org/2000/svg";
  const svg = document.createElementNS(ns, "svg");
  svg.setAttribute("viewBox", "0 0 24 24");
  svg.setAttribute("width", String(size));
  svg.setAttribute("height", String(size));
  svg.setAttribute("aria-hidden", "true");
  const path = document.createElementNS(ns, "path");
  path.setAttribute("d", ICONS[name] ?? ICONS.dot!);
  svg.append(path);
  return svg;
}

export function iconBox(name: string, size = 16): HTMLElement {
  return el("span", { class: "nav-icon" }, icon(name, size));
}
