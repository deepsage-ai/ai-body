package wework

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"time"
)

// 错误码定义（对应Python的ierror）
const (
	WXBizMsgCrypt_OK                      = 0
	WXBizMsgCrypt_ValidateSignature_Error = -40001
	WXBizMsgCrypt_ParseJson_Error         = -40002
	WXBizMsgCrypt_ComputeSignature_Error  = -40003
	WXBizMsgCrypt_IllegalAesKey           = -40004
	WXBizMsgCrypt_ValidateCorpid_Error    = -40005
	WXBizMsgCrypt_EncryptAES_Error        = -40006
	WXBizMsgCrypt_DecryptAES_Error        = -40007
	WXBizMsgCrypt_IllegalBuffer           = -40008
	WXBizMsgCrypt_EncodeBase64_Error      = -40009
	WXBizMsgCrypt_DecodeBase64_Error      = -40010
)

// PKCS7Encoder PKCS7填充算法实现
type PKCS7Encoder struct {
	BlockSize int
}

// NewPKCS7Encoder 创建PKCS7编码器
func NewPKCS7Encoder() *PKCS7Encoder {
	return &PKCS7Encoder{BlockSize: 32} // 对应Python的block_size = 32
}

// Encode 对明文进行PKCS7填充
func (p *PKCS7Encoder) Encode(text []byte) []byte {
	textLength := len(text)
	amountToPad := p.BlockSize - (textLength % p.BlockSize)
	if amountToPad == 0 {
		amountToPad = p.BlockSize
	}

	// 填充字节
	padBytes := make([]byte, amountToPad)
	for i := range padBytes {
		padBytes[i] = byte(amountToPad)
	}

	return append(text, padBytes...)
}

// Decode 移除PKCS7填充
func (p *PKCS7Encoder) Decode(text []byte) []byte {
	if len(text) == 0 {
		return text
	}

	pad := int(text[len(text)-1])
	if pad < 1 || pad > 32 {
		pad = 0
	}

	if pad > len(text) {
		return text
	}

	return text[:len(text)-pad]
}

// Prpcrypt AES加解密实现
type Prpcrypt struct {
	Key  []byte
	Mode cipher.BlockMode
}

// NewPrpcrypt 创建加解密器
func NewPrpcrypt(key []byte) *Prpcrypt {
	return &Prpcrypt{Key: key}
}

// getRandomStr 生成16位随机字符串（对应Python的get_random_str）
func (p *Prpcrypt) getRandomStr() ([]byte, error) {
	// Python: return str(random.randint(1000000000000000, 9999999999999999)).encode()
	min := big.NewInt(1000000000000000)
	max := big.NewInt(9999999999999999)

	n, err := rand.Int(rand.Reader, new(big.Int).Sub(max, min))
	if err != nil {
		return nil, err
	}

	n.Add(n, min)
	return []byte(n.String()), nil
}

// Encrypt 加密消息（对应Python的encrypt方法）
func (p *Prpcrypt) Encrypt(text, receiveID string) (int, []byte, error) {
	// 1. 生成16位随机字符串
	randomStr, err := p.getRandomStr()
	if err != nil {
		return WXBizMsgCrypt_EncryptAES_Error, nil, err
	}

	// 2. 构造消息格式：16位随机字符串 + 4字节长度 + 消息内容 + receiveid
	textBytes := []byte(text)
	receiveIDBytes := []byte(receiveID)

	// 4字节长度（大端序）
	lengthBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(lengthBytes, uint32(len(textBytes)))

	// 组合消息
	message := make([]byte, 0, len(randomStr)+4+len(textBytes)+len(receiveIDBytes))
	message = append(message, randomStr...)
	message = append(message, lengthBytes...)
	message = append(message, textBytes...)
	message = append(message, receiveIDBytes...)

	// 3. PKCS7填充
	pkcs7 := NewPKCS7Encoder()
	paddedMessage := pkcs7.Encode(message)

	// 4. AES-CBC加密（IV使用密钥前16位）
	block, err := aes.NewCipher(p.Key)
	if err != nil {
		return WXBizMsgCrypt_EncryptAES_Error, nil, err
	}

	iv := p.Key[:16] // 使用密钥前16位作为IV
	mode := cipher.NewCBCEncrypter(block, iv)

	ciphertext := make([]byte, len(paddedMessage))
	mode.CryptBlocks(ciphertext, paddedMessage)

	// 5. Base64编码
	encoded := base64.StdEncoding.EncodeToString(ciphertext)

	return WXBizMsgCrypt_OK, []byte(encoded), nil
}

