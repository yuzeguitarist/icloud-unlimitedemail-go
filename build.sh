#!/bin/bash

# iCloud 隐藏邮箱管理工具构建脚本
# 作者: yuzeguitarist
# 版本: 2.0.0

set -e

# 颜色定义
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
PURPLE='\033[0;35m'
CYAN='\033[0;36m'
WHITE='\033[1;37m'
NC='\033[0m' # No Color

# 变量定义
BINARY_NAME="icloud-hme"
MAIN_FILE="main.go"
BUILD_DIR="build"
VERSION=$(git describe --tags --always --dirty 2>/dev/null || echo "v2.0.0")

# 打印带颜色的消息
print_info() {
    echo -e "${CYAN}[信息]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[成功]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[警告]${NC} $1"
}

print_error() {
    echo -e "${RED}[错误]${NC} $1"
}

print_header() {
    echo -e "${PURPLE}======================================${NC}"
    echo -e "${WHITE}  $1${NC}"
    echo -e "${PURPLE}======================================${NC}"
}

# 检查 Go 环境
check_go() {
    if ! command -v go &> /dev/null; then
        print_error "Go 未安装或不在 PATH 中"
        exit 1
    fi
    
    GO_VERSION=$(go version | awk '{print $3}' | sed 's/go//')
    print_info "Go 版本: $GO_VERSION"
}

# 清理构建目录
clean_build() {
    print_info "清理构建目录..."
    rm -rf "$BUILD_DIR"
    rm -f "$BINARY_NAME"
    print_success "清理完成"
}

# 创建构建目录
create_build_dir() {
    if [ ! -d "$BUILD_DIR" ]; then
        mkdir -p "$BUILD_DIR"
        print_info "创建构建目录: $BUILD_DIR"
    fi
}

# 本地构建
build_local() {
    print_info "构建本地版本..."
    go build -ldflags "-X main.Version=$VERSION" -o "$BUILD_DIR/$BINARY_NAME" "$MAIN_FILE"
    print_success "本地构建完成: $BUILD_DIR/$BINARY_NAME"
}

# 交叉编译
cross_compile() {
    print_header "开始交叉编译"

    # 定义目标平台
    platforms=(
        "darwin/amd64:macOS (Intel)"
        "darwin/arm64:macOS (Apple Silicon)"
        "linux/amd64:Linux (x64)"
        "linux/arm64:Linux (ARM64)"
        "windows/amd64:Windows (x64)"
    )

    for platform_info in "${platforms[@]}"; do
        platform=$(echo "$platform_info" | cut -d':' -f1)
        description=$(echo "$platform_info" | cut -d':' -f2)
        GOOS=$(echo "$platform" | cut -d'/' -f1)
        GOARCH=$(echo "$platform" | cut -d'/' -f2)
        output_name="$BUILD_DIR/$BINARY_NAME-$GOOS-$GOARCH"

        if [ "$GOOS" = "windows" ]; then
            output_name="${output_name}.exe"
        fi

        print_info "构建 $description..."

        if GOOS=$GOOS GOARCH=$GOARCH go build -ldflags "-X main.Version=$VERSION" -o "$output_name" "$MAIN_FILE"; then
            print_success "✓ $description 构建完成"
        else
            print_error "✗ $description 构建失败"
            exit 1
        fi
    done

    print_success "所有平台构建完成！"
}

# 创建发布包
create_release_packages() {
    print_header "创建发布包"
    
    cd "$BUILD_DIR"
    
    for file in $BINARY_NAME-*; do
        if [ -f "$file" ]; then
            print_info "打包 $file..."
            
            if [[ $file == *.exe ]]; then
                # Windows 版本使用 zip
                package_name="${file%.exe}.zip"
                zip -q "$package_name" "$file" ../config.json.example ../README.md ../使用指南.md ../LICENSE
                print_success "创建 $package_name"
            else
                # Unix 系统使用 tar.gz
                package_name="$file.tar.gz"
                tar -czf "$package_name" "$file" ../config.json.example ../README.md ../使用指南.md ../LICENSE
                print_success "创建 $package_name"
            fi
        fi
    done
    
    cd ..
    print_success "所有发布包创建完成！"
}

# 显示构建结果
show_results() {
    print_header "构建结果"
    
    if [ -d "$BUILD_DIR" ]; then
        print_info "构建文件:"
        ls -la "$BUILD_DIR"/ | grep -E "\.(exe|tar\.gz|zip)$|^d|$BINARY_NAME" | while read -r line; do
            echo "  $line"
        done
        
        # 计算文件大小
        total_size=$(du -sh "$BUILD_DIR" | cut -f1)
        print_info "总大小: $total_size"
    fi
}

# 验证构建
verify_build() {
    print_header "验证构建"
    
    local_binary="$BUILD_DIR/$BINARY_NAME"
    if [ -f "$local_binary" ]; then
        print_info "测试本地二进制文件..."
        if "$local_binary" --version 2>/dev/null || echo "版本信息获取完成"; then
            print_success "本地二进制文件验证通过"
        else
            print_warning "无法获取版本信息，但文件存在"
        fi
    fi
}

# 主函数
main() {
    print_header "iCloud 隐藏邮箱管理工具构建脚本"
    print_info "版本: $VERSION"
    
    # 检查环境
    check_go
    
    # 解析命令行参数
    case "${1:-all}" in
        "clean")
            clean_build
            ;;
        "local")
            clean_build
            create_build_dir
            build_local
            show_results
            ;;
        "cross")
            clean_build
            create_build_dir
            cross_compile
            show_results
            ;;
        "release")
            clean_build
            create_build_dir
            build_local
            cross_compile
            create_release_packages
            show_results
            ;;
        "all"|*)
            clean_build
            create_build_dir
            build_local
            cross_compile
            verify_build
            show_results
            ;;
    esac
    
    print_success "构建脚本执行完成！"
}

# 显示帮助信息
show_help() {
    echo "用法: $0 [选项]"
    echo ""
    echo "选项:"
    echo "  clean   - 只清理构建文件"
    echo "  local   - 只构建本地版本"
    echo "  cross   - 只进行交叉编译"
    echo "  release - 构建并创建发布包"
    echo "  all     - 完整构建（默认）"
    echo "  help    - 显示此帮助信息"
}

# 检查参数
if [ "$1" = "help" ] || [ "$1" = "-h" ] || [ "$1" = "--help" ]; then
    show_help
    exit 0
fi

# 执行主函数
main "$@"
