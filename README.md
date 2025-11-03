<div align="center">

# iCloud 隐藏邮件地址管理工具

<p align="center">
  <img src="https://img.shields.io/badge/Go-1.19+-00ADD8?style=for-the-badge&logo=go&logoColor=white" alt="Go Version">
  <img src="https://img.shields.io/badge/Platform-macOS%20%7C%20Linux%20%7C%20Windows-lightgrey?style=for-the-badge" alt="Platform">
  <img src="https://img.shields.io/badge/License-MIT-green?style=for-the-badge" alt="License">
  <img src="https://img.shields.io/github/stars/yuzeguitarist/icloud-unlimitedemail-go?style=for-the-badge" alt="Stars">
</p>

<p align="center">
  一款使用 Go 开发的 CLI 工具，帮助你在 macOS、Linux、Windows 下批量创建与管理 iCloud “隐藏我的邮箱”（Hide My Email）。
</p>

</div>

---

## 目录

- [功能亮点](#功能亮点)
- [快速开始](#快速开始)
- [配置要点](#配置要点)
- [文档](#文档)
- [安装与构建](#安装与构建)
- [CLI 体验](#cli-体验)
- [常见问题](#常见问题)
- [项目结构](#项目结构)
- [贡献](#贡献)
- [许可证](#许可证)

## 功能亮点

- **完整生命周期**：生成 → 确认 → 列表 → 停用 → 删除 → 重新激活
- **智能邮箱评分**：基于前缀结构、长度、可读性、安全性的多维度评分算法
- **配置热重载**：运行时自动检测配置文件变化，支持错误重试和安全退出
- **批量自动化**：支持批量创建，每个任务可设置标签前缀与请求间隔
- **邮箱保存功能**：自动保存生成的邮箱到文件，支持时间戳记录
- **开发者模式**：可选的调试功能，包含评分算法测试
- **人性化交互**：数字与字母快捷键并存，确认操作支持中英文
- **终端视觉优化**：60% 以内的着色占比，渐变进度条与彩虹 Spinner
- **跨平台验证**：重点在 macOS Terminal、iTerm2 以及 Linux/Windows 常见终端完成适配

## 快速开始

```bash
git clone https://github.com/yuzeguitarist/icloud-unlimitedemail-go.git
cd icloud-unlimitedemail-go
cp config.json.example config.json
go build -o icloud-hme main.go
./icloud-hme
```

> macOS 用户推荐在 Terminal.app / iTerm2 中配合 SF Mono 等等宽字体使用，界面表现最佳。

## 配置要点

`config.json` 示例：

```json
{
  "base_url": "https://pXXX-maildomainws.icloud.com/v1/hme/reserve",
  "client_build_number": "XXXX_BUILD_NUMBER",
  "client_mastering_number": "XXXX_BUILD_NUMBER",
  "client_id": "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx",
  "dsid": "YOUR_DSID_HERE",
  "headers": {
    "Cookie": "你的完整 iCloud Cookie",
    "User-Agent": "浏览器 User-Agent"
  },
  "lang_code": "en-us",
  "count": 5,
  "delay_seconds": 2,
  "output_file": "generated_emails.txt",
  "email_quality": {
    "auto_select": false,
    "min_score": 70,
    "max_regenerate_count": 3,
    "show_scores": true,
    "allow_manual": true,
    "weights": {
      "prefix_structure": 40,
      "length": 20,
      "readability": 25,
      "security": 15
    }
  },
  "save_generated_emails": false,
  "email_list_file": "generated_emails.txt",
  "developer_mode": false
}
```

- **请保留 `/v1/hme/reserve` 作为基准路径**，程序会在内部构造 `generate`、`list`、`deactivate`、`delete`、`reactivate` 等接口。
- `client_id`、`dsid`、`client_build_number`、`client_mastering_number` 均来自浏览器抓包所得的查询参数。
- `headers.Cookie` 必须为完整 Cookie，优先使用近期的登录会话（macOS Safari/Chrome 均可）。

详细方法可参考 [`docs/使用指南.md`](docs/%E4%BD%BF%E7%94%A8%E6%8C%87%E5%8D%97.md)。

## 文档

- 使用指南：[`docs/使用指南.md`](docs/%E4%BD%BF%E7%94%A8%E6%8C%87%E5%8D%97.md)
- 发布说明：[`docs/RELEASE_NOTES.md`](docs/RELEASE_NOTES.md)

## 安装与构建

| 场景 | 命令 |
| --- | --- |
| 下载预编译包 | 访问 [Releases](https://github.com/yuzeguitarist/icloud-unlimitedemail-go/releases) 获取对应平台压缩包 |
| 手动编译（当前平台） | `go build -o icloud-hme main.go` |
| 交叉编译（示例：Linux x64） | `GOOS=linux GOARCH=amd64 go build -o icloud-hme-linux-amd64 main.go` |
| 构建脚本 | `./build.sh release` / `./build.sh local` |
| Makefile | `make build` / `make build-all` / `make release` / `make clean` |

## CLI 体验

```
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  iCloud 隐藏邮箱管理工具
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

  [1] 查看邮箱列表
  [2] 创建新邮箱
  [3] 停用邮箱
  [4] 批量创建邮箱
  [5] 彻底删除停用的邮箱 (不可恢复)
  [6] 重新激活停用的邮箱
  [0] 退出 (或输入 q/quit/exit)

──────────────────────────────────────────────

  › 选择操作 (0-6):
```

- 进度条根据百分比自动切换红 → 黄 → 绿
- 彩虹色 Spinner 保持界面灵动，同时在任务结束时自动清理
- 多种快捷键：`1/l/list`、`0/q/quit/exit/e`、确认支持 `y/yes/是`

## 常见问题

| 问题 | 可能原因 | 解决方案 |
| --- | --- | --- |
| 401 / 403 | Cookie 过期或参数错误 | 重新抓取 Cookie，确认 `client_id`、`dsid`、`base_url` 保持一致 |
| 429 Too Many Requests | 请求过快 | 提高 `delay_seconds`，减少批量数量，稍后重试 |
| 无法解析响应 | API 返回结构变化或网络异常 | 记录原始响应，检查 `base_url` 是否仍指向 `/v1/hme/reserve` |
| 终端渲染异常 | 字体或编码不匹配 | 使用 UTF-8，macOS 建议启用 SF Mono；Windows 建议使用 Windows Terminal |

## 项目结构

```
.
├── main.go
├── config.json.example
├── docs/
│   ├── RELEASE_NOTES.md
│   └── 使用指南.md
├── build.sh / install.sh / Makefile
├── README.md
└── LICENSE
```

## 贡献

欢迎提交 Issue 与 Pull Request：

1. Fork 本仓库
2. `git checkout -b feature/your-feature`
3. 完成修改并运行 `go build ./...`
4. `git commit -m "feat: introduce your feature"`
5. `git push origin feature/your-feature`
6. 创建 Pull Request 并说明动机与验证方式

## 许可证

项目遵循 [MIT License](LICENSE)。

---

<div align="center">

![GitHub repo size](https://img.shields.io/github/repo-size/yuzeguitarist/icloud-unlimitedemail-go?style=for-the-badge)
![GitHub code size](https://img.shields.io/github/languages/code-size/yuzeguitarist/icloud-unlimitedemail-go?style=for-the-badge)
![GitHub last commit](https://img.shields.io/github/last-commit/yuzeguitarist/icloud-unlimitedemail-go?style=for-the-badge)

如果这个项目对你有帮助，欢迎点一个 ⭐️ 支持！

由 [@yuzeguitarist](https://github.com/yuzeguitarist) 维护

</div>

