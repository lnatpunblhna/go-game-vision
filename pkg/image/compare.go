package image

import (
	"errors"
	"fmt"
	"image"
	_ "image/jpeg" // 导入jpeg解码器
	_ "image/png"  // 导入png解码器
	"math"
	"os"
	"sort"
	"strings"

	"github.com/lnatpunblhna/go-game-vision/pkg/capture"
	"github.com/lnatpunblhna/go-game-vision/pkg/mouse"
	"github.com/lnatpunblhna/go-game-vision/pkg/utils"
	"gocv.io/x/gocv"
)

// ErrNoMatch is returned by the find-and-click helpers when the best match
// found is below the requested similarity threshold.
var ErrNoMatch = errors.New("no image match found above threshold")

// DefaultMatchThreshold is the default similarity threshold used by the
// convenience find-and-click helpers (FindAndLeftClick / FindAndRightClick).
const DefaultMatchThreshold = 0.8

// CompareMethod image comparison method
type CompareMethod int

const (
	TemplateMatching     CompareMethod = iota // Template matching
	FeatureMatching                           // Feature point matching
	HistogramComparison                       // Histogram comparison
	StructuralSimilarity                      // Structural similarity
	MultiScaleTemplate                        // Multi-scale template matching
)

// Image processing constants
const (
	maxImageWidth  = 32768     // Maximum image width (to prevent overflow)
	maxImageHeight = 32768     // Maximum image height
	maxImagePixels = 268435456 // Maximum total pixels (16384 * 16384)

	// Feature matching constants
	featureRatioTestThreshold = 0.75 // Lowe's ratio test threshold (nearest/second-nearest)

	// SSIM constants (L = dynamic range for 8-bit images = 255)
	ssimC1 = 6.5025  // (0.01 * 255)^2
	ssimC2 = 58.5225 // (0.03 * 255)^2

	// Histogram constants
	histChannelH    = 0     // Hue channel index
	histChannelS    = 1     // Saturation channel index
	histSizeH       = 50    // Histogram bins for Hue channel
	histSizeS       = 60    // Histogram bins for Saturation channel
	histRangeHueMin = 0.0   // Minimum Hue value
	histRangeHueMax = 180.0 // Maximum Hue value (OpenCV uses 0-180 for Hue)
	histRangeSatMin = 0.0   // Minimum Saturation value
	histRangeSatMax = 256.0 // Maximum Saturation value
)

// MatchResult matching result
type MatchResult struct {
	Similarity  float64         // Similarity (0-1)
	Location    image.Point     // Match location (relative to source image)
	Confidence  float64         // Confidence
	Method      CompareMethod   // Comparison method used
	Scale       float64         // Scale factor used in multi-scale matching
	BoundingBox image.Rectangle // Bounding box of the matched region (relative to source image)
}

// MultiScaleConfig multi-scale template matching configuration
type MultiScaleConfig struct {
	MinScale   float64 // Minimum scale factor (default: 0.5)
	MaxScale   float64 // Maximum scale factor (default: 2.0)
	ScaleStep  float64 // Scale step (default: 0.1)
	Threshold  float64 // Minimum similarity threshold (default: 0.7)
	MaxResults int     // Maximum number of results to return (default: 5)
}

// DefaultMultiScaleConfig returns default multi-scale configuration
func DefaultMultiScaleConfig() *MultiScaleConfig {
	return &MultiScaleConfig{
		MinScale:   0.5,
		MaxScale:   2.0,
		ScaleStep:  0.1,
		Threshold:  0.7,
		MaxResults: 5,
	}
}

// ImageComparer image comparer
type ImageComparer struct {
	method           CompareMethod
	multiScaleConfig *MultiScaleConfig
}

// NewImageComparer creates image comparer
func NewImageComparer(method CompareMethod) *ImageComparer {
	return &ImageComparer{
		method:           method,
		multiScaleConfig: DefaultMultiScaleConfig(),
	}
}

