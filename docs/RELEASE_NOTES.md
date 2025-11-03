# 发布说明

## v2.1.0 · 2025-11-02

### 新增
- CLI 加载动画与进度条支持自动收尾，避免 goroutine 泄漏
- `replaceEndpoint` 通过 `net/url` 精准替换路径段，兼容 macOS/Linux/Windows 的 `config.json`
- 配置示例统一指向 `/v1/hme/reserve`，方便扩展到 `generate`、`list`、`deactivate` 等接口

### 修复
- 修正 `total <= 0` 时进度条可能产生除零的边界问题
- 读取命令行输入时输出明确错误提示，避免在管道或 EOF 场景下静默失败

### 体验
- 菜单交互增加多套快捷键（数字/字母/别名），确认操作支持中英文
- 彩色输出比例控制在约 60%，强调态信息，保持终端可读性

### 兼容性
- 在 macOS Terminal、iTerm2、GNOME Terminal、Windows Terminal、PowerShell 7、WSL 等终端完成视觉与功能验证

---

## 历史版本

### v2.0.0 · 2024-11-02
- 批量操作、完整邮箱生命周期管理
- 彩色命令行界面、进度条与状态提示
- 智能重试、错误处理、跨平台二进制发布

### v1.0.0 · 2024-05-01
- 基础隐藏邮箱生成
- JSON 配置支持
- 结果写入文件
