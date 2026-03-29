#!/bin/sh
# 从 GitHub 恢复配置

set -e

GITHUB_TOKEN="${GITHUB_TOKEN}"
GITHUB_REPO="apkcccc/ccx"
CONFIG_FILE=".config/config.json"

if [ -z "$GITHUB_TOKEN" ]; then
  echo "错误: GITHUB_TOKEN 环境变量未设置"
  exit 1
fi

echo "正在从 GitHub 下载配置..."

# 下载配置文件
REMOTE_DATA=$(curl -s -H "Authorization: token $GITHUB_TOKEN" \
  "https://api.github.com/repos/$GITHUB_REPO/contents/$CONFIG_FILE")

# 检查是否成功获取
if echo "$REMOTE_DATA" | grep -q '"content"'; then
  # 解码并保存
  echo "$REMOTE_DATA" | grep -o '"content": *"[^"]*"' | sed 's/"content": *"//' | sed 's/"$//' | tr -d '\n' | base64 -d > "$CONFIG_FILE"
  echo "配置恢复成功！"
else
  echo "GitHub 上没有配置文件，使用默认配置"
  exit 0
fi
