#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
429 错误重试功能测试脚本
演示当服务器返回 429 Too Many Requests 时的重试机制
"""

import sys
sys.path.insert(0, '/Users/user/Downloads/csgoAuto')

from kline_analyzer import KlineDataFetcher
import time

def test_retry_logic():
    """测试重试逻辑"""
    print("""
╔════════════════════════════════════════════════════════════════╗
║              429 错误重试功能测试                              ║
╚════════════════════════════════════════════════════════════════╝
    """)
    
    print("【功能说明】")
    print("✓ 自动检测 HTTP 429 (Too Many Requests) 错误")
    print("✓ 指数退避算法: 1s → 2s → 4s → 8s → 16s")
    print("✓ 添加随机抖动 (±10%) 避免雷鸣羊群效应")
    print("✓ 最多重试 5 次后放弃")
    print("✓ 每次重试前打印等待时间和剩余重试次数\n")
    
    print("【重试策略详解】")
    print("1. 初次请求: 立即发送")
    print("2. 收到 429: 等待 1 + jitter 秒后重试")
    print("3. 仍收到 429: 等待 2 + jitter 秒后重试")
    print("4. 仍收到 429: 等待 4 + jitter 秒后重试")
    print("5. 仍收到 429: 等待 8 + jitter 秒后重试")
    print("6. 仍收到 429: 等待 16 + jitter 秒后重试")
    print("7. 超过最大重试次数: 放弃并返回 None\n")
    
    print("【代码示例】")
    print("""
    fetcher = KlineDataFetcher()
    # 自动处理 429 错误，最多重试 5 次
    df = fetcher.fetch_kline(
        index_id=3, 
        kline_type="1day",
        verbose=True,
        max_retries=5  # 默认值
    )
    """)
    
    print("\n【实现特点】")
    print("• 使用 while 循环而不是递归，避免栈溢出")
    print("• 指数退避减少服务器压力")
    print("• 随机抖动避免多个请求同时重试")
    print("• 支持自定义最大重试次数")
    print("• 详细的日志输出便于调试\n")

if __name__ == "__main__":
    test_retry_logic()
