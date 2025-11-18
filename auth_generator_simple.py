#!/usr/bin/env python3
"""
简化版 Authorization 生成器 - 只需传入已知token即可生成新token
"""

import random
import string


def random_hex(length):
    """生成指定长度的随机hex字符串"""
    return ''.join(random.choices('0123456789abcdef', k=length))


def random_letter():
    """生成随机小写字母"""
    return random.choice(string.ascii_lowercase)


def generate_from_known(known_authorization):
    """
    从已知的authorization生成新的authorization
    
    参数:
        known_authorization: 已知的authorization token
        例如: "920a52c7a91s-473d87-x1143679025m-1764000-a20a52c"
    
    返回:
        str: 新的authorization token
    """
    # 解析已知token
    parts = known_authorization.split('-')
    if len(parts) != 5:
        raise ValueError(f"Authorization格式错误，应该有5个部分，实际有{len(parts)}个")
    
    # 提取UserID和Expiry
    part3 = parts[2]
    if len(part3) != 12:
        raise ValueError(f"Part3长度错误，应该是12，实际是{len(part3)}")
    
    user_id = part3[1:11]  # 提取中间10位UserID
    expiry = parts[3]       # 提取过期时间
    
    # 生成新token
    # Part 1: XXXaXXc7a91X
    hex1 = random_hex(2)
    digit = str(random.randint(0, 9))
    hex2 = random_hex(2)
    letter1 = random_letter()
    part1 = f"{hex1}{digit}a{hex2}c7a91{letter1}"

    # Part 2: 6位随机hex
    part2 = random_hex(6)

    # Part 3: X + UserID + X
    letter_prefix = random_letter()
    letter_suffix = random_letter()
    new_part3 = f"{letter_prefix}{user_id}{letter_suffix}"

    # Part 4: 使用提取的expiry
    new_part4 = expiry

    # Part 5: 1位hex + part1[1:7]
    hex5 = random_hex(1)
    part5 = f"{hex5}{part1[1:7]}"

    return f"{part1}-{part2}-{new_part3}-{new_part4}-{part5}"


def batch_generate(known_authorization, count=1):
    """
    批量生成authorization tokens
    
    参数:
        known_authorization: 已知的authorization token
        count: 生成数量（默认1个）
    
    返回:
        list: authorization token列表
    """
    return [generate_from_known(known_authorization) for _ in range(count)]


# 示例用法
if __name__ == "__main__":
    # 你的已知authorization（从浏览器抓包获取）
    known_auth = "920a52c7a91s-473d87-x1143679025m-1764000-a20a52c"
    
    print("=" * 80)
    print("简化版 Authorization 生成器")
    print("=" * 80)
    print()
    print(f"输入的authorization:\n{known_auth}")
    print()
    
    # 生成1个新token
    print("生成1个新token:")
    print("-" * 80)
    new_token = generate_from_known(known_auth)
    print(new_token)
    print()
    
    # 批量生成5个新tokens
    print("批量生成5个新tokens:")
    print("-" * 80)
    tokens = batch_generate(known_auth, count=5)
    for i, token in enumerate(tokens, 1):
        print(f"{i}. {token}")
    print()
    
    print("=" * 80)
    print("在代码中使用:")
    print()
    print("from auth_generator_simple import generate_from_known")
    print()
    print("# 传入你的已知authorization")
    print("known = 'your_known_authorization_here'")
    print("new_auth = generate_from_known(known)")
    print("=" * 80)

