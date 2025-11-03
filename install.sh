#!/bin/bash

# iCloud 隐藏邮箱管理工具安装脚本
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

# 检测系统和架构
detect_platform() {
    local os=$(uname -s | tr '[:upper:]' '[:lower:]')
    local arch=$(uname -m)
    
    case "$os" in
        darwin)
            OS="darwin"
            ;;
        linux)
            OS="linux"
            ;;
        *)
            print_error "不支持的操作系统: $os"
            exit 1
            ;;
    esac
    
    case "$arch" in
        x86_64|amd64)
            ARCH="amd64"
            ;;
        arm64|aarch64)
            ARCH="arm64"
            ;;
        *)
            print_error "不支持的架构: $arch"
            exit 1
            ;;
    esac
    
    PLATFORM="${OS}-${ARCH}"
    print_info "检测到平台: $PLATFORM"
}

# 安装程序
install_binary() {
    local binary_name="icloud-hme-${PLATFORM}"
    local install_dir="/usr/local/bin"
    local binary_path="build/${binary_name}"
    
    if [ ! -f "$binary_path" ]; then
        print_error "找不到二进制文件: $binary_path"
        print_info "请先运行构建脚本: ./build.sh"
        exit 1
    fi
    
    print_info "安装 $binary_name 到 $install_dir..."
    
    # 检查是否需要 sudo
    if [ ! -w "$install_dir" ]; then
        print_warning "需要管理员权限安装到 $install_dir"
        sudo cp "$binary_path" "$install_dir/icloud-hme"
        sudo chmod +x "$install_dir/icloud-hme"
    else
        cp "$binary_path" "$install_dir/icloud-hme"
        chmod +x "$install_dir/icloud-hme"
    fi
    
    print_success "安装完成！"
    print_info "现在可以在任何地方运行: icloud-hme"
}

# 创建配置文件
setup_config() {
    local config_dir="$HOME/.config/icloud-hme"
    local config_file="$config_dir/config.json"
    
    if [ ! -d "$config_dir" ]; then
        mkdir -p "$config_dir"
        print_info "创建配置目录: $config_dir"
    fi
    
    if [ ! -f "$config_file" ]; then
        if [ -f "config.json.example" ]; then
            cp "config.json.example" "$config_file"
            print_success "配置文件已创建: $config_file"
            print_warning "请编辑配置文件填入你的认证信息"
        else
            print_warning "找不到配置示例文件"
        fi
    else
        print_info "配置文件已存在: $config_file"
    fi
}

# 卸载程序
uninstall() {
    local install_dir="/usr/local/bin"
    local binary_path="$install_dir/icloud-hme"
    
    if [ -f "$binary_path" ]; then
        print_info "卸载程序..."
        if [ ! -w "$install_dir" ]; then
            sudo rm -f "$binary_path"
        else
            rm -f "$binary_path"
        fi
        print_success "程序已卸载"
    else
        print_info "程序未安装"
    fi
    
    # 询问是否删除配置
    read -p "是否删除配置文件? (y/N): " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        rm -rf "$HOME/.config/icloud-hme"
        print_success "配置文件已删除"
    fi
}

# 显示帮助
show_help() {
    echo "iCloud 隐藏邮箱管理工具安装脚本"
    echo ""
    echo "用法: $0 [选项]"
    echo ""
    echo "选项:"
    echo "  install     - 安装程序（默认）"
    echo "  uninstall   - 卸载程序"
    echo "  help        - 显示帮助信息"
    echo ""
    echo "安装后可以通过 'icloud-hme' 命令运行程序"
}

# 主函数
main() {
    print_header "iCloud 隐藏邮箱管理工具安装脚本"
    
    case "${1:-install}" in
        "install")
            detect_platform
            install_binary
            setup_config
            print_success "安装完成！运行 'icloud-hme' 开始使用"
            ;;
        "uninstall")
            uninstall
            ;;
        "help"|"-h"|"--help")
            show_help
            ;;
        *)
            print_error "未知选项: $1"
            show_help
            exit 1
            ;;
    esac
}

# 执行主函数
main "$@"
