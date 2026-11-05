package admin

import (
	"encoding/base64"
	"fmt"

	qrcode "github.com/skip2/go-qrcode"
)

// qrDataURI 把内容编码成二维码，返回可直接塞进 <img src> 的 data URI。
//
// 为什么在服务端生成：iLink 的登录返回的是一个 web 链接（不是图片），
// 网页版文件传输助手给的是「图片地址」，直接当 <img> 用虽然能显示，
// 但用户真正要「扫」的内容其实是长链接（/l/<uuid>），两者并不等价。
// 统一在服务端把**可扫内容**编码成图，两条通道的前端就一致了。
func qrDataURI(content string, size int) (string, error) {
	if content == "" {
		return "", fmt.Errorf("二维码内容为空")
	}
	if size <= 0 {
		size = 240
	}
	png, err := qrcode.Encode(content, qrcode.Medium, size)
	if err != nil {
		return "", fmt.Errorf("生成二维码失败: %w", err)
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(png), nil
}