// NewImageComparerWithConfig creates image comparer with custom multi-scale config
func NewImageComparerWithConfig(method CompareMethod, config *MultiScaleConfig) *ImageComparer {
	if config == nil {
		config = DefaultMultiScaleConfig()
	}
	return &ImageComparer{
		method:           method,
		multiScaleConfig: config,
	}
}

// CompareImages 对比两张图片
func (ic *ImageComparer) CompareImages(img1, img2 image.Image) (*MatchResult, error) {
	// Validate input images
	if img1 == nil || img2 == nil {
		return nil, fmt.Errorf("input images cannot be nil")
	}

	// 将Go image转换为OpenCV Mat
	mat1, err := imageToMat(img1)
	if err != nil {
		return nil, utils.WrapError(err, "转换第一张图片失败")
	}
	defer mat1.Close()

	mat2, err := imageToMat(img2)
	if err != nil {
		return nil, utils.WrapError(err, "转换第二张图片失败")
	}
	defer mat2.Close()

	switch ic.method {
	case TemplateMatching:
		return ic.templateMatching(mat1, mat2)
	case FeatureMatching:
		return ic.featureMatching(mat1, mat2)
	case HistogramComparison:
		return ic.histogramComparison(mat1, mat2)
	case StructuralSimilarity:
		return ic.structuralSimilarity(mat1, mat2)
	case MultiScaleTemplate:
		return ic.multiScaleTemplateMatching(mat1, mat2)
	default:
		return ic.templateMatching(mat1, mat2)
	}
}

// templateMatching 模板匹配
func (ic *ImageComparer) templateMatching(source, template gocv.Mat) (*MatchResult, error) {
	// OpenCV 的 MatchTemplate 要求模板尺寸不大于源图，否则会抛出异常导致崩溃。
	if template.Cols() > source.Cols() || template.Rows() > source.Rows() {
		utils.Warn("模板(%dx%d)大于源图(%dx%d)，无法进行模板匹配",
			template.Cols(), template.Rows(), source.Cols(), source.Rows())
		return &MatchResult{
			Similarity:  0.0,
			Location:    image.Point{},
			Confidence:  0.0,
			Method:      TemplateMatching,
			Scale:       1.0,
			BoundingBox: image.Rectangle{},
		}, nil
	}

	result := gocv.NewMat()
	defer result.Close()

	// 使用归一化相关系数匹配
	gocv.MatchTemplate(source, template, &result, gocv.TmCcoeffNormed, gocv.NewMat())

	_, maxVal, _, maxLoc := gocv.MinMaxLoc(result)

	// 计算边界框
	boundingBox := image.Rectangle{
		Min: maxLoc,
		Max: image.Point{
			X: maxLoc.X + template.Cols(),
			Y: maxLoc.Y + template.Rows(),
		},
	}

	return &MatchResult{
		Similarity:  float64(maxVal),
		Location:    maxLoc,
		Confidence:  float64(maxVal),
		Method:      TemplateMatching,
		Scale:       1.0,
		BoundingBox: boundingBox,
	}, nil
}

