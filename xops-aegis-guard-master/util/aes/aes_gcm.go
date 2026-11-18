package aes

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/base64"
)

// GcmEncrypt 加密
func GcmEncrypt(key []byte, nonce []byte, plaintext string) (ciphertext string, err error) {
	if len(nonce) > 16 {
		return plaintext, ErrNotSupportNonceLen
	}

	plaintextBytes := []byte(plaintext)

	block, err := aes.NewCipher(key)
	if err != nil {
		return plaintext, err
	}

	aesgcm, err := cipher.NewGCMWithNonceSize(block, len(nonce))
	if err != nil {
		return plaintext, err
	}

	result := aesgcm.Seal(nil, nonce, plaintextBytes, nil)
	ciphertext = base64.StdEncoding.EncodeToString(result)
	return
}

// GcmDecrypt 解密
func GcmDecrypt(key, nonce, ciphertext []byte) (plaintext string, err error) {
	if len(nonce) > 16 {
		return string(ciphertext), ErrNotSupportNonceLen
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return string(ciphertext), err
	}

	aesgcm, err := cipher.NewGCMWithNonceSize(block, len(nonce))
	if err != nil {
		return string(ciphertext), err
	}

	plaintextBytes, err := aesgcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return string(ciphertext), err
	}

	plaintext = string(plaintextBytes)
	return
}
