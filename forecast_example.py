#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
CSGO饰品指数预测功能示例
演示如何使用新增的预测功能

用法:
    python forecast_example.py
"""

from kline_analyzer import (
    KlineDataFetcher,
    AdvancedPredictiveAnalysis,
    export_forecast_to_json,
    forecast_only
)
import json

def example_1_simple_forecast():
    """示例1：简单预测"""
    print("\n" + "="*80)
    print("示例1：简单预测")
    print("="*80 + "\n")
    
    print("仅运行预测模块...\n")
    forecast_only()


def example_2_programmatic_forecast():
    """示例2：程序化预测"""
    print("\n" + "="*80)
    print("示例2：程序化预测")
    print("="*80 + "\n")
    
    # 获取数据
    fetcher = KlineDataFetcher()
    df = fetcher.fetch_kline(index_id=3, kline_type="1day", verbose=False)
    
    if df is None:
        print("❌ 获取数据失败")
        return
    
    close = df['close']
    timestamp = df['timestamp']
    current_price = close.iloc[-1]
    
    # 1天预测
    print(f"当前价格: {current_price:.2f}\n")
    
    for periods in [1, 7, 14]:
        print(f"📊 {periods}天预测:")
        
        # 获取集成预测
        result = AdvancedPredictiveAnalysis.ensemble_forecast(
            close, timestamp, forecast_periods=periods
        )
        
        if result is None:
            print("  数据不足\n")
            continue
        
        forecast_value = result['ensemble_forecast'][-1]
        change = (forecast_value / current_price - 1) * 100
        
        print(f"  目标价: {forecast_value:.2f}")
        print(f"  预期变化: {change:+.2f}%")
        
        # 显示各模型预测
        for model_name, values in result['individual_forecasts'].items():
            model_change = (values[-1] / current_price - 1) * 100
            print(f"    - {model_name}: {values[-1]:.2f} ({model_change:+.2f}%)")
        
        print()


def example_3_export_json():
    """示例3：导出JSON"""
    print("\n" + "="*80)
    print("示例3：导出预测结果到JSON")
    print("="*80 + "\n")
    
    result = export_forecast_to_json('forecast_output.json')
    
    if result:
        print("\n✅ 导出成功！")
        print(f"当前价格: {result['metadata']['current_price']:.2f}")
        print(f"数据点: {result['metadata']['data_points']}")
        
        for period_key, forecast_data in result['forecasts'].items():
            if forecast_data:
                print(f"\n{period_key}:")
                print(f"  目标价: {forecast_data['forecast_value']:.2f}")
                print(f"  变化: {forecast_data['change_percent']:+.2f}%")


def example_4_confidence_intervals():
    """示例4：置信区间分析"""
    print("\n" + "="*80)
    print("示例4：置信区间分析")
    print("="*80 + "\n")
    
    fetcher = KlineDataFetcher()
    df = fetcher.fetch_kline(index_id=3, kline_type="1day", verbose=False)
    
    if df is None:
        print("❌ 获取数据失败")
        return
    
    close = df['close']
    timestamp = df['timestamp']
    
    # 7天预测
    result = AdvancedPredictiveAnalysis.ensemble_forecast(close, timestamp, 7)
    
    if result is None:
        print("❌ 预测失败")
        return
    
    ensemble_forecast = result['ensemble_forecast']
    
    # 计算置信区间
    intervals = AdvancedPredictiveAnalysis.calculate_forecast_confidence(
        close, ensemble_forecast, 7
    )
    
    print("7天预测的置信区间：\n")
    print(f"{'Day':<5} {'预测价格':<12} {'下界(95%)':<12} {'上界(95%)':<12}")
    print("-" * 50)
    
    for interval in intervals:
        day = int(interval['period'])
        forecast = interval['forecast']
        lower = interval['lower_95']
        upper = interval['upper_95']
        
        print(f"{day:<5} {forecast:<12.2f} {lower:<12.2f} {upper:<12.2f}")


def example_5_compare_models():
    """示例5：模型对比分析"""
    print("\n" + "="*80)
    print("示例5：模型对比分析")
    print("="*80 + "\n")
    
    fetcher = KlineDataFetcher()
    df = fetcher.fetch_kline(index_id=3, kline_type="1day", verbose=False)
    
    if df is None:
        print("❌ 获取数据失败")
        return
    
    close = df['close']
    timestamp = df['timestamp']
    current_price = close.iloc[-1]
    
    print("7天预测 - 各模型对比：\n")
    
    # ARIMA
    arima = AdvancedPredictiveAnalysis.arima_forecast(close, 7)
    if arima:
        arima_7day = arima['forecast'][-1]
        print(f"ARIMA:")
        print(f"  预测: {arima_7day:.2f}")
        print(f"  变化: {(arima_7day/current_price-1)*100:+.2f}%")
    
    # 指数平滑
    es = AdvancedPredictiveAnalysis.exponential_smoothing_advanced(close, 7)
    if es:
        es_7day = es['forecast'][-1]
        print(f"\n指数平滑:")
        print(f"  预测: {es_7day:.2f}")
        print(f"  变化: {(es_7day/current_price-1)*100:+.2f}%")
    
    # 趋势分析
    trend = AdvancedPredictiveAnalysis.prophet_forecast(close, timestamp, 7)
    if trend:
        trend_7day = trend['forecast'][-1]
        print(f"\n趋势分析:")
        print(f"  预测: {trend_7day:.2f}")
        print(f"  变化: {(trend_7day/current_price-1)*100:+.2f}%")
        print(f"  趋势斜率: {trend['trend_slope']:.6f}")
    
    # 动量
    momentum = AdvancedPredictiveAnalysis.weighted_momentum_forecast(close, 7)
    if momentum:
        momentum_7day = momentum['forecast'][-1]
        print(f"\n加权动量:")
        print(f"  预测: {momentum_7day:.2f}")
        print(f"  变化: {(momentum_7day/current_price-1)*100:+.2f}%")
        print(f"  加权动量: {momentum['momentum']:+.4f}")
        print(f"  动量详情:")
        for scale, value in momentum['momenta_detail'].items():
            print(f"    - {scale}: {value:+.4f}")


def main():
    """主菜单"""
    print("""
    ╔══════════════════════════════════════════════════════════╗
    ║     CSGO饰品指数预测功能演示 - 示例脚本                  ║
    ║                                                          ║
    ║     可选示例：                                            ║
    ║     1. 简单预测 (一键运行)                               ║
    ║     2. 程序化预测 (代码调用)                             ║
    ║     3. 导出JSON                                          ║
    ║     4. 置信区间分析                                      ║
    ║     5. 模型对比分析                                      ║
    ║     0. 运行所有示例                                      ║
    ║                                                          ║
    ╚══════════════════════════════════════════════════════════╝
    """)
    
    # 默认运行所有示例
    examples = [
        ("简单预测", example_1_simple_forecast),
        ("程序化预测", example_2_programmatic_forecast),
        ("导出JSON", example_3_export_json),
        ("置信区间分析", example_4_confidence_intervals),
        ("模型对比分析", example_5_compare_models),
    ]
    
    print("▶ 运行所有示例...\n")
    
    for name, func in examples:
        try:
            func()
        except Exception as e:
            print(f"\n❌ {name} 出错: {e}")
    
    print("\n" + "="*80)
    print("✅ 所有示例运行完成！")
    print("="*80)


if __name__ == "__main__":
    main()
