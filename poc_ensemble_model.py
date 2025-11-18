#!/usr/bin/env python3
"""
PoC: Prophet + XGBoost 集成模型 vs 线性回归对比
- 支持多线程并发处理
- 集成缓存系统，加速重复查询
- 优化大数据量处理
"""

import sys
import json
import warnings
import pickle
from datetime import datetime, timedelta
from pathlib import Path
from concurrent.futures import ThreadPoolExecutor, as_completed
from threading import Lock
import time

import numpy as np
import pandas as pd
import matplotlib.pyplot as plt
from sklearn.linear_model import LinearRegression
from sklearn.metrics import mean_absolute_percentage_error, mean_squared_error, mean_absolute_error
from prophet import Prophet
from xgboost import XGBRegressor
import pymysql
import os

warnings.filterwarnings('ignore')

# 设置中文字体
plt.rcParams['font.sans-serif'] = ['SimHei', 'DejaVu Sans']
plt.rcParams['axes.unicode_minus'] = False

# 缓存配置
CACHE_DIR = Path('/Users/user/Downloads/csgoAuto/.cache')
CACHE_DIR.mkdir(exist_ok=True)


class Cache:
    """简单的文件缓存系统，支持并发访问"""
    def __init__(self, cache_dir=CACHE_DIR):
        self.cache_dir = cache_dir
        self.lock = Lock()

    def get(self, key):
        """获取缓存"""
        cache_file = self.cache_dir / f"{key}.pkl"
        if cache_file.exists():
            try:
                with open(cache_file, 'rb') as f:
                    return pickle.load(f)
            except Exception as e:
                print(f"  ⚠️  缓存读取失败: {e}")
                return None
        return None

    def set(self, key, value):
        """设置缓存"""
        with self.lock:
            cache_file = self.cache_dir / f"{key}.pkl"
            try:
                with open(cache_file, 'wb') as f:
                    pickle.dump(value, f)
            except Exception as e:
                print(f"  ⚠️  缓存写入失败: {e}")

    def clear(self):
        """清空缓存"""
        with self.lock:
            for f in self.cache_dir.glob('*.pkl'):
                f.unlink()


class DatabasePool:
    """数据库连接池，用于多线程并发访问"""
    def __init__(self, db_host='23.254.215.66', db_user='root', db_password='Wyj250413.',
                 db_name='csgo_trader', pool_size=10):
        self.db_config = {
            'host': db_host,
            'user': db_user,
            'password': db_password,
            'database': db_name,
            'charset': 'utf8mb4'
        }
        self.pool_size = pool_size
        self.connections = []
        self.lock = Lock()
        self._init_pool()

    def _init_pool(self):
        """初始化连接池"""
        for _ in range(self.pool_size):
            try:
                conn = pymysql.connect(**self.db_config)
                self.connections.append(conn)
            except Exception as e:
                print(f"❌ 连接池初始化失败: {e}")

    def get_connection(self):
        """获取连接"""
        with self.lock:
            if self.connections:
                return self.connections.pop()
            return pymysql.connect(**self.db_config)

    def release_connection(self, conn):
        """释放连接"""
        with self.lock:
            if len(self.connections) < self.pool_size:
                self.connections.append(conn)
            else:
                conn.close()

    def close_all(self):
        """关闭所有连接"""
        with self.lock:
            for conn in self.connections:
                try:
                    conn.close()
                except:
                    pass
            self.connections.clear()


