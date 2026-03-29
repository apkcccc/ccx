#!/bin/sh
# CCX 启动脚本（带自动同步）

set -e

echo "=== CCX 启动中 ==="

# 1. 从 GitHub 恢复配置
if [ -n "$GITHUB_TOKEN" ]; then
  echo "从 GitHub 恢复配置..."
  /app/scripts/restore-from-github.sh || echo "恢复失败，使用默认配置"
else
  echo "未设置 GITHUB_TOKEN，跳过配置恢复"
fi

# 2. 启动定时同步任务（后台运行）
if [ -n "$GITHUB_TOKEN" ]; then
  echo "启动定时同步任务（每小时）..."
  (
    while true; do
      sleep 3600  # 每小时同步一次
      /app/scripts/sync-to-github.sh || echo "同步失败"
    done
  ) &
fi

# 3. 启动 CCX 服务
echo "启动 CCX 服务..."
exec /app/ccx
