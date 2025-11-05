package youpin

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// RSASigner RSA签名器 - 用于悠悠有品开放平台API签名
type RSASigner struct {
	privateKey *rsa.PrivateKey
	appKey     string
}

// NewRSASigner 创建RSA签名器
// privateKeyBase64: Base64编码的PKCS8格式私钥
// appKey: 悠悠有品分配的AppKey
func NewRSASigner(privateKeyBase64 string, appKey string) (*RSASigner, error) {
	// 1. Base64解码私钥
	privateKeyBytes, err := base64.StdEncoding.DecodeString(privateKeyBase64)
	if err != nil {
		return nil, fmt.Errorf("解码私钥失败: %w (私钥长度: %d字节)", err, len(privateKeyBase64))
	}

	// 2. 解析PKCS8格式私钥
	privateKey, err := x509.ParsePKCS8PrivateKey(privateKeyBytes)
	if err != nil {
		return nil, fmt.Errorf("解析私钥失败: %w", err)
	}

	// 3. 类型断言为RSA私钥
	rsaPrivateKey, ok := privateKey.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("私钥不是RSA类型")
	}

	return &RSASigner{
		privateKey: rsaPrivateKey,
		appKey:     appKey,
	}, nil
}

// SignParams 对请求参数进行签名
// params: 请求参数（不包含sign字段）
// 返回: Base64编码的签名字符串
func (s *RSASigner) SignParams(params map[string]interface{}) (string, error) {
	// 1. 移除sign字段（如果存在）
	delete(params, "sign")

	// 2. 按照key的ASCII码排序
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// 3. 拼接字符串: key + JSON(value)
	var builder strings.Builder
	for _, key := range keys {
		value := params[key]
		if value == nil || value == "" {
			continue
		}

		// 将value转为JSON字符串
		valueJSON, err := json.Marshal(value)
		if err != nil {
			return "", fmt.Errorf("序列化参数 %s 失败: %w", key, err)
		}

		builder.WriteString(key)
		builder.Write(valueJSON)
	}

	signString := builder.String()

	// 4. 使用SHA256withRSA签名
	hashed := sha256.Sum256([]byte(signString))
	signature, err := rsa.SignPKCS1v15(rand.Reader, s.privateKey, crypto.SHA256, hashed[:])
	if err != nil {
		return "", fmt.Errorf("签名失败: %w", err)
	}

	// 5. Base64编码签名结果
	return base64.StdEncoding.EncodeToString(signature), nil
}

// AddSignatureToParams 为请求参数添加签名
// 自动添加appKey、timestamp、sign字段
func (s *RSASigner) AddSignatureToParams(params map[string]interface{}, timestamp string) error {
	// 1. 添加公共参数
	params["appKey"] = s.appKey
	params["timestamp"] = timestamp

	// 2. 生成签名
	sign, err := s.SignParams(params)
	if err != nil {
		return err
	}

	// 3. 添加签名到参数
	params["sign"] = sign

	return nil
}

// GetAppKey 获取AppKey
func (s *RSASigner) GetAppKey() string {
	return s.appKey
}

// GenerateKeyPair 生成RSA密钥对（用于初始化）
// 返回: Base64编码的公钥和私钥（PKCS8格式）
func GenerateKeyPair() (publicKey string, privateKey string, err error) {
	// 生成2048位RSA密钥对
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", "", fmt.Errorf("生成密钥对失败: %w", err)
	}

	// 导出私钥（PKCS8格式）
	privateKeyBytes, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return "", "", fmt.Errorf("导出私钥失败: %w", err)
	}

	// 导出公钥（PKIX格式）
	publicKeyBytes, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return "", "", fmt.Errorf("导出公钥失败: %w", err)
	}

	// Base64编码
	publicKeyBase64 := base64.StdEncoding.EncodeToString(publicKeyBytes)
	privateKeyBase64 := base64.StdEncoding.EncodeToString(privateKeyBytes)

	return publicKeyBase64, privateKeyBase64, nil
}
