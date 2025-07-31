# Go Game Vision

[English](README_EN.md) | 中文

一个跨平台的Go项目，实现Windows和macOS的程序窗口捕获功能，包含进程管理、窗口截图、图像处理和OCR文字识别等功能模块。

## 功能特性

### 🔍 进程管理模块
- 根据程序名称获取进程PID
- 支持模糊匹配和精确匹配两种模式
- 处理多个同名进程的情况
- 跨平台兼容（Windows/macOS）

### 📸 窗口截图模块
- **Windows平台**: 使用Windows API（BitBlt/PrintWindow）实现窗口截图
- **macOS平台**: 使用系统命令和AppleScript实现窗口截图
- **关键特性**: Windows下即使窗口被其他窗口遮挡也能正常截图
- 支持多种图片格式输出（PNG、JPEG、BMP、GIF）
- 提供根据PID获取对应程序窗口截图的方法

### 🖼️ 图像处理模块
- 集成GoCV库实现图片对比功能
- 提供图片相似度计算方法
- 支持多种对比算法：
  - 模板匹配（Template Matching）
  - 特征点匹配（Feature Matching）
  - 直方图对比（Histogram Comparison）
  - 结构相似性（Structural Similarity）

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
    "github.com/lnatpunblhna/go-game-vision/pkg/ocr"
)

func main() {
    // 1. 进程管理
    pid, err := process.GetProcessPIDByName("notepad", process.FuzzyMatch)
    if err != nil {
        panic(err)
    }
    fmt.Printf("找到记事本进程，PID: %d\n", pid)

    // 2. 窗口截图
    img, err := capture.CaptureWindowByPID(pid, capture.DefaultCaptureOptions())
    if err != nil {
        panic(err)
    }

    // 3. 保存截图
    err = capture.CaptureAndSave(pid, "notepad.png", capture.PNG, 90)
    if err != nil {
        panic(err)
    }

    // 4. 图像对比
    similarity, err := image.CalculateSimilarity(img1, img2)
    if err != nil {
        panic(err)
    }
    fmt.Printf("图像相似度: %.2f\n", similarity)

    // 5. OCR文字识别
    text, err := ocr.RecognizeTextFromFile("notepad.png", ocr.English)
    if err != nil {
        panic(err)
    }
    fmt.Printf("识别的文字: %s\n", text)
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

// 截取窗口
options := capture.DefaultCaptureOptions()
img, err := capturer.CaptureWindowByPID(pid, options)

// 截取屏幕
img, err := capturer.CaptureScreen(options)

// 保存图片
err = capturer.SaveImage(img, "output.png", capture.PNG, 90)
```

### 图像处理 (pkg/image)

```go
// 创建图像对比器
comparer := image.NewImageComparer(image.TemplateMatching)

// 对比图像
result, err := comparer.CompareImages(img1, img2)
fmt.Printf("相似度: %.2f, 位置: (%d, %d)\n", 
    result.Similarity, result.Location.X, result.Location.Y)
```



## 项目结构

```
go-game-vision/
├── pkg/                    # 核心包
│   ├── process/           # 进程管理
│   │   ├── process.go     # 跨平台接口
│   │   ├── process_windows.go  # Windows实现
│   │   └── process_darwin.go   # macOS实现
│   ├── capture/           # 窗口截图
│   │   ├── capture.go     # 跨平台接口
│   │   ├── capture_windows.go  # Windows实现
│   │   └── capture_darwin.go   # macOS实现
│   ├── image/             # 图像处理
│   │   └── compare.go     # 图像对比功能
│   ├── ocr/               # OCR识别
│   │   └── ocr.go         # OCR功能
│   └── utils/             # 工具模块
│       ├── logger.go      # 日志记录
│       └── errors.go      # 错误处理
├── tests/                 # 测试文件
│   ├── process_test.go    # 进程管理测试
│   └── capture_test.go    # 截图功能测试
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
- OCR识别比较耗时，建议在后台线程执行
- 图像对比操作的性能取决于图片大小和算法选择

## 贡献指南

1. Fork 项目
2. 创建功能分支 (`git checkout -b feature/AmazingFeature`)
3. 提交更改 (`git commit -m 'Add some AmazingFeature'`)
4. 推送到分支 (`git push origin feature/AmazingFeature`)
5. 打开 Pull Request

## 许可证

本项目采用 MIT 许可证 - 查看 [LICENSE](LICENSE) 文件了解详情。