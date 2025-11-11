#! /bin/bash

# 检查参数
if [ -z "$1" ]; then
    echo "Usage: $0 <config_name>"
    echo "Example: $0 05"
    exit 1
fi

CONFIG_NAME=$1
SOURCE_FILE="configs/${CONFIG_NAME}.yaml"
TARGET_FILE="internal/config/app.yaml"

# 检查源文件是否存在
if [ ! -f "$SOURCE_FILE" ]; then
    echo "Error: Config file not found: $SOURCE_FILE"
    exit 1
fi

# 复制配置文件
echo "Copying $SOURCE_FILE to $TARGET_FILE"
cp "$SOURCE_FILE" "$TARGET_FILE"

# 构建
CGO_ENABLED=0 go build
