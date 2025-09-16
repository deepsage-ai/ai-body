package wework

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/xml"
	"fmt"
	"io"
	"sort"
	"strings"
)

// Crypto 企业微信加解密处理器
type Crypto struct {
	token  string
	aesKey []byte
}

// NewCrypto 创建加解密处理器
func NewCrypto(token, encodingAESKey string) (*Crypto, error) {
	if len(encodingAESKey) != 43 {
		return nil, fmt.Errorf("invalid AES key length: expected 43, got %d", len(encodingAESKey))
	}

	// 企业微信的AESKey需要补充一个=进行base64解码
	aesKey, err := base64.StdEncoding.DecodeString(encodingAESKey + "=")
	if err != nil {
		return nil, fmt.Errorf("failed to decode AES key: %w", err)
	}

	return &Crypto{
		token:  token,
		aesKey: aesKey,
	}, nil
}

// VerifyURL 验证URL（GET请求）
func (c *Crypto) VerifyURL(signature, timestamp, nonce, echostr string) (string, error) {
	// 验证签名
	if !c.verifySignature(signature, timestamp, nonce, echostr) {
		return "", fmt.Errorf("signature verification failed")
	}

	// 解密echostr
	decrypted, err := c.decrypt(echostr)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt echostr: %w", err)
	}

	return string(decrypted), nil
}

// DecryptMessage 解密消息（POST请求）
func (c *Crypto) DecryptMessage(signature, timestamp, nonce, encryptedData string) ([]byte, error) {
	// 验证签名
	if !c.verifySignature(signature, timestamp, nonce, encryptedData) {
		return nil, fmt.Errorf("signature verification failed")
	}

	// 解密消息
	return c.decrypt(encryptedData)
}

// EncryptMessage 加密消息（用于回复）
func (c *Crypto) EncryptMessage(message []byte, timestamp, nonce string) (string, error) {
	// 加密消息
	encrypted, err := c.encrypt(message)
	if err != nil {
		return "", fmt.Errorf("failed to encrypt message: %w", err)
	}

	// 生成签名
	signature := c.generateSignature(timestamp, nonce, encrypted)

	// 构造响应XML
	response := EncryptedResponse{
		Encrypt:   encrypted,
		Signature: signature,
		Timestamp: timestamp,
		Nonce:     nonce,
	}

	xmlData, err := xml.Marshal(response)
	if err != nil {
		return "", fmt.Errorf("failed to marshal XML: %w", err)
	}

	return string(xmlData), nil
}

// verifySignature 验证签名
func (c *Crypto) verifySignature(signature, timestamp, nonce, data string) bool {
	expected := c.generateSignature(timestamp, nonce, data)
	return signature == expected
}

// generateSignature 生成签名
func (c *Crypto) generateSignature(timestamp, nonce, data string) string {
	strs := []string{c.token, timestamp, nonce, data}
	sort.Strings(strs)

	h := sha1.New()
	h.Write([]byte(strings.Join(strs, "")))
	return fmt.Sprintf("%x", h.Sum(nil))
}

// decrypt AES解密
func (c *Crypto) decrypt(encryptedData string) ([]byte, error) {
	// Base64解码
	ciphertext, err := base64.StdEncoding.DecodeString(encryptedData)
	if err != nil {
		return nil, fmt.Errorf("failed to decode base64: %w", err)
	}

	// AES解密
	block, err := aes.NewCipher(c.aesKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}

	if len(ciphertext) < aes.BlockSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	iv := ciphertext[:aes.BlockSize]
	ciphertext = ciphertext[aes.BlockSize:]

	mode := cipher.NewCBCDecrypter(block, iv)
	mode.CryptBlocks(ciphertext, ciphertext)

	// 去除PKCS7填充
	plaintext := c.pkcs7Unpad(ciphertext)
	if len(plaintext) < 20 {
		return nil, fmt.Errorf("invalid plaintext length")
	}

	// 提取消息内容
	// 格式：random(16) + msg_len(4) + msg + receiveid
	msgLen := binary.BigEndian.Uint32(plaintext[16:20])
	msg := plaintext[20 : 20+msgLen]

	return msg, nil
}

// encrypt AES加密
func (c *Crypto) encrypt(plaintext []byte) (string, error) {
	// 生成随机数
	random := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, random); err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}

	// 构造消息格式：random(16) + msg_len(4) + msg + receiveid（企业内部机器人为空）
	msgLen := make([]byte, 4)
	binary.BigEndian.PutUint32(msgLen, uint32(len(plaintext)))

	var buffer bytes.Buffer
	buffer.Write(random)
	buffer.Write(msgLen)
	buffer.Write(plaintext)
	// 企业内部智能机器人，receiveid为空

	// PKCS7填充
	data := c.pkcs7Pad(buffer.Bytes(), aes.BlockSize)

	// AES加密
	block, err := aes.NewCipher(c.aesKey)
	if err != nil {
		return "", fmt.Errorf("failed to create AES cipher: %w", err)
	}

	// 生成IV
	iv := make([]byte, aes.BlockSize)
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return "", fmt.Errorf("failed to generate IV: %w", err)
	}

	// 加密
	ciphertext := make([]byte, len(data))
	mode := cipher.NewCBCEncrypter(block, iv)
	mode.CryptBlocks(ciphertext, data)

	// 组合IV和密文
	result := make([]byte, 0, len(iv)+len(ciphertext))
	result = append(result, iv...)
	result = append(result, ciphertext...)

	// Base64编码
	return base64.StdEncoding.EncodeToString(result), nil
}

// pkcs7Pad PKCS7填充
func (c *Crypto) pkcs7Pad(data []byte, blockSize int) []byte {
	padding := blockSize - len(data)%blockSize
	padtext := bytes.Repeat([]byte{byte(padding)}, padding)
	return append(data, padtext...)
}

// pkcs7Unpad PKCS7去填充
func (c *Crypto) pkcs7Unpad(data []byte) []byte {
	length := len(data)
	if length == 0 {
		return data
	}

	padding := int(data[length-1])
	if padding > length || padding == 0 {
		return data
	}

	return data[:length-padding]
}

// EncryptedResponse 加密响应XML结构
type EncryptedResponse struct {
	XMLName   xml.Name `xml:"xml"`
	Encrypt   string   `xml:"Encrypt"`
	Signature string   `xml:"MsgSignature"`
	Timestamp string   `xml:"TimeStamp"`
	Nonce     string   `xml:"Nonce"`
}

// EncryptedRequest 加密请求XML结构
type EncryptedRequest struct {
	XMLName xml.Name `xml:"xml"`
	Encrypt string   `xml:"Encrypt"`
}

// ParseEncryptedXML 解析加密的XML请求
func ParseEncryptedXML(xmlData []byte) (*EncryptedRequest, error) {
	var req EncryptedRequest
	if err := xml.Unmarshal(xmlData, &req); err != nil {
		return nil, fmt.Errorf("failed to parse XML: %w", err)
	}
	return &req, nil
}
