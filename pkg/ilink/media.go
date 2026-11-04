package ilink

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/md5"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// mediaTypeImage 对应 getuploadurl 的 media_type。
const mediaTypeImage = 1

// SendImage 发送一张图片。
//
// 微信 CDN 上的媒体是 **AES-128-ECB 加密**的，流程分三步：
//  1. 生成随机 AES-128 key，对图片做 ECB + PKCS#7 加密；
//  2. getuploadurl 拿预签名上传地址，PUT 密文；
//  3. sendmessage 里用 image_item 引用（带 aes_key，对方下载后解密）。
//
// contextToken 与发文本一致：必须是该会话最近一条 inbound 消息的。
func (c *Client) SendImage(ctx context.Context, toUserID, contextToken string, data []byte) error {
	if len(data) == 0 {
		return fmt.Errorf("ilink: SendImage: empty image")
	}

	aesKey := make([]byte, 16)
	if _, err := rand.Read(aesKey); err != nil {
		return fmt.Errorf("ilink: SendImage: generate key: %w", err)
	}
	encrypted, err := encryptECB(aesKey, data)
	if err != nil {
		return fmt.Errorf("ilink: SendImage: encrypt: %w", err)
	}
	// 两种编码别搞混（用反了 CDN 会以 500 拒绝，且不带错误信息）：
	//   getuploadurl 的 aeskey 用 hex（32 字符）；
	//   image_item.media.aes_key 用 base64(hex)。
	keyHex := hex.EncodeToString(aesKey)
	keyB64 := base64.StdEncoding.EncodeToString([]byte(keyHex))

	// filekey：16 字节随机数的 hex
	var fk [16]byte
	if _, err := rand.Read(fk[:]); err != nil {
		return fmt.Errorf("ilink: SendImage: filekey: %w", err)
	}
	filekey := hex.EncodeToString(fk[:])

	// 1. 预签名上传地址
	urlReq := map[string]any{
		"media_type":    mediaTypeImage,
		"to_user_id":    toUserID,
		"filekey":       filekey,
		"rawsize":       int64(len(data)),
		"rawfilemd5":    md5Hex(data),
		"filesize":      int64(len(encrypted)),
		"aeskey":        keyHex,
		"no_need_thumb": true, // 二维码这类图不需要缩略图
		// 所有 POST 都必须带 base_info，漏了会返回 ret=-2
		"base_info": baseInfo{ChannelVersion: channelVersion},
	}
	req, err := c.newRequest(ctx, http.MethodPost, "/ilink/bot/getuploadurl", urlReq)
	if err != nil {
		return err
	}
	body, err := c.do(req)
	if err != nil {
		return err
	}
	var urlResp struct {
		Ret              int    `json:"ret"`
		UploadParam      string `json:"upload_param"`
		UploadFullURL    string `json:"upload_full_url"`
		ThumbUploadParam string `json:"thumb_upload_param"`
		ErrCode          *int   `json:"errcode"`
		ErrMsg           string `json:"errmsg"`
	}
	if err := json.Unmarshal(body, &urlResp); err != nil {
		return fmt.Errorf("parse getuploadurl: %w (body=%s)", err, truncate(string(body), 200))
	}
	if urlResp.Ret != 0 {
		return &APIError{Op: "getuploadurl", Ret: urlResp.Ret, ErrMsg: urlResp.ErrMsg}
	}

	// 2. 上传密文到 CDN。
	//
	// upload_full_url 有时为空；upload_param 是个**不透明 token**（不是 URL），
	// 需要拼成 <cdn>/upload?encrypted_query_param=<token>&filekey=<key>。
	// 服务端在响应头 x-encrypted-param 里返回真正用于下载引用的参数。
	mediaParam := urlResp.UploadParam
	if urlResp.UploadFullURL != "" {
		if err := putBytes(ctx, urlResp.UploadFullURL, encrypted); err != nil {
			return fmt.Errorf("ilink: SendImage: upload(full url): %w", err)
		}
	} else {
		filekey, _ := urlReq["filekey"].(string)
		uploadURL := fmt.Sprintf("%s/upload?encrypted_query_param=%s&filekey=%s",
			c.cdnBase, url.QueryEscape(urlResp.UploadParam), url.QueryEscape(filekey))
		encParam, err := postBytes(ctx, uploadURL, encrypted)
		if err != nil {
			return fmt.Errorf("ilink: SendImage: upload: %w", err)
		}
		if encParam != "" {
			mediaParam = encParam
		}
	}

	// 3. 发送图片消息
	payload := map[string]any{
		"msg": map[string]any{
			"to_user_id":    toUserID,
			"client_id":     newClientID(),
			"message_type":  int(MessageTypeBot),
			"message_state": 2,
			"context_token": contextToken,
			"item_list": []map[string]any{
				{
					"type": int(ItemTypeImage),
					"image_item": map[string]any{
						"media": map[string]any{
							"encrypt_query_param": mediaParam,
							"aes_key":             keyB64,
							// 参考实现用 1（打包了缩略图/中图信息）；用 0 时微信不渲染
							"encrypt_type": 1,
						},
						// mid_size 是**密文**长度（不是原图长度）
						"mid_size": len(encrypted),
					},
				},
			},
		},
		"base_info": baseInfo{ChannelVersion: channelVersion},
	}
	req, err = c.newRequest(ctx, http.MethodPost, "/ilink/bot/sendmessage", payload)
	if err != nil {
		return err
	}
	body, err = c.do(req)
	if err != nil {
		return err
	}
	var resp struct {
		Ret       int    `json:"ret"`
		MessageID int64  `json:"message_id"`
		ErrCode   *int   `json:"errcode"`
		ErrMsg    string `json:"errmsg"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return fmt.Errorf("parse sendmessage(image): %w (body=%s)", err, truncate(string(body), 200))
	}
	if resp.ErrCode != nil && *resp.ErrCode == errcodeSessionExpired {
		return ErrSessionExpired
	}
	if resp.Ret != 0 || (resp.ErrCode != nil && *resp.ErrCode != 0) {
		code := 0
		if resp.ErrCode != nil {
			code = *resp.ErrCode
		}
		return &APIError{Op: "sendmessage(image)", Ret: resp.Ret, ErrCode: code, ErrMsg: resp.ErrMsg}
	}
	c.logger.InfoContext(ctx, "ilink: image accepted",
		"message_id", resp.MessageID, "to", toUserID, "bytes", len(data))
	return nil
}

// cdnBaseURL 微信 CDN 基座（媒体上传/下载）。
const cdnBaseURL = "https://novac2c.cdn.weixin.qq.com/c2c"

// postBytes 以 octet-stream 上传密文，返回响应头 x-encrypted-param（下载引用参数）。
func postBytes(ctx context.Context, url string, data []byte) (string, error) {
	// CDN 偶发 500，参考实现也是重试 3 次；4xx 直接失败（参数错，重试无意义）
	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
		if err != nil {
			return "", err
		}
		req.ContentLength = int64(len(data))
		req.Header.Set("Content-Type", "application/octet-stream")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		resp.Body.Close()

		if resp.StatusCode >= 400 && resp.StatusCode < 500 {
			return "", fmt.Errorf("CDN 上传被拒 %d: %s %s", resp.StatusCode,
				resp.Header.Get("x-error-message"), string(body))
		}
		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("CDN 上传失败 %d: %s %s", resp.StatusCode,
				resp.Header.Get("x-error-message"), string(body))
			continue
		}
		param := resp.Header.Get("x-encrypted-param")
		if param == "" {
			lastErr = fmt.Errorf("CDN 上传成功但缺少 x-encrypted-param 响应头")
			continue
		}
		return param, nil
	}
	return "", lastErr
}

func putBytes(ctx context.Context, url string, data []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.ContentLength = int64(len(data))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 200))
		return fmt.Errorf("PUT %s: %s (%s)", url, resp.Status, string(b))
	}
	return nil
}

func md5Hex(data []byte) string {
	sum := md5.Sum(data)
	return hex.EncodeToString(sum[:])
}

// ---- AES-128-ECB（Go 标准库不直接暴露 ECB，用单块加密串联实现）----

func encryptECB(key, plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	bs := block.BlockSize()
	padded := pkcs7Pad(plaintext, bs)
	out := make([]byte, len(padded))
	for i := 0; i < len(padded); i += bs {
		block.Encrypt(out[i:i+bs], padded[i:i+bs])
	}
	return out, nil
}

func decryptECB(key, ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	bs := block.BlockSize()
	if len(ciphertext) == 0 || len(ciphertext)%bs != 0 {
		return nil, fmt.Errorf("ilink: decryptECB: bad length %d", len(ciphertext))
	}
	out := make([]byte, len(ciphertext))
	for i := 0; i < len(ciphertext); i += bs {
		block.Decrypt(out[i:i+bs], ciphertext[i:i+bs])
	}
	return pkcs7Unpad(out)
}

func pkcs7Pad(data []byte, blockSize int) []byte {
	pad := blockSize - len(data)%blockSize
	return append(data, bytes.Repeat([]byte{byte(pad)}, pad)...)
}

func pkcs7Unpad(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("ilink: pkcs7Unpad: empty")
	}
	pad := int(data[len(data)-1])
	if pad == 0 || pad > aes.BlockSize || pad > len(data) {
		return nil, fmt.Errorf("ilink: pkcs7Unpad: invalid padding %d", pad)
	}
	return data[:len(data)-pad], nil
}
