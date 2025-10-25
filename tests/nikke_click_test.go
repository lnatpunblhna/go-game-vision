package tests

import (
	"os"
	"testing"
	"time"

	"github.com/lnatpunblhna/go-game-vision/pkg/capture"
	"github.com/lnatpunblhna/go-game-vision/pkg/image"
	"github.com/lnatpunblhna/go-game-vision/pkg/mouse"
	"github.com/lnatpunblhna/go-game-vision/pkg/process"
	"github.com/lnatpunblhna/go-game-vision/pkg/utils"
)

func init() {
	// 设置日志级别为 DEBUG 以便查看详细信息
	utils.SetLogLevel(utils.DEBUG)
}

// TestNikkeImageMatchAndClick 测试图像匹配并点击 email.png (使用多尺度匹配)
func TestNikkeImageMatchAndClick(t *testing.T) {
	processName := "nikke.exe"
	templatePath := "../testdata/images/template/email.png"

	// 1. 检查模板图片是否存在
	if _, err := os.Stat(templatePath); os.IsNotExist(err) {
		t.Fatalf("模板图片不存在: %s", templatePath)
	}
	t.Logf("找到模板图片: %s", templatePath)

	// 2. 获取进程信息
	pm := process.NewProcessManager()
	procs, err := pm.GetProcessByName(processName, process.ExactMatch)
	if err != nil || len(procs) == 0 {
		t.Fatalf("未找到运行中的 %s 进程,请先启动游戏", processName)
	}

	pid := procs[0].PID
	t.Logf("找到进程: %s (PID: %d)", processName, pid)

	// 3. 获取窗口信息
	windowInfo, err := capture.GetWindowInfoByPID(pid)
	if err != nil {
		t.Fatalf("获取窗口信息失败: %v", err)
	}
	t.Logf("窗口信息: %s [%dx%d]", windowInfo.Title, windowInfo.Rect.Dx(), windowInfo.Rect.Dy())

	// 4. 截取窗口截图
	t.Log("正在截取窗口...")
	windowImage, err := capture.CaptureWindowByPID(pid, nil)
	if err != nil {
		t.Fatalf("截取窗口失败: %v", err)
	}
	t.Logf("窗口截图成功: %dx%d", windowImage.Bounds().Dx(), windowImage.Bounds().Dy())

	// 5. 加载模板图片
	t.Logf("加载模板图片: %s", templatePath)
	templateImage, err := image.LoadImage(templatePath)
	if err != nil {
		t.Fatalf("加载模板图片失败: %v", err)
	}
	t.Logf("模板图片大小: %dx%d", templateImage.Bounds().Dx(), templateImage.Bounds().Dy())

	// 6. 配置多尺度匹配参数
	config := &image.MultiScaleConfig{
		MinScale:   0.7,  // 最小缩放 70% - 适应窗口缩小情况
		MaxScale:   1.3,  // 最大缩放 130% - 适应窗口放大情况
		ScaleStep:  0.05, // 步长 5% - 精细搜索
		Threshold:  0.75, // 相似度阈值 75%
		MaxResults: 5,    // 最多返回 5 个结果
	}

	// 7. 执行多尺度模板匹配
	t.Log("\n开始多尺度图像匹配...")
	t.Logf("配置: 缩放范围[%.0f%%-%.0f%%], 步长%.0f%%, 阈值%.0f%%",
		config.MinScale*100, config.MaxScale*100, config.ScaleStep*100, config.Threshold*100)

	result, err := image.MultiScaleTemplateMatch(windowImage, templateImage, config)
	if err != nil {
		t.Fatalf("多尺度匹配失败: %v", err)
	}

	// 8. 显示匹配结果
	if result.Similarity == 0 {
		t.Fatalf("未找到匹配项 (相似度为 0)")
	}

	t.Logf("\n最佳匹配结果:")
	t.Logf("  - 相似度: %.4f (%.1f%%)", result.Similarity, result.Similarity*100)
	t.Logf("  - 缩放比例: %.2f (%.0f%%)", result.Scale, result.Scale*100)
	t.Logf("  - 窗口内位置: (%d, %d)", result.Location.X, result.Location.Y)
	t.Logf("  - 边界框: %v", result.BoundingBox)

	// 转换为屏幕坐标
	screenCoords := result.ToScreenCoordinates(windowInfo)
	t.Logf("  - 屏幕坐标: (%d, %d)", screenCoords.X, screenCoords.Y)

	// 9. 检查相似度阈值
	if result.Similarity < config.Threshold {
		t.Fatalf("匹配相似度 %.4f 低于阈值 %.2f,未找到匹配项", result.Similarity, config.Threshold)
	}

	// 10. 执行点击（抗反作弊增强版）
	t.Log("\n执行抗反作弊点击 (SendInput + 随机延迟)...")

	// 使用 SendInput 配合随机延迟和焦点恢复
	// 这种方式会短暂激活窗口，但通过硬件输入队列，更难被检测
	clicker := mouse.NewMouseClicker()
	clickOptions := &mouse.ClickOptions{
		Button:       mouse.LeftButton,
		Delay:        50,
		RandomDelay:  true, // 添加随机延迟，模拟真人
		RestoreFocus: true, // 点击后恢复原窗口焦点
	}

	t.Logf("点击屏幕坐标: (%d, %d)", screenCoords.X, screenCoords.Y)
	t.Log("策略: SendInput + 随机延迟(5-15ms) + 焦点恢复")

	err = clicker.BackgroundClick(screenCoords.X, screenCoords.Y, clickOptions)
	if err != nil {
		t.Fatalf("点击失败: %v", err)
	}

	t.Log("\n✓ 多尺度图像匹配并点击成功!")
	t.Log("说明:")
	t.Log("  - 使用 SendInput API（通过硬件输入队列）")
	t.Log("  - 添加随机延迟模拟真人操作")
	t.Log("  - 自动恢复原窗口焦点")
	t.Log("  - 窗口会短暂获得焦点但会立即恢复")
	time.Sleep(500 * time.Millisecond)
}
