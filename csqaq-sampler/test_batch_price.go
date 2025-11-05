package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	youpin "csqaq-sampler/internal/services/youpin"
)

func main() {
	// 创建OpenAPI客户端
	client, err := youpin.NewOpenAPIClientWithDefaultKeys()
	if err != nil {
		log.Fatalf("创建客户端失败: %v", err)
	}

	ctx := context.Background()

	// 测试template_id = 113047
	templateID := 113047
	requestList := []youpin.BatchPriceQueryItem{
		{
			TemplateID: &templateID,
		},
	}

	fmt.Printf("正在查询 TemplateID: %d\n", templateID)
	fmt.Println("=========================================")

	resp, err := client.BatchGetOnSaleCommodityInfo(ctx, requestList)
	if err != nil {
		log.Fatalf("API调用失败: %v", err)
	}

	fmt.Printf("API响应代码: %d\n", resp.Code)
	fmt.Printf("API响应消息: %s\n", resp.Msg)
	fmt.Println()

	if resp.Code == 0 && resp.Data != nil {
		fmt.Printf("返回数据个数: %d\n", len(resp.Data))
		fmt.Println()

		for i, item := range resp.Data {
			fmt.Printf("=== 商品 %d ===\n", i)
			data, _ := json.MarshalIndent(item, "", "  ")
			fmt.Println(string(data))
		}
	} else {
		fmt.Printf("完整响应:\n%+v\n", resp)
	}
}
