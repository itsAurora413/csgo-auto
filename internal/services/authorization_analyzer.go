package services

import (
	"encoding/hex"
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"time"
)

// AuthorizationGenerator 生成csqaq.com的authorization token
type AuthorizationGenerator struct {
	userID string // 用户ID: 1143679025
}

// NewAuthorizationGenerator 创建新的生成器
func NewAuthorizationGenerator(userID string) *AuthorizationGenerator {
	return &AuthorizationGenerator{
		userID: userID,
	}
}

// GenerateAuthorization 生成authorization token
// 格式: {part1}-{part2}-{part3}-{part4}-{part5}
//
// 分析结果:
// Part 1: 12字符 (动态生成的随机hex + 随机字母)
// Part 2: 6字符 (随机hex)
// Part 3: 12字符 (随机字母 + userID + 随机字母)
// Part 4: 7字符 (固定为 "1764000" - 可能是过期时间戳)
// Part 5: 7字符 (part1的部分变体)
//
// 样本分析:
// 920a52c7a91s-473d87-x1143679025m-1764000-a20a52c
// 6f2a52c7a91m-4577a4-n1143679025d-1764000-8f2a52c
// ae4a52c7a91t-43e886-h1143679025k-1764000-be4a52c
//
// Part3中包含固定的userID: 1143679025
// Part4固定为: 1764000 (这是未来的时间戳，大约2025年11月份)
func (g *AuthorizationGenerator) GenerateAuthorization() string {
	// Part 1: 生成11位hex + 1位随机字母
	part1Hex := g.randomHex(11)
	part1Letter := g.randomLetter()
	part1 := part1Hex + part1Letter

	// Part 2: 生成6位随机hex
	part2 := g.randomHex(6)

	// Part 3: 随机字母 + userID + 随机字母
	part3Letter1 := g.randomLetter()
	part3Letter2 := g.randomLetter()
	part3 := part3Letter1 + g.userID + part3Letter2

	// Part 4: 固定值 1764000 (未来的时间戳，代表过期时间)
	part4 := "1764000"

	// Part 5: 基于part1生成 (取part1的部分内容变换)
	// 规律: 第一位是随机字母，然后是part1的2-7位
	part5Letter := g.randomLetter()
	part5 := part5Letter + part1[1:7]

	return fmt.Sprintf("%s-%s-%s-%s-%s", part1, part2, part3, part4, part5)
}

// randomHex 生成指定长度的随机hex字符串
func (g *AuthorizationGenerator) randomHex(length int) string {
	bytes := make([]byte, (length+1)/2)
	rand.Read(bytes)
	hex := hex.EncodeToString(bytes)
	return hex[:length]
}

// randomLetter 生成随机小写字母
func (g *AuthorizationGenerator) randomLetter() string {
	letters := "abcdefghijklmnopqrstuvwxyz"
	return string(letters[rand.Intn(len(letters))])
}

// 根据HAR文件样本分析，authorization的生成规律：
//
// 观察样本:
// 920a52c7a91s-473d87-x1143679025m-1764000-a20a52c
// 6f2a52c7a91m-4577a4-n1143679025d-1764000-8f2a52c
// ae4a52c7a91t-43e886-h1143679025k-1764000-be4a52c
// 976a52c7a91u-7949ba-i1143679025z-1764000-a76a52c
// 6c7a52c7a91l-50813f-k1143679025n-1764000-7c7a52c
//
// 关键发现:
// 1. Part3始终包含: 1143679025 (这是用户ID)
// 2. Part4始终为: 1764000 (固定值，可能是过期时间戳秒)
// 3. Part1和Part5有关联:
//    - 920a52c7a91s -> a20a52c (part5是part1的变体)
//    - 6f2a52c7a91m -> 8f2a52c (part5是part1的变体)
// 4. 所有字符都是小写字母和数字

// GenerateAuthorizationV2 更精确的生成方法
// 通过更详细的样本分析发现的规律
func (g *AuthorizationGenerator) GenerateAuthorizationV2() string {
	rand.Seed(time.Now().UnixNano())

	// Part 1: 2位hex + 1位数字 + "a" + 2位hex + "c7a91" + 1位随机字母
	// 观察: 所有样本都包含 "c7a91" 这个固定序列
	// 920a52c7a91s, 6f2a52c7a91m, ae4a52c7a91t
	hex1 := g.randomHex(2)
	digit1 := strconv.Itoa(rand.Intn(10))
	hex2 := g.randomHex(2)
	letter1 := g.randomLetter()
	part1 := hex1 + digit1 + "a" + hex2 + "c7a91" + letter1

	// Part 2: 6位随机hex
	part2 := g.randomHex(6)

	// Part 3: 1位随机字母 + userID + 1位随机字母
	letterPrefix := g.randomLetter()
	letterSuffix := g.randomLetter()
	part3 := letterPrefix + g.userID + letterSuffix

	// Part 4: 固定值
	part4 := "1764000"

	// Part 5: 1位随机字母 + part1的第3-8位 (跳过前2位)
	letter5 := g.randomLetter()
	part5 := letter5 + part1[2:8]

	return fmt.Sprintf("%s-%s-%s-%s-%s", part1, part2, part3, part4, part5)
}

// AnalyzeSample 分析单个authorization样本
func AnalyzeSample(auth string) {
	parts := strings.Split(auth, "-")
	if len(parts) != 5 {
		fmt.Println("Invalid format")
		return
	}

	fmt.Printf("Authorization: %s\n", auth)
	fmt.Printf("Part 1: %s (len=%d)\n", parts[0], len(parts[0]))
	fmt.Printf("Part 2: %s (len=%d)\n", parts[1], len(parts[1]))
	fmt.Printf("Part 3: %s (len=%d)\n", parts[2], len(parts[2]))
	fmt.Printf("Part 4: %s (len=%d)\n", parts[3], len(parts[3]))
	fmt.Printf("Part 5: %s (len=%d)\n", parts[4], len(parts[4]))

	// 检查part3中的userID
	if len(parts[2]) == 12 {
		userID := parts[2][1:11]
		fmt.Printf("Extracted UserID: %s\n", userID)
	}

	// 检查part1和part5的关系
	if len(parts[0]) >= 8 && len(parts[4]) >= 7 {
		fmt.Printf("Part1[2:8]: %s\n", parts[0][2:8])
		fmt.Printf("Part5[1:7]: %s\n", parts[4][1:7])
	}
}
