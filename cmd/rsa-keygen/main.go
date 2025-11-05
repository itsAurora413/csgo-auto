package main

import (
	youpin "csgo-trader/internal/services/youpin"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
)

// 购买信息结构
type PurchasingInfo struct {
	CommodityID    int     `json:"commodityId"`
	CommodityPrice float64 `json:"commodityPrice"`
	TradeLinks     string  `json:"tradeLinks"`
}

// 生成密钥文件
func generateKeys(outputDir string) error {
	// 生成Base64格式的密钥对
	publicKeyStr, privateKeyStr, err := youpin.GenerateKeyPair()
	if err != nil {
		log.Printf("生成密钥对失败: %v\n", err)
		return err
	}

	// 创建输出目录
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		log.Printf("创建输出目录失败: %v\n", err)
		return err
	}

	// 保存Base64格式的密钥
	publicKeyFile := fmt.Sprintf("%s/public_key_base64.txt", outputDir)
	privateKeyFile := fmt.Sprintf("%s/private_key_base64.txt", outputDir)

	if err := os.WriteFile(publicKeyFile, []byte(publicKeyStr), 0644); err != nil {
		log.Printf("保存公钥失败: %v\n", err)
		return err
	}
	log.Printf("公钥已保存到: %s\n", publicKeyFile)

	if err := os.WriteFile(privateKeyFile, []byte(privateKeyStr), 0600); err != nil {
		log.Printf("保存私钥失败: %v\n", err)
		return err
	}
	log.Printf("私钥已保存到: %s\n", privateKeyFile)

	log.Println("\n密钥生成成功！")
	log.Println("公钥format: PKIX (X509)")
	log.Println("私钥format: PKCS8")
	log.Printf("\n公钥: %s\n", publicKeyStr[:80]+"...")
	log.Printf("私钥: %s\n", privateKeyStr[:80]+"...")

	return nil
}

// 测试签名功能
func testSign(privateKeyFile, appKey string) error {
	// 读取私钥
	privateKeyBytes, err := os.ReadFile(privateKeyFile)
	if err != nil {
		log.Printf("读取私钥文件失败: %v\n", err)
		return err
	}
	privateKeyStr := string(privateKeyBytes)

	// 创建签名器
	signer, err := youpin.NewRSASigner(privateKeyStr, appKey)
	if err != nil {
		log.Printf("创建签名器失败: %v\n", err)
		return err
	}

	// 准备签名参数
	params := map[string]interface{}{
		"timestamp":    "2023-12-05 16:15:00",
		"idempotentId": "202212050001",
		"purchasingInfoList": []PurchasingInfo{
			{
				CommodityID:    28347880,
				CommodityPrice: 0.12,
				TradeLinks:     "https://steamcommunity.com/tradeoffer/new/?partner=12345678912&token=LBPW679",
			},
		},
	}

	// 对参数排序并生成签名字符串（用于展示）
	stringBuilder := buildSignString(params)
	log.Printf("待签名的字符串: %s\n\n", stringBuilder)

	// 使用签名器进行签名
	signature, err := signer.SignParams(params)
	if err != nil {
		log.Printf("签名失败: %v\n", err)
		return err
	}

	log.Printf("签名成功:\n%s\n", signature)

	// 验证签名的长度（RSA-2048 的签名经过 Base64 编码通常是 344 字符）
	log.Printf("\n签名长度: %d 字符\n", len(signature))

	return nil
}

// 构建签名字符串（按照文档要求排序）
func buildSignString(params map[string]interface{}) string {
	// 获取所有key并排序
	keys := make([]string, 0)
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// 构建签名字符串
	stringBuilder := ""
	for _, key := range keys {
		value := params[key]
		// 将值序列化为JSON
		jsonValue, err := json.Marshal(value)
		if err != nil {
			log.Printf("JSON序列化失败: %v\n", err)
			continue
		}
		stringBuilder += key + string(jsonValue)
	}

	return stringBuilder
}

func main() {
	generateCmd := flag.NewFlagSet("generate", flag.ExitOnError)
	generateOutput := generateCmd.String("output", "./rsa_keys", "输出目录")

	testCmd := flag.NewFlagSet("test", flag.ExitOnError)
	testPrivateKeyFile := testCmd.String("privatekey", "./rsa_keys/private_key_base64.txt", "私钥文件路径")
	testAppKey := testCmd.String("appkey", "123456", "悠悠有品 AppKey")

	if len(os.Args) < 2 {
		fmt.Println("RSA 密钥生成和签名工具")
		fmt.Println("\n使用方法:")
		fmt.Println("  生成密钥: go run main.go generate -output <输出目录>")
		fmt.Println("  测试签名: go run main.go test -privatekey <私钥文件路径> -appkey <AppKey>")
		fmt.Println("\n示例:")
		fmt.Println("  go run main.go generate -output ./rsa_keys")
		fmt.Println("  go run main.go test -privatekey ./rsa_keys/private_key_base64.txt -appkey 123456")
		os.Exit(1)
	}

	switch os.Args[1] {
	case "generate":
		generateCmd.Parse(os.Args[2:])
		log.Println("==================================================")
		log.Println("开始生成RSA密钥对...")
		log.Println("==================================================")
		if err := generateKeys(*generateOutput); err != nil {
			log.Fatalf("生成密钥失败: %v\n", err)
		}
		log.Println("密钥对生成完成!")

	case "test":
		testCmd.Parse(os.Args[2:])
		log.Println("==================================================")
		log.Println("开始测试签名功能...")
		log.Println("==================================================")
		if err := testSign(*testPrivateKeyFile, *testAppKey); err != nil {
			log.Fatalf("测试签名失败: %v\n", err)
		}
		log.Println("\n测试完成!")

	default:
		fmt.Println("未知命令:", os.Args[1])
		os.Exit(1)
	}
}