// featureMatching 特征点匹配
//
// 使用 SIFT + KNN(k=2) 匹配并应用 Lowe's ratio test 过滤误匹配。相似度定义为
// “好匹配数 / 两图关键点数的较小值”，落在 [0,1]，比原先“1 - 平均距离/100”
// 这种无理论依据的换算更能反映匹配质量。位置取好匹配在 img2(train) 中的质心。
func (ic *ImageComparer) featureMatching(img1, img2 gocv.Mat) (*MatchResult, error) {
	// 创建SIFT检测器
	sift := gocv.NewSIFT()
	defer sift.Close()

	// 检测关键点和描述符
	kp1, desc1 := sift.DetectAndCompute(img1, gocv.NewMat())
	defer desc1.Close()

	kp2, desc2 := sift.DetectAndCompute(img2, gocv.NewMat())
	defer desc2.Close()

	emptyResult := &MatchResult{
		Similarity: 0.0,
		Location:   image.Point{},
		Confidence: 0.0,
		Method:     FeatureMatching,
		Scale:      1.0,
	}

	if desc1.Empty() || desc2.Empty() || len(kp1) == 0 || len(kp2) == 0 {
		return emptyResult, nil
	}

	// 创建匹配器并进行 KNN 匹配（每个查询点取最近的 2 个候选）
	matcher := gocv.NewBFMatcher()
	defer matcher.Close()

	knnMatches := matcher.KnnMatch(desc1, desc2, 2)
	if len(knnMatches) == 0 {
		return emptyResult, nil
	}

	// Lowe's ratio test：最近邻明显优于次近邻才算“好匹配”
	var centerX, centerY float64
	goodMatches := 0
	for _, m := range knnMatches {
		if len(m) < 2 {
			continue
		}
		if m[0].Distance < featureRatioTestThreshold*m[1].Distance {
			trainIdx := m[0].TrainIdx
			if trainIdx >= 0 && trainIdx < len(kp2) {
				centerX += kp2[trainIdx].X
				centerY += kp2[trainIdx].Y
				goodMatches++
			}
		}
	}

	if goodMatches == 0 {
		return emptyResult, nil
	}

	minKeypoints := len(kp1)
	if len(kp2) < minKeypoints {
		minKeypoints = len(kp2)
	}
	similarity := math.Min(1.0, float64(goodMatches)/float64(minKeypoints))

	location := image.Point{
		X: int(centerX / float64(goodMatches)),
		Y: int(centerY / float64(goodMatches)),
	}

	return &MatchResult{
		Similarity:  similarity,
		Location:    location,
		Confidence:  similarity,
		Method:      FeatureMatching,
		Scale:       1.0,
		BoundingBox: image.Rectangle{},
	}, nil
}

// histogramComparison 直方图对比
func (ic *ImageComparer) histogramComparison(img1, img2 gocv.Mat) (*MatchResult, error) {
	// 转换为HSV颜色空间
	hsv1 := gocv.NewMat()
	defer hsv1.Close()
	gocv.CvtColor(img1, &hsv1, gocv.ColorBGRToHSV)

	hsv2 := gocv.NewMat()
	defer hsv2.Close()
	gocv.CvtColor(img2, &hsv2, gocv.ColorBGRToHSV)

	// 计算直方图
	hist1 := gocv.NewMat()
	defer hist1.Close()
	hist2 := gocv.NewMat()
	defer hist2.Close()

	mask := gocv.NewMat()
	defer mask.Close()

	// 设置直方图参数
	channels := []int{histChannelH, histChannelS} // H和S通道
	histSize := []int{histSizeH, histSizeS}
	ranges := []float64{histRangeHueMin, histRangeHueMax, histRangeSatMin, histRangeSatMax}

	gocv.CalcHist([]gocv.Mat{hsv1}, channels, mask, &hist1, histSize, ranges, false)
	gocv.CalcHist([]gocv.Mat{hsv2}, channels, mask, &hist2, histSize, ranges, false)

	// 归一化直方图
	gocv.Normalize(hist1, &hist1, 0, 1, gocv.NormL2)
	gocv.Normalize(hist2, &hist2, 0, 1, gocv.NormL2)

	// 计算相关性
	similarity := gocv.CompareHist(hist1, hist2, gocv.HistCmpCorrel)

	return &MatchResult{
		Similarity:  float64(similarity),
		Location:    image.Point{}, // 直方图对比不提供位置信息
		Confidence:  float64(similarity),
		Method:      HistogramComparison,
		Scale:       1.0,
		BoundingBox: image.Rectangle{},
	}, nil
}

