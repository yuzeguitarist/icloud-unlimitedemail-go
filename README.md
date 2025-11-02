<div align="center">

# iCloud 隐藏邮件地址管理工具

<p align="center">
  <img src="https://img.shields.io/badge/Go-1.19+-00ADD8?style=for-the-badge&logo=go&logoColor=white" alt="Go Version">
  <img src="https://img.shields.io/badge/Platform-macOS%20%7C%20Linux%20%7C%20Windows-lightgrey?style=for-the-badge" alt="Platform">
  <img src="https://img.shields.io/badge/License-MIT-green?style=for-the-badge" alt="License">
  <img src="https://img.shields.io/github/stars/yuzeguitarist/icloud-hme?style=for-the-badge" alt="Stars">
</p>

<p align="center">
  一个功能强大的 Go 语言工具，用于批量创建、管理和删除 iCloud 的"隐藏我的邮件"(Hide My Email) 地址
</p>

<p align="center">
  <a href="#快速开始">快速开始</a> •
  <a href="#功能特性">功能特性</a> •
  <a href="#安装">安装</a> •
  <a href="#使用方法">使用方法</a> •
  <a href="#配置说明">配置说明</a>
</p>

</div>

---

## 功能特性

<table>
<tr>
<td>

### 核心功能
- 🎯 **批量创建** - 一次性创建多个隐藏邮箱
- 📋 **邮箱管理** - 查看、停用、删除、重新激活
- 🔄 **智能重试** - 自动处理网络错误和限流
- 💾 **结果保存** - 自动保存到文件，支持追加模式

</td>
<td>

### 用户体验
- 🎨 **彩色界面** - 美观的命令行界面
- 📊 **进度显示** - 实时进度条和状态提示
- ⚙️ **灵活配置** - JSON配置文件，无需修改代码
- 🛡️ **安全可靠** - 支持自定义请求头和认证信息

</td>
</tr>
</table>

## 安装

### 方法一：下载预编译版本（推荐）

