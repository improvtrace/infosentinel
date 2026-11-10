// Package crypto 提供敏感数据静态加密:AES-256-GCM。
//
// 主密钥来自环境变量 INFOSENTINEL_ENCRYPT_KEY(任意字符串,经 SHA-256 派生)。
// 密文格式:"v1:" + base64(nonce || ciphertext)。明文永不落库;
// 未配置主密钥时,加密/解密操作返回明确错误(不静默降级)。
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// EnvKey 是主密钥的环境变量名。
const EnvKey = "INFOSENTINEL_ENCRYPT_KEY"

// Prefix 是密文前缀,用于识别已加密值。
const Prefix = "v1:"

var errNoKey = errors.New("未设置环境变量 INFOSENTINEL_ENCRYPT_KEY:敏感数据加密需要主密钥")

func derive() ([]byte, error) {
	master := os.Getenv(EnvKey)
	if master == "" {
		return nil, errNoKey
	}
	sum := sha256.Sum256([]byte("infosentinel:" + master))
	return sum[:], nil
}

// Encrypt 加密明文;输出带 v1: 前缀的 base64 密文。
func Encrypt(plain string) (string, error) {
	key, err := derive()
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ct := gcm.Seal(nonce, nonce, []byte(plain), nil)
	return Prefix + base64.StdEncoding.EncodeToString(ct), nil
}

// Decrypt 解密带 v1: 前缀的密文;无前缀的值原样返回(兼容明文)。
func Decrypt(enc string) (string, error) {
	if !strings.HasPrefix(enc, Prefix) {
		return enc, nil
	}
	key, err := derive()
	if err != nil {
		return "", err
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(enc, Prefix))
	if err != nil {
		return "", fmt.Errorf("密文 base64 解码失败: %w", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize() {
		return "", errors.New("密文过短")
	}
	nonce, ct := raw[:gcm.NonceSize()], raw[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", fmt.Errorf("解密失败(主密钥不匹配?): %w", err)
	}
	return string(plain), nil
}

// IsEncrypted 判断值是否为密文格式。
func IsEncrypted(s string) bool {
	return strings.HasPrefix(s, Prefix)
}