// Decrypt 解密消息（对应Python的decrypt方法）
func (p *Prpcrypt) Decrypt(encryptedText, receiveID string) (int, string, error) {
	// 1. Base64解码
	ciphertext, err := base64.StdEncoding.DecodeString(encryptedText)
	if err != nil {
		return WXBizMsgCrypt_DecryptAES_Error, "", err
	}

	// 2. AES-CBC解密
	block, err := aes.NewCipher(p.Key)
	if err != nil {
		return WXBizMsgCrypt_DecryptAES_Error, "", err
	}

	iv := p.Key[:16] // 使用密钥前16位作为IV
	mode := cipher.NewCBCDecrypter(block, iv)

	plaintext := make([]byte, len(ciphertext))
	mode.CryptBlocks(plaintext, ciphertext)

	// 3. 移除PKCS7填充
	pkcs7 := NewPKCS7Encoder()
	unpaddedText := pkcs7.Decode(plaintext)

	if len(unpaddedText) < 20 { // 至少需要16字节随机字符串 + 4字节长度
		return WXBizMsgCrypt_IllegalBuffer, "", fmt.Errorf("解密后数据长度不足")
	}

	// 4. 解析消息格式
	// 跳过16位随机字符串
	content := unpaddedText[16:]

	// 读取4字节长度
	if len(content) < 4 {
		return WXBizMsgCrypt_IllegalBuffer, "", fmt.Errorf("无法读取消息长度")
	}

	jsonLen := binary.BigEndian.Uint32(content[:4])
	content = content[4:]

	if len(content) < int(jsonLen) {
		return WXBizMsgCrypt_IllegalBuffer, "", fmt.Errorf("消息长度不匹配")
	}

	// 提取JSON内容
	jsonContent := string(content[:jsonLen])

	// 提取receiveID
	fromReceiveID := string(content[jsonLen:])

	// 5. 验证receiveID
	if fromReceiveID != receiveID {
		// receiveID不匹配
		return WXBizMsgCrypt_ValidateCorpid_Error, "", fmt.Errorf("receiveID验证失败")
	}

	return WXBizMsgCrypt_OK, jsonContent, nil
}

// SHA1Helper SHA1签名计算辅助类
type SHA1Helper struct{}

// GetSHA1 计算SHA1签名（对应Python的getSHA1）
func (s *SHA1Helper) GetSHA1(token, timestamp, nonce, encrypt string) (int, string, error) {
	// 对应Python的sortlist = [str(token), str(timestamp), str(nonce), str(encrypt)]
	sortList := []string{token, timestamp, nonce, encrypt}
	sort.Strings(sortList) // 排序

	// 连接字符串并计算SHA1
	combined := strings.Join(sortList, "")
	h := sha1.New()
	h.Write([]byte(combined))
	hash := fmt.Sprintf("%x", h.Sum(nil))

	return WXBizMsgCrypt_OK, hash, nil
}

// JsonHelper JSON消息解析和生成辅助类
type JsonHelper struct{}

// ResponseTemplate JSON响应模板
const ResponseTemplate = `{
    "encrypt": "%s",
    "msgsignature": "%s", 
    "timestamp": "%s",
    "nonce": "%s"
}`

// Extract 从JSON中提取加密消息（对应Python的extract）
func (j *JsonHelper) Extract(jsonText string) (int, string, error) {
	var jsonDict map[string]interface{}
	if err := json.Unmarshal([]byte(jsonText), &jsonDict); err != nil {
		return WXBizMsgCrypt_ParseJson_Error, "", err
	}

	encrypt, ok := jsonDict["encrypt"].(string)
	if !ok {
		return WXBizMsgCrypt_ParseJson_Error, "", fmt.Errorf("encrypt字段不存在或类型错误")
	}

	return WXBizMsgCrypt_OK, encrypt, nil
}

// Generate 生成JSON响应（对应Python的generate）
func (j *JsonHelper) Generate(encrypt, signature, timestamp, nonce string) string {
	return fmt.Sprintf(ResponseTemplate, encrypt, signature, timestamp, nonce)
}