从 [Releases](https://github.com/yuzeguitarist/icloud-unlimitedemail-go/releases) 页面下载适合你系统的预编译版本。

| 平台 | 架构 | 文件名 |
|------|------|--------|
| macOS | Intel (x64) | `icloud-hme-darwin-amd64.tar.gz` |
| macOS | Apple Silicon (ARM64) | `icloud-hme-darwin-arm64.tar.gz` |
| Linux | x64 | `icloud-hme-linux-amd64.tar.gz` |
| Linux | ARM64 | `icloud-hme-linux-arm64.tar.gz` |
| Windows | x64 | `icloud-hme-windows-amd64.zip` |

### 方法二：使用构建脚本

```bash
# 克隆仓库
git clone https://github.com/yuzeguitarist/icloud-unlimitedemail-go.git
cd icloud-unlimitedemail-go

# 构建所有平台版本
./build.sh release

# 或者只构建本地版本
./build.sh local

# 安装到系统（macOS/Linux）
./install.sh
```

### 方法三：使用 Makefile

```bash
# 构建本地版本
make build

# 构建所有平台
make build-all

# 创建发布包
make release

# 清理构建文件
make clean
```

### 方法四：手动编译

```bash
# 编译当前平台
go build -o icloud-hme main.go

# 交叉编译（示例：Linux x64）
GOOS=linux GOARCH=amd64 go build -o icloud-hme-linux-amd64 main.go
```

## 快速开始

### 1. 配置认证信息

复制配置文件模板：
```bash
cp config.json.example config.json
```

编辑 `config.json` 文件，填入你的认证信息：

```json
{
  "client_id": "你的client_id",
  "dsid": "你的dsid",
  "headers": {
    "Cookie": "你的完整Cookie字符串"
  }
}
```

### 2. 运行程序

```bash
# 如果是预编译版本
./icloud-hme

# 如果是从源码编译
./icloud-hme

# 或者直接运行源码
go run main.go
```

## 使用方法

程序启动后会显示交互式菜单：

```
======================================================================
  iCloud 隐藏邮箱管理工具
======================================================================
  [1] 查看邮箱列表
  [2] 创建新邮箱
  [3] 停用邮箱
  [4] 批量创建邮箱
  [5] 彻底删除停用的邮箱（不可恢复！）
  [6] 重新激活停用的邮箱
  [0] 退出
======================================================================
请选择操作 (0-6):
```

### 主要功能说明

| 功能 | 说明 | 用途 |
|------|------|------|
| 查看邮箱列表 | 显示所有已创建的隐藏邮箱及其状态 | 管理现有邮箱 |
| 创建新邮箱 | 创建单个隐藏邮箱地址 | 快速创建 |
| 停用邮箱 | 暂时停用邮箱（可恢复） | 临时禁用 |
| 批量创建邮箱 | 一次性创建多个邮箱 | 批量操作 |
| 彻底删除邮箱 | 永久删除已停用的邮箱 | 清理无用邮箱 |
| 重新激活邮箱 | 恢复已停用的邮箱 | 重新启用 |

## 配置说明

### 必需配置项

| 配置项 | 说明 | 示例 |
|--------|------|------|
| `base_url` | API 端点地址 | `https://pXXX-maildomainws.icloud.com/v1/hme/generate` |
| `client_build_number` | 客户端构建号 | `XXXX_BUILD_NUMBER` |
| `client_mastering_number` | 客户端主版本号 | `XXXX_BUILD_NUMBER` |
| `client_id` | 客户端唯一标识 | `xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx` |
| `dsid` | 用户 DSID | `YOUR_DSID_HERE` |
| `headers.Cookie` | 认证 Cookie | 从浏览器获取的完整 Cookie 字符串 |

### 可选配置项

| 配置项 | 说明 | 默认值 |
|--------|------|--------|
| `lang_code` | 语言代码 | `en-us` |
| `count` | 生成邮箱数量 | `5` |
| `delay_seconds` | 每次请求间隔（秒） | `2` |
| `output_file` | 输出文件路径 | `generated_emails.txt` |

## 如何获取认证信息

### 方法一：从浏览器获取

1. 打开浏览器，访问 [iCloud.com](https://www.icloud.com)
2. 登录你的 iCloud 账号
3. 打开浏览器开发者工具（F12）
4. 切换到 Network（网络）标签
5. 访问"隐藏我的邮件"功能，手动生成一个邮箱
6. 在网络请求中找到 `generate` 请求
7. 查看请求详情，复制以下信息：
   - URL 中的 `dsid` 参数
   - URL 中的 `clientId` 参数
   - 请求头中的 `Cookie`

### 方法二：使用 Cookie 导出工具

1. 安装浏览器扩展如 "Cookie-Editor" 或 "EditThisCookie"
2. 访问 iCloud.com 并登录
3. 使用扩展导出所有 Cookie
4. 将 Cookie 格式化为字符串形式

## 输出示例

```
开始批量生成 5 个隐藏邮件地址...

[1/5] 正在生成... [成功] 成功: wags-faded-5l@icloud.com
[2/5] 正在生成... [成功] 成功: blue-happy-7k@icloud.com
[3/5] 正在生成... [成功] 成功: red-sunny-3m@icloud.com
[4/5] 正在生成... [成功] 成功: green-cool-9n@icloud.com
[5/5] 正在生成... [成功] 成功: yellow-warm-2p@icloud.com

==================================================
生成完成！
成功: 5 个
失败: 0 个
==================================================

成功生成的邮箱地址:
1. wags-faded-5l@icloud.com
2. blue-happy-7k@icloud.com
3. red-sunny-3m@icloud.com
4. green-cool-9n@icloud.com
5. yellow-warm-2p@icloud.com

结果已保存到: generated_emails.txt
```

## 注意事项

**[重要提示]**

1. **Cookie 时效性**：iCloud 的认证 Cookie 会过期，通常有效期为几小时到几天。如果程序返回认证错误，需要重新获取 Cookie。

2. **请求频率**：建议设置合理的 `delay_seconds`（建议 2-5 秒），避免请求过快被 iCloud 限制。

3. **账号安全**：
   - 不要分享你的 `config.json` 文件，其中包含敏感的认证信息
   - 建议将 `config.json` 添加到 `.gitignore`
   - 定期更换密码以保护账号安全

4. **使用限制**：iCloud 可能对隐藏邮件地址的生成数量有限制，请合理使用。

## 故障排除

### 问题：返回 401 或 403 错误

**原因**：认证信息过期或无效

**解决方案**：
1. 重新从浏览器获取最新的 Cookie
2. 确认 `dsid` 和 `client_id` 是否正确
3. 检查是否在浏览器中能正常使用隐藏邮件功能

### 问题：返回 429 错误

**原因**：请求过于频繁

**解决方案**：
1. 增加 `delay_seconds` 的值（如设置为 5 或更高）
2. 减少单次生成的数量
3. 等待一段时间后再试

### 问题：无法解析响应

**原因**：API 返回格式变化或网络问题

**解决方案**：
1. 检查网络连接
2. 查看程序输出的原始响应内容
3. 确认 API 端点地址是否正确

## 项目结构

```
.
├── main.go                 # 主程序文件
├── config.json            # 配置文件（包含认证信息）
├── config.json.example    # 配置文件模板
├── generated_emails.txt   # 生成的邮箱地址（程序运行后生成）
├── cookies.txt           # Cookie 文件（可选）
└── README.md             # 说明文档
```

## 技术栈

<div align="center">

### 核心技术
![Go](https://img.shields.io/badge/Go-00ADD8?style=for-the-badge&logo=go&logoColor=white)
![HTTP](https://img.shields.io/badge/HTTP-Client-blue?style=for-the-badge)
![JSON](https://img.shields.io/badge/JSON-Config-orange?style=for-the-badge)

### 开发工具
![Git](https://img.shields.io/badge/Git-F05032?style=for-the-badge&logo=git&logoColor=white)
![VS Code](https://img.shields.io/badge/VS%20Code-007ACC?style=for-the-badge&logo=visual-studio-code&logoColor=white)
![Terminal](https://img.shields.io/badge/Terminal-4D4D4D?style=for-the-badge&logo=windows-terminal&logoColor=white)

</div>

### 依赖库

- **标准库**: `net/http`, `encoding/json`, `io`, `os`, `fmt`, `time`
- **第三方库**: 无（纯标准库实现）
- **Go 版本**: 1.19+

## 项目统计

<div align="center">

![GitHub repo size](https://img.shields.io/github/repo-size/yuzeguitarist/icloud-hme?style=for-the-badge)
![GitHub code size](https://img.shields.io/github/languages/code-size/yuzeguitarist/icloud-hme?style=for-the-badge)
![GitHub last commit](https://img.shields.io/github/last-commit/yuzeguitarist/icloud-hme?style=for-the-badge)

</div>

## 贡献

欢迎提交 Issue 和 Pull Request！

1. Fork 本仓库
2. 创建你的特性分支 (`git checkout -b feature/AmazingFeature`)
3. 提交你的修改 (`git commit -m 'Add some AmazingFeature'`)
4. 推送到分支 (`git push origin feature/AmazingFeature`)
5. 打开一个 Pull Request

## 许可证

本项目采用 MIT 许可证 - 查看 [LICENSE](LICENSE) 文件了解详情

## 免责声明

⚠️ **重要声明**

本工具仅供学习和研究使用。使用本工具时请遵守 Apple 的服务条款和相关法律法规。作者不对使用本工具造成的任何后果负责。

---

<div align="center">

**如果这个项目对你有帮助，请给个 ⭐ Star 支持一下！**

Made with ❤️ by [yuzeguitarist](https://github.com/yuzeguitarist)

</div>

