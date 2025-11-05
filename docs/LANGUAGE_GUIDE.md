# 语言设置使用指南 / Language Settings Guide

## 🌍 两种设置方式 / Two Configuration Methods

### 方法一：配置文件 / Method 1: Config File

编辑 `config.json` 文件：
```json
{
  "language": "en"
}
```

支持的值 / Supported values: `zh`, `en`, `de`

### 方法二：软件菜单 ⭐ 推荐 / Method 2: Software Menu ⭐ Recommended

```
主菜单 Main Menu
  ↓
[8] 程序设置 / Program Settings
  ↓
[4] 语言设置 / Language Settings
  ↓
选择语言 / Select Language:
  [1] 中文 (Chinese)
  [2] English
  [3] Deutsch (German)
```

## ✨ 特性 / Features

- ✅ **实时切换** - 无需重启程序 / Real-time switching - No restart required
- ✅ **自动保存** - 切换后自动保存到配置文件 / Auto-save - Automatically saves to config
- ✅ **即时生效** - 返回菜单后立即看到新语言 / Immediate effect - See new language after returning to menu
- ✅ **三语支持** - 中文、英语、德语 / Trilingual support - Chinese, English, German

## 📝 示例 / Example

### 中文 → English
```
[8] 程序设置

当前配置

  [1] 邮箱质量设置
  [2] 邮箱保存设置
  [3] 开发者模式: 禁用
  [4] 语言设置          ← 选择这里 / Select this
  [0] 返回主菜单
```

```
[4] 语言设置

当前语言: 中文 (Chinese)

请选择语言 / Select Language / Sprache wählen

  [1] 中文 (Chinese)
  [2] English             ← 选择 English
  [3] Deutsch (German)
  [0] 返回上级菜单

选择 (0-3): 2
```

```
✓ 语言已切换为: English
ℹ 提示：部分文本需要返回主菜单后生效

按回车键继续...
```

### 返回主菜单后 / After returning to main menu
```
[8] Program Settings

Current Configuration

  [1] Email Quality Settings
  [2] Email Save Settings
  [3] Developer Mode: Disabled
  [4] Language Settings      ← 已切换为英语 / Switched to English
  [0] Return to Main Menu
```

## 🔄 语言代码对照表 / Language Code Reference

| 语言 Language | 主要代码 Primary | 其他可用代码 Alternatives |
|--------------|----------------|------------------------|
| 中文 | `zh` | `zh-CN`, `zh-TW`, `chinese` |
| English | `en` | `en-US`, `en-GB`, `english` |
| Deutsch | `de` | `de-DE`, `german`, `deutsch` |

## 💡 提示 / Tips

1. **推荐使用菜单方式** - 更直观，即时生效
   **Recommended to use menu** - More intuitive, immediate effect

2. **配置文件方式** - 适合批量部署或脚本配置
   **Config file method** - Suitable for batch deployment or script configuration

3. **语言持久化** - 设置后永久保存，下次启动自动使用
   **Language persistence** - Settings saved permanently, automatically used on next start

4. **多环境支持** - 不同环境可使用不同语言的配置文件
   **Multi-environment support** - Different environments can use different language configs

## 🎯 快速切换 / Quick Switch

中文用户切换到英语 / Chinese users switch to English:
```
8 → 4 → 2 → Enter
```

English users switch to Chinese:
```
8 → 4 → 1 → Enter
```

Deutsche Benutzer wechseln zu Englisch / German users switch to English:
```
8 → 4 → 2 → Enter
```

---

## 📚 更多信息 / More Information

详细文档：[i18n.md](./i18n.md)
Detailed documentation: [i18n.md](./i18n.md)