// structuralSimilarity 结构相似性对比 (SSIM)
//
// 实现标准 SSIM 指标（全局单窗口版本）：
//
//	SSIM = ((2μxμy + C1)(2σxy + C2)) / ((μx² + μy² + C1)(σx² + σy² + C2))
//
// 其中 μ 为均值、σ² 为方差、σxy 为协方差，C1/C2 为稳定常数。结果范围约 [-1,1]，
// 1 表示完全一致。相比旧实现（仅按平均绝对差近似），这是真正的结构相似性度量。
func (ic *ImageComparer) structuralSimilarity(img1, img2 gocv.Mat) (*MatchResult, error) {
	// 转换为灰度图
	gray1 := gocv.NewMat()
	defer gray1.Close()
	gocv.CvtColor(img1, &gray1, gocv.ColorBGRToGray)

	gray2 := gocv.NewMat()
	defer gray2.Close()
	gocv.CvtColor(img2, &gray2, gocv.ColorBGRToGray)

	// 确保图像大小相同（SSIM 要求逐像素对齐）
	if gray1.Rows() != gray2.Rows() || gray1.Cols() != gray2.Cols() {
		gocv.Resize(gray2, &gray2, image.Point{X: gray1.Cols(), Y: gray1.Rows()}, 0, 0, gocv.InterpolationLinear)
	}

	// 转为 32F 以避免乘积溢出
	f1 := gocv.NewMat()
	defer f1.Close()
	gray1.ConvertTo(&f1, gocv.MatTypeCV32F)

	f2 := gocv.NewMat()
	defer f2.Close()
	gray2.ConvertTo(&f2, gocv.MatTypeCV32F)

	mu1 := f1.Mean().Val1
	mu2 := f2.Mean().Val1

	// E[x²], E[y²], E[xy]
	f1sq := gocv.NewMat()
	defer f1sq.Close()
	gocv.Multiply(f1, f1, &f1sq)

	f2sq := gocv.NewMat()
	defer f2sq.Close()
	gocv.Multiply(f2, f2, &f2sq)

	f1f2 := gocv.NewMat()
	defer f1f2.Close()
	gocv.Multiply(f1, f2, &f1f2)

	varX := f1sq.Mean().Val1 - mu1*mu1
	varY := f2sq.Mean().Val1 - mu2*mu2
	covXY := f1f2.Mean().Val1 - mu1*mu2

	numerator := (2*mu1*mu2 + ssimC1) * (2*covXY + ssimC2)
	denominator := (mu1*mu1 + mu2*mu2 + ssimC1) * (varX + varY + ssimC2)

	ssim := 0.0
	if denominator != 0 {
		ssim = numerator / denominator
	}

	// SSIM 理论范围 [-1,1]，作为相似度对外裁剪到 [0,1]
	similarity := math.Max(0, math.Min(1, ssim))

	return &MatchResult{
		Similarity:  similarity,
		Location:    image.Point{},
		Confidence:  similarity,
		Method:      StructuralSimilarity,
		Scale:       1.0,
		BoundingBox: image.Rectangle{},
	}, nil
}

