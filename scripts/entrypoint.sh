#!/bin/sh
# CCX 启动脚本（无自动同步）

set -e

echo "=== CCX 启动中 ==="

# 启动 CCX 服务
echo "启动 CCX 服务..."
exec /app/ccx
