package aes

import (
	"encoding/base64"
	"errors"
)

const (
	// DefaultKeyStr 默认key
	DefaultKeyStr = "o2GpJzzCaCkjbs9cX9UERixQa91hOXSNB+68HyS/Awc="
	// DefaultNonceStr 默认nonce
	DefaultNonceStr = "1UhNSl7jcvUk1d8jYD5Bag=="
)

var (
	// key = []byte("CzV6QgcfVh5gKgJOM/ExJg==")
	// key 32位秘钥
	key = []byte(DefaultKeyStr)
	// nonce 16位nonce
	nonce = []byte(DefaultNonceStr)
)

// InitKey 使用配置的key替换默认的key
func InitKey(keyP, nonceP string) {
	if len(keyP) != 0 {
		key = []byte(keyP)
	}
	if len(nonceP) != 0 {
		nonce = []byte(nonceP)
	}
}

const (
	// CategoryGCM 使用gcm
	CategoryGCM = "gcm"
)

var (
	// ErrNotSupportCategory 不支持的加密算法
	ErrNotSupportCategory = errors.New("不支持的Aes加密算法")
	// ErrNotSupportNonceLen 不支持的GCM nonce长度
	ErrNotSupportNonceLen = errors.New("不支持的Aes加密算法长度(只支持12B～16B)")
)

// aesCrypto Aes加密
type aesCrypto struct {
	// gcm等算法需要Nonce, 一般是随机的，但是也可以自己指定，不建议使用
	nonce string
	// 秘钥,允许用户自定义
	key string
}

// InitAesCrypto 初始化Aes加密工具
func InitAesCrypto() *aesCrypto {
	return &aesCrypto{
		key:   string(key),
		nonce: string(nonce),
	}
}

func (ac *aesCrypto) Nonce(nonce string) *aesCrypto {
	ac.nonce = nonce
	return ac
}

func (ac *aesCrypto) Key(key string) *aesCrypto {
	ac.key = key
	return ac
}

// EncryptByAes 使用AES算法加密
func (ac *aesCrypto) Encrypt(password string) (result string, err error) {
	if password == "" {
		return password, nil
	}

	key, _ := base64.StdEncoding.DecodeString(ac.key)

	nonce, err := base64.StdEncoding.DecodeString(ac.nonce)
	if err != nil {
		return password, err
	}
	return GcmEncrypt(key, nonce, password)

}

// DecryptByAes 使用AES算法解密
func (ac *aesCrypto) Decrypt(password string) (result string, err error) {
	if password == "" {
		return password, nil
	}

	key, _ := base64.StdEncoding.DecodeString(ac.key)
	ciphertext, err := base64.StdEncoding.DecodeString(password)
	if err != nil {
		return password, err
	}

	nonce, err := base64.StdEncoding.DecodeString(ac.nonce)
	if err != nil {
		return password, err
	}
	return GcmDecrypt(key, nonce, ciphertext)
}
