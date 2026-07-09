package weixin

import (
	"context"
	"crypto/aes"
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"

	"go.uber.org/zap"
)

type UploadedFileInfo struct {
	FileKey                     string
	DownloadEncryptedQueryParam string
	AESKey                      string
	FileSize                    int
	FileSizeCiphertext          int
}

func pkcs7Pad(data []byte, blockSize int) []byte {
	padding := blockSize - (len(data) % blockSize)
	padBytes := make([]byte, padding)
	for i := range padBytes {
		padBytes[i] = byte(padding)
	}
	return append(data, padBytes...)
}

func pkcs7Unpad(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("weixin crypto: empty data")
	}
	padding := int(data[len(data)-1])
	if padding == 0 || padding > aes.BlockSize || padding > len(data) {
		return nil, fmt.Errorf("weixin crypto: invalid padding")
	}
	for i := len(data) - padding; i < len(data); i++ {
		if data[i] != byte(padding) {
			return nil, fmt.Errorf("weixin crypto: invalid padding bytes")
		}
	}
	return data[:len(data)-padding], nil
}

func encryptAES128ECB(plaintext, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("weixin crypto: new cipher: %w", err)
	}
	padded := pkcs7Pad(plaintext, aes.BlockSize)
	ciphertext := make([]byte, len(padded))
	for i := 0; i < len(padded); i += aes.BlockSize {
		block.Encrypt(ciphertext[i:i+aes.BlockSize], padded[i:i+aes.BlockSize])
	}
	return ciphertext, nil
}

func decryptAES128ECB(ciphertext, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("weixin crypto: new cipher: %w", err)
	}
	if len(ciphertext)%aes.BlockSize != 0 {
		return nil, fmt.Errorf("weixin crypto: ciphertext not block-aligned")
	}
	plaintext := make([]byte, len(ciphertext))
	for i := 0; i < len(ciphertext); i += aes.BlockSize {
		block.Decrypt(plaintext[i:i+aes.BlockSize], ciphertext[i:i+aes.BlockSize])
	}
	return pkcs7Unpad(plaintext)
}

func generateAESKey() ([]byte, error) {
	key := make([]byte, 16)
	if _, err := rand.Read(key); err != nil {
		return nil, fmt.Errorf("weixin crypto: generate key: %w", err)
	}
	return key, nil
}

func (b *WeixinBot) UploadMedia(ctx context.Context, filePath string, toUserID string, mediaType int) (*UploadedFileInfo, error) {
	fileData, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("weixin media: read file: %w", err)
	}

	rawSize := len(fileData)
	hash := md5.Sum(fileData)
	rawMD5 := hex.EncodeToString(hash[:])

	aesKey, err := generateAESKey()
	if err != nil {
		return nil, err
	}

	fileKey := fmt.Sprintf("%x_%d", md5.Sum([]byte(filePath)), rawSize)

	req := &GetUploadUrlReq{
		FileKey:    fileKey,
		MediaType:  mediaType,
		ToUserID:   toUserID,
		RawSize:    rawSize,
		RawFileMD5: rawMD5,
		FileSize:   ((rawSize + aes.BlockSize) / aes.BlockSize) * aes.BlockSize,
		NoNeedThumb: true,
		AESKey:     hex.EncodeToString(aesKey),
	}

	uploadResp, err := b.api.GetUploadURL(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("weixin media: get upload url: %w", err)
	}

	encrypted, err := encryptAES128ECB(fileData, aesKey)
	if err != nil {
		return nil, fmt.Errorf("weixin media: encrypt: %w", err)
	}

	if err := b.api.UploadToCDN(ctx, uploadResp.UploadFullURL, encrypted); err != nil {
		return nil, fmt.Errorf("weixin media: upload cdn: %w", err)
	}

	zap.L().Info("weixin media uploaded",
		zap.String("file_key", fileKey),
		zap.Int("raw_size", rawSize),
		zap.Int("encrypted_size", len(encrypted)))

	return &UploadedFileInfo{
		FileKey:                     fileKey,
		DownloadEncryptedQueryParam: uploadResp.UploadParam,
		AESKey:                     hex.EncodeToString(aesKey),
		FileSize:                   rawSize,
		FileSizeCiphertext:          len(encrypted),
	}, nil
}
