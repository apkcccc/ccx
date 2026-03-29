#!/bin/sh
# CCX 启动脚本

set -e

echo "=== CCX 启动中 ==="

# 启动时从 GitHub 恢复配置
if [ -n "$GITHUB_TOKEN" ]; then
  echo "从 GitHub 恢复配置..."
  /app/scripts/restore-from-github.sh || echo "恢复失败，使用默认配置"
  # 启动自动同步任务（每小时）
  echo "启动自动同步任务（每小时检查，仅在配置变化时同步）..."
  (
    while true; do
      sleep 3600
      /app/scripts/sync-to-github.sh || echo "同步失败"
    done
  ) &
else
  echo "未设置 GITHUB_TOKEN，跳过配置恢复和自动同步"
fi

# 启动 CCX 服务
echo "启动 CCX 服务..."
exec /app/ccx
