<div align="center">

# 🎬 video-compress 视频批量压缩工具

基于 **Wails + Vue3 + Go** 的 Windows 桌面视频批量压缩软件，
将视频批量转换为 **H.265 (HEVC)**，保留原始分辨率与帧率，支持 GPU 硬编加速、断点续传与实时进度。

[![Release](https://img.shields.io/github/v/release/orangbus/video-compress?style=flat-square&label=Release)](https://github.com/orangbus/video-compress/releases)
[![Go](https://img.shields.io/badge/Go-1.23-00ADD8?style=flat-square&logo=go)](https://go.dev/)
[![Wails](https://img.shields.io/badge/Wails-v2-DF4F4F?style=flat-square&logo=wails)](https://wails.io/zh-Hans/)
[![License](https://img.shields.io/badge/License-MIT-green?style=flat-square)](LICENSE)

**中文** | [English](./README_EN.md)（*待补充*）

</div>

---

## ✨ 功能特性

- 🖥️ **现代化无边框界面**：左侧导航（开始 / 转化 / 完成 / 说明），顶部自定义标题栏可拖拽，卡片式布局
- ⚙️ **环境自动检测**：CPU / 内存 / 显卡 / HEVC 硬编能力 / ffmpeg 状态一目了然
- 📥 **ffmpeg 一键自动下载**：缺失时自动下载到软件目录（gyan.dev / BtbN 双源兜底），无需手动配置
- 🎯 **模式智能推荐**：根据硬件自动推荐「CPU 软编 / GPU 硬编 / 更高画质」三种模式，并支持高级参数自定义（CRF / 预设 / 编码器 / 并行数 / 输出目录）
- 📂 **批量处理**：支持拖拽、多文件选择、整个文件夹递归扫描
- ⏱️ **实时进度**：每个文件独立进度条 + 总览统计，转化页实时刷新
- 🔁 **断点续传**：中途出错不中断整个批次，失败任务可一键重试，已完成文件自动跳过（跨会话保留记录）
- 📊 **完成统计**：文件名 / 转化时长 / 压缩比 / 文件路径 / 保存位置，一键打开输出目录
- 🚀 **CI 自动发布**：推送 Git tag 即自动构建并发布到 GitHub Releases

## 📷 截图

> 待补充：可将应用截图放入 `docs/screenshots/` 后在 README 中引用，例如
> `<img src="docs/screenshots/main.png" width="700" />`

## 🚀 快速开始

### 方式一：直接下载（推荐）

到 [Releases](https://github.com/orangbus/video-compress/releases) 下载最新版
`video_compress-<版本>-windows-amd64.zip`，解压后双击 `video_compress.exe` 即可使用。

> 首次使用若检测不到 ffmpeg，在「开始」页点击 **「自动下载 ffmpeg 到软件目录」** 即可。

### 方式二：从源码构建

环境要求：**Go 1.23+**、**Node 18+**、**Wails v2 CLI**、FFmpeg（可选，缺了也能自动下载）。

```powershell
# 1. 安装 Wails CLI
go install github.com/wailsapp/wails/v2/cmd/wails@latest

# 2. 克隆并构建
git clone git@github.com:orangbus/video-compress.git
cd video-compress
wails build

# 产物：build\bin\video_compress.exe
```

## 📖 使用说明

### 图形界面（推荐）

1. **开始**：查看环境检测 → 选择模式（或展开高级选项自定义）→ 拖入/选择视频 → 设置并行数 → 开始压缩
2. **转化**：查看每个文件的实时进度；出错不中断，失败可「重试失败」，可「停止压缩」
3. **完成**：查看转化时长、压缩比、保存位置，一键「打开位置」
4. **说明**：内置完整使用文档与常见问题

### 命令行模式

```powershell
# CPU 软编（画质最好、文件最小），默认 fast 预设，4 个任务并行
video_compress.exe -dir "D:\视频" -concurrency 4

# GPU 硬编（最快，推荐日常用，需 NVIDIA GTX 10 系以上）
video_compress.exe -dir "D:\视频" -encoder nvenc -concurrency 4

# 更高画质（CRF 数值调小）
video_compress.exe -dir "D:\视频" -crf 16 -encoder nvenc

# 只压缩指定格式
video_compress.exe -dir "D:\视频" -ext mp4,mkv

# 预览将要执行的命令，不真正压缩
video_compress.exe -dir "D:\视频" -dry-run
```

| 参数 | 默认值 | 说明 |
|---|---|---|
| `-dir` | `.` | 要压缩的视频所在目录（递归） |
| `-out` | `<dir>\compressed` | 输出目录（自动创建，保留目录结构） |
| `-crf` | `18` | 画质参数，越小越清晰、文件越大（建议 18~23） |
| `-preset` | `fast` | 仅软编生效：`ultrafast / fast / medium / slow / veryslow` |
| `-encoder` | `libx265` | `libx265`（CPU 软编）或 `nvenc`（GPU 硬编） |
| `-concurrency` | `2` | 并行压缩任务数 |
| `-ext` | 常见视频格式 | 处理的扩展名，逗号分隔 |
| `-dry-run` | `false` | 只打印命令，不真正压缩 |

> 输出为 mp4/mov 时自动加 `hvc1` 标签，兼容 iPhone / QuickTime 播放器；音频流直接复制不重编码。

## ⚙️ GitHub Action 自动发布

仓库已内置发布工作流 `.github/workflows/release.yml`：

- **触发条件**：推送形如 `v*` 的 Git tag（如 `v0.1.0`、`v1.0.0`）
- **流程**：Windows 环境构建 → 打包 zip → 自动创建 GitHub Release 并上传产物
- **手动触发**：也可在 Actions 页面点击 `Run workflow` 手动触发

```bash
# 发布一个新版本
git tag v0.1.0
git push origin v0.1.0
```

## 🛠️ 技术栈

- **桌面框架**：[Wails v2](https://wails.io/zh-Hans/)（Go + WebView2）
- **前端**：Vue 3 + Vite（原生 JS，无 UI 框架依赖）
- **后端**：Go（并发任务引擎、断点续传、环境检测、ffmpeg 自动下载）
- **媒体处理**：FFmpeg（libx265 软编 / hevc_nvenc 硬编）

## 📁 目录结构

```
├── main.go / app.go          # Wails 入口与后端绑定
├── internal/video/           # 共享压缩引擎
│   ├── env.go                # 环境检测（CPU/内存/显卡/ffmpeg）
│   ├── download.go           # ffmpeg 自动下载
│   ├── engine.go             # 并发任务引擎 + 断点续传
│   ├── cmd.go                # ffmpeg 命令构造与进度解析
│   └── *_test.go             # 单元测试
├── cmd/cli/                  # 命令行版入口
└── frontend/                 # Vue3 界面
```

## 📝 License

[MIT](LICENSE) © OrangBus
