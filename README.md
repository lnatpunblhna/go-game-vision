# Go Game Vision

<div align="center">

[English](README_EN.md) | 中文

一个功能强大的跨平台 Go 自动化框架，为 Windows 和 macOS 提供进程管理、屏幕截图、图像识别和智能鼠标模拟。

[![Go Version](https://img.shields.io/badge/Go-%3E%3D1.23-blue)](https://go.dev/)
[![License](https://img.shields.io/badge/license-MIT-green)](LICENSE)
[![Platform](https://img.shields.io/badge/platform-Windows%20%7C%20macOS-lightgrey)](https://github.com/lnatpunblhna/go-game-vision)

</div>

---

## ✨ 核心特性

### 🔍 进程管理模块
- ✅ 根据程序名称快速获取进程 PID
- ✅ 支持模糊匹配和精确匹配两种模式
- ✅ 智能处理多个同名进程
- ✅ 跨平台兼容（Windows/macOS）

### 📸 屏幕截图模块
- ✅ **Windows**: 使用 BitBlt/PrintWindow API，即使窗口被遮挡也能截图
- ✅ **macOS**: 使用 Core Graphics API，支持特定窗口截图
- ✅ 自动处理多进程应用（Chrome、Safari 等）
- ✅ 支持多种格式（PNG、JPEG、BMP、GIF）
- ✅ 完整的窗口信息（位置、大小、状态）

### 🖼️ 图像识别模块
- ✅ **多尺度模板匹配** - 自动适应窗口缩放和 DPI 变化
- ✅ **智能坐标系统** - 窗口相对坐标 ⇄ 屏幕绝对坐标自动转换
- ✅ **一站式匹配点击** - 图像识别后直接执行点击
- ✅ 多种对比算法：
  - 模板匹配（Template Matching）
  - 特征点匹配（Feature Matching）
  - 直方图对比（Histogram Comparison）
  - 结构相似性（Structural Similarity）
  - **多尺度模板匹配（Multi-Scale Template Matching）**⭐

### 🖱️ 智能鼠标模拟模块
- ✅ **抗反作弊点击** - 通过硬件输入队列，难以被游戏检测
- ✅ **真正后台点击** - PostMessage/SendMessage 方式
- ✅ **随机延迟** - 模拟真人操作（5-15ms 随机延迟）
- ✅ **焦点自动恢复** - 点击后自动恢复原窗口焦点
- ✅ **子窗口智能查找** - 自动定位实际渲染窗口
- ✅ 支持左键、右键、中键点击
- ✅ 屏幕坐标验证和边界检查

---

## 📦 安装

### 前置要求
- **Go**: 1.23 或更高版本
- **系统**: Windows 10+ 或 macOS 10.14+
- **OpenCV**: 图像处理功能依赖

### 快速安装

```bash
go get github.com/lnatpunblhna/go-game-vision
```

### OpenCV 安装

#### Windows
```bash
# 方法 1: 使用 vcpkg
vcpkg install opencv4

# 方法 2: 下载预编译版本
# 访问 https://github.com/hybridgroup/gocv#windows
```

#### macOS
```bash
brew install opencv
```

---

## 🚀 快速开始

### 基础示例 - 窗口截图

```go
package main

import (
    "log"
    "github.com/lnatpunblhna/go-game-vision/pkg/capture"
    "github.com/lnatpunblhna/go-game-vision/pkg/process"
)

func main() {
    // 1. 查找进程
    pid, err := process.GetProcessPIDByName("notepad.exe", process.ExactMatch)
    if err != nil {
        log.Fatal(err)
    }

    // 2. 截图并保存
    err = capture.CaptureAndSave(pid, "window.png", capture.PNG, 90)
    if err != nil {
        log.Fatal(err)
    }

    log.Println("截图成功!")
}
```

### 进阶示例 - 多尺度图像匹配与智能点击

```go
package main

import (
    "log"
    "github.com/lnatpunblhna/go-game-vision/pkg/capture"
    "github.com/lnatpunblhna/go-game-vision/pkg/image"
    "github.com/lnatpunblhna/go-game-vision/pkg/mouse"
    "github.com/lnatpunblhna/go-game-vision/pkg/process"
)

func main() {
    // 1. 查找游戏进程
    pid, _ := process.GetProcessPIDByName("game.exe", process.ExactMatch)

    // 2. 获取窗口信息
    windowInfo, _ := capture.GetWindowInfoByPID(pid)

    // 3. 截取游戏窗口
    windowImage, _ := capture.CaptureWindowByPID(pid, nil)

    // 4. 加载要查找的按钮图片
    buttonTemplate, _ := image.LoadImage("button.png")

    // 5. 多尺度匹配（自动适应窗口缩放）
    config := &image.MultiScaleConfig{
        MinScale:   0.7,  // 最小 70%
        MaxScale:   1.3,  // 最大 130%
        ScaleStep:  0.05, // 步长 5%
        Threshold:  0.75, // 相似度阈值 75%
        MaxResults: 5,
    }

    result, err := image.MultiScaleTemplateMatch(windowImage, buttonTemplate, config)
    if err != nil {
        log.Fatal("未找到按钮:", err)
    }

    log.Printf("找到按钮! 相似度: %.2f%%", result.Similarity*100)

    // 6. 抗反作弊智能点击
    screenCoords := result.ToScreenCoordinates(windowInfo)
    clicker := mouse.NewMouseClicker()

    clickOptions := &mouse.ClickOptions{
        Button:       mouse.LeftButton,
        Delay:        50,
        RandomDelay:  true,  // 添加随机延迟，模拟真人
        RestoreFocus: true,  // 点击后恢复焦点
    }

    err = clicker.BackgroundClick(screenCoords.X, screenCoords.Y, clickOptions)
    if err != nil {
        log.Fatal("点击失败:", err)
    }

    log.Println("✓ 点击成功!")
}
```

---

## 📖 详细用法

### 进程管理

```go
// 精确匹配
pid, err := process.GetProcessPIDByName("notepad.exe", process.ExactMatch)

// 模糊匹配
pid, err := process.GetProcessPIDByName("note", process.FuzzyMatch)

// 获取所有匹配的进程
pm := process.NewProcessManager()
procs, err := pm.GetProcessByName("chrome", process.FuzzyMatch)
for _, proc := range procs {
    fmt.Printf("PID: %d, Name: %s\n", proc.PID, proc.Name)
}
```

### 窗口截图

```go
// 方式 1: 便捷函数
err := capture.CaptureAndSave(pid, "output.png", capture.PNG, 90)

// 方式 2: 完整控制
capturer := capture.NewScreenCapture()
img, err := capturer.CaptureWindowByPID(pid, nil)

// 获取窗口信息
windowInfo, err := capture.GetWindowInfoByPID(pid)
fmt.Printf("窗口: %s, 大小: %dx%d\n",
    windowInfo.Title,
    windowInfo.Rect.Dx(),
    windowInfo.Rect.Dy())
```

### 图像匹配

#### 基础模板匹配
```go
comparer := image.NewImageComparer(image.TemplateMatching)
result, err := comparer.CompareImages(sourceImage, templateImage)

fmt.Printf("相似度: %.2f\n", result.Similarity)
fmt.Printf("窗口坐标: (%d, %d)\n", result.Location.X, result.Location.Y)
```

#### 多尺度匹配（推荐）⭐
```go
config := &image.MultiScaleConfig{
    MinScale:   0.7,   // 最小缩放 70%
    MaxScale:   1.3,   // 最大缩放 130%
    ScaleStep:  0.05,  // 步长 5%
    Threshold:  0.75,  // 阈值 75%
    MaxResults: 5,     // 返回前 5 个结果
}

result, err := image.MultiScaleTemplateMatch(sourceImage, templateImage, config)

// 查找所有匹配项
results, err := image.MultiScaleTemplateMatchAll(sourceImage, templateImage, config)
for i, r := range results {
    fmt.Printf("[%d] 相似度: %.2f, 缩放: %.0f%%\n",
        i+1, r.Similarity*100, r.Scale*100)
}
```

#### 坐标转换
```go
// 窗口相对坐标 → 屏幕绝对坐标
screenCoords := result.ToScreenCoordinates(windowInfo)

// 边界框转换
screenBBox := result.ToScreenBoundingBox(windowInfo)
```

### 智能点击

#### 方式 1: 抗反作弊点击（推荐用于游戏）⭐
```go
clicker := mouse.NewMouseClicker()

options := &mouse.ClickOptions{
    Button:       mouse.LeftButton,
    Delay:        50,              // 基础延迟 50ms
    RandomDelay:  true,            // 添加 5-15ms 随机延迟
    RestoreFocus: true,            // 自动恢复原窗口焦点
}

// 使用 SendInput（通过硬件输入队列，难以检测）
err := clicker.BackgroundClick(x, y, options)
```

#### 方式 2: 真正后台点击（可能被反作弊检测）
```go
// PostMessage 方式（不激活窗口，但容易被检测）
err := clicker.PostMessageClick(windowInfo.Handle, x, y, options)

// 自动查找子窗口并点击
if windowsClicker, ok := clicker.(*mouse.WindowsMouseClicker); ok {
    err = windowsClicker.PostMessageClickAtScreenCoords(
        windowInfo.Handle, screenX, screenY, options)
}
```

#### 方式 3: 一站式匹配并点击
```go
// 图像匹配后直接点击
result, err := image.FindAndLeftClick(
    sourceImage,
    templateImage,
    windowInfo,
    image.TemplateMatching)
```

---

## 🎯 使用场景

### 游戏辅助自动化
- ✅ 自动点击游戏按钮
- ✅ 自动领取奖励
- ✅ 自动战斗/升级
- ✅ 抗反作弊检测

### UI 自动化测试
- ✅ 桌面应用自动化测试
- ✅ 跨平台 UI 测试
- ✅ 截图对比测试

### 办公自动化
- ✅ 批量处理窗口
- ✅ 自动化点击流程
- ✅ 窗口监控

---

## 🛡️ 反作弊策略

### 点击方式对比

| 方式 | 原理 | 窗口激活 | 检测难度 | 成功率 | 适用场景 |
|------|------|----------|----------|--------|----------|
| **SendInput + 随机延迟** ⭐ | 硬件输入队列 | ✅ 短暂激活 | 🟢 难检测 | 🟢 高 | 大多数游戏 |
| **PostMessage/SendMessage** | 窗口消息 | ❌ 不激活 | 🔴 易检测 | 🔴 低 | 简单应用 |
| **SendInput（无延迟）** | 硬件输入队列 | ✅ 短暂激活 | 🟡 中等 | 🟡 中 | 一般应用 |

### 推荐配置

#### 绕过反作弊（推荐）⭐
```go
options := &mouse.ClickOptions{
    Button:       mouse.LeftButton,
    Delay:        50,
    RandomDelay:  true,   // ✅ 随机延迟 5-15ms
    RestoreFocus: true,   // ✅ 自动恢复焦点
}
```

#### 完全后台（可能被检测）
```go
options := &mouse.ClickOptions{
    Button: mouse.LeftButton,
    Delay:  50,
    RandomDelay:  false,  // ❌ 无随机延迟
    RestoreFocus: false,  // ❌ 不恢复焦点
}
```

### 进阶技巧
1. **变化点击间隔** - 每次点击间隔不同
2. **添加微小偏移** - 点击位置 ±2 像素随机
3. **使用多尺度匹配** - 适应窗口缩放
4. **随机延迟** - 模拟人类操作的不确定性

---

## 📁 项目结构

```
go-game-vision/
├── pkg/                        # 核心库代码
│   ├── capture/               # 屏幕截图模块
│   │   ├── capture.go         # 跨平台接口
│   │   ├── capture_windows.go # Windows 实现
│   │   └── capture_darwin.go  # macOS 实现
│   ├── image/                 # 图像处理模块
│   │   └── compare.go         # 图像对比、多尺度匹配
│   ├── mouse/                 # 鼠标模拟模块
│   │   ├── mouse.go           # 跨平台接口
│   │   ├── mouse_windows.go   # Windows 实现（SendInput/PostMessage）
│   │   └── mouse_darwin.go    # macOS 实现
│   ├── process/               # 进程管理模块
│   │   ├── process.go         # 跨平台接口
│   │   ├── process_windows.go # Windows 实现
│   │   └── process_darwin.go  # macOS 实现
│   └── utils/                 # 工具模块
│       ├── logger.go          # 日志系统
│       └── errors.go          # 错误处理
├── tests/                     # 测试文件
│   ├── capture_test.go        # 截图测试
│   ├── image_compare_test.go  # 图像对比测试
│   ├── mouse_test.go          # 鼠标模拟测试
│   ├── process_test.go        # 进程管理测试
│   └── nikke_click_test.go    # 集成测试示例
├── go.mod                     # Go 模块配置
├── go.sum                     # 依赖锁定
├── LICENSE                    # MIT 许可证
├── README.md                  # 中文文档
└── README_EN.md               # 英文文档
```

---

## 🧪 运行测试

```bash
# 运行所有测试（需要 OpenCV）
go test -v ./...

# 运行单元测试（不依赖 OpenCV）
go test -v -short ./pkg/process/... ./pkg/utils/...

# 运行特定测试
go test -v ./tests/ -run TestProcessManager

# 禁用测试缓存
go test -v -count=1 ./tests/...
```

---

## ⚠️ 注意事项

### Windows 平台
- ✅ 某些系统进程需要管理员权限
- ✅ PrintWindow API 可截取被遮挡窗口
- ✅ 支持 DPI 缩放
- ✅ SendInput 通过硬件输入队列，更难检测

### macOS 平台
- ✅ 需要屏幕录制权限（系统偏好设置 → 安全性与隐私）
- ✅ 某些系统进程可能无法截取
- ✅ 使用 Core Graphics API

### 性能优化
- 🔸 大量截图时复用 `ScreenCapture` 实例
- 🔸 图像对比性能取决于图片大小和算法
- 🔸 推荐使用多尺度匹配而非多次单尺度匹配
- 🔸 点击操作建议添加延迟（50-100ms）

---

## 🤝 贡献

欢迎贡献代码、报告问题或提出建议！

1. Fork 本仓库
2. 创建功能分支 (`git checkout -b feature/AmazingFeature`)
3. 提交更改 (`git commit -m 'Add some AmazingFeature'`)
4. 推送到分支 (`git push origin feature/AmazingFeature`)
5. 创建 Pull Request

详见 [CONTRIBUTING.md](CONTRIBUTING.md)（待添加）

---

## 📄 许可证

本项目采用 MIT 许可证 - 详见 [LICENSE](LICENSE) 文件

---

## 🌟 Star History

如果这个项目对你有帮助，请给个 Star ⭐！

---

## 📮 联系方式

- **Issues**: [GitHub Issues](https://github.com/lnatpunblhna/go-game-vision/issues)
- **Discussions**: [GitHub Discussions](https://github.com/lnatpunblhna/go-game-vision/discussions)

---

<div align="center">

**[⬆ 回到顶部](#go-game-vision)**

Made with ❤️ by Go Game Vision Contributors

</div>
