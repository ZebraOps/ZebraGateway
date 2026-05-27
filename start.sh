#!/bin/bash
# ZebraGateway 启动脚本（集成 Nacos）

# Nacos 连接配置
export NACOS_SERVER_ADDR="localhost:8848"
export NACOS_NAMESPACE=""
export NACOS_USERNAME="nacos"
export NACOS_PASSWORD="nacos"

# 服务配置（可选，用于服务注册）
export SERVICE_IP="127.0.0.1"
export SERVICE_PORT="4121"

echo "=========================================="
echo "启动 ZebraGateway 服务"
echo "=========================================="
echo "Nacos 服务器: $NACOS_SERVER_ADDR"
echo "命名空间: ${NACOS_NAMESPACE:-public(default)}"
echo "服务地址: $SERVICE_IP:$SERVICE_PORT"
echo "=========================================="
echo ""

# 启动服务
go run main.go