// multiScaleTemplateMatching 多尺度模板匹配
func (ic *ImageComparer) multiScaleTemplateMatching(source, template gocv.Mat) (*MatchResult, error) {
	config := ic.multiScaleConfig
	if config == nil {
		config = DefaultMultiScaleConfig()
	}

	var bestResult *MatchResult
	bestSimilarity := 0.0

	utils.Debug("开始多尺度模板匹配: 范围[%.2f-%.2f], 步长%.2f",
		config.MinScale, config.MaxScale, config.ScaleStep)

	// 遍历不同的缩放尺度
	for scale := config.MinScale; scale <= config.MaxScale; scale += config.ScaleStep {
		// 缩放模板
		scaledTemplate := gocv.NewMat()
		templateSize := image.Point{
			X: int(float64(template.Cols()) * scale),
			Y: int(float64(template.Rows()) * scale),
		}

		// 确保缩放后的尺寸有效
		if templateSize.X <= 0 || templateSize.Y <= 0 ||
			templateSize.X >= source.Cols() || templateSize.Y >= source.Rows() {
			scaledTemplate.Close()
			continue
		}

		gocv.Resize(template, &scaledTemplate, templateSize, 0, 0, gocv.InterpolationLinear)

		// 执行模板匹配
		result := gocv.NewMat()
		gocv.MatchTemplate(source, scaledTemplate, &result, gocv.TmCcoeffNormed, gocv.NewMat())

		_, maxVal, _, maxLoc := gocv.MinMaxLoc(result)
		similarity := float64(maxVal)

		utils.Debug("尺度 %.2f: 相似度 %.4f, 位置 (%d,%d)",
			scale, similarity, maxLoc.X, maxLoc.Y)

		// 检查是否是最佳匹配
		if similarity > bestSimilarity && similarity >= config.Threshold {
			// 计算实际坐标和边界框
			actualLocation := maxLoc
			boundingBox := image.Rectangle{
				Min: actualLocation,
				Max: image.Point{
					X: actualLocation.X + templateSize.X,
					Y: actualLocation.Y + templateSize.Y,
				},
			}

			bestResult = &MatchResult{
				Similarity:  similarity,
				Location:    actualLocation,
				Confidence:  similarity,
				Method:      MultiScaleTemplate,
				Scale:       scale,
				BoundingBox: boundingBox,
			}
			bestSimilarity = similarity
		}

		result.Close()
		scaledTemplate.Close()
	}

	// 如果没有找到满足阈值的匹配
	if bestResult == nil {
		utils.Debug("未找到满足阈值%.2f的匹配", config.Threshold)
		return &MatchResult{
			Similarity:  0.0,
			Location:    image.Point{},
			Confidence:  0.0,
			Method:      MultiScaleTemplate,
			Scale:       1.0,
			BoundingBox: image.Rectangle{},
		}, nil
	}

	utils.Info("最佳匹配: 尺度%.2f, 相似度%.4f, 位置(%d,%d)",
		bestResult.Scale, bestResult.Similarity, bestResult.Location.X, bestResult.Location.Y)

	return bestResult, nil
}

// MultiScaleTemplateMatchingAll 多尺度模板匹配 - 返回多个结果
func (ic *ImageComparer) MultiScaleTemplateMatchingAll(source, template gocv.Mat) ([]*MatchResult, error) {
	config := ic.multiScaleConfig
	if config == nil {
		config = DefaultMultiScaleConfig()
	}

	var results []*MatchResult

	utils.Debug("开始多尺度模板匹配(全部结果): 范围[%.2f-%.2f], 步长%.2f",
		config.MinScale, config.MaxScale, config.ScaleStep)

	// 遍历不同的缩放尺度
	for scale := config.MinScale; scale <= config.MaxScale; scale += config.ScaleStep {
		// 缩放模板
		scaledTemplate := gocv.NewMat()
		templateSize := image.Point{
			X: int(float64(template.Cols()) * scale),
			Y: int(float64(template.Rows()) * scale),
		}

		// 确保缩放后的尺寸有效
		if templateSize.X <= 0 || templateSize.Y <= 0 ||
			templateSize.X >= source.Cols() || templateSize.Y >= source.Rows() {
			scaledTemplate.Close()
			continue
		}

		gocv.Resize(template, &scaledTemplate, templateSize, 0, 0, gocv.InterpolationLinear)

		// 执行模板匹配
		result := gocv.NewMat()
		gocv.MatchTemplate(source, scaledTemplate, &result, gocv.TmCcoeffNormed, gocv.NewMat())

		_, maxVal, _, maxLoc := gocv.MinMaxLoc(result)
		similarity := float64(maxVal)

		// 如果满足阈值，添加到结果中
		if similarity >= config.Threshold {
			actualLocation := maxLoc
			boundingBox := image.Rectangle{
				Min: actualLocation,
				Max: image.Point{
					X: actualLocation.X + templateSize.X,
					Y: actualLocation.Y + templateSize.Y,
				},
			}

			matchResult := &MatchResult{
				Similarity:  similarity,
				Location:    actualLocation,
				Confidence:  similarity,
				Method:      MultiScaleTemplate,
				Scale:       scale,
				BoundingBox: boundingBox,
			}

			results = append(results, matchResult)
			utils.Debug("添加匹配: 尺度%.2f, 相似度%.4f, 位置(%d,%d)",
				scale, similarity, maxLoc.X, maxLoc.Y)
		}

		result.Close()
		scaledTemplate.Close()

		// 限制结果数量
		if len(results) >= config.MaxResults {
			break
		}
	}

	// 按相似度排序 (降序)
	sort.Slice(results, func(i, j int) bool {
		return results[i].Similarity > results[j].Similarity
	})

	utils.Info("多尺度匹配完成，找到 %d 个匹配结果", len(results))
	return results, nil
}

