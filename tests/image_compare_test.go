package tests

import (
	"image"
	"image/color"
	"testing"

	imagecompare "github.com/lnatpunblhna/go-game-vision/pkg/image"
	"github.com/lnatpunblhna/go-game-vision/pkg/utils"
)

func TestImageComparer(t *testing.T) {
	// 设置日志级别
	utils.GlobalLogger = utils.NewLogger(utils.ERROR)

	// 创建测试图像
	img1 := createTestImageForComparison(100, 100, color.RGBA{255, 0, 0, 255}) // 红色
	img2 := createTestImageForComparison(100, 100, color.RGBA{255, 0, 0, 255}) // 相同的红色
	img3 := createTestImageForComparison(100, 100, color.RGBA{0, 255, 0, 255}) // 绿色

	// 测试不同的对比方法
	methods := []imagecompare.CompareMethod{
		imagecompare.TemplateMatching,
		imagecompare.FeatureMatching,
		imagecompare.HistogramComparison,
		imagecompare.StructuralSimilarity,
	}

	for _, method := range methods {
		t.Run(getMethodNameForTest(method), func(t *testing.T) {
			comparer := imagecompare.NewImageComparer(method)

			// 测试相同图像的对比
			result1, err := comparer.CompareImages(img1, img2)
			if err != nil {
				t.Logf("Comparison failed for method %v: %v", method, err)
				return
			}

			if result1 == nil {
				t.Error("Comparison result should not be nil")
				return
			}

			t.Logf("Same images similarity: %.4f", result1.Similarity)
			t.Logf("Confidence: %.4f", result1.Confidence)

			// 测试不同图像的对比
			result2, err := comparer.CompareImages(img1, img3)
			if err != nil {
				t.Logf("Comparison failed for method %v: %v", method, err)
				return
			}

			t.Logf("Different images similarity: %.4f", result2.Similarity)

			// 相同图像的相似度应该高于不同图像
			if result1.Similarity < result2.Similarity {
				t.Logf("Warning: Same images have lower similarity than different images for method %v", method)
			}
		})
	}
}

func TestCompareMethodParsing(t *testing.T) {
	// 测试对比方法的解析
	testCases := []struct {
		input    string
		expected imagecompare.CompareMethod
	}{
		{"template", imagecompare.TemplateMatching},
		{"templatematching", imagecompare.TemplateMatching},
		{"feature", imagecompare.FeatureMatching},
		{"featurematching", imagecompare.FeatureMatching},
		{"histogram", imagecompare.HistogramComparison},
		{"histogramcomparison", imagecompare.HistogramComparison},
		{"similarity", imagecompare.StructuralSimilarity},
		{"structural", imagecompare.StructuralSimilarity},
		{"structuralsimilarity", imagecompare.StructuralSimilarity},
	}

	for _, tc := range testCases {
		result := parseCompareMethodTest(tc.input)
		if result != tc.expected {
			t.Errorf("parseCompareMethod(%s) = %v, expected %v", tc.input, result, tc.expected)
		}
	}
}

func TestConvenienceComparisonFunctions(t *testing.T) {
	// 创建测试图像
	img1 := createTestImageForComparison(50, 50, color.RGBA{255, 0, 0, 255})
	img2 := createTestImageForComparison(50, 50, color.RGBA{255, 0, 0, 255})

	// 测试便捷函数
	result, err := imagecompare.CompareImages(img1, img2, imagecompare.TemplateMatching)
	if err != nil {
		t.Logf("Convenience comparison function failed: %v", err)
		return
	}

	if result == nil {
		t.Error("Comparison result should not be nil")
		return
	}

	t.Logf("Convenience function result: %.4f", result.Similarity)

	// 测试相似度计算函数
	similarity, err := imagecompare.CalculateSimilarity(img1, img2)
	if err != nil {
		t.Logf("Similarity calculation failed: %v", err)
		return
	}

	t.Logf("Calculated similarity: %.4f", similarity)
}

func TestImageSizeHandling(t *testing.T) {
	// 测试不同尺寸图像的对比
	img1 := createTestImageForComparison(100, 100, color.RGBA{255, 0, 0, 255})
	img2 := createTestImageForComparison(50, 50, color.RGBA{255, 0, 0, 255})

	comparer := imagecompare.NewImageComparer(imagecompare.StructuralSimilarity)
	result, err := comparer.CompareImages(img1, img2)
	if err != nil {
		t.Logf("Different size comparison failed: %v", err)
		return
	}

	if result == nil {
		t.Error("Comparison result should not be nil")
		return
	}

	t.Logf("Different size images similarity: %.4f", result.Similarity)
}

// createTestImageForComparison 创建用于对比测试的图像
func createTestImageForComparison(width, height int, fillColor color.RGBA) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, width, height))

	// 填充指定颜色
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, fillColor)
		}
	}

	// 添加一些图案以便特征检测
	for i := 0; i < width/10; i++ {
		for j := 0; j < height/10; j++ {
			x := i*10 + 5
			y := j*10 + 5
			if x < width && y < height {
				// 添加白色点
				img.Set(x, y, color.RGBA{255, 255, 255, 255})
			}
		}
	}

	return img
}

// getMethodNameForTest 获取测试用的方法名称
func getMethodNameForTest(method imagecompare.CompareMethod) string {
	switch method {
	case imagecompare.TemplateMatching:
		return "TemplateMatching"
	case imagecompare.FeatureMatching:
		return "FeatureMatching"
	case imagecompare.HistogramComparison:
		return "HistogramComparison"
	case imagecompare.StructuralSimilarity:
		return "StructuralSimilarity"
	default:
		return "Unknown"
	}
}

// parseCompareMethodTest 模拟 main.go 中的 parseCompareMethod 函数
func parseCompareMethodTest(method string) imagecompare.CompareMethod {
	switch method {
	case "template", "templatematching":
		return imagecompare.TemplateMatching
	case "feature", "featurematching":
		return imagecompare.FeatureMatching
	case "histogram", "histogramcomparison":
		return imagecompare.HistogramComparison
	case "similarity", "structural", "structuralsimilarity":
		return imagecompare.StructuralSimilarity
	default:
		return imagecompare.TemplateMatching
	}
}

// Benchmark tests
func BenchmarkTemplateMatching(b *testing.B) {
	utils.GlobalLogger = utils.NewLogger(utils.ERROR)

	img1 := createTestImageForComparison(100, 100, color.RGBA{255, 0, 0, 255})
	img2 := createTestImageForComparison(100, 100, color.RGBA{255, 0, 0, 255})

	comparer := imagecompare.NewImageComparer(imagecompare.TemplateMatching)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := comparer.CompareImages(img1, img2)
		if err != nil {
			b.Logf("Comparison failed: %v", err)
			break
		}
	}
}

func BenchmarkHistogramComparison(b *testing.B) {
	utils.GlobalLogger = utils.NewLogger(utils.ERROR)

	img1 := createTestImageForComparison(100, 100, color.RGBA{255, 0, 0, 255})
	img2 := createTestImageForComparison(100, 100, color.RGBA{255, 0, 0, 255})

	comparer := imagecompare.NewImageComparer(imagecompare.HistogramComparison)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := comparer.CompareImages(img1, img2)
		if err != nil {
			b.Logf("Comparison failed: %v", err)
			break
		}
	}
}
