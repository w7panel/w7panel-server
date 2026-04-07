#!/bin/sh

# 执行构建脚本
/kaniko/buildimage.sh

if [ $? -eq 0 ]; then
    echo "build image success"
    # 检查 NOTIFY_COMPLETION_URL 是否存在且非空字符串（更简洁的判断）
    if [ -n "${NOTIFY_COMPLETION_URL:-}" ]; then
        echo "sending success notification to: ${NOTIFY_COMPLETION_URL}"
        curl --header "User-Agent: ${USER_AGENT}" -s "${NOTIFY_COMPLETION_URL}"
    else
        echo "NOTIFY_COMPLETION_URL is empty or not set, skipping notification"
    fi
else
    echo "build image error"
    # 检查 NOTIFY_FAILED_URL 是否存在且非空字符串
    if [ -n "${NOTIFY_FAILED_URL:-}" ]; then
        echo "sending error notification to: ${NOTIFY_FAILED_URL}"
        curl --header "User-Agent: ${USER_AGENT}" -s "${NOTIFY_FAILED_URL}"
    else
        echo "NOTIFY_FAILED_URL is empty or not set, skipping notification"
    fi
    exit 1
fi