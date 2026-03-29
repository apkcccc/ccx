#!/bin/sh
# CCX 启动脚本（智能自动同步）

set -e

echo "=== CCX 启动中 ==="

# 启动定时同步任务（后台运行）
# 每小时检查一次，只在配置变化时才同步到 GitHub
if [ -n "$GITHUB_TOKEN" ]; then
  echo "启动智能同步任务（每小时检查，仅在配置变化时同步）..."
  (
    while true; do
      sleep 3600  # 每小时检查一次
      /app/scripts/sync-to-github.sh || echo "同步失败"
    done
  ) &
else
  echo "未设置 GITHUB_TOKEN，跳过自动同步"
fi

# 启动 CCX 服务
echo "启动 CCX 服务..."
exec /app/ccx
