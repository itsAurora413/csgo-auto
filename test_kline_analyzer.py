#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
K线分析系统测试脚本
测试所有分析模块的功能
"""

import numpy as np
import pandas as pd
from datetime import datetime, timedelta
import sys

# 导入分析模块
try:
    from kline_analyzer import (
        TechnicalAnalysis,
        StatisticalAnalysis,
        TrendAnalysis,
        PredictiveAnalysis,
        AnalysisReporter
    )
except ImportError as e:
    print(f"❌ 导入失败: {e}")
    print("请确保 kline_analyzer.py 在当前目录")
    sys.exit(1)


def generate_sample_kline_data(days=100):
    """
    生成模拟K线数据用于测试
    """
    np.random.seed(42)
    
    # 基础价格
    base_price = 1000
    returns = np.random.normal(0.001, 0.02, days)
    prices = base_price * np.exp(np.cumsum(returns))
    
    # 生成OHLCV数据
    data = []
    for i in range(days):
        open_price = prices[i] + np.random.normal(0, 5)
        close_price = prices[i] + np.random.normal(0, 5)
        high_price = max(open_price, close_price) + np.random.uniform(0, 10)
        low_price = min(open_price, close_price) - np.random.uniform(0, 10)
        volume = np.random.randint(1000, 10000)
        
        timestamp = datetime.now() - timedelta(days=days-i-1)
        
        data.append({
            'timestamp': timestamp,
            'open': open_price,
            'close': close_price,
            'high': high_price,
            'low': low_price,
            'volume': volume
        })
    
    df = pd.DataFrame(data)
    df = df.sort_values('timestamp').reset_index(drop=True)
    return df


def test_technical_analysis(df):
    """测试技术分析模块"""
    print("\n" + "="*70)
    print("【测试1】技术分析模块")
    print("="*70)
    
    close = df['close']
    high = df['high']
    low = df['low']
    
    try:
        # 测试MA
        ma5 = TechnicalAnalysis.moving_average(close, 5)
        print(f"✅ MA5 计算成功: {ma5.iloc[-1]:.2f}")
        
        # 测试EMA
        ema12 = TechnicalAnalysis.exponential_moving_average(close, 12)
        print(f"✅ EMA12 计算成功: {ema12.iloc[-1]:.2f}")
        
        # 测试MACD
        macd, signal, hist = TechnicalAnalysis.macd(close)
        print(f"✅ MACD 计算成功: MACD={macd.iloc[-1]:.6f}, Signal={signal.iloc[-1]:.6f}")
        
        # 测试RSI
        rsi = TechnicalAnalysis.rsi(close, 14)
        print(f"✅ RSI 计算成功: {rsi.iloc[-1]:.2f}")
        
        # 测试布林带
        upper, mid, lower = TechnicalAnalysis.bollinger_bands(close, 20, 2)
        print(f"✅ 布林带计算成功: 上={upper.iloc[-1]:.2f}, 中={mid.iloc[-1]:.2f}, 下={lower.iloc[-1]:.2f}")
        
        # 测试ATR
        atr = TechnicalAnalysis.atr(high, low, close, 14)
        print(f"✅ ATR 计算成功: {atr.iloc[-1]:.2f}")
        
        # 测试随机指标
        k, d = TechnicalAnalysis.stochastic_oscillator(high, low, close, 14)
        print(f"✅ 随机指标计算成功: K={k.iloc[-1]:.2f}, D={d.iloc[-1]:.2f}")
        
        return True
    except Exception as e:
        print(f"❌ 技术分析测试失败: {e}")
        return False


def test_statistical_analysis(df):
    """测试统计分析模块"""
    print("\n" + "="*70)
    print("【测试2】统计分析模块")
    print("="*70)
    
    close = df['close']
    
    try:
        # 测试收益率
        returns = StatisticalAnalysis.calculate_returns(close)
        print(f"✅ 收益率计算成功: 平均={returns.mean()*100:.4f}%, 标准差={returns.std()*100:.4f}%")
        
        # 测试波动率
        vol = StatisticalAnalysis.volatility(returns)
        print(f"✅ 年化波动率计算成功: {vol*100:.2f}%")
        
        # 测试偏度和峰度
        skew, kurt = StatisticalAnalysis.skewness_kurtosis(returns)
        print(f"✅ 偏度/峰度计算成功: 偏度={skew:.4f}, 峰度={kurt:.4f}")
        
        # 测试自相关
        acf = StatisticalAnalysis.autocorrelation(returns, 1)
        print(f"✅ 自相关计算成功: {acf:.4f}")
        
        # 测试最大回撤
        dd, max_dd = StatisticalAnalysis.draw_down(close)
        print(f"✅ 最大回撤计算成功: {max_dd*100:.2f}%")
        
        # 测试夏普比率
        sharpe = StatisticalAnalysis.sharpe_ratio(returns)
        print(f"✅ 夏普比率计算成功: {sharpe:.4f}")
        
        return True
    except Exception as e:
        print(f"❌ 统计分析测试失败: {e}")
        return False


def test_trend_analysis(df):
    """测试趋势分析模块"""
    print("\n" + "="*70)
    print("【测试3】趋势分析模块")
    print("="*70)
    
    close = df['close']
    high = df['high']
    low = df['low']
    
    try:
        # 测试线性回归趋势
        trend_info = TrendAnalysis.linear_regression_trend(close)
        print(f"✅ 线性趋势计算成功: 斜率={trend_info['slope']:.6f}, R²={trend_info['r_squared']:.4f}")
        
        # 测试峰值谷值
        peaks, valleys = TrendAnalysis.find_peaks_and_valleys(close)
        print(f"✅ 峰值谷值识别成功: 峰值={len(peaks)}个, 谷值={len(valleys)}个")
        
        # 测试趋势强度
        di_plus, di_minus = TrendAnalysis.trend_strength(high, low, close, 14)
        print(f"✅ 趋势强度计算成功: +DI={di_plus.iloc[-1]:.2f}, -DI={di_minus.iloc[-1]:.2f}")
        
        # 测试周期检测
        cycles = TrendAnalysis.cycle_detection(close)
        print(f"✅ 周期检测成功: 发现{len(cycles)}个显著周期")
        
        return True
    except Exception as e:
        print(f"❌ 趋势分析测试失败: {e}")
        return False


def test_predictive_analysis(df):
    """测试预测分析模块"""
    print("\n" + "="*70)
    print("【测试4】预测分析模块")
    print("="*70)
    
    close = df['close']
    
    try:
        # 测试指数平滑
        smoothed = PredictiveAnalysis.exponential_smoothing(close.values, alpha=0.3)
        print(f"✅ 指数平滑成功: 最后值={smoothed[-1]:.2f}")
        
        # 测试趋势外推
        forecast = PredictiveAnalysis.trend_extrapolation(close, forecast_periods=10)
        print(f"✅ 趋势外推预测成功: 预测10期价格")
        
        # 测试均线收敛
        divergence, signal = PredictiveAnalysis.moving_average_convergence(close)
        print(f"✅ 均线收敛分析成功: 信号={signal}, 最后乖离率={divergence.iloc[-1]:.2f}%")
        
        return True
    except Exception as e:
        print(f"❌ 预测分析测试失败: {e}")
        return False


def test_full_report(df):
    """测试完整报告生成"""
    print("\n" + "="*70)
    print("【测试5】完整报告生成")
    print("="*70)
    
    try:
        print("\n生成完整分析报告...\n")
        AnalysisReporter.generate_report(df)
        return True
    except Exception as e:
        print(f"❌ 报告生成失败: {e}")
        import traceback
        traceback.print_exc()
        return False


def main():
    """主测试程序"""
    print("""
    ╔════════════════════════════════════════════════════════════════╗
    ║          K线分析系统 - 单元测试                                 ║
    ║                                                                ║
    ║  本测试使用模拟数据验证所有分析模块的功能                        ║
    ╚════════════════════════════════════════════════════════════════╝
    """)
    
    # 生成测试数据
    print("📊 正在生成测试数据...")
    df = generate_sample_kline_data(days=200)
    print(f"✅ 生成了 {len(df)} 条模拟K线数据")
    print(f"   时间范围: {df['timestamp'].min().date()} 至 {df['timestamp'].max().date()}")
    print(f"   价格范围: {df['close'].min():.2f} - {df['close'].max():.2f}\n")
    
    # 运行所有测试
    results = []
    results.append(("技术分析模块", test_technical_analysis(df)))
    results.append(("统计分析模块", test_statistical_analysis(df)))
    results.append(("趋势分析模块", test_trend_analysis(df)))
    results.append(("预测分析模块", test_predictive_analysis(df)))
    results.append(("完整报告生成", test_full_report(df)))
    
    # 输出测试总结
    print("\n" + "="*70)
    print("【测试总结】")
    print("="*70)
    
    passed = sum(1 for _, result in results if result)
    total = len(results)
    
    for name, result in results:
        status = "✅ PASS" if result else "❌ FAIL"
        print(f"{status} - {name}")
    
    print(f"\n总计: {passed}/{total} 测试通过")
    
    if passed == total:
        print("\n🎉 所有测试通过！K线分析系统已准备好使用。")
        print("\n下一步: 运行 'python kline_analyzer.py' 分析实际数据")
        return 0
    else:
        print(f"\n⚠️  有 {total - passed} 个测试失败，请检查错误信息")
        return 1


if __name__ == "__main__":
    exit_code = main()
    sys.exit(exit_code)
