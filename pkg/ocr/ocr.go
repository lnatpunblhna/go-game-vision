package ocr

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"strings"

	"github.com/lnatpunblhna/go-game-vision/pkg/utils"
	"github.com/otiai10/gosseract/v2"
)

// Language OCR supported languages
type Language string

const (
	English            Language = "eng"
	Chinese            Language = "chi_sim"
	ChineseTraditional Language = "chi_tra"
	Japanese           Language = "jpn"
	Korean             Language = "kor"
	German             Language = "deu"
	French             Language = "fra"
	Spanish            Language = "spa"
	Russian            Language = "rus"
)

// OCROptions OCR options
type OCROptions struct {
	Language      Language // Recognition language
	PSM           int      // Page Segmentation Mode
	OEM           int      // OCR Engine Mode
	Whitelist     string   // Character whitelist
	Blacklist     string   // Character blacklist
	DPI           int      // Image DPI
	MinConfidence float32  // Minimum confidence
}

// DefaultOCROptions default OCR options
func DefaultOCROptions() *OCROptions {
	return &OCROptions{
		Language:      English,
		PSM:           3, // Automatic page segmentation
		OEM:           3, // Default OCR engine
		DPI:           300,
		MinConfidence: 0.0,
	}
}

// OCRResult OCR识别结果
type OCRResult struct {
	Text       string      // 识别的文本
	Confidence float32     // 整体置信度
	Words      []WordInfo  // 单词信息
	Lines      []LineInfo  // 行信息
	Blocks     []BlockInfo // 块信息
}

// WordInfo 单词信息
type WordInfo struct {
	Text        string          // 单词文本
	Confidence  float32         // 置信度
	BoundingBox image.Rectangle // 边界框
}

// LineInfo 行信息
type LineInfo struct {
	Text        string          // 行文本
	Confidence  float32         // 置信度
	BoundingBox image.Rectangle // 边界框
	Words       []WordInfo      // 包含的单词
}

// BlockInfo 块信息
type BlockInfo struct {
	Text        string          // 块文本
	Confidence  float32         // 置信度
	BoundingBox image.Rectangle // 边界框
	Lines       []LineInfo      // 包含的行
}

// OCREngine OCR引擎接口
type OCREngine interface {
	// RecognizeText 识别图片中的文字
	RecognizeText(img image.Image, options *OCROptions) (*OCRResult, error)

	// RecognizeTextFromFile 从文件识别文字
	RecognizeTextFromFile(filename string, options *OCROptions) (*OCRResult, error)

	// SetLanguage 设置识别语言
	SetLanguage(lang Language) error

	// Close 关闭OCR引擎
	Close() error
}

// TesseractEngine Tesseract OCR引擎实现
type TesseractEngine struct {
	client *gosseract.Client
}

// NewOCREngine 创建OCR引擎
func NewOCREngine() OCREngine {
	return &TesseractEngine{
		client: gosseract.NewClient(),
	}
}

// RecognizeText 识别图片中的文字
func (t *TesseractEngine) RecognizeText(img image.Image, options *OCROptions) (*OCRResult, error) {
	if options == nil {
		options = DefaultOCROptions()
	}

	// 设置语言
	err := t.client.SetLanguage(string(options.Language))
	if err != nil {
		utils.Warn("设置OCR语言失败: %v", err)
	}

	// 设置页面分割模式
	err = t.client.SetPageSegMode(gosseract.PageSegMode(options.PSM))
	if err != nil {
		utils.Warn("设置页面分割模式失败: %v", err)
	}

	// 设置字符白名单和黑名单
	if options.Whitelist != "" {
		err = t.client.SetVariable("tessedit_char_whitelist", options.Whitelist)
		if err != nil {
			utils.Warn("设置字符白名单失败: %v", err)
		}
	}

	if options.Blacklist != "" {
		err = t.client.SetVariable("tessedit_char_blacklist", options.Blacklist)
		if err != nil {
			utils.Warn("设置字符黑名单失败: %v", err)
		}
	}

	// 设置图像
	var buf bytes.Buffer
	err = jpeg.Encode(&buf, img, nil) // 或者 png.Encode
	if err != nil {
		return nil, err
	}
	err = t.client.SetImageFromBytes(buf.Bytes())
	if err != nil {
		return nil, utils.WrapError(err, "设置OCR图像失败")
	}

	// 获取文本
	text, err := t.client.Text()
	if err != nil {
		return nil, utils.WrapError(err, "OCR文字识别失败")
	}

	// 获取置信度
	confidence := 0.00
	boundingBoxes, err := t.client.GetBoundingBoxesVerbose()
	if err != nil {
		utils.Warn("获取OCR置信度失败: %v", err)
	}

	var total float64
	var count int

	for _, r := range boundingBoxes {
		total += r.Confidence
		count++
	}

	if count > 0 {
		confidence = float64(int(total / float64(count)))
		fmt.Printf("平均识别置信度：%.2f%%\n", confidence)
	}

	// 获取详细信息
	words := t.getWordInfo()
	lines := t.getLineInfo()
	blocks := t.getBlockInfo()

	result := &OCRResult{
		Text:       strings.TrimSpace(text),
		Confidence: float32(confidence),
		Words:      words,
		Lines:      lines,
		Blocks:     blocks,
	}

	utils.Info("OCR识别完成，置信度: %.2f", confidence)
	return result, nil
}

