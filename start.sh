#!/bin/bash
# ZebraGateway 启动脚本
# Usage:
#   ./start.sh              # 直接启动
#   ./start.sh --wait       # 等待上游服务就绪后启动

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
WAIT_UPSTREAM=false

# 解析参数
for arg in "$@"; do
    case $arg in
        --wait) WAIT_UPSTREAM=true ;;
    esac
done

# Nacos 连接配置
export NACOS_SERVER_ADDR="${NACOS_SERVER_ADDR:-localhost:8848}"
export NACOS_NAMESPACE="${NACOS_NAMESPACE:-}"
export NACOS_USERNAME="${NACOS_USERNAME:-nacos}"
export NACOS_PASSWORD="${NACOS_PASSWORD:-nacos}"

# 服务配置
export SERVICE_IP="${SERVICE_IP:-127.0.0.1}"
export SERVICE_PORT="${SERVICE_PORT:-4121}"
export ZEBRA_GW_APP_PORT="${ZEBRA_GW_APP_PORT:-4121}"

# 上游服务健康检查URL
declare -A UPSTREAM_SERVICES=(
    ["ZebraRBAC"]="http://127.0.0.1:4122/health"
    ["ZebraRAG"]="http://127.0.0.1:4124/health"
    ["ZebraCICD"]="http://127.0.0.1:4123/health"
)

# 健康检查函数
check_health() {
    local name="$1"
    local url="$2"
    local max_retries=30
    local retry=0

    echo -n "  $name: "
    while [[ $retry -lt $max_retries ]]; do
        if curl -sf --connect-timeout 2 "$url" > /dev/null 2>&1; then
            echo "✅ 就绪"
            return 0
        fi
        retry=$((retry + 1))
        sleep 1
    done
    echo "⚠️  未就绪 (跳过)"
    return 1
}

echo "=========================================="
echo "启动 ZebraGateway 服务"
echo "=========================================="
echo "Nacos 服务器: $NACOS_SERVER_ADDR"
echo "命名空间: ${NACOS_NAMESPACE:-public(default)}"
echo "服务地址: $SERVICE_IP:$SERVICE_PORT"
echo "=========================================="

# 等待上游服务就绪
if [[ "$WAIT_UPSTREAM" = true ]]; then
    echo ""
    echo "🔍 检查上游服务状态..."
    for name in "${!UPSTREAM_SERVICES[@]}"; do
        check_health "$name" "${UPSTREAM_SERVICES[$name]}" &
    done
    wait
    echo ""
fi

cd "$SCRIPT_DIR"

echo "🚀 启动 Gateway..."
echo ""

# 启动服务
go run main.go
