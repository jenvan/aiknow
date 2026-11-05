// Package securebox 用对称密钥加解密小块敏感数据（微信网页会话凭据等）。
//
// 设计取舍：
//   - AES-256-GCM，随机 nonce 前置，输出 nonce||ciphertext；
//   - 密钥由 SESSION_SECRET 经 SHA-256 派生，不额外引入配置项，也不落盘；
//   - 只用于「字段级加密」（几十 KB 以内），不做流式。
//
// 注意：这不是"密钥管理"，只是让凭据不以明文躺在库里/备份里。
// 真正的密钥轮换需要重加密全表，v1 不做。
package securebox

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
)

// ErrNoKey 未提供密钥，加密/解密直接失败（绝不静默降级为明文）。
var ErrNoKey = errors.New("securebox: 未配置密钥")

// Box 加解密器。
type Box struct {
	aead cipher.AEAD
}

// New 从任意长度的 secret 派生密钥；secret 为空返回 ErrNoKey。
func New(secret string) (*Box, error) {
	if secret == "" {
		return nil, ErrNoKey
	}
	key := sha256.Sum256([]byte(secret))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Box{aead: aead}, nil
}

// Encrypt 加密，返回可直接存 BYTEA 的密文。
func (b *Box) Encrypt(plain []byte) ([]byte, error) {
	if b == nil || b.aead == nil {
		return nil, ErrNoKey
	}
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return b.aead.Seal(nonce, nonce, plain, nil), nil
}

// Decrypt 解密。
func (b *Box) Decrypt(ciphertext []byte) ([]byte, error) {
	if b == nil || b.aead == nil {
		return nil, ErrNoKey
	}
	ns := b.aead.NonceSize()
	if len(ciphertext) < ns {
		return nil, fmt.Errorf("securebox: 密文过短")
	}
	return b.aead.Open(nil, ciphertext[:ns], ciphertext[ns:], nil)
}

// EncryptString 便于存字符串字段。
func (b *Box) EncryptString(s string) ([]byte, error) { return b.Encrypt([]byte(s)) }

// DecryptString 解密为字符串。
func (b *Box) DecryptString(c []byte) (string, error) {
	plain, err := b.Decrypt(c)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}
