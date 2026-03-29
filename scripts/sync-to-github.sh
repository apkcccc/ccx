#!/bin/sh
# 自动同步配置到 GitHub（仅在有变化时）

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

# 获取当前文件的 SHA 和内容
REMOTE_DATA=$(curl -s -H "Authorization: token $GITHUB_TOKEN" \
  "https://api.github.com/repos/$GITHUB_REPO/contents/$CONFIG_FILE")

CURRENT_SHA=$(echo "$REMOTE_DATA" | grep '"sha"' | head -1 | sed 's/.*"sha": "\(.*\)".*/\1/')
REMOTE_CONTENT=$(echo "$REMOTE_DATA" | grep -o '"content": *"[^"]*"' | sed 's/"content": *"//' | sed 's/"$//' | tr -d '\n' | base64 -d)

# 读取本地文件内容
LOCAL_CONTENT=$(cat "$CONFIG_FILE")

# 比较内容，只在有变化时同步
if [ "$LOCAL_CONTENT" = "$REMOTE_CONTENT" ]; then
  echo "配置无变化，跳过同步"
  exit 0
fi

echo "检测到配置变化，开始同步到 GitHub..."

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
