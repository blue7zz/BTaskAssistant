#!/bin/sh
# 全量验证：BTask 后端 + BTask 前端 + Reasonix 前端（三套测试 + 构建产物检查）。
# 用法：scripts/test-all.sh
set -e
ROOT="$(cd "$(dirname "$0")/.." && pwd)"

echo "== 1/4 BTask 后端 =="
cd "$ROOT"
go test ./...

echo "== 2/4 BTask 前端 =="
cd "$ROOT/frontend"
pnpm typecheck
pnpm vitest run

echo "== 3/4 Reasonix 前端 =="
cd "$ROOT/reasonix-app/desktop/frontend"
pnpm typecheck
pnpm test

echo "== 4/5 构建产物 =="
test -f "$ROOT/reasonix-app/desktop/frontend/src/generated/scoped-styles.css" \
  && echo "reasonix scoped styles: OK" \
  || { echo "reasonix dist 缺失，请运行 wails build（prebuild 自动构建）"; exit 1; }

echo "== 5/5 RX 运行时自检 =="
cd "$ROOT"
BTA_RX_SELFCHECK=1 go run . 2>&1 | tail -2 || { echo "RX 运行时自检失败"; exit 1; }

echo "全部验证通过"