// RecognizeTextFromFile 从文件识别文字
func (t *TesseractEngine) RecognizeTextFromFile(filename string, options *OCROptions) (*OCRResult, error) {
	if options == nil {
		options = DefaultOCROptions()
	}

	// 设置语言
	err := t.client.SetLanguage(string(options.Language))
	if err != nil {
		utils.Warn("设置OCR语言失败: %v", err)
	}

	// 设置图像文件
	err = t.client.SetImage(filename)
	if err != nil {
		return nil, utils.WrapError(err, "设置OCR图像文件失败")
	}

	// 获取文本
	text, err := t.client.Text()
	if err != nil {
		return nil, utils.WrapError(err, "OCR文字识别失败")
	}

	// 获取置信度
	confidence := 0.00
	boundingBoxes, err := t.client.GetBoundingBoxesVerbose()
	if err != nil {
		utils.Warn("获取OCR置信度失败: %v", err)
	}

	var total float64
	var count int

	for _, r := range boundingBoxes {
		total += r.Confidence
		count++
	}

	if count > 0 {
		confidence = float64(int(total / float64(count)))
		fmt.Printf("平均识别置信度：%.2f%%\n", confidence)
	}

	result := &OCRResult{
		Text:       strings.TrimSpace(text),
		Confidence: float32(confidence),
		Words:      t.getWordInfo(),
		Lines:      t.getLineInfo(),
		Blocks:     t.getBlockInfo(),
	}

	utils.Info("OCR识别完成，置信度: %.2f", confidence)
	return result, nil
}

// SetLanguage 设置识别语言
func (t *TesseractEngine) SetLanguage(lang Language) error {
	err := t.client.SetLanguage(string(lang))
	if err != nil {
		return utils.WrapError(err, "设置OCR语言失败")
	}
	return nil
}

// Close 关闭OCR引擎
func (t *TesseractEngine) Close() error {
	return t.client.Close()
}

// getWordInfo 获取单词信息
func (t *TesseractEngine) getWordInfo() []WordInfo {
	// 这里是一个简化实现，实际项目中可以使用Tesseract的详细API获取更多信息
	var words []WordInfo

	// Gosseract库的限制，这里提供基础实现
	// 实际项目中可能需要使用其他方法获取详细的边界框信息

	return words
}

// getLineInfo 获取行信息
func (t *TesseractEngine) getLineInfo() []LineInfo {
	var lines []LineInfo
	return lines
}

// getBlockInfo 获取块信息
func (t *TesseractEngine) getBlockInfo() []BlockInfo {
	var blocks []BlockInfo
	return blocks
}

// 便捷函数

// RecognizeText 便捷函数：识别图片中的文字
func RecognizeText(img image.Image, language Language) (string, error) {
	engine := NewOCREngine()
	defer engine.Close()

	options := &OCROptions{
		Language: language,
		PSM:      3,
		OEM:      3,
	}

	result, err := engine.RecognizeText(img, options)
	if err != nil {
		return "", err
	}

	return result.Text, nil
}

// RecognizeTextFromFile 便捷函数：从文件识别文字
func RecognizeTextFromFile(filename string, language Language) (string, error) {
	engine := NewOCREngine()
	defer engine.Close()

	options := &OCROptions{
		Language: language,
		PSM:      3,
		OEM:      3,
	}

	result, err := engine.RecognizeTextFromFile(filename, options)
	if err != nil {
		return "", err
	}

	return result.Text, nil
}
