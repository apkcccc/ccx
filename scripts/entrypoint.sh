#!/bin/sh
# CCX 启动脚本（启动时恢复配置）

set -e

echo "=== CCX 启动中 ==="

# 启动时从 GitHub 恢复配置（如果存在）
if [ -n "$GITHUB_TOKEN" ]; then
  echo "从 GitHub 恢复配置..."
  /app/scripts/restore-from-github.sh || echo "恢复失败，使用默认配置"
else
  echo "未设置 GITHUB_TOKEN，使用默认配置"
fi

# 启动 CCX 服务
echo "启动 CCX 服务..."
exec /app/ccx
