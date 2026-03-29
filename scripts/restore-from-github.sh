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

echo "从 GitHub 恢复配置..."

# 下载配置文件
curl -s -H "Authorization: token $GITHUB_TOKEN" \
  "https://api.github.com/repos/$GITHUB_REPO/contents/$CONFIG_FILE" | \
  grep '"content"' | sed 's/.*"content": "\(.*\)".*/\1/' | \
  tr -d '\n' | base64 -d > "$CONFIG_FILE"

if [ -f "$CONFIG_FILE" ]; then
  echo "配置恢复成功！"
else
  echo "配置恢复失败"
  exit 1
fi
