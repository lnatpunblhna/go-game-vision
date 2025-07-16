package image

import (
	"image"
	"math"

	"github.com/lnatpunblhna/go-game-vision/pkg/utils"
	"gocv.io/x/gocv"
)

// CompareMethod image comparison method
type CompareMethod int

const (
	TemplateMatching     CompareMethod = iota // Template matching
	FeatureMatching                           // Feature point matching
	HistogramComparison                       // Histogram comparison
	StructuralSimilarity                      // Structural similarity
)

// MatchResult matching result
type MatchResult struct {
	Similarity float64       // Similarity (0-1)
	Location   image.Point   // Match location
	Confidence float64       // Confidence
	Method     CompareMethod // Comparison method used
}

// ImageComparer image comparer
type ImageComparer struct {
	method CompareMethod
}

// NewImageComparer creates image comparer
func NewImageComparer(method CompareMethod) *ImageComparer {
	return &ImageComparer{
		method: method,
	}
}

// CompareImages 对比两张图片
func (ic *ImageComparer) CompareImages(img1, img2 image.Image) (*MatchResult, error) {
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
	default:
		return ic.templateMatching(mat1, mat2)
	}
}

// templateMatching 模板匹配
func (ic *ImageComparer) templateMatching(source, template gocv.Mat) (*MatchResult, error) {
	result := gocv.NewMat()
	defer result.Close()

	// 使用归一化相关系数匹配
	gocv.MatchTemplate(source, template, &result, gocv.TmCcoeffNormed, gocv.NewMat())

	_, maxVal, _, maxLoc := gocv.MinMaxLoc(result)

	return &MatchResult{
		Similarity: float64(maxVal),
		Location:   maxLoc,
		Confidence: float64(maxVal),
		Method:     TemplateMatching,
	}, nil
}

// featureMatching 特征点匹配
func (ic *ImageComparer) featureMatching(img1, img2 gocv.Mat) (*MatchResult, error) {
	// 创建SIFT检测器
	sift := gocv.NewSIFT()
	defer sift.Close()

	// 检测关键点和描述符
	_, desc1 := sift.DetectAndCompute(img1, gocv.NewMat())
	defer desc1.Close()

	kp2, desc2 := sift.DetectAndCompute(img2, gocv.NewMat())
	defer desc2.Close()

	if desc1.Empty() || desc2.Empty() {
		return &MatchResult{
			Similarity: 0.0,
			Location:   image.Point{},
			Confidence: 0.0,
			Method:     FeatureMatching,
		}, nil
	}

	// 创建匹配器
	matcher := gocv.NewBFMatcher()
	defer matcher.Close()

	// 进行匹配
	matches := matcher.Match(desc1, desc2)

	if len(matches) == 0 {
		return &MatchResult{
			Similarity: 0.0,
			Location:   image.Point{},
			Confidence: 0.0,
			Method:     FeatureMatching,
		}, nil
	}

	// 计算平均距离作为相似度
	totalDistance := 0.0
	for _, match := range matches {
		totalDistance += float64(match.Distance)
	}
	avgDistance := totalDistance / float64(len(matches))

	// 将距离转换为相似度 (距离越小，相似度越高)
	similarity := math.Max(0, 1.0-avgDistance/100.0)

	// 计算匹配点的中心位置
	var centerX, centerY float64
	validMatches := 0
	for i, match := range matches {
		if i < len(kp2) {
			centerX += float64(kp2[match.TrainIdx].X)
			centerY += float64(kp2[match.TrainIdx].Y)
			validMatches++
		}
	}

	location := image.Point{}
	if validMatches > 0 {
		location = image.Point{
			X: int(centerX / float64(validMatches)),
			Y: int(centerY / float64(validMatches)),
		}
	}

	return &MatchResult{
		Similarity: similarity,
		Location:   location,
		Confidence: similarity,
		Method:     FeatureMatching,
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
	channels := []int{0, 1} // H和S通道
	histSize := []int{50, 60}
	ranges := []float64{0, 180, 0, 256}

	gocv.CalcHist([]gocv.Mat{hsv1}, channels, mask, &hist1, histSize, ranges, false)
	gocv.CalcHist([]gocv.Mat{hsv2}, channels, mask, &hist2, histSize, ranges, false)

	// 归一化直方图
	gocv.Normalize(hist1, &hist1, 0, 1, gocv.NormL2)
	gocv.Normalize(hist2, &hist2, 0, 1, gocv.NormL2)

	// 计算相关性
	similarity := gocv.CompareHist(hist1, hist2, gocv.HistCmpCorrel)

	return &MatchResult{
		Similarity: float64(similarity),
		Location:   image.Point{}, // 直方图对比不提供位置信息
		Confidence: float64(similarity),
		Method:     HistogramComparison,
	}, nil
}

// structuralSimilarity 结构相似性对比
func (ic *ImageComparer) structuralSimilarity(img1, img2 gocv.Mat) (*MatchResult, error) {
	// 转换为灰度图
	gray1 := gocv.NewMat()
	defer gray1.Close()
	gocv.CvtColor(img1, &gray1, gocv.ColorBGRToGray)

	gray2 := gocv.NewMat()
	defer gray2.Close()
	gocv.CvtColor(img2, &gray2, gocv.ColorBGRToGray)

	// 确保图像大小相同
	if gray1.Rows() != gray2.Rows() || gray1.Cols() != gray2.Cols() {
		gocv.Resize(gray2, &gray2, image.Point{X: gray1.Cols(), Y: gray1.Rows()}, 0, 0, gocv.InterpolationLinear)
	}

	// 简化的结构相似性计算
	diff := gocv.NewMat()
	defer diff.Close()
	gocv.AbsDiff(gray1, gray2, &diff)

	mean := gocv.NewMat()
	stddev := gocv.NewMat()
	defer mean.Close()
	defer stddev.Close()

	gocv.MeanStdDev(diff, &mean, &stddev)

	// 灰度图只有一个通道，所以取第一个值即可
	meanVal := mean.GetFloatAt(0, 0)
	similarity := 1.0 - (float64(meanVal) / 255.0)

	return &MatchResult{
		Similarity: math.Max(0, similarity),
		Location:   image.Point{},
		Confidence: math.Max(0, similarity),
		Method:     StructuralSimilarity,
	}, nil
}

// imageToMat 将Go image转换为OpenCV Mat
func imageToMat(img image.Image) (gocv.Mat, error) {
	bounds := img.Bounds()
	width := bounds.Max.X - bounds.Min.X
	height := bounds.Max.Y - bounds.Min.Y

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

// 便捷函数
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
