# iCloud 隐藏邮箱管理工具 Makefile

# 变量定义
BINARY_NAME=icloud-hme
MAIN_FILE=main.go
BUILD_DIR=build
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS=-ldflags "-X main.Version=$(VERSION)"

# 默认目标
.PHONY: all
all: clean build

# 清理构建文件
.PHONY: clean
clean:
	@echo "清理构建文件..."
	@rm -rf $(BUILD_DIR)
	@rm -f $(BINARY_NAME)

# 创建构建目录
$(BUILD_DIR):
	@mkdir -p $(BUILD_DIR)

# 本地构建
.PHONY: build
build: $(BUILD_DIR)
	@echo "构建本地版本..."
	@go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) $(MAIN_FILE)
	@echo "构建完成: $(BUILD_DIR)/$(BINARY_NAME)"

# 快速构建（当前目录）
.PHONY: build-local
build-local:
	@echo "快速构建到当前目录..."
	@go build $(LDFLAGS) -o $(BINARY_NAME) $(MAIN_FILE)
	@echo "构建完成: ./$(BINARY_NAME)"

# 交叉编译所有平台
.PHONY: build-all
build-all: $(BUILD_DIR)
	@echo "开始交叉编译所有平台..."
	
	@echo "构建 macOS (Intel)..."
	@GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 $(MAIN_FILE)
	
	@echo "构建 macOS (Apple Silicon)..."
	@GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 $(MAIN_FILE)
	
	@echo "构建 Linux (x64)..."
	@GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 $(MAIN_FILE)
	
	@echo "构建 Linux (ARM64)..."
	@GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 $(MAIN_FILE)
	
	@echo "构建 Windows (x64)..."
	@GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe $(MAIN_FILE)
	
	@echo "所有平台构建完成！"
	@ls -la $(BUILD_DIR)/

# 构建发布包
.PHONY: release
release: build-all
	@echo "创建发布包..."
	@cd $(BUILD_DIR) && \
	for file in $(BINARY_NAME)-*; do \
		if [[ $$file == *.exe ]]; then \
			zip "$${file%.exe}.zip" "$$file" ../config.json.example ../README.md; \
		else \
			tar -czf "$$file.tar.gz" "$$file" ../config.json.example ../README.md; \
		fi; \
	done
	@echo "发布包创建完成！"
	@ls -la $(BUILD_DIR)/*.{zip,tar.gz} 2>/dev/null || true

# 运行程序
.PHONY: run
run:
	@go run $(MAIN_FILE)

# 格式化代码
.PHONY: fmt
fmt:
	@echo "格式化代码..."
	@go fmt ./...

# 代码检查
.PHONY: vet
vet:
	@echo "代码检查..."
	@go vet ./...

# 运行测试
.PHONY: test
test:
	@echo "运行测试..."
	@go test -v ./...

# 安装依赖
.PHONY: deps
deps:
	@echo "安装依赖..."
	@go mod tidy
	@go mod download

# 显示帮助
.PHONY: help
help:
	@echo "可用的命令："
	@echo "  build       - 构建本地版本"
	@echo "  build-local - 快速构建到当前目录"
	@echo "  build-all   - 交叉编译所有平台"
	@echo "  release     - 创建发布包"
	@echo "  run         - 运行程序"
	@echo "  clean       - 清理构建文件"
	@echo "  fmt         - 格式化代码"
	@echo "  vet         - 代码检查"
	@echo "  test        - 运行测试"
	@echo "  deps        - 安装依赖"
	@echo "  help        - 显示此帮助信息"

# 开发环境设置
.PHONY: dev-setup
dev-setup: deps
	@echo "设置开发环境..."
	@if [ ! -f config.json ]; then \
		cp config.json.example config.json; \
		echo "已创建 config.json，请编辑填入你的认证信息"; \
	fi
	@echo "开发环境设置完成！"
