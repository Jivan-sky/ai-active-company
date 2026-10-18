// 构建看板前端：TS → 一个静态 app.js，出到 internal/board/ui/（随后被 go:embed 打进二进制）。
//
// 为什么产物要落在 Go 包里：embed 只能嵌**自己包目录之下**的文件。
// 所以源码与产物分两处 —— 源码在这里（带 node_modules，不入库），
// 产物在 Go 包那侧（入库，现场编译 Go 二进制不需要 node）。

import { build } from "esbuild";
import fs from "node:fs";
import { fileURLToPath } from "node:url";
import path from "node:path";

const here = path.dirname(fileURLToPath(import.meta.url));
const outfile = path.resolve(here, "..", "internal", "board", "ui", "app.js");

const result = await build({
  entryPoints: [path.join(here, "src", "app.ts")],
  outfile,
  bundle: true,
  format: "esm",
  target: ["chrome100", "safari15", "firefox100"],
  minify: true,
  legalComments: "none",
  sourcemap: false,
  metafile: false,
  logLevel: "info",
});

if (result.errors.length > 0) {
  console.error("构建失败：", result.errors);
  process.exit(1);
}
console.log(`已生成 ${path.relative(here, outfile)}`);

// 样式同样分模块写，构建期按 index.css 里的 @import 顺序原样拼成一份 app.css。
// 不用 esbuild 处理 CSS：它会重排/展开格式，让 diff 变成一大片无关变更；
// 这里只需要保序拼接，产物与源文件逐行一致。
const cssEntry = path.join(here, "src", "styles", "index.css");
const cssOut = path.resolve(here, "..", "internal", "board", "ui", "app.css");
const cssSource = fs.readFileSync(cssEntry, "utf8");
const css = cssSource.replace(/@import\s+"([^"]+)";/g, (_all, rel) =>
  fs.readFileSync(path.resolve(path.dirname(cssEntry), rel), "utf8").trimEnd() + "\n");
fs.writeFileSync(cssOut, css.trimEnd() + "\n", "utf8");
console.log(`已生成 ${path.relative(here, cssOut)}`);
