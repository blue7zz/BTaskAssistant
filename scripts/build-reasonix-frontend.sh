#!/bin/sh
# 构建 Reasonix 前端（融入 BTask 构建流程：wails build 前自动执行）。
# 产物 reasonix-app/desktop/frontend/dist 由 BTask 的 asset server 在同源
# /reasonix/ 路径提供，RX 标签页 iframe 以 ?host=1 模式加载。
set -e
cd "$(dirname "$0")/../reasonix-app/desktop/frontend"
if [ ! -d node_modules ]; then
  pnpm install --frozen-lockfile
fi
pnpm build
