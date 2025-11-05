#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
测试增量学习功能的演示脚本

演示如何：
1. 第一次训练（从零开始）
2. 第二次训练（增量学习）
3. 查看训练历史和改进
"""

from kline_analyzer import (
    KlineDataFetcher,
    ModelPersistenceManager,
    BaselineModelTrainer,
    TechnicalAnalysis,
    run_model_training_pipeline
)
import pandas as pd
import numpy as np

def demo_incremental_learning():
    """演示增量学习的完整流程"""
    
    print("\n" + "="*80)
    print("🚀 CSGO 饰品指数 - 增量学习演示")
    print("="*80)
    
    # ============================================================================
    # 第一步：获取数据
    # ============================================================================
    print("\n【步骤 1】获取 K 线数据...")
    fetcher = KlineDataFetcher()
    df = fetcher.fetch_kline(index_id=3, kline_type="1hour", verbose=False)
    
    if df is None or len(df) < 10:
        print("❌ 数据获取失败")
        return
    
    print(f"✅ 成功获取 {len(df)} 条数据\n")
    
    # ============================================================================
    # 第二步：运行完整的训练流程（包括增量学习）
    # ============================================================================
    print("【步骤 2】运行模型训练流程...")
    print("-" * 80)
    
    results = run_model_training_pipeline(df)
    
    # ============================================================================
    # 第三步：显示模型持久化管理器的信息
    # ============================================================================
    print("\n【步骤 3】查看模型持久化信息...")
    print("-" * 80)
    
    pm = ModelPersistenceManager()
    
    # 检查模型文件
    print("\n📁 已保存的模型文件:")
    models = ['arima', 'prophet', 'xgboost']
    for model_name in models:
        if pm.model_exists(model_name):
            print(f"  ✅ {model_name.upper()}: {pm._get_model_path(model_name)}")
        else:
            print(f"  ❌ {model_name.upper()}: 不存在")
    
    # 检查元数据
    metadata = pm.load_metadata()
    if metadata:
        print("\n📊 最新元数据:")
        print(f"  时间戳: {metadata.get('timestamp')}")
        print(f"  训练集大小: {metadata.get('train_size')}")
        print(f"  测试集大小: {metadata.get('test_size')}")
        print(f"\n  ARIMA RMSE:    {metadata.get('arima_rmse', 'N/A')}")
        if 'arima_improvement' in metadata:
            print(f"  改进幅度:      {metadata.get('arima_improvement', 'N/A'):.2f}%")
        print(f"  Prophet RMSE:  {metadata.get('prophet_rmse', 'N/A')}")
        if 'prophet_improvement' in metadata:
            print(f"  改进幅度:      {metadata.get('prophet_improvement', 'N/A'):.2f}%")
        print(f"  XGBoost RMSE:  {metadata.get('xgb_rmse', 'N/A')}")
        if 'xgb_improvement' in metadata:
            print(f"  改进幅度:      {metadata.get('xgb_improvement', 'N/A'):.2f}%")
    
    # ============================================================================
    # 第四步：显示训练历史
    # ============================================================================
    print("\n【步骤 4】显示训练历史...")
    print("-" * 80)
    pm.show_training_history()
    
    print("\n" + "="*80)
    print("✨ 演示完成！")
    print("="*80)
    print("""
💡 关键信息：
  1. 模型已保存到 models/ 目录
  2. 下次训练会自动加载并继续优化
  3. 每次训练的性能对比都会记录在历史中
  4. RMSE 逐次下降 = 预测精度不断提升

📚 查看完整指南：INCREMENTAL_LEARNING_GUIDE.md
""")

if __name__ == "__main__":
    demo_incremental_learning()
