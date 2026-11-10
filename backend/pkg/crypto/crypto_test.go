package crypto

import (
	"strings"
	"testing"
)

func TestEncryptDecryptRoundTrip(t *testing.T) {
	t.Setenv(EnvKey, "master-key-1")

	enc, err := Encrypt("sk-test-123456")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if !strings.HasPrefix(enc, Prefix) {
		t.Errorf("密文应带 %q 前缀: %q", Prefix, enc)
	}
	if strings.Contains(enc, "sk-test-123456") {
		t.Error("密文不应包含明文")
	}

	plain, err := Decrypt(enc)
	if err != nil || plain != "sk-test-123456" {
		t.Errorf("Decrypt = %q err=%v", plain, err)
	}

	// 相同明文两次加密应产生不同密文(随机 nonce)
	enc2, _ := Encrypt("sk-test-123456")
	if enc == enc2 {
		t.Error("nonce 随机化失效:两次密文相同")
	}

	// 无前缀值原样返回(兼容明文迁移)
	if got, err := Decrypt("plain-value"); err != nil || got != "plain-value" {
		t.Errorf("明文应原样返回: %q %v", got, err)
	}
}

func TestDecryptWrongKey(t *testing.T) {
	t.Setenv(EnvKey, "key-a")
	enc, _ := Encrypt("secret")

	t.Setenv(EnvKey, "key-b")
	if _, err := Decrypt(enc); err == nil {
		t.Fatal("错误主密钥解密应失败")
	}
}

func TestMissingKey(t *testing.T) {
	t.Setenv(EnvKey, "")
	if _, err := Encrypt("x"); err == nil {
		t.Fatal("未配置主密钥应返回错误")
	}
}

func TestIsEncrypted(t *testing.T) {
	if !IsEncrypted("v1:abc") || IsEncrypted("abc") {
		t.Error("IsEncrypted 判断有误")
	}
}
