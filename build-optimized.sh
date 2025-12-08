#!/bin/bash

# Ech0-Noise 优化构建脚本
# 提供多种构建选项以加速 Docker 镜像构建

set -e

VERSION=${1:-v2.3.2}
TARGET=${2:-final}
USE_UPX=${3:-0}  # 默认禁用 UPX 以加速构建

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# 输出函数
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# 检查 Docker 是否安装
check_docker() {
    if ! command -v docker &> /dev/null; then
        log_error "Docker 未安装，请先安装 Docker"
        exit 1
    fi
}

# 检查构建器是否存在，不存在则创建
setup_buildx() {
    if ! docker buildx ls | grep -q "multiarch"; then
        log_info "创建多架构构建器..."
        docker buildx create --name multiarch --driver docker-container --use
    else
        log_info "使用现有多架构构建器"
        docker buildx use multiarch
    fi
    docker buildx inspect --bootstrap
}

# 快速构建（仅当前架构，禁用 UPX）
build_fast() {
    log_info "执行快速构建（仅当前架构，禁用 UPX）..."
    docker buildx build \
        --target $TARGET \
        --build-arg VERSION=$VERSION \
        --build-arg USE_UPX=0 \
        -t noise233/echo-noise:$VERSION \
        -t noise233/echo-noise:latest \
        --load \
        --no-cache .
    log_success "快速构建完成"
}

# 多架构构建（禁用 UPX）
build_multi_arch_no_upx() {
    log_info "执行多架构构建（禁用 UPX）..."
    docker buildx build \
        --platform linux/amd64,linux/arm64 \
        --target $TARGET \
        --build-arg VERSION=$VERSION \
        --build-arg USE_UPX=0 \
        -t noise233/echo-noise:$VERSION \
        -t noise233/echo-noise:latest \
        --push \
        --no-cache .
    log_success "多架构构建完成"
}

# 完整构建（包含 UPX）
build_full() {
    log_info "执行完整构建（包含 UPX 压缩）..."
    docker buildx build \
        --platform linux/amd64,linux/arm64 \
        --target $TARGET \
        --build-arg VERSION=$VERSION \
        --build-arg USE_UPX=1 \
        -t noise233/echo-noise:$VERSION \
        -t noise233/echo-noise:latest \
        --push \
        --no-cache .
    log_success "完整构建完成"
}

# 增量构建（使用缓存）
build_incremental() {
    log_info "执行增量构建（使用缓存）..."
    docker buildx build \
        --platform linux/amd64,linux/arm64 \
        --target $TARGET \
        --build-arg VERSION=$VERSION \
        --build-arg USE_UPX=$USE_UPX \
        -t noise233/echo-noise:$VERSION \
        -t noise233/echo-noise:latest \
        --push .
    log_success "增量构建完成"
}

# 显示帮助信息
show_help() {
    echo "Ech0-Noise Docker 构建脚本"
    echo ""
    echo "用法: $0 [版本] [目标] [UPX标志]"
    echo "  - 版本: 镜像版本 (默认: v2.3.2)"
    echo "  - 目标: 构建目标 (默认: final)"
    echo "  - UPX标志: 是否使用 UPX (默认: 0=禁用)"
    echo ""
    echo "快速构建选项:"
    echo "  $0 fast                    # 快速构建（当前架构，禁用UPX）"
    echo "  $0 multi                   # 多架构构建（禁用UPX）"
    echo "  $0 full                    # 完整构建（包含UPX）"
    echo "  $0 incremental             # 增量构建（使用缓存）"
    echo ""
    echo "示例:"
    echo "  $0 v2.3.3 final 0          # 构建 v2.3.3，final目标，禁用UPX"
    echo "  $0 fast                    # 快速构建"
    echo "  $0 multi                   # 多架构构建（禁用UPX）"
}

# 主函数
main() {
    check_docker
    
    case "$1" in
        "help"|"-h"|"--help")
            show_help
            exit 0
            ;;
        "fast")
            setup_buildx
            build_fast
            ;;
        "multi")
            setup_buildx
            build_multi_arch_no_upx
            ;;
        "full")
            setup_buildx
            build_full
            ;;
        "incremental")
            setup_buildx
            build_incremental
            ;;
        *)
            if [ $# -eq 0 ]; then
                log_warning "未指定构建模式，使用默认参数"
                setup_buildx
                build_multi_arch_no_upx
            else
                # 使用参数化构建
                setup_buildx
                log_info "执行参数化构建..."
                docker buildx build \
                    --platform linux/amd64,linux/arm64 \
                    --target $TARGET \
                    --build-arg VERSION=$VERSION \
                    --build-arg USE_UPX=$USE_UPX \
                    -t noise233/echo-noise:$VERSION \
                    -t noise233/echo-noise:latest \
                    --push \
                    --no-cache .
                log_success "参数化构建完成"
            fi
            ;;
    esac
}

# 执行主函数
main "$@"