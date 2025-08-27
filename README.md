# Go Game Vision

[English](README_EN.md) | 中文

一个跨平台的Go工具框架，为Windows和macOS提供进程管理、屏幕截图、图像处理和鼠标模拟等功能模块。专为其他项目或程序调用而设计。

## 功能特性

### 🔍 进程管理模块
- 根据程序名称获取进程PID
- 支持模糊匹配和精确匹配两种模式
- 处理多个同名进程的情况
- 跨平台兼容（Windows/macOS）

### 📸 屏幕截图模块
- **Windows平台**: 使用Windows API（BitBlt/PrintWindow）实现窗口截图
- **macOS平台**: 使用Core Graphics API和screencapture命令实现真正的窗口截图
- **关键特性**: 
  - Windows下即使窗口被其他窗口遮挡也能正常截图
  - macOS下能够根据进程ID获取特定窗口并截图（即使被遮挡）
  - 自动处理多进程应用程序（如Safari、Chrome等）
- 支持多种图片格式输出（PNG、JPEG、BMP、GIF）
- 提供窗口信息获取功能（位置、大小、状态等）
- 提供便捷的截图和保存方法

### 🖼️ 图像处理模块
- 集成GoCV库实现图片对比功能
- 提供图片相似度计算方法
- 支持多种对比算法：
  - 模板匹配（Template Matching）
  - 特征点匹配（Feature Matching）
  - 直方图对比（Histogram Comparison）
  - 结构相似性（Structural Similarity）

### 🖱️ 鼠标模拟模块
- 跨平台后台鼠标点击功能
- 支持左键、右键、中键点击
- 不移动鼠标光标的后台点击
- 屏幕坐标验证和边界检查
- 可配置的点击延迟设置

## 系统要求

### 基础要求
- Go 1.19 或更高版本
- Windows 10+ 或 macOS 10.14+

