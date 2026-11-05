package securebox

import (
	"bytes"
	"errors"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	b, err := New("test-secret")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	for _, plain := range []string{"", "skey=@crypt_x", `{"uin":"1","cookies":{"a":"b"}}`} {
		enc, err := b.EncryptString(plain)
		if err != nil {
			t.Fatalf("Encrypt: %v", err)
		}
		if bytes.Contains(enc, []byte(plain)) && plain != "" {
			t.Error("密文里出现了明文")
		}
		got, err := b.DecryptString(enc)
		if err != nil || got != plain {
			t.Fatalf("roundtrip: %q -> %q, %v", plain, got, err)
		}
	}
}

func TestDifferentNonceEachTime(t *testing.T) {
	b, _ := New("s")
	a, _ := b.EncryptString("same")
	c, _ := b.EncryptString("same")
	if bytes.Equal(a, c) {
		t.Error("同一明文两次加密结果相同，nonce 未随机")
	}
}

func TestTamperedCiphertext(t *testing.T) {
	b, _ := New("s")
	enc, _ := b.EncryptString("hello")
	enc[len(enc)-1] ^= 0xFF
	if _, err := b.DecryptString(enc); err == nil {
		t.Error("篡改后的密文应解密失败")
	}
}

func TestNoKey(t *testing.T) {
	if _, err := New(""); !errors.Is(err, ErrNoKey) {
		t.Fatal("空密钥应返回 ErrNoKey")
	}
	var b *Box
	if _, err := b.EncryptString("x"); !errors.Is(err, ErrNoKey) {
		t.Fatal("nil Box 应返回 ErrNoKey")
	}
}