// ParseCompareMethod 解析对比方法参数
func ParseCompareMethod(method string) CompareMethod {
	switch strings.ToLower(method) {
	case "template", "templatematching":
		return TemplateMatching
	case "feature", "featurematching":
		return FeatureMatching
	case "histogram", "histogramcomparison":
		return HistogramComparison
	case "similarity", "structural", "structuralsimilarity":
		return StructuralSimilarity
	case "multiscale", "multiscaletemplate":
		return MultiScaleTemplate
	default:
		utils.Warn("Unknown comparison method '%s', using template matching", method)
		return TemplateMatching
	}
}

// GetMethodName 获取对比方法名称
func GetMethodName(method CompareMethod) string {
	switch method {
	case TemplateMatching:
		return "Template Matching"
	case FeatureMatching:
		return "Feature Matching"
	case HistogramComparison:
		return "Histogram Comparison"
	case StructuralSimilarity:
		return "Structural Similarity"
	case MultiScaleTemplate:
		return "Multi-Scale Template Matching"
	default:
		return "Unknown"
	}
}

// LoadImage 加载图像文件
func LoadImage(filename string) (image.Image, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, utils.WrapError(err, "打开图像文件失败")
	}
	defer file.Close()

	// 尝试自动检测格式，Go的image包会自动处理各种格式
	img, _, err := image.Decode(file)
	if err != nil {
		return nil, utils.WrapError(err, "解码图像失败")
	}
	return img, nil
}

// imageToMat 将Go image转换为OpenCV Mat
func imageToMat(img image.Image) (gocv.Mat, error) {
	if img == nil {
		return gocv.NewMat(), fmt.Errorf("image cannot be nil")
	}

	bounds := img.Bounds()
	width := bounds.Max.X - bounds.Min.X
	height := bounds.Max.Y - bounds.Min.Y

	// Validate image dimensions to prevent overflow
	if width <= 0 || height <= 0 {
		return gocv.NewMat(), fmt.Errorf("invalid image dimensions: %dx%d", width, height)
	}
	if width > maxImageWidth || height > maxImageHeight {
		return gocv.NewMat(), fmt.Errorf("image too large: %dx%d (max: %dx%d)", width, height, maxImageWidth, maxImageHeight)
	}
	totalPixels := int64(width) * int64(height)
	if totalPixels > maxImagePixels {
		return gocv.NewMat(), fmt.Errorf("image has too many pixels: %d (max: %d)", totalPixels, maxImagePixels)
	}

	// 创建字节数组
	data := make([]byte, width*height*3)
	index := 0

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			data[index] = byte(b >> 8)   // B
			data[index+1] = byte(g >> 8) // G
			data[index+2] = byte(r >> 8) // R
			index += 3
		}
	}

	// 创建Mat
	mat, err := gocv.NewMatFromBytes(height, width, gocv.MatTypeCV8UC3, data)
	if err != nil {
		return gocv.NewMat(), utils.WrapError(err, "创建Mat失败")
	}

	return mat, nil
}

// ToScreenCoordinates converts window-relative coordinates to screen coordinates
func (m *MatchResult) ToScreenCoordinates(windowInfo *capture.WindowInfo) image.Point {
	return image.Point{
		X: windowInfo.Rect.Min.X + m.Location.X,
		Y: windowInfo.Rect.Min.Y + m.Location.Y,
	}
}

// ToScreenBoundingBox converts window-relative bounding box to screen coordinates
func (m *MatchResult) ToScreenBoundingBox(windowInfo *capture.WindowInfo) image.Rectangle {
	offsetX := windowInfo.Rect.Min.X
	offsetY := windowInfo.Rect.Min.Y
	return image.Rectangle{
		Min: image.Point{
			X: m.BoundingBox.Min.X + offsetX,
			Y: m.BoundingBox.Min.Y + offsetY,
		},
		Max: image.Point{
			X: m.BoundingBox.Max.X + offsetX,
			Y: m.BoundingBox.Max.Y + offsetY,
		},
	}
}