### 依赖库
- [GoCV](https://gocv.io/) - OpenCV的Go绑定（用于图像处理）
- golang.org/x/sys - 系统调用支持

### 外部依赖
- **OpenCV**: 图像处理功能需要

## 安装指南

### 1. 克隆项目
```bash
git clone https://github.com/lnatpunblhna/go-game-vision.git
cd go-game-vision
```

### 2. 安装Go依赖
```bash
go mod tidy
```

### 3. 安装外部依赖

#### Windows
```bash
# 安装OpenCV (使用vcpkg或预编译版本)
```

#### macOS
```bash
# 使用Homebrew安装
brew install opencv
```

## 快速开始

### 编程接口使用

```go
package main

import (
    "fmt"
    "github.com/lnatpunblhna/go-game-vision/pkg/capture"
    "github.com/lnatpunblhna/go-game-vision/pkg/process"
    "github.com/lnatpunblhna/go-game-vision/pkg/image"
    "github.com/lnatpunblhna/go-game-vision/pkg/mouse"
)

func main() {
    // 1. 进程管理
    pid, err := process.GetProcessPIDByName("notepad", process.FuzzyMatch)
    if err != nil {
        panic(err)
    }
    fmt.Printf("找到记事本进程，PID: %d\n", pid)

    // 2. 获取窗口信息
    windowInfo, err := capture.GetWindowInfoByPID(pid)
    if err != nil {
        panic(err)
    }
    fmt.Printf("窗口大小: %dx%d\n", windowInfo.Rect.Dx(), windowInfo.Rect.Dy())

    // 3. 截取窗口（即使被遮挡）
    img, err := capture.CaptureWindowByPID(pid, capture.DefaultCaptureOptions())
    if err != nil {
        panic(err)
    }

    // 4. 保存截图
    err = capture.CaptureAndSave(pid, "window_capture.png", capture.PNG, 90)
    if err != nil {
        panic(err)
    }

    // 5. 图像对比
    img1, _ := image.LoadImage("image1.png")
    img2, _ := image.LoadImage("image2.png")
    similarity, err := image.CalculateSimilarity(img1, img2)
    if err != nil {
        panic(err)
    }
    fmt.Printf("图像相似度: %.2f\n", similarity)

    // 6. 鼠标模拟点击（在窗口坐标系内）
    clickX := windowInfo.Rect.Min.X + 100 // 窗口内相对位置
    clickY := windowInfo.Rect.Min.Y + 100
    err = mouse.BackgroundLeftClick(clickX, clickY)
    if err != nil {
        panic(err)
    }
    fmt.Println("后台点击完成")
}
```

## API文档

### 进程管理 (pkg/process)

```go
// 获取进程PID
pid, err := process.GetProcessPIDByName("程序名", process.ExactMatch)

// 获取所有匹配的PID
pids, err := process.GetAllProcessPIDsByName("程序名", process.FuzzyMatch)

// 创建进程管理器
manager := process.NewProcessManager()
processes, err := manager.ListAllProcesses()
```

### 窗口截图 (pkg/capture)

```go
// 创建截图器
capturer := capture.NewScreenCapture()

// 截取特定进程的窗口（即使被遮挡）
options := capture.DefaultCaptureOptions()
img, err := capturer.CaptureWindowByPID(pid, options)

// 获取窗口信息
windowInfo, err := capturer.GetWindowInfoByPID(pid)
fmt.Printf("窗口位置: (%d, %d), 大小: %dx%d\n", 
    windowInfo.Rect.Min.X, windowInfo.Rect.Min.Y,
    windowInfo.Rect.Dx(), windowInfo.Rect.Dy())

// 保存图片
err = capturer.SaveImage(img, "output.png", capture.PNG, 90)

// 便捷函数：直接截图并保存
err = capture.CaptureAndSave(pid, "window.png", capture.PNG, 90)

// 便捷函数：获取窗口信息
windowInfo, err := capture.GetWindowInfoByPID(pid)
```

### 图像处理 (pkg/image)

```go
// 创建图像对比器
comparer := image.NewImageComparer(image.TemplateMatching)

// 对比图像
result, err := comparer.CompareImages(img1, img2)
fmt.Printf("相似度: %.2f, 位置: (%d, %d)\n", 
    result.Similarity, result.Location.X, result.Location.Y)

// 加载图像文件
img, err := image.LoadImage("example.png")

// 便捷函数计算相似度
similarity, err := image.CalculateSimilarity(img1, img2)
```

### 鼠标模拟 (pkg/mouse)

```go
// 创建鼠标控制器
clicker := mouse.NewMouseClicker()

// 后台点击（不移动光标）
options := mouse.DefaultClickOptions()
err := clicker.BackgroundClick(100, 100, options)

// 便捷函数
err = mouse.BackgroundLeftClick(100, 100)    // 左键点击
err = mouse.BackgroundRightClick(100, 100)   // 右键点击
err = mouse.BackgroundMiddleClick(100, 100)  // 中键点击

// 坐标验证
err = mouse.ValidateCoordinates(100, 100)

// 获取屏幕大小
width, height, err := clicker.GetScreenSize()
```



## 项目结构

```
go-game-vision/
├── pkg/                    # 核心包
│   ├── process/           # 进程管理
│   │   ├── process.go     # 跨平台接口
│   │   ├── process_windows.go  # Windows实现
│   │   └── process_darwin.go   # macOS实现
│   ├── capture/           # 屏幕截图
│   │   ├── capture.go     # 跨平台接口
│   │   ├── capture_windows.go  # Windows实现
│   │   └── capture_darwin.go   # macOS实现
│   ├── image/             # 图像处理
│   │   └── compare.go     # 图像对比功能
│   ├── mouse/             # 鼠标模拟
│   │   ├── mouse.go       # 跨平台接口
│   │   ├── mouse_windows.go    # Windows实现
│   │   └── mouse_darwin.go     # macOS实现
│   └── utils/             # 工具模块
│       ├── logger.go      # 日志记录
│       └── errors.go      # 错误处理
├── tests/                 # 测试文件
│   ├── process_test.go    # 进程管理测试
│   ├── capture_test.go    # 截图功能测试
│   ├── image_compare_test.go  # 图像对比测试
│   └── mouse_test.go      # 鼠标模拟测试
├── go.mod                # Go模块文件
└── README.md             # 项目文档
```

## 运行测试

```bash
# 运行所有测试
go test ./tests/...

# 运行特定测试
go test ./tests/ -run TestProcessManager

# 运行测试并显示详细输出
go test -v ./tests/...
```

## 注意事项

### Windows平台
- 需要管理员权限才能截取某些系统进程的窗口
- 使用PrintWindow API可以截取被遮挡的窗口
- 支持DPI感知

### macOS平台
- 需要授予屏幕录制权限
- 某些系统进程可能无法截取
- 使用AppleScript获取窗口信息可能需要辅助功能权限

### 性能优化
- 大量截图操作时建议复用截图器实例
- 图像对比操作的性能取决于图片大小和算法选择
- 鼠标模拟操作建议添加适当延迟避免过于频繁

## 贡献指南

1. Fork 项目
2. 创建功能分支 (`git checkout -b feature/AmazingFeature`)
3. 提交更改 (`git commit -m 'Add some AmazingFeature'`)
4. 推送到分支 (`git push origin feature/AmazingFeature`)
5. 打开 Pull Request

## 许可证

本项目采用 MIT 许可证 - 查看 [LICENSE](LICENSE) 文件了解详情。