// WXBizJsonMsgCrypt 企业微信消息加解密主类（对应Python的WXBizJsonMsgCrypt）
type WXBizJsonMsgCrypt struct {
	Token     string
	Key       []byte
	ReceiveID string
}

// NewWXBizJsonMsgCrypt 创建加解密实例
func NewWXBizJsonMsgCrypt(token, encodingAESKey, receiveID string) (*WXBizJsonMsgCrypt, error) {
	// 对应Python的：self.key = base64.b64decode(sEncodingAESKey+"=")
	key, err := base64.StdEncoding.DecodeString(encodingAESKey + "=")
	if err != nil {
		return nil, fmt.Errorf("EncodingAESKey解码失败: %v", err)
	}

	if len(key) != 32 {
		return nil, fmt.Errorf("EncodingAESKey长度必须为32字节，实际为%d字节", len(key))
	}

	return &WXBizJsonMsgCrypt{
		Token:     token,
		Key:       key,
		ReceiveID: receiveID,
	}, nil
}

// VerifyURL URL验证（对应Python的VerifyURL）
func (w *WXBizJsonMsgCrypt) VerifyURL(msgSignature, timestamp, nonce, echoStr string) (int, string, error) {
	// 1. 计算签名
	sha1Helper := &SHA1Helper{}
	ret, signature, err := sha1Helper.GetSHA1(w.Token, timestamp, nonce, echoStr)
	if ret != WXBizMsgCrypt_OK {
		return ret, "", err
	}

	// 2. 验证签名
	if signature != msgSignature {
		return WXBizMsgCrypt_ValidateSignature_Error, "", fmt.Errorf("签名验证失败")
	}

	// 3. 解密echoStr
	pc := NewPrpcrypt(w.Key)
	ret, replyEchoStr, err := pc.Decrypt(echoStr, w.ReceiveID)

	return ret, replyEchoStr, err
}

// EncryptMsg 加密消息（对应Python的EncryptMsg）
func (w *WXBizJsonMsgCrypt) EncryptMsg(replyMsg, nonce string, timestamp *string) (int, string, error) {
	// 1. 如果timestamp为空，使用当前时间
	var ts string
	if timestamp == nil {
		ts = strconv.FormatInt(time.Now().Unix(), 10)
	} else {
		ts = *timestamp
	}

	// 2. 加密消息
	pc := NewPrpcrypt(w.Key)
	ret, encryptBytes, err := pc.Encrypt(replyMsg, w.ReceiveID)
	if ret != WXBizMsgCrypt_OK {
		return ret, "", err
	}

	encrypt := string(encryptBytes)

	// 3. 生成签名
	sha1Helper := &SHA1Helper{}
	ret, signature, err := sha1Helper.GetSHA1(w.Token, ts, nonce, encrypt)
	if ret != WXBizMsgCrypt_OK {
		return ret, "", err
	}

	// 4. 生成JSON响应
	jsonHelper := &JsonHelper{}
	response := jsonHelper.Generate(encrypt, signature, ts, nonce)

	return WXBizMsgCrypt_OK, response, nil
}

// DecryptMsg 解密消息（对应Python的DecryptMsg）
func (w *WXBizJsonMsgCrypt) DecryptMsg(postData, msgSignature, timestamp, nonce string) (int, string, error) {
	// 1. 从JSON中提取加密数据
	jsonHelper := &JsonHelper{}
	ret, encrypt, err := jsonHelper.Extract(postData)
	if ret != WXBizMsgCrypt_OK {
		return ret, "", err
	}

	// 2. 验证签名
	sha1Helper := &SHA1Helper{}
	ret, signature, err := sha1Helper.GetSHA1(w.Token, timestamp, nonce, encrypt)
	if ret != WXBizMsgCrypt_OK {
		return ret, "", err
	}

	if signature != msgSignature {
		// 签名验证失败
		return WXBizMsgCrypt_ValidateSignature_Error, "", fmt.Errorf("签名验证失败")
	}

	// 3. 解密消息
	pc := NewPrpcrypt(w.Key)
	ret, jsonContent, err := pc.Decrypt(encrypt, w.ReceiveID)

	return ret, jsonContent, err
}
