// 「谁在点 / 谁在归档」：看板没有免登，这一栏由人填、本机记住。
// 它只影响留痕里的 by 字段，不是凭据，也不参与任何权限判断。
export const BY_KEY = "anc.board.by";

export let byWho = "";
try {
  byWho = localStorage.getItem(BY_KEY) ?? "";
} catch {
  byWho = "";
}

export function setByWho(value: string): void {
  byWho = value;
  try {
    localStorage.setItem(BY_KEY, value);
  } catch {
    // 记不住就算了：留痕照写，只是下次要重打。
  }
}
