// 页面不直接 import 外壳：外壳依赖页面注册表，直接互引会绕成一圈。
// 这里只放两个注入口，由 app.ts 在启动时把 shell / boot 接上。
let rerenderImpl: () => void = () => {};
let reloadImpl: (force?: boolean) => void = () => {};

export function setRerender(fn: () => void): void {
  rerenderImpl = fn;
}

export function rerender(): void {
  rerenderImpl();
}

export function setReloader(fn: (force?: boolean) => void): void {
  reloadImpl = fn;
}

export function reload(force = false): void {
  reloadImpl(force);
}
