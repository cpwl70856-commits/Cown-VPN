package config

import (
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"strings"

	"github.com/makiuchi-d/gozxing"
	"github.com/makiuchi-d/gozxing/qrcode"
)

// LoadQRImage decodes a WireGuard (or vpn://) QR code from an image file path.
// Camera capture is intentionally out of scope for the desktop client; import a
// screenshot / exported QR PNG/JPEG instead.
func LoadQRImage(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("打开二维码图片失败: %w", err)
	}
	defer f.Close()

	img, format, err := image.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("无法解码图片（支持 PNG/JPEG/GIF）: %w", err)
	}
	_ = format

	bmp, err := gozxing.NewBinaryBitmapFromImage(img)
	if err != nil {
		return nil, fmt.Errorf("二维码预处理失败: %w", err)
	}
	result, err := qrcode.NewQRCodeReader().Decode(bmp, nil)
	if err != nil {
		return nil, fmt.Errorf("未识别到二维码，请使用清晰的 WireGuard 配置二维码截图: %w", err)
	}
	text := strings.TrimSpace(result.GetText())
	if text == "" {
		return nil, fmt.Errorf("二维码内容为空")
	}
	cfg, err := ParseImportText(text)
	if err != nil {
		return nil, fmt.Errorf("二维码内容不是有效 WireGuard 配置: %w", err)
	}
	return cfg, nil
}
