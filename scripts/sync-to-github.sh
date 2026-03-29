#!/bin/sh
# 自动同步配置到 GitHub

set -e

GITHUB_TOKEN="${GITHUB_TOKEN}"
GITHUB_REPO="apkcccc/ccx"
CONFIG_FILE=".config/config.json"

if [ -z "$GITHUB_TOKEN" ]; then
  echo "错误: GITHUB_TOKEN 环境变量未设置"
  exit 1
fi

if [ ! -f "$CONFIG_FILE" ]; then
  echo "配置文件不存在，跳过同步"
  exit 0
fi

echo "开始同步配置到 GitHub..."

# 获取当前文件的 SHA
CURRENT_SHA=$(curl -s -H "Authorization: token $GITHUB_TOKEN" \
  "https://api.github.com/repos/$GITHUB_REPO/contents/$CONFIG_FILE" | \
  grep '"sha"' | head -1 | sed 's/.*"sha": "\(.*\)".*/\1/')

# Base64 编码配置文件
CONTENT=$(base64 -w 0 "$CONFIG_FILE" 2>/dev/null || base64 "$CONFIG_FILE")

# 提交到 GitHub
curl -X PUT -H "Authorization: token $GITHUB_TOKEN" \
  -H "Content-Type: application/json" \
  "https://api.github.com/repos/$GITHUB_REPO/contents/$CONFIG_FILE" \
  -d "{
    \"message\": \"Auto-sync config from container\",
    \"content\": \"$CONTENT\",
    \"sha\": \"$CURRENT_SHA\"
  }"

echo "配置同步完成！"