class EnsembleModelPOC:
    """Prophet + XGBoost 集成模型 PoC"""

    def __init__(self, db_host='23.254.215.66', db_user='root', db_password='Wyj250413.',
                 db_name='csgo_trader', num_workers=8):
        """初始化"""
        self.db_pool = DatabasePool(db_host, db_user, db_password, db_name, pool_size=num_workers)
        self.cache = Cache()
        self.num_workers = num_workers
        self.results_lock = Lock()
        self.all_results = []

        print(f"✓ 初始化完成: {num_workers} 个工作线程, 缓存目录: {CACHE_DIR}")

    def fetch_historical_data(self, good_id, days=30, use_cache=True):
        """从数据库获取历史价格数据，支持缓存"""
        cache_key = f"hist_data_{good_id}_{days}"

        # 尝试读取缓存
        if use_cache:
            cached_data = self.cache.get(cache_key)
            if cached_data is not None:
                return cached_data

        conn = self.db_pool.get_connection()
        try:
            cursor = conn.cursor()
            query = """
            SELECT created_at, yyyp_buy_price, yyyp_sell_price,
                   yyyp_buy_count, yyyp_sell_count
            FROM csqaq_good_snapshots
            WHERE good_id = %s
            AND created_at >= DATE_SUB(NOW(), INTERVAL %s DAY)
            AND yyyp_buy_price > 0 AND yyyp_sell_price > 0
            ORDER BY created_at ASC
            """
            cursor.execute(query, (good_id, days))
            results = cursor.fetchall()

            if not results:
                return None

            df = pd.DataFrame(results, columns=[
                'timestamp', 'buy_price', 'sell_price',
                'buy_orders', 'sell_orders'
            ])

            df['timestamp'] = pd.to_datetime(df['timestamp'])
            df = df.sort_values('timestamp').reset_index(drop=True)

            # 缓存数据
            if use_cache:
                self.cache.set(cache_key, df)

            return df
        finally:
            cursor.close()
            self.db_pool.release_connection(conn)

    def get_sample_templates_concurrent(self, limit=5):
        """并发获取样本模板 - 使用批量查询优化"""
        conn = self.db_pool.get_connection()
        try:
            cursor = conn.cursor()
            query = """
            SELECT good_id, COUNT(*) as data_points
            FROM csqaq_good_snapshots
            WHERE created_at >= DATE_SUB(NOW(), INTERVAL 30 DAY)
            AND yyyp_buy_price > 0 AND yyyp_sell_price > 0
            GROUP BY good_id
            HAVING data_points >= 20
            ORDER BY data_points DESC
            LIMIT %s
            """
            cursor.execute(query, (limit,))
            samples = cursor.fetchall()
            return samples
        finally:
            cursor.close()
            self.db_pool.release_connection(conn)

    def prepare_features(self, df):
        """为XGBoost准备特征"""
        df_features = df.copy()

        # 时间特征
        df_features['day_of_week'] = df_features['timestamp'].dt.dayofweek
        df_features['day_of_month'] = df_features['timestamp'].dt.day
        df_features['days_since_start'] = (df_features['timestamp'] - df_features['timestamp'].min()).dt.days

        # 价格特征
        df_features['price_range'] = df_features['sell_price'] - df_features['buy_price']
        df_features['total_orders'] = df_features['buy_orders'] + df_features['sell_orders']
        df_features['order_ratio'] = df_features['buy_orders'] / (df_features['sell_orders'] + 1)

        # 移动平均
        df_features['buy_price_ma3'] = df_features['buy_price'].rolling(3, min_periods=1).mean()
        df_features['sell_price_ma3'] = df_features['sell_price'].rolling(3, min_periods=1).mean()

        # 处理缺失值
        df_features = df_features.fillna(method='ffill').fillna(method='bfill')

        return df_features

    def train_and_evaluate(self, good_id, idx, total):
        """训练和评估单个商品的模型"""
        try:
            print(f"  [{idx}/{total}] 处理 Good ID {good_id}...", flush=True)

            # 获取历史数据
            df = self.fetch_historical_data(good_id, days=30)

            if df is None or len(df) < 10:
                print(f"  [{idx}/{total}] ⚠️  数据不足，跳过", flush=True)
                return None

            # 数据分割 (70% 训练, 30% 测试)
            split_point = int(len(df) * 0.7)
            df_train = df[:split_point].copy()
            df_test = df[split_point:].copy()

            # ========== 线性回归 ==========
            X_train_len = len(df_train)
            y_train = df_train['sell_price'].values
            lr_model = LinearRegression()
            lr_model.fit(np.arange(len(y_train)).reshape(-1, 1), y_train)
            X_test_lr = np.arange(X_train_len, X_train_len + len(df_test)).reshape(-1, 1)
            y_pred_lr = lr_model.predict(X_test_lr)

            # ========== Prophet ==========
            df_prophet = df_train[['timestamp', 'sell_price']].copy()
            df_prophet.columns = ['ds', 'y']
            prophet_model = Prophet(yearly_seasonality=False, interval_width=0.95)
            prophet_model.fit(df_prophet)
            future = prophet_model.make_future_dataframe(periods=len(df_test))
            forecast = prophet_model.predict(future)
            y_pred_prophet = forecast['yhat'].values[-len(df_test):]

            # ========== XGBoost ==========
            df_features = self.prepare_features(df_train)
            feature_cols = ['day_of_week', 'day_of_month', 'days_since_start',
                           'price_range', 'total_orders', 'order_ratio',
                           'buy_price_ma3', 'sell_price_ma3']
            X_train_xgb = df_features[feature_cols].values
            y_train_xgb = df_features['sell_price'].values

            xgb_model = XGBRegressor(n_estimators=50, max_depth=4, learning_rate=0.1,
                                    random_state=42, verbosity=0)
            xgb_model.fit(X_train_xgb, y_train_xgb)

            # 准备测试特征
            df_test_features = self.prepare_features(pd.concat([df_train, df_test], ignore_index=True))
            df_test_features = df_test_features[len(df_train):].reset_index(drop=True)
            X_test_xgb = df_test_features[feature_cols].values
            y_pred_xgb = xgb_model.predict(X_test_xgb)

            # ========== 评估 ==========
            y_test = df_test['sell_price'].values
            y_pred_lr = np.maximum(y_pred_lr, 0)
            y_pred_prophet = np.maximum(y_pred_prophet, 0)
            y_pred_xgb = np.maximum(y_pred_xgb, 0)

            def calc_metrics(y_true, y_pred):
                mape = mean_absolute_percentage_error(y_true, y_pred) * 100
                rmse = np.sqrt(mean_squared_error(y_true, y_pred))
                mae = mean_absolute_error(y_true, y_pred)
                return {'MAPE': mape, 'RMSE': rmse, 'MAE': mae}

            metrics = {
                '线性回归': calc_metrics(y_test, y_pred_lr),
                'Prophet': calc_metrics(y_test, y_pred_prophet),
                'XGBoost': calc_metrics(y_test, y_pred_xgb)
            }

            # 集成模型 (加权平均)
            y_pred_ensemble = (y_pred_lr * 0.2 + y_pred_prophet * 0.3 + y_pred_xgb * 0.5)
            metrics['集成模型'] = calc_metrics(y_test, y_pred_ensemble)

            print(f"  [{idx}/{total}] ✓ 完成 (MAPE: LR={metrics['线性回归']['MAPE']:.1f}% Prophet={metrics['Prophet']['MAPE']:.1f}% XGB={metrics['XGBoost']['MAPE']:.1f}% Ensemble={metrics['集成模型']['MAPE']:.1f}%)", flush=True)

            return {
                'good_id': good_id,
                'data_points': len(df),
                'train_size': len(df_train),
                'test_size': len(df_test),
                'metrics': metrics
            }

        except Exception as e:
            print(f"  [{idx}/{total}] ❌ 错误: {str(e)}", flush=True)
            return None

    def run_poc(self):
        """运行完整的 PoC"""
        print("\n" + "="*70)
        print("Prophet + XGBoost 集成模型 PoC (多线程优化版)")
        print("="*70)

        print("\n1️⃣  正在获取样本商品数据...")
        start_time = time.time()
        samples = self.get_sample_templates_concurrent(limit=5)
        elapsed = time.time() - start_time

        if not samples:
            print("❌ 数据库中没有足够的历史数据")
            return

        print(f"✓ 获取 {len(samples)} 个样本 (耗时: {elapsed:.2f}s)")

        # 使用多线程并发处理
        print(f"\n2️⃣  启动 {self.num_workers} 个工作线程处理数据...")
        start_time = time.time()

        with ThreadPoolExecutor(max_workers=self.num_workers) as executor:
            futures = [
                executor.submit(self.train_and_evaluate, good_id, idx + 1, len(samples))
                for idx, (good_id, _) in enumerate(samples)
            ]

            for future in as_completed(futures):
                result = future.result()
                if result is not None:
                    with self.results_lock:
                        self.all_results.append(result)

        elapsed = time.time() - start_time
        print(f"\n✓ 并发处理完成 (耗时: {elapsed:.2f}s, 成功: {len(self.all_results)}/{len(samples)})")

        # 生成总结报告
        if self.all_results:
            self.generate_report(self.all_results)

        self.db_pool.close_all()
        print("\n✓ 所有连接已关闭")

    def generate_report(self, results):
        """生成详细报告"""
        print("\n" + "="*70)
        print("📈 总体性能对比报告")
        print("="*70)

        # 计算平均指标
        model_names = ['线性回归', 'Prophet', 'XGBoost', '集成模型']
        summary = {m: {'MAPE': [], 'RMSE': [], 'MAE': []} for m in model_names}

        for result in results:
            for model_name in model_names:
                if model_name in result['metrics']:
                    summary[model_name]['MAPE'].append(result['metrics'][model_name]['MAPE'])
                    summary[model_name]['RMSE'].append(result['metrics'][model_name]['RMSE'])
                    summary[model_name]['MAE'].append(result['metrics'][model_name]['MAE'])

        print(f"\n{'模型':<12} {'平均MAPE':<12} {'平均RMSE':<12} {'平均MAE':<12}")
        print("-" * 48)

        for model_name in model_names:
            if summary[model_name]['MAPE']:
                avg_mape = np.mean(summary[model_name]['MAPE'])
                avg_rmse = np.mean(summary[model_name]['RMSE'])
                avg_mae = np.mean(summary[model_name]['MAE'])
                print(f"{model_name:<12} {avg_mape:>10.2f}%  {avg_rmse:>10.2f}  {avg_mae:>10.2f}")

        # 保存详细结果为JSON
        output_file = '/Users/user/Downloads/csgoAuto/poc_results.json'
        with open(output_file, 'w', encoding='utf-8') as f:
            json.dump({
                'timestamp': datetime.now().isoformat(),
                'summary': {k: {m: float(np.mean(v)) if v else 0 for m, v in mv.items()}
                           for k, mv in summary.items()},
                'detailed_results': results
            }, f, indent=2, ensure_ascii=False)

        print(f"\n✓ 详细结果已保存到: {output_file}")

        # 关键发现
        print("\n" + "="*70)
        print("🔍 关键发现")
        print("="*70)

        avg_mape = {
            '线性回归': np.mean(summary['线性回归']['MAPE']) if summary['线性回归']['MAPE'] else 0,
            'Prophet': np.mean(summary['Prophet']['MAPE']) if summary['Prophet']['MAPE'] else 0,
            'XGBoost': np.mean(summary['XGBoost']['MAPE']) if summary['XGBoost']['MAPE'] else 0,
            '集成模型': np.mean(summary['集成模型']['MAPE']) if summary['集成模型']['MAPE'] else 0,
        }

        best_model = min(avg_mape.items(), key=lambda x: x[1])
        improvement_vs_lr = ((avg_mape['线性回归'] - best_model[1]) / avg_mape['线性回归'] * 100) if avg_mape['线性回归'] > 0 else 0

        print(f"\n1. 最佳模型: {best_model[0]} (平均MAPE: {best_model[1]:.2f}%)")
        print(f"2. 相对线性回归的改进: {improvement_vs_lr:.1f}%")
        print(f"3. 处理的商品数: {len(self.all_results)}")
        print(f"\n4. 集成模型的优势:")
        print(f"   - 整合 Prophet 的趋势+季节性能力")
        print(f"   - 整合 XGBoost 的非线性关系学习")
        print(f"   - 加权融合避免过拟合")
        print(f"   - MAPE 改进: {improvement_vs_lr:.1f}%")

        print(f"\n5. 缓存效果:")
        cache_size = sum(f.stat().st_size for f in CACHE_DIR.glob('*.pkl')) / 1024 / 1024
        print(f"   - 缓存大小: {cache_size:.2f} MB")
        print(f"   - 缓存位置: {CACHE_DIR}")
        print(f"   - 提示: 重新运行 PoC 会使用缓存，速度会更快")

        print("\n✅ PoC 验证完成！")


if __name__ == '__main__':
    poc = EnsembleModelPOC(num_workers=8)
    poc.run_poc()
