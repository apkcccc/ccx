#!/bin/sh
# CCX 启动脚本（禁用自动恢复，保留手动同步）

set -e

echo "=== CCX 启动中 ==="

# 禁用自动恢复功能（避免编码问题）
# 配置文件已在 Docker 镜像中，不需要从 GitHub 恢复
echo "使用镜像中的配置文件"

# 启动定时同步任务（后台运行）
if [ -n "$GITHUB_TOKEN" ]; then
  echo "启动定时同步任务（每分钟）..."
  (
    while true; do
      sleep 60  # 每分钟同步一次（测试用）
      /app/scripts/sync-to-github.sh || echo "同步失败"
    done
  ) &
else
  echo "未设置 GITHUB_TOKEN，跳过自动同步"
fi

# 启动 CCX 服务
echo "启动 CCX 服务..."
exec /app/ccx