// ClickAtMatch performs a mouse click at the matched location
// Note: This may activate the target window on some platforms
func (m *MatchResult) ClickAtMatch(windowInfo *capture.WindowInfo, options *mouse.ClickOptions) error {
	screenCoords := m.ToScreenCoordinates(windowInfo)
	clicker := mouse.NewMouseClicker()
	return clicker.BackgroundClick(screenCoords.X, screenCoords.Y, options)
}

// screenCoordClicker is an optional capability implemented by the Windows
// clicker: it takes SCREEN coordinates and internally resolves the child window
// under that point and converts to client-area coordinates via ScreenToClient.
type screenCoordClicker interface {
	PostMessageClickAtScreenCoords(parentHwnd uintptr, screenX, screenY int, options *mouse.ClickOptions) error
}

// PostMessageClickAtMatch performs a true background click using PostMessage API
// This method does NOT activate the target window (Windows only).
//
// Coordinate handling: MatchResult.Location is relative to the captured image,
// whose origin equals the window's top-left (GetWindowRect origin), i.e. it is
// NON-client (window) space. WM_LBUTTONDOWN etc. expect CLIENT coordinates, so
// we must convert. We do this by mapping to absolute screen coordinates and
// letting the Windows clicker perform ScreenToClient. On platforms without that
// capability we fall back to the raw path.
func (m *MatchResult) PostMessageClickAtMatch(windowInfo *capture.WindowInfo, options *mouse.ClickOptions) error {
	if windowInfo.Handle == 0 {
		return fmt.Errorf("invalid window handle")
	}

	clicker := mouse.NewMouseClicker()

	// Preferred path (Windows): convert window-relative → screen → client.
	if sc, ok := clicker.(screenCoordClicker); ok {
		screen := m.ToScreenCoordinates(windowInfo)
		return sc.PostMessageClickAtScreenCoords(windowInfo.Handle, screen.X, screen.Y, options)
	}

	// Fallback: pass coordinates through unchanged.
	return clicker.PostMessageClick(windowInfo.Handle, m.Location.X, m.Location.Y, options)
}

// LeftClickAtMatch performs a left mouse click at the matched location
// Note: This may activate the target window
func (m *MatchResult) LeftClickAtMatch(windowInfo *capture.WindowInfo) error {
	return m.ClickAtMatch(windowInfo, &mouse.ClickOptions{
		Button: mouse.LeftButton,
		Delay:  50,
	})
}

// PostMessageLeftClickAtMatch performs a true background left click using PostMessage API
// This method does NOT activate the target window (Windows only)
func (m *MatchResult) PostMessageLeftClickAtMatch(windowInfo *capture.WindowInfo) error {
	return m.PostMessageClickAtMatch(windowInfo, &mouse.ClickOptions{
		Button: mouse.LeftButton,
		Delay:  50,
	})
}

// RightClickAtMatch performs a right mouse click at the matched location
// Note: This may activate the target window
func (m *MatchResult) RightClickAtMatch(windowInfo *capture.WindowInfo) error {
	return m.ClickAtMatch(windowInfo, &mouse.ClickOptions{
		Button: mouse.RightButton,
		Delay:  50,
	})
}

// PostMessageRightClickAtMatch performs a true background right click using PostMessage API
// This method does NOT activate the target window (Windows only)
func (m *MatchResult) PostMessageRightClickAtMatch(windowInfo *capture.WindowInfo) error {
	return m.PostMessageClickAtMatch(windowInfo, &mouse.ClickOptions{
		Button: mouse.RightButton,
		Delay:  50,
	})
}

// CompareImages 便捷函数
func CompareImages(img1, img2 image.Image, method CompareMethod) (*MatchResult, error) {
	comparer := NewImageComparer(method)
	return comparer.CompareImages(img1, img2)
}

