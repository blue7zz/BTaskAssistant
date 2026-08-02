#!/usr/bin/env node
// scope-rx-css.mjs — 把 reasonix styles.css 改写为 :host 作用域版本（阶段 5）。
//
// 替代 embedEntry 的运行时正则改写：用 PostCSS AST 处理——
//  1. html / body / :root 元素选择器 → :host（含属性/伪类组合与多行选择器）；
//  2. url() 相对资源（seti.woff）内联为 data URI（?raw 注入 shadow 后
//     相对路径失效）；
//  3. 输出到 src/generated/scoped-styles.css，embedEntry 以 ?raw import。
//
// 构建链：frontend package.json 的 prebuild 运行本脚本；reasonix 独立应用
// 不引用 embedEntry，不受影响。

import { readFileSync, writeFileSync, mkdirSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { createRequire } from "node:module";

// postcss 装在 frontend/（pnpm 项目依赖）——从 frontend 目录解析
const root = join(dirname(fileURLToPath(import.meta.url)), "..");

// postcss 装在 frontend/（pnpm 项目依赖）——从 frontend 目录解析
const require = createRequire(join(root, "frontend/package.json"));
const postcss = require("postcss");
const srcDir = join(root, "reasonix-app/desktop/frontend/src");
const cssPath = join(srcDir, "styles.css");
const outPath = join(srcDir, "generated", "scoped-styles.css");

const css = readFileSync(cssPath, "utf8");

// seti.woff 内联
const setiPath = join(srcDir, "assets/file-icons/seti/seti.woff");
let setiDataUri = "";
try {
  const font = readFileSync(setiPath);
  setiDataUri = `data:font/woff;base64,${font.toString("base64")}`;
} catch {
  console.error("[scope-rx-css] 警告: seti.woff 缺失，@font-face 保持原样");
}

const root2 = postcss.parse(css, { from: cssPath });

root2.walkRules((rule) => {
  // 选择器改写：html/body/:root → :host
  rule.selector = rule.selector
    .replace(/:root(?=[\s,:[]|$)/g, ":host")
    .replace(/(^|[\s,>+~])(html|body)(?=[\s.,:>[]|$)/g, "$1:host")
    .replace(/(^|[\s,>+~])(html|body)(?=\s)/g, "$1:host");
});

// url() 内联（仅相对路径的字体/资源；绝对与 data URI 保持）
if (setiDataUri) {
  root2.walkDecls(/^(src|background|background-image)$/i, (decl) => {
    decl.value = decl.value.replace(
      /url\(["']?\.\/assets\/file-icons\/seti\/seti\.woff["']?\)/g,
      `url("${setiDataUri}")`,
    );
  });
}

mkdirSync(dirname(outPath), { recursive: true });
writeFileSync(outPath, root2.toString());
console.log(`[scope-rx-css] ${cssPath} → ${outPath}`);
