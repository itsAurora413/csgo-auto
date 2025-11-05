package main

import (
	"context"
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

	// 测试多个template_id - 随机选择一些常见的
	templateIDs := []int{113047, 113048, 113049, 113050, 113040}

	requestList := make([]youpin.BatchPriceQueryItem, len(templateIDs))
	for i, tid := range templateIDs {
		t := tid
		requestList[i] = youpin.BatchPriceQueryItem{TemplateID: &t}
	}

	fmt.Printf("正在批量查询 %d 个商品\n", len(templateIDs))
	fmt.Printf("TemplateID列表: %v\n", templateIDs)
	fmt.Println("=========================================")

	resp, err := client.BatchGetOnSaleCommodityInfo(ctx, requestList)
	if err != nil {
		log.Fatalf("API调用失败: %v", err)
	}

	fmt.Printf("API响应代码: %d\n", resp.Code)
	fmt.Printf("API响应消息: %s\n", resp.Msg)
	fmt.Printf("返回数据个数: %d\n\n", len(resp.Data))

	if resp.Code == 0 && resp.Data != nil {
		for i, item := range resp.Data {
			if item != nil && item.SaleCommodityResponse != nil {
				fmt.Printf("[%d] TemplateID: %d | 商品名: %s | 最低售价: %s | 在售数量: %d\n",
					i,
					item.SaleTemplateResponse.TemplateID,
					item.SaleTemplateResponse.TemplateName,
					item.SaleCommodityResponse.MinSellPrice,
					item.SaleCommodityResponse.SellNum,
				)
			}
		}
	}
}