// CalculateSimilarity 计算两张图片的相似度
func CalculateSimilarity(img1, img2 image.Image) (float64, error) {
	result, err := CompareImages(img1, img2, TemplateMatching)
	if err != nil {
		return 0, err
	}
	return result.Similarity, nil
}

// MultiScaleTemplateMatch 多尺度模板匹配便利函数
func MultiScaleTemplateMatch(source, template image.Image, config *MultiScaleConfig) (*MatchResult, error) {
	comparer := NewImageComparerWithConfig(MultiScaleTemplate, config)
	return comparer.CompareImages(source, template)
}

// MultiScaleTemplateMatchAll 多尺度模板匹配 - 返回所有结果
func MultiScaleTemplateMatchAll(source, template image.Image, config *MultiScaleConfig) ([]*MatchResult, error) {
	if config == nil {
		config = DefaultMultiScaleConfig()
	}

	// 将Go image转换为OpenCV Mat
	sourceMat, err := imageToMat(source)
	if err != nil {
		return nil, utils.WrapError(err, "转换源图片失败")
	}
	defer sourceMat.Close()

	templateMat, err := imageToMat(template)
	if err != nil {
		return nil, utils.WrapError(err, "转换模板图片失败")
	}
	defer templateMat.Close()

	comparer := NewImageComparerWithConfig(MultiScaleTemplate, config)
	return comparer.MultiScaleTemplateMatchingAll(sourceMat, templateMat)
}

// FindAndClick finds template in source image and clicks it, using the default
// similarity threshold (DefaultMatchThreshold). If no match reaches the
// threshold it returns the best result together with ErrNoMatch and does not
// click. Use FindAndClickWithThreshold to supply a custom threshold.
func FindAndClick(source, template image.Image, windowInfo *capture.WindowInfo, method CompareMethod, options *mouse.ClickOptions) (*MatchResult, error) {
	return FindAndClickWithThreshold(source, template, windowInfo, method, DefaultMatchThreshold, options)
}

// FindAndClickWithThreshold finds template in source image and clicks only if
// the match similarity is at least threshold. When no match reaches the
// threshold it returns the best result together with ErrNoMatch and performs no
// click.
func FindAndClickWithThreshold(source, template image.Image, windowInfo *capture.WindowInfo, method CompareMethod, threshold float64, options *mouse.ClickOptions) (*MatchResult, error) {
	result, err := CompareImages(source, template, method)
	if err != nil {
		return nil, utils.WrapError(err, "图像对比失败")
	}

	if result.Similarity < threshold {
		utils.Warn("最佳匹配相似度%.4f 低于阈值%.4f，跳过点击", result.Similarity, threshold)
		return result, fmt.Errorf("%w: similarity %.4f < threshold %.4f", ErrNoMatch, result.Similarity, threshold)
	}

	err = result.ClickAtMatch(windowInfo, options)
	if err != nil {
		return result, utils.WrapError(err, "点击失败")
	}

	utils.Info("图像匹配成功并完成点击: 相似度%.4f, 位置(%d,%d)",
		result.Similarity, result.Location.X, result.Location.Y)
	return result, nil
}

// FindAndLeftClick finds template and performs a left click using the default
// match threshold (DefaultMatchThreshold).
func FindAndLeftClick(source, template image.Image, windowInfo *capture.WindowInfo, method CompareMethod) (*MatchResult, error) {
	options := &mouse.ClickOptions{
		Button: mouse.LeftButton,
		Delay:  50,
	}
	return FindAndClickWithThreshold(source, template, windowInfo, method, DefaultMatchThreshold, options)
}

// FindAndRightClick finds template and performs a right click using the default
// match threshold (DefaultMatchThreshold).
func FindAndRightClick(source, template image.Image, windowInfo *capture.WindowInfo, method CompareMethod) (*MatchResult, error) {
	options := &mouse.ClickOptions{
		Button: mouse.RightButton,
		Delay:  50,
	}
	return FindAndClickWithThreshold(source, template, windowInfo, method, DefaultMatchThreshold, options)
}
