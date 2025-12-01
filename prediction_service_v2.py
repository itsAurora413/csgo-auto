#!/usr/bin/env python3
"""
CSGO 饰品市场 - 高级预测服务 v2.1 (性能优化版)
特性:
- 模型持久化（pickle保存）
- 增量训练（加载旧模型继续训练）
- 质量指标跟踪（MSE、MAPE、MAE等）
- 训练历史记录
- 高并发优化 (细粒度锁, 连接池, 线程池)
- 动态权重分配
"""

import sys
import json
import warnings
import pickle
import logging
import queue
from datetime import datetime, timedelta
from pathlib import Path
from threading import Lock
from dataclasses import asdict
import time
from collections import defaultdict
from concurrent.futures import ThreadPoolExecutor, as_completed

import numpy as np
import pandas as pd
from sklearn.linear_model import LinearRegression
from sklearn.metrics import mean_absolute_percentage_error, mean_squared_error, mean_absolute_error
from prophet import Prophet
from xgboost import XGBRegressor
import pymysql
from flask import Flask, jsonify, request
from flask_cors import CORS

# 导入数据质量和漂移检测模块
from data_quality_monitor import DataQualityChecker, DataDriftDetector, DataCleaner
from drift_alert_system import DriftAlertSystem, RetrainingTrigger

warnings.filterwarnings('ignore')

# ============================================================================
# 配置
# ============================================================================

# ============================================================================
# 日志配置 - 结构化输出
# ============================================================================

class StructuredFormatter(logging.Formatter):
    """结构化日志格式化器，便于追踪特定商品"""
    
    def format(self, record):
        # 从消息中提取good_id（如果有[good_id=XXX]格式）
        msg = record.getMessage()
        
        # 基础格式
        base_time = self.formatTime(record, '%H:%M:%S')
        level = record.levelname
        
        # 根据日志级别使用不同颜色（如果支持）
        level_colors = {
            'DEBUG': '🔵',
            'INFO': '🟢',
            'WARNING': '🟡',
            'ERROR': '🔴'
        }
        level_icon = level_colors.get(level, '  ')
        
        # 格式: [时间] 🟢 [模块.函数] [good_id=XXX] 消息
        func_name = f"{record.name.split('.')[-1]}.{record.funcName}" if record.funcName else record.name
        
        return f"[{base_time}] {level_icon} [{func_name}] {msg}"

LOG_FORMAT = '%(asctime)s - %(name)s - %(levelname)s - %(message)s'
logging.basicConfig(
    level=logging.INFO,
    format=LOG_FORMAT,
    handlers=[
        logging.FileHandler('/tmp/prediction_service.log'),
        logging.StreamHandler()
    ]
)

# 设置自定义格式化器用于控制台输出
console_handler = logging.StreamHandler()
console_handler.setFormatter(StructuredFormatter())
logger = logging.getLogger(__name__)
logger.handlers.clear()
logger.addHandler(console_handler)

# 同时保留文件输出
file_handler = logging.FileHandler('/tmp/prediction_service.log')
file_handler.setFormatter(logging.Formatter(LOG_FORMAT))
logger.addHandler(file_handler)

DB_CONFIG = {
    'host': 'localhost',
    # 'host': '	192.3.81.194',
    'user': 'root',
    'password': 'Wyj250413.',
    'database': 'csgo_trader',
    'charset': 'utf8mb4'
}

CACHE_DIR = Path('/root/csgo_prediction/.cache')
# CACHE_DIR = Path('/Users/user/Downloads/csgoAuto/.cache')
CACHE_DIR.mkdir(parents=True, exist_ok=True)
MODEL_DIR = CACHE_DIR / 'models'
MODEL_DIR.mkdir(exist_ok=True)
METRICS_DIR = CACHE_DIR / 'metrics'
METRICS_DIR.mkdir(exist_ok=True)
ALERTS_DIR = CACHE_DIR / 'alerts'
ALERTS_DIR.mkdir(exist_ok=True)

# Flask 应用
app = Flask(__name__)
CORS(app)

# 初始化数据质量检查和告警系统
QUALITY_CHECKER = DataQualityChecker(outlier_method='iqr', outlier_threshold=1.5)
DRIFT_DETECTOR = DataDriftDetector(recent_ratio=0.3, drift_threshold=0.5)
DATA_CLEANER = DataCleaner()
ALERT_SYSTEM = DriftAlertSystem(alert_dir=ALERTS_DIR)
RETRAINING_TRIGGER = RetrainingTrigger(models_dir=MODEL_DIR)

# ============================================================================
# 资源管理 (缓存, 锁, 连接池)
# ============================================================================

class SimpleConnectionPool:
    """简单的数据库连接池"""
    def __init__(self, max_size=10):
        self.pool = queue.Queue(maxsize=max_size)
        self.max_size = max_size
        self.current_size = 0
        self.lock = Lock()
        
    def get_connection(self):
        try:
            # 尝试从池中获取
            return self.pool.get_nowait()
        except queue.Empty:
            # 池空，尝试新建
            with self.lock:
                if self.current_size < self.max_size:
                    conn = pymysql.connect(**DB_CONFIG)
                    self.current_size += 1
                    return conn
            # 达到最大连接数，阻塞等待
            return self.pool.get()

    def release_connection(self, conn):
        try:
            # 检查连接是否存活
            conn.ping(reconnect=True)
            self.pool.put_nowait(conn)
        except Exception:
            # 连接已死，丢弃
            with self.lock:
                self.current_size -= 1
                try:
                    conn.close()
                except:
                    pass

class ModelCacheManager:
    """线程安全的模型缓存管理器 (LRU简化版)"""
    def __init__(self, max_size=1000):
        self.cache = {}
        self.access_time = {}
        self.max_size = max_size
        self.global_lock = Lock()
        self.item_locks = defaultdict(Lock) # 细粒度锁
        
    def get_lock(self, good_id):
        with self.global_lock:
            return self.item_locks[good_id]
            
    def get(self, good_id):
        with self.global_lock:
            if good_id in self.cache:
                self.access_time[good_id] = time.time()
                return self.cache[good_id]
            return None
            
    def put(self, good_id, model):
        with self.global_lock:
            if len(self.cache) >= self.max_size and good_id not in self.cache:
                # 清理最久未使用的
                oldest_id = min(self.access_time, key=self.access_time.get)
                del self.cache[oldest_id]
                del self.access_time[oldest_id]
                # 注意：item_locks 不清理，为了安全
                
            self.cache[good_id] = model
            self.access_time[good_id] = time.time()

    def clear(self):
        with self.global_lock:
            self.cache.clear()
            self.access_time.clear()
            return True
            
    def size(self):
        with self.global_lock:
            return len(self.cache)

# 全局资源
DB_POOL = SimpleConnectionPool(max_size=20)
CACHE_MANAGER = ModelCacheManager(max_size=500)

# ============================================================================
# 数据库操作
# ============================================================================

def fetch_historical_data(good_id, days=30):
    """从数据库获取历史价格数据 (使用连接池)"""
    conn = DB_POOL.get_connection()
    if not conn:
        return None

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
        cursor.close()

        if not results:
            return None

        df = pd.DataFrame(results, columns=[
            'timestamp', 'buy_price', 'sell_price',
            'buy_orders', 'sell_orders'
        ])

        df['timestamp'] = pd.to_datetime(df['timestamp'])
        return df.sort_values('timestamp').reset_index(drop=True)
    except Exception as e:
        logger.error(f"数据库查询失败: {e}")
        return None
    finally:
        DB_POOL.release_connection(conn)


# ============================================================================
# 特征工程
# ============================================================================

def prepare_features(df):
    """为 XGBoost 准备特征"""
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

    # 价格变化率
    df_features['price_change'] = df_features['sell_price'].pct_change().fillna(0)
    df_features['price_change_ma'] = df_features['price_change'].rolling(3, min_periods=1).mean()

    # ===== 新增：长期趋势特征（关键！）=====
    # 7天和30天的价格趋势（相对于当前价格）
    df_features['trend_7d'] = df_features['sell_price'].pct_change(periods=min(7, len(df_features))).fillna(0)
    df_features['trend_30d'] = df_features['sell_price'].pct_change(periods=min(30, len(df_features))).fillna(0)
    
    # 价格动量：7天移动平均 vs 30天移动平均
    df_features['ma7'] = df_features['sell_price'].rolling(7, min_periods=1).mean()
    df_features['ma30'] = df_features['sell_price'].rolling(30, min_periods=1).mean()
    df_features['momentum'] = (df_features['ma7'] - df_features['ma30']) / df_features['ma30']
    
    # 价格相对位置（当前价格相对于30天最高/最低的位置）
    df_features['price_max_30d'] = df_features['sell_price'].rolling(30, min_periods=1).max()
    df_features['price_min_30d'] = df_features['sell_price'].rolling(30, min_periods=1).min()
    df_features['price_position'] = (df_features['sell_price'] - df_features['price_min_30d']) / (df_features['price_max_30d'] - df_features['price_min_30d'] + 0.01)

    # 处理缺失值
    df_features = df_features.fillna(method='ffill').fillna(method='bfill')

    return df_features


# ============================================================================
# 模型训练与持久化
# ============================================================================

class PredictionModel:
    """预测模型集合 - 支持增量训练和持久化"""

    # 基础特征（兼容旧模型）
    FEATURE_COLS_BASE = [
        'day_of_week', 'day_of_month', 'days_since_start',
        'price_range', 'total_orders', 'order_ratio',
        'buy_price_ma3', 'sell_price_ma3', 'price_change_ma'
    ]
    
    # 新增趋势特征
    FEATURE_COLS_TREND = [
        'trend_7d', 'trend_30d', 'momentum', 'price_position'
    ]
    
    # 所有特征
    FEATURE_COLS = FEATURE_COLS_BASE + FEATURE_COLS_TREND

    def __init__(self, good_id):
        self.good_id = good_id
        self.lr = None
        self.prophet = None
        self.xgb = None
        self.last_price = None
        self.last_timestamp = None
        self.train_size = 0
        self.weights = {'lr': 0.2, 'prophet': 0.3, 'xgb': 0.5} # 默认权重
        self.model_version = '2.1'  # 当前模型版本
        self.feature_set = 'full'  # 特征集标识：'base' 或 'full'

        # 质量指标
        self.metrics = {
            'lr_mse': None, 'lr_mae': None, 'lr_mape': None,
            'xgb_mse': None, 'xgb_mae': None, 'xgb_mape': None,
            'prophet_mse': None, 'prophet_mae': None, 'prophet_mape': None,
            'ensemble_mse': None, 'ensemble_mae': None, 'ensemble_mape': None,
            'training_time': None,
            'training_count': 0,
            'last_training': None
        }

    def get_model_path(self):
        """获取模型保存路径"""
        return MODEL_DIR / f"model_{self.good_id}.pkl"

    def get_metrics_path(self):
        """获取指标保存路径"""
        return METRICS_DIR / f"metrics_{self.good_id}.json"

    def save_model(self):
        """保存模型到磁盘"""
        try:
            model_data = {
                'lr': self.lr,
                'prophet': self.prophet,
                'xgb': self.xgb,
                'last_price': self.last_price,
                'last_timestamp': self.last_timestamp,
                'train_size': self.train_size,
                'metrics': self.metrics,
                'weights': self.weights,
                'model_version': self.model_version,
                'feature_set': self.feature_set,
                'feature_cols': self.FEATURE_COLS,  # 保存具体的特征列
                'feature_cols_count': len(self.FEATURE_COLS)
            }
            with open(self.get_model_path(), 'wb') as f:
                pickle.dump(model_data, f)
            training_strategy = self.metrics.get('training_strategy', 'unknown')
            ensemble_mape = self.metrics.get('ensemble_mape', 0)
            logger.info(f"[good_id={self.good_id}] 💾 模型已保存 | 训练次数={self.metrics.get('training_count', 0)} | 策略={training_strategy} | MAPE={ensemble_mape:.4f}")
        except Exception as e:
            logger.error(f"[good_id={self.good_id}] 模型保存失败: {e}")

    def load_model(self):
        """从磁盘加载模型"""
        try:
            model_path = self.get_model_path()
            if not model_path.exists():
                # logger.info(f"[good_id={self.good_id}] 模型文件不存在，将创建新模型")
                return False

            with open(model_path, 'rb') as f:
                model_data = pickle.load(f)

            # 检查模型版本和特征兼容性
            saved_version = model_data.get('model_version', '2.0')
            saved_feature_set = model_data.get('feature_set', None)  # 可能为None（旧模型）
            saved_feature_cols = model_data.get('feature_cols', None)  # 可能为None（旧模型）
            current_feature_cols = self.FEATURE_COLS
            
            # 如果没有保存特征集信息（旧模型），根据保存的特征列表来判断
            if saved_feature_set is None:
                # 旧模型没有feature_set字段，需要重新训练
                saved_count = len(saved_feature_cols) if saved_feature_cols else 9
                logger.warning(f"[good_id={self.good_id}] 检测到旧版本模型({saved_count}特征)，升级到新版本(13特征)，将重新训练")
                return False
            
            # 如果特征集或特征列表不匹配，说明需要重新训练
            if saved_feature_set != 'full' or saved_feature_cols != current_feature_cols:
                saved_count = len(saved_feature_cols) if saved_feature_cols else 0
                logger.warning(f"[good_id={self.good_id}] 特征集不匹配: 保存={saved_count}个特征, 当前=13个特征，将重新训练")
                return False
            
            self.lr = model_data['lr']
            self.prophet = model_data['prophet']
            self.xgb = model_data['xgb']
            self.last_price = model_data['last_price']
            self.last_timestamp = model_data['last_timestamp']
            self.train_size = model_data['train_size']
            # 合并加载的metrics，确保保留已有的训练计数
            loaded_metrics = model_data.get('metrics', {})
            self.metrics.update(loaded_metrics)  # 用update替代get+赋值，保留初始化的默认值
            self.weights = model_data.get('weights', self.weights)
            self.model_version = model_data.get('model_version', '2.0')
            self.feature_set = 'full'  # 成功加载的模型一定是'full'特征集

            training_count = self.metrics.get('training_count', 0)
            last_training = self.metrics.get('last_training', '未知')
            ensemble_mape = self.metrics.get('ensemble_mape', 0)
            logger.info(f"[good_id={self.good_id}] ✓ 模型已从磁盘加载 | 训练次数={training_count} | 最后更新={last_training[:10]} | MAPE={ensemble_mape:.4f}")
            return True
        except Exception as e:
            logger.error(f"[good_id={self.good_id}] 模型加载失败: {e}")
            return False

    def save_metrics(self):
        """保存指标到JSON文件"""
        try:
            metrics_data = {
                'good_id': self.good_id,
                'timestamp': datetime.now().isoformat(),
                'metrics': self.metrics,
                'weights': self.weights
            }
            with open(self.get_metrics_path(), 'w') as f:
                json.dump(metrics_data, f, indent=2, default=str)
            # logger.info(f"[good_id={self.good_id}] 指标已保存")
        except Exception as e:
            logger.error(f"[good_id={self.good_id}] 指标保存失败: {e}")

    def _calculate_metrics(self, y_true, y_pred, prefix=''):
        """计算MSE、MAE、MAPE"""
        try:
            mse = mean_squared_error(y_true, y_pred)
            mae = mean_absolute_error(y_true, y_pred)
            # MAPE容易被零值影响，需要特殊处理
            mape = mean_absolute_percentage_error(y_true, y_pred) if len(y_true) > 0 else 0

            return {
                f'{prefix}mse': float(mse),
                f'{prefix}mae': float(mae),
                f'{prefix}mape': float(mape)
            }
        except Exception as e:
            logger.warning(f"[good_id={self.good_id}] 指标计算失败: {e}")
            return {}
    
    def _update_weights(self):
        """根据验证集MAPE动态调整权重 (Inverse Variance Weighting变体)"""
        try:
            # 获取各模型MAPE，若无则给一个较大默认值(1.0)
            mapes = {
                'lr': self.metrics.get('lr_mape') or 1.0,
                'prophet': self.metrics.get('prophet_mape') or 1.0,
                'xgb': self.metrics.get('xgb_mape') or 1.0
            }
            
            # 计算倒数 (误差越小权重越大)
            # 加一个小epsilon防止除零
            inv_mapes = {k: 1.0 / (v + 0.001) for k, v in mapes.items()}
            total_inv = sum(inv_mapes.values())
            
            if total_inv > 0:
                old_weights = self.weights.copy()
                self.weights = {k: v / total_inv for k, v in inv_mapes.items()}
                logger.info(f"[good_id={self.good_id}] 权重更新: LR {old_weights['lr']:.2f}→{self.weights['lr']:.2f} | Prophet {old_weights['prophet']:.2f}→{self.weights['prophet']:.2f} | XGB {old_weights['xgb']:.2f}→{self.weights['xgb']:.2f}")
            else:
                self.weights = {'lr': 0.2, 'prophet': 0.3, 'xgb': 0.5} # 回退默认

        except Exception as e:
            logger.warning(f"[good_id={self.good_id}] 权重计算失败: {e}")
            self.weights = {'lr': 0.2, 'prophet': 0.3, 'xgb': 0.5}
    
    def _log_metrics_comparison(self, strategy, training_time):
        """记录详细的训练效果对比"""
        try:
            # 提取关键指标
            lr_mape = self.metrics.get('lr_mape', 0)
            prophet_mape = self.metrics.get('prophet_mape', 0)
            xgb_mape = self.metrics.get('xgb_mape', 0)
            ensemble_mape = self.metrics.get('ensemble_mape', 0)
            
            lr_mae = self.metrics.get('lr_mae', 0)
            xgb_mae = self.metrics.get('xgb_mae', 0)
            ensemble_mae = self.metrics.get('ensemble_mae', 0)
            
            training_count = self.metrics.get('training_count', 0)
            
            # 构建对比信息
            logger.info(f"[good_id={self.good_id}] ═══════════════════════════════════════════════════════")
            logger.info(f"[good_id={self.good_id}] 📊 训练完成 (策略={strategy}, 次数={training_count}, 耗时={training_time:.2f}s)")
            logger.info(f"[good_id={self.good_id}] ───────────────────────────────────────────────────────")
            logger.info(f"[good_id={self.good_id}] MAPE精度 (越小越好):")
            logger.info(f"[good_id={self.good_id}]   • 线性回归 LR    : {lr_mape:.4f}")
            logger.info(f"[good_id={self.good_id}]   • Prophet预测   : {prophet_mape:.4f}")
            logger.info(f"[good_id={self.good_id}]   • XGBoost       : {xgb_mape:.4f}")
            logger.info(f"[good_id={self.good_id}]   • 集成预测 📈    : {ensemble_mape:.4f} (权重: LR={self.weights['lr']:.2f}, Prophet={self.weights['prophet']:.2f}, XGB={self.weights['xgb']:.2f})")
            logger.info(f"[good_id={self.good_id}] ───────────────────────────────────────────────────────")
            logger.info(f"[good_id={self.good_id}] MAE绝对误差 (真实价格波动):")
            logger.info(f"[good_id={self.good_id}]   • 线性回归 LR    : {lr_mae:.4f}")
            logger.info(f"[good_id={self.good_id}]   • XGBoost       : {xgb_mae:.4f}")
            logger.info(f"[good_id={self.good_id}]   • 集成预测 📈    : {ensemble_mae:.4f}")
            logger.info(f"[good_id={self.good_id}] ═══════════════════════════════════════════════════════")
            
        except Exception as e:
            logger.warning(f"[good_id={self.good_id}] 指标记录失败: {e}")

    def train(self, df):
        """智能训练：根据数据漂移和模型年龄选择训练策略"""
        logger.info(f"[good_id={self.good_id}] 🎯 开始训练流程 | 数据点={len(df)}")
        
        if len(df) < 10:
            logger.warning(f"[good_id={self.good_id}] ⚠️  数据不足: {len(df)} < 10")
            return False

        try:
            # ===== 0. 基础检查 =====
            last_timestamp = df.iloc[-1]['timestamp']
            if last_timestamp < datetime.now() - timedelta(hours=24):
                logger.warning(f"[good_id={self.good_id}] ⚠️  数据过旧: 最后更新于 {last_timestamp} ({(datetime.now() - last_timestamp).total_seconds() / 3600:.1f}小时前)")
                return False

            latest_sell_orders = df.iloc[-1]['sell_orders']
            if latest_sell_orders < 90:
                logger.warning(f"[good_id={self.good_id}] ⚠️  卖单过少: {latest_sell_orders} < 90")
                return False

            # ===== 1. 数据质量检查 =====
            quality_report = QUALITY_CHECKER.check_quality(df, self.good_id)
            logger.info(f"[good_id={self.good_id}] 📊 数据质量: {quality_report.quality_level} | 缺失值={quality_report.missing_ratio:.2%} | 异常值={quality_report.outlier_count}")

            # 检查12小时内价格是否完全相同
            recent_6h = df[df['timestamp'] >= df['timestamp'].max() - timedelta(hours=12)]
            if len(recent_6h) > 0 and recent_6h['sell_price'].nunique() == 1:
                logger.warning(f"[good_id={self.good_id}] ⚠️  检测到价格停滞: 近12小时价格无变化")
                return False

            # 数据质量严重时清理
            if quality_report.quality_level == 'critical':
                logger.warning(f"[good_id={self.good_id}] 🧹 数据质量严重，执行数据清理...")
                df_clean, clean_stats = DATA_CLEANER.clean_data(df)
                if len(df_clean) >= 10:
                    logger.info(f"[good_id={self.good_id}] ✓ 数据清理完成: {clean_stats}")
                    df = df_clean
                else:
                    logger.error(f"[good_id={self.good_id}] ❌ 数据清理后数据不足: {len(df_clean)}")
                    return False

            # ===== 2. 数据漂移检测 =====
            drift_report = DRIFT_DETECTOR.detect_drift(df['sell_price'].values)
            drift_report_dict = asdict(drift_report)
            drift_report_dict['good_id'] = self.good_id
            from data_quality_monitor import DataDriftReport
            drift_report = DataDriftReport(**drift_report_dict)

            # ===== 3. 生成告警 =====
            alerts = ALERT_SYSTEM.check_alerts(
                self.good_id,
                quality_report=asdict(quality_report),
                drift_report=asdict(drift_report),
                performance_metrics=self.metrics
            )

            if alerts:
                logger.warning(f"[good_id={self.good_id}] ⚠️  数据告警 ({len(alerts)} 个):")
                for alert in alerts:
                    alert_icon = '🔴' if alert.alert_level == 'critical' else '🟡'
                    logger.warning(f"[good_id={self.good_id}]   {alert_icon} [{alert.alert_level.upper()}] {alert.title}")
                ALERT_SYSTEM.save_alerts(alerts)

            # ===== 4. 智能训练决策 =====
            training_strategy = self._decide_training_strategy(df, drift_report, quality_report)
            
            if training_strategy == 'skip':
                logger.info(f"[good_id={self.good_id}] 数据稳定，模型较新，跳过训练")
                return True
            elif training_strategy == 'incremental':
                logger.info(f"[good_id={self.good_id}] 执行增量训练 (中度漂移)")
                return self._incremental_train(df, drift_report, quality_report)
            else:  # 'full'
                logger.info(f"[good_id={self.good_id}] 执行全量训练 (严重漂移或首次训练)")
                return self._full_retrain(df, drift_report, quality_report)

        except Exception as e:
            logger.error(f"[good_id={self.good_id}] 训练失败: {e}")
            import traceback
            logger.error(traceback.format_exc())
            return False

    def _decide_training_strategy(self, df, drift_report, quality_report):
        """决定训练策略: 'skip', 'incremental', 'full'"""
        # 1. 如果没有现有模型，必须全量训练
        if self.xgb is None or self.lr is None or self.prophet is None:
            logger.info(f"[good_id={self.good_id}] 📌 首次训练：无现有模型")
            return 'full'
        
        # 2. 检查模型年龄
        model_age_days = 999
        if self.last_timestamp:
            model_age = datetime.now() - self.last_timestamp
            model_age_days = model_age.total_seconds() / 86400
        
        # 3. 获取漂移程度
        drift_level = drift_report.drift_level  # 'none', 'mild', 'moderate', 'severe'
        ks_statistic = drift_report.ks_statistic
        
        logger.info(f"[good_id={self.good_id}] 🔍 训练决策评估 | 模型年龄={model_age_days:.1f}天 | 漂移度={drift_level}(KS={ks_statistic:.3f})")
        
        # 4. 决策逻辑
        if model_age_days > 7:
            # 模型太旧 (>7天)，全量重训练
            logger.info(f"[good_id={self.good_id}] 📌 决策: 全量训练 (原因: 模型太旧 {model_age_days:.1f}天 > 7天)")
            return 'full'
        
        if drift_level == 'severe' or ks_statistic > 0.5:
            # 严重漂移，全量重训练
            logger.info(f"[good_id={self.good_id}] 📌 决策: 全量训练 (原因: 严重漂移 KS={ks_statistic:.3f} > 0.5)")
            return 'full'
        
        if drift_level == 'moderate' or (0.3 < ks_statistic <= 0.5):
            # 中度漂移，增量训练
            logger.info(f"[good_id={self.good_id}] 📌 决策: 增量训练 (原因: 中度漂移 KS={ks_statistic:.3f})")
            return 'incremental'
        
        if model_age_days > 3:
            # 模型稍旧 (3-7天)，增量训练
            logger.info(f"[good_id={self.good_id}] 📌 决策: 增量训练 (原因: 模型偏旧 {model_age_days:.1f}天)")
            return 'incremental'
        
        # 轻微或无漂移，模型较新，跳过训练
        logger.info(f"[good_id={self.good_id}] 📌 决策: 跳过训练 (原因: 数据稳定 KS={ks_statistic:.3f}, 模型新鲜 {model_age_days:.1f}天)")
        return 'skip'

    def _full_retrain(self, df, drift_report, quality_report):
        """全量重训练所有模型"""
        train_start = time.time()
        
        try:
            # 改进: 使用80%训练集，让模型看到更多近期数据
            split_point = int(len(df) * 0.8)
            df_train = df[:split_point].copy()
            df_test = df[split_point:].copy()
            self.train_size = len(df_train)

            self.last_price = df.iloc[-1]['sell_price']
            self.last_timestamp = df.iloc[-1]['timestamp']

            y_test = df_test['sell_price'].values

            # ===== 线性回归 (全新模型 - 加权训练，近期数据权重更高) =====
            y_train = df_train['sell_price'].values
            X_train = np.arange(len(y_train)).reshape(-1, 1)
            
            # 指数衰减权重: 近期数据权重高，远期数据权重低
            # 权重范围: [0.1, 1.0]，最近的数据权重为1.0
            weights = np.exp(np.linspace(-2, 0, len(y_train)))
            
            self.lr = LinearRegression()  # 创建新模型
            self.lr.fit(X_train, y_train, sample_weight=weights)

            X_test = np.arange(len(y_train), len(y_train) + len(y_test)).reshape(-1, 1)
            y_pred_lr = self.lr.predict(X_test)
            lr_metrics = self._calculate_metrics(y_test, y_pred_lr, 'lr_')
            self.metrics.update(lr_metrics)

            # ===== Prophet (全新模型) =====
            df_prophet = df_train[['timestamp', 'sell_price']].copy()
            df_prophet.columns = ['ds', 'y']
            self.prophet = Prophet(  # 创建新模型
                yearly_seasonality=False,
                weekly_seasonality=True,
                daily_seasonality=False,
                interval_width=0.95
            )
            self.prophet.fit(df_prophet)

            future_test = df_test[['timestamp']].copy()
            future_test.columns = ['ds']
            forecast_test = self.prophet.predict(future_test)
            y_pred_prophet = forecast_test['yhat'].values
            prophet_metrics = self._calculate_metrics(y_test, y_pred_prophet, 'prophet_')
            self.metrics.update(prophet_metrics)

            # ===== XGBoost (全新模型) =====
            df_features = prepare_features(df_train)
            # 只选择FEATURE_COLS中实际存在的列
            available_cols = [col for col in self.FEATURE_COLS if col in df_features.columns]
            X_train_xgb = df_features[available_cols].values
            y_train_xgb = df_features['sell_price'].values

            self.xgb = XGBRegressor(  # 创建新模型
                n_estimators=50,
                max_depth=4,
                learning_rate=0.1,
                subsample=0.8,
                colsample_bytree=0.8,
                random_state=42,
                verbosity=0
            )
            self.xgb.fit(X_train_xgb, y_train_xgb)

            df_test_features = prepare_features(df_test)
            # 只选择FEATURE_COLS中实际存在的列
            available_cols = [col for col in self.FEATURE_COLS if col in df_test_features.columns]
            X_test_xgb = df_test_features[available_cols].values
            y_pred_xgb = self.xgb.predict(X_test_xgb)
            xgb_metrics = self._calculate_metrics(y_test, y_pred_xgb, 'xgb_')
            self.metrics.update(xgb_metrics)

            # ===== 动态权重与集成预测 =====
            self._update_weights()
            
            ensemble_pred = (
                y_pred_lr * self.weights['lr'] +
                y_pred_prophet * self.weights['prophet'] +
                y_pred_xgb * self.weights['xgb']
            )
            ensemble_metrics = self._calculate_metrics(y_test, ensemble_pred, 'ensemble_')
            self.metrics.update(ensemble_metrics)

            training_time = time.time() - train_start
            self.metrics['training_time'] = training_time
            # 确保training_count正确累加
            current_count = self.metrics.get('training_count', 0)
            self.metrics['training_count'] = current_count + 1
            self.metrics['last_training'] = datetime.now().isoformat()
            self.metrics['training_strategy'] = 'full_retrain'

            self.metrics['quality_report'] = asdict(quality_report)
            self.metrics['drift_report'] = asdict(drift_report)

            # 标记为完整特征集
            self.feature_set = 'full'
            self.save_model()
            self.save_metrics()

            self._log_metrics_comparison(strategy='full_retrain', training_time=training_time)
            return True

        except Exception as e:
            logger.error(f"[good_id={self.good_id}] 全量训练失败: {e}")
            import traceback
            logger.error(f"[good_id={self.good_id}] 错误堆栈: {traceback.format_exc()}")
            return False

    def _incremental_train(self, df, drift_report, quality_report):
        """增量训练：仅更新XGBoost，LR和Prophet重训练"""
        train_start = time.time()
        
        try:
            # 改进: 使用80%训练集，让模型看到更多近期数据
            split_point = int(len(df) * 0.8)
            df_train = df[:split_point].copy()
            df_test = df[split_point:].copy()
            self.train_size = len(df_train)

            self.last_price = df.iloc[-1]['sell_price']
            self.last_timestamp = df.iloc[-1]['timestamp']

            y_test = df_test['sell_price'].values

            # ===== 线性回归 (重训练，轻量 - 加权训练) =====
            y_train = df_train['sell_price'].values
            X_train = np.arange(len(y_train)).reshape(-1, 1)
            
            # 指数衰减权重: 近期数据权重高
            weights = np.exp(np.linspace(-2, 0, len(y_train)))
            
            self.lr = LinearRegression()
            self.lr.fit(X_train, y_train, sample_weight=weights)

            X_test = np.arange(len(y_train), len(y_train) + len(y_test)).reshape(-1, 1)
            y_pred_lr = self.lr.predict(X_test)
            lr_metrics = self._calculate_metrics(y_test, y_pred_lr, 'lr_')
            self.metrics.update(lr_metrics)

            # ===== Prophet (重训练，较重) =====
            df_prophet = df_train[['timestamp', 'sell_price']].copy()
            df_prophet.columns = ['ds', 'y']
            self.prophet = Prophet(
                yearly_seasonality=False,
                weekly_seasonality=True,
                daily_seasonality=False,
                interval_width=0.95
            )
            self.prophet.fit(df_prophet)

            future_test = df_test[['timestamp']].copy()
            future_test.columns = ['ds']
            forecast_test = self.prophet.predict(future_test)
            y_pred_prophet = forecast_test['yhat'].values
            prophet_metrics = self._calculate_metrics(y_test, y_pred_prophet, 'prophet_')
            self.metrics.update(prophet_metrics)

            # ===== XGBoost (增量训练 - 关键改进) =====
            df_features = prepare_features(df_train)
            # 只选择FEATURE_COLS中实际存在的列
            available_cols = [col for col in self.FEATURE_COLS if col in df_features.columns]
            X_train_xgb = df_features[available_cols].values
            y_train_xgb = df_features['sell_price'].values

            if self.xgb is not None:
                # 基于旧模型继续训练
                logger.info(f"[good_id={self.good_id}] XGBoost 增量更新...")
                self.xgb.fit(
                    X_train_xgb, y_train_xgb,
                    xgb_model=self.xgb.get_booster()  # 增量训练
                )
            else:
                # 没有旧模型，创建新的
                self.xgb = XGBRegressor(
                    n_estimators=50,
                    max_depth=4,
                    learning_rate=0.1,
                    subsample=0.8,
                    colsample_bytree=0.8,
                    random_state=42,
                    verbosity=0
                )
                self.xgb.fit(X_train_xgb, y_train_xgb)

            df_test_features = prepare_features(df_test)
            # 只选择FEATURE_COLS中实际存在的列
            available_cols = [col for col in self.FEATURE_COLS if col in df_test_features.columns]
            X_test_xgb = df_test_features[available_cols].values
            y_pred_xgb = self.xgb.predict(X_test_xgb)
            xgb_metrics = self._calculate_metrics(y_test, y_pred_xgb, 'xgb_')
            self.metrics.update(xgb_metrics)

            # ===== 动态权重与集成预测 =====
            self._update_weights()
            
            ensemble_pred = (
                y_pred_lr * self.weights['lr'] +
                y_pred_prophet * self.weights['prophet'] +
                y_pred_xgb * self.weights['xgb']
            )
            ensemble_metrics = self._calculate_metrics(y_test, ensemble_pred, 'ensemble_')
            self.metrics.update(ensemble_metrics)

            training_time = time.time() - train_start
            self.metrics['training_time'] = training_time
            # 确保training_count正确累加
            current_count = self.metrics.get('training_count', 0)
            self.metrics['training_count'] = current_count + 1
            self.metrics['last_training'] = datetime.now().isoformat()
            self.metrics['training_strategy'] = 'incremental'

            self.metrics['quality_report'] = asdict(quality_report)
            self.metrics['drift_report'] = asdict(drift_report)

            # 标记为完整特征集
            self.feature_set = 'full'
            self.save_model()
            self.save_metrics()

            self._log_metrics_comparison(strategy='incremental', training_time=training_time)
            return True

        except Exception as e:
            logger.error(f"[good_id={self.good_id}] 增量训练失败: {e}")
            import traceback
            logger.error(f"[good_id={self.good_id}] 错误堆栈: {traceback.format_exc()}")
            return False

    def predict(self, days=7):
        """预测未来N天的价格（以天为单位）"""
        if self.lr is None or self.prophet is None or self.xgb is None:
            return None

        predictions = {
            'current_price': float(self.last_price),
            'last_timestamp': self.last_timestamp.isoformat(),
            'forecast_days': days,
            'predictions': {}
        }

        try:
            future_dates = pd.date_range(
                start=self.last_timestamp + timedelta(days=1),
                periods=days,
                freq='D'
            )

            # ===== 线性回归预测 =====
            day_indices = np.arange(self.train_size, self.train_size + days).reshape(-1, 1)
            lr_pred = np.maximum(self.lr.predict(day_indices), 0)
            predictions['predictions']['lr'] = {
                'forecast': [float(p) for p in lr_pred],
                'dates': [d.isoformat() for d in future_dates],
                'model': 'LinearRegression'
            }

            # ===== Prophet 预测 =====
            future_df = pd.DataFrame({'ds': future_dates})
            forecast = self.prophet.predict(future_df)
            prophet_pred = np.maximum(forecast['yhat'].values, 0)
            prophet_lower = np.maximum(forecast['yhat_lower'].values, 0)
            prophet_upper = forecast['yhat_upper'].values

            predictions['predictions']['prophet'] = {
                'forecast': [float(p) for p in prophet_pred],
                'lower': [float(l) for l in prophet_lower],
                'upper': [float(u) for u in prophet_upper],
                'dates': [d.isoformat() for d in future_dates],
                'model': 'Facebook Prophet'
            }

            # ===== XGBoost 预测 =====
            last_features = self._generate_future_features(days)
            # 只选择FEATURE_COLS中实际存在的列
            available_cols = [col for col in self.FEATURE_COLS if col in last_features.columns]
            X_future_xgb = last_features[available_cols].values
            xgb_pred = np.maximum(self.xgb.predict(X_future_xgb), 0)

            predictions['predictions']['xgb'] = {
                'forecast': [float(p) for p in xgb_pred],
                'dates': [d.isoformat() for d in future_dates],
                'model': 'XGBoost'
            }

            # ===== 集成预测 (动态权重) =====
            ensemble_pred = (
                np.array(predictions['predictions']['lr']['forecast']) * self.weights['lr'] +
                np.array(predictions['predictions']['prophet']['forecast']) * self.weights['prophet'] +
                np.array(predictions['predictions']['xgb']['forecast']) * self.weights['xgb']
            )

            predictions['ensemble'] = {
                'forecast': [float(p) for p in ensemble_pred],
                'dates': [d.isoformat() for d in future_dates],
                'weights': self.weights,
                'model': 'Weighted Ensemble'
            }

            predictions['quality_metrics'] = {
                'ensemble_mape': self.metrics.get('ensemble_mape'),
                'ensemble_mae': self.metrics.get('ensemble_mae'),
                'training_count': self.metrics.get('training_count', 0),
                'last_training': self.metrics.get('last_training')
            }

            # ===== 生成推荐 (交易策略：现在买入 -> 7天后卖出) =====
            avg_future_price = np.mean(ensemble_pred)
            future_change_pct = ((avg_future_price - self.last_price) / self.last_price) * 100

            # 计算近期趋势（过去7天的价格变化）
            try:
                recent_data = fetch_historical_data(self.good_id, days=7)
                if recent_data is not None and len(recent_data) >= 2:
                    recent_start_price = recent_data.iloc[0]['sell_price']
                    recent_trend_pct = ((self.last_price - recent_start_price) / recent_start_price) * 100
                else:
                    recent_trend_pct = 0
            except:
                recent_trend_pct = 0

            # 手续费阈值 (双边手续费约1%)
            FEE_THRESHOLD = 1.0
            PROFIT_THRESHOLD = 3.0  # 期望收益阈值
            CHASE_HIGH_THRESHOLD = 5.0  # 追高风险阈值

            # 智能推荐逻辑
            if future_change_pct > PROFIT_THRESHOLD:
                # 预测未来会涨 > 3%
                if recent_trend_pct > CHASE_HIGH_THRESHOLD:
                    # 近期已经大涨 > 5%，追高风险
                    recommendation = 'hold'
                    reason = f'虽然预测7天后上涨{future_change_pct:.1f}%，但近7天已涨{recent_trend_pct:.1f}%，追高风险大，建议观望'
                else:
                    # 近期未大涨，可以买入
                    recommendation = 'buy'
                    expected_profit = future_change_pct - FEE_THRESHOLD
                    reason = f'预测7天后上涨{future_change_pct:.1f}%，扣除手续费约{FEE_THRESHOLD}%，预期收益{expected_profit:.1f}%，建议买入'
            elif future_change_pct < -FEE_THRESHOLD:
                # 预测未来会跌
                recommendation = 'hold'
                reason = f'预测7天后下跌{future_change_pct:.1f}%，现在买入会亏损，不建议操作'
            else:
                # 预测变化不大
                recommendation = 'hold'
                reason = f'预测7天后价格变化{future_change_pct:.1f}%，收益不足以覆盖手续费{FEE_THRESHOLD}%，建议观望'

            # 计算预期净收益（考虑手续费）
            # 如果预测涨3%，扣除手续费后净收益约1%
            # 如果预测跌3%，加上手续费后亏损约5%
            if future_change_pct > 0:
                expected_profit = future_change_pct - FEE_THRESHOLD  # 上涨扣手续费
            else:
                expected_profit = future_change_pct - FEE_THRESHOLD  # 下跌还要扣手续费，亏更多
            
            predictions['recommendation'] = {
                'action': recommendation,
                'avg_future_price': float(avg_future_price),
                'future_change_pct': float(future_change_pct),
                'recent_trend_pct': float(recent_trend_pct),
                'expected_profit': float(expected_profit),
                'reason': reason,
                'confidence': 0.95
            }

            return predictions

        except Exception as e:
            logger.error(f"[good_id={self.good_id}] 预测失败: {e}")
            import traceback
            logger.error(traceback.format_exc())
            return None

    def _generate_future_features(self, days):
        """生成未来特征（以天为单位）"""
        future_dates = pd.date_range(
            start=self.last_timestamp + timedelta(days=1),
            periods=days,
            freq='D'
        )

        # 获取历史数据来计算趋势特征
        df_hist = fetch_historical_data(self.good_id, days=30)
        
        # 计算当前的趋势值
        if df_hist is not None and len(df_hist) >= 30:
            recent_7d = df_hist.tail(7)['sell_price'].values
            recent_30d = df_hist.tail(30)['sell_price'].values
            trend_7d = (recent_7d[-1] - recent_7d[0]) / recent_7d[0] if len(recent_7d) > 0 else 0
            trend_30d = (recent_30d[-1] - recent_30d[0]) / recent_30d[0] if len(recent_30d) > 0 else 0
            ma7 = recent_7d.mean()
            ma30 = recent_30d.mean()
            momentum = (ma7 - ma30) / ma30 if ma30 > 0 else 0
            price_max = recent_30d.max()
            price_min = recent_30d.min()
            price_position = (self.last_price - price_min) / (price_max - price_min + 0.01)
        else:
            trend_7d = trend_30d = momentum = price_position = 0

        future_df = pd.DataFrame({
            'timestamp': future_dates,
            'day_of_week': future_dates.dayofweek,
            'day_of_month': future_dates.day,
            'days_since_start': [(d - self.last_timestamp).days for d in future_dates],
            'price_range': 0.5,
            'total_orders': 100,
            'order_ratio': 0.5,
            'buy_price_ma3': self.last_price,
            'sell_price_ma3': self.last_price,
            'price_change_ma': 0.0,
            # 新增趋势特征（使用历史数据计算）
            'trend_7d': trend_7d,
            'trend_30d': trend_30d,
            'momentum': momentum,
            'price_position': price_position
        })
        
        # 确保只返回FEATURE_COLS中需要的特征列
        future_df = future_df[[col for col in self.FEATURE_COLS if col in future_df.columns]]

        return future_df


# ============================================================================
# API 端点
# ============================================================================

@app.route('/api/health', methods=['GET'])
def health_check():
    """健康检查"""
    return jsonify({
        'status': 'ok',
        'timestamp': datetime.now().isoformat(),
        'cached_models': CACHE_MANAGER.size(),
        'version': '2.1.0-optimized',
        'features': [
            'concurrency-optimized',
            'connection-pooling',
            'dynamic-weights',
            'fee-aware-recommendation',
            'model-persistence'
        ]
    }), 200


@app.route('/api/predict/<int:good_id>', methods=['GET'])
def predict_endpoint(good_id):
    """预测单个商品"""
    try:
        days = request.args.get('days', default=7, type=int)
        if days < 1 or days > 30:
            return jsonify({'error': '预测天数必须在 1-30 之间'}), 400

        logger.info(f"[good_id={good_id}] 📤 收到预测请求 | 预测天数={days}天")
        
        # 使用细粒度锁
        item_lock = CACHE_MANAGER.get_lock(good_id)
        
        with item_lock:
            # 尝试从缓存获取
            model = CACHE_MANAGER.get(good_id)
            
            if model is None:
                logger.info(f"[good_id={good_id}] 💾 缓存未命中，尝试从磁盘加载或训练")
                # 缓存未命中，创建新模型
                model = PredictionModel(good_id)
                # 尝试加载磁盘模型
                if not model.load_model():
                    # 磁盘无模型，需要训练
                    logger.info(f"[good_id={good_id}] 🔄 开始训练新模型...")
                    df = fetch_historical_data(good_id, days=30)
                    if df is None or len(df) < 10:
                        logger.warning(f"[good_id={good_id}] ❌ 数据不足: {len(df) if df is not None else 0} < 10")
                        return jsonify({'error': '数据不足'}), 400

                    if not model.train(df):
                        logger.error(f"[good_id={good_id}] ❌ 模型训练失败")
                        return jsonify({'error': '模型训练失败'}), 400
                else:
                    logger.info(f"[good_id={good_id}] ✓ 从磁盘加载成功")
                
                # 存入缓存
                CACHE_MANAGER.put(good_id, model)
            else:
                logger.debug(f"[good_id={good_id}] ⚡ 模型来自内存缓存")

            result = model.predict(days=days)
            if result is None:
                logger.error(f"[good_id={good_id}] ❌ 预测计算失败")
                return jsonify({'error': '预测失败'}), 400

            result['good_id'] = good_id
            recommendation = result.get('recommendation', {})
            action = recommendation.get('action', 'unknown')
            confidence = recommendation.get('confidence', 0)
            logger.info(f"[good_id={good_id}] ✅ 预测完成 | 推荐={action} | 置信度={confidence} | 预期收益={recommendation.get('expected_profit', 0):.2f}%")
            
            return jsonify(result), 200

    except Exception as e:
        logger.error(f"[good_id={good_id}] ❌ 异常: {e}", exc_info=True)
        return jsonify({'error': str(e)}), 500

def process_single_good(good_id, days):
    """处理单个商品的函数 (用于线程池)"""
    try:
        item_lock = CACHE_MANAGER.get_lock(good_id)
        with item_lock:
            model = CACHE_MANAGER.get(good_id)
            status = "cached"
            
            if model is None:
                logger.info(f"[good_id={good_id}] [批处理] 缓存未命中，创建新模型")
                model = PredictionModel(good_id)
                if not model.load_model():
                    logger.info(f"[good_id={good_id}] [批处理] 磁盘无模型，开始训练...")
                    df = fetch_historical_data(good_id, days=30)
                    if df is None or len(df) < 10:
                        logger.warning(f"[good_id={good_id}] [批处理] ⚠️  跳过: 数据不足 ({len(df) if df is not None else 0}条)")
                        return None, "skipped_no_data"

                    if not model.train(df):
                        logger.error(f"[good_id={good_id}] [批处理] ❌ 训练失败")
                        return None, "skipped_train_failed"
                    status = "trained"
                    logger.info(f"[good_id={good_id}] [批处理] ✓ 训练成功 | MAPE={model.metrics.get('ensemble_mape', 0):.4f}")
                else:
                    status = "loaded_disk"
                    logger.info(f"[good_id={good_id}] [批处理] ✓ 从磁盘加载 | 训练次数={model.metrics.get('training_count', 0)}")
                
                CACHE_MANAGER.put(good_id, model)
            else:
                logger.debug(f"[good_id={good_id}] [批处理] ⚡ 来自内存缓存")

            result = model.predict(days=days)
            if result:
                result['good_id'] = good_id
                recommendation = result.get('recommendation', {})
                action = recommendation.get('action', 'unknown')
                change_pct = recommendation.get('future_change_pct', 0)
                expected_profit = recommendation.get('expected_profit', 0)
                logger.info(f"[good_id={good_id}] [批处理] ✅ 预测完成 | 推荐={action} | 预计变化={change_pct:.2f}% | 预期收益={expected_profit:.2f}%")
                return result, status
            else:
                logger.error(f"[good_id={good_id}] [批处理] ❌ 预测失败")
                return None, "predict_failed"
            
    except Exception as e:
        logger.error(f"[good_id={good_id}] [批处理] ❌ 异常: {e}", exc_info=True)
        return None, "error"

@app.route('/api/batch-predict', methods=['POST'])
def batch_predict_endpoint():
    """批量预测 (并发优化版)"""
    try:
        data = request.get_json()
        good_ids = data.get('good_ids', [])
        days = data.get('days', 7)

        if not good_ids or len(good_ids) > 100:
            return jsonify({'error': '商品数必须在 1-100 之间'}), 400

        batch_id = datetime.now().strftime('%Y%m%d_%H%M%S')
        logger.info(f"")
        logger.info(f"🚀 ═══════════════════════════════════════════════════════════════════════════")
        logger.info(f"🚀 批量预测开始 [batch_id={batch_id}] | 商品数={len(good_ids)} | 预测天数={days}天")
        logger.info(f"🚀 商品列表: {good_ids}")
        logger.info(f"🚀 ───────────────────────────────────────────────────────────────────────────")
        
        results = []
        stats = defaultdict(int)
        processed = 0
        start_time = time.time()
        
        # 使用线程池并发处理
        # max_workers 根据机器性能调整，一般 CPU核心数 * 2
        with ThreadPoolExecutor(max_workers=8) as executor:
            future_to_good = {executor.submit(process_single_good, gid, days): gid for gid in good_ids}
            total = len(good_ids)
            
            for future in as_completed(future_to_good):
                good_id = future_to_good[future]
                processed += 1
                try:
                    result, status = future.result()
                    stats[status] += 1
                    if result:
                        results.append(result)
                    progress_pct = (processed / total) * 100
                    elapsed_time = time.time() - start_time
                    eta_seconds = (elapsed_time / processed * (total - processed)) if processed > 0 else 0
                    logger.info(f"🚀 [进度] {processed:2d}/{total} ({progress_pct:5.1f}%) | ETA={int(eta_seconds)}s | [{status:15s}] good_id={good_id}")
                except Exception as e:
                    logger.error(f"🚀 [good_id={good_id}] ❌ 线程异常: {e}", exc_info=True)
                    stats['thread_error'] += 1

        total_time = time.time() - start_time
        logger.info(f"🚀 ───────────────────────────────────────────────────────────────────────────")
        logger.info(f"🚀 ✅ 批量处理完成 | 成功={len(results)}/{len(good_ids)} | 耗时={total_time:.2f}s")
        logger.info(f"🚀 统计明细:")
        logger.info(f"🚀   • trained        (新训练): {stats.get('trained', 0):3d}个")
        logger.info(f"🚀   • loaded_disk    (磁盘加载): {stats.get('loaded_disk', 0):3d}个")
        logger.info(f"🚀   • cached         (内存缓存): {stats.get('cached', 0):3d}个")
        logger.info(f"🚀   • predict_failed (预测失败): {stats.get('predict_failed', 0):3d}个")
        logger.info(f"🚀   • skipped_no_data (数据不足): {stats.get('skipped_no_data', 0):3d}个")
        logger.info(f"🚀   • skipped_train_failed (训练失败): {stats.get('skipped_train_failed', 0):3d}个")
        logger.info(f"🚀   • thread_error   (线程异常): {stats.get('thread_error', 0):3d}个")
        logger.info(f"🚀 ═══════════════════════════════════════════════════════════════════════════")
        logger.info(f"")

        return jsonify({
            'total_requested': len(good_ids),
            'total_success': len(results),
            'stats': stats,
            'results': results
        }), 200

    except Exception as e:
        logger.error(f"批量预测异常: {e}")
        return jsonify({'error': str(e)}), 500


@app.route('/api/model-metrics/<int:good_id>', methods=['GET'])
def model_metrics_endpoint(good_id):
    """获取模型质量指标"""
    try:
        item_lock = CACHE_MANAGER.get_lock(good_id)
        with item_lock:
            model = CACHE_MANAGER.get(good_id)
            if model is None:
                model = PredictionModel(good_id)
                if not model.load_model():
                    return jsonify({'error': '模型不存在'}), 404
                CACHE_MANAGER.put(good_id, model)

            return jsonify({
                'good_id': good_id,
                'metrics': model.metrics,
                'weights': model.weights,
                'timestamp': datetime.now().isoformat()
            }), 200

    except Exception as e:
        logger.error(f"异常: {e}")
        return jsonify({'error': str(e)}), 500


@app.route('/api/alerts/<int:good_id>', methods=['GET'])
def get_alerts_endpoint(good_id):
    """获取某商品的告警信息"""
    try:
        active_alerts = ALERT_SYSTEM.get_active_alerts(good_id)
        return jsonify({
            'good_id': good_id,
            'total_alerts': len(active_alerts),
            'alerts': [asdict(a) for a in active_alerts],
            'timestamp': datetime.now().isoformat()
        }), 200
    except Exception as e:
        logger.error(f"获取告警异常: {e}")
        return jsonify({'error': str(e)}), 500


@app.route('/api/alerts/active', methods=['GET'])
def get_active_alerts_endpoint():
    """获取所有未确认的告警"""
    try:
        active_alerts = ALERT_SYSTEM.get_active_alerts()
        alert_summary = ALERT_SYSTEM.get_alert_summary()
        return jsonify({
            'summary': alert_summary,
            'timestamp': datetime.now().isoformat()
        }), 200
    except Exception as e:
        logger.error(f"获取活跃告警异常: {e}")
        return jsonify({'error': str(e)}), 500


@app.route('/api/alerts/summary', methods=['GET'])
def alerts_summary_endpoint():
    """获取告警摘要"""
    try:
        summary = ALERT_SYSTEM.get_alert_summary()
        return jsonify(summary), 200
    except Exception as e:
        logger.error(f"获取告警摘要异常: {e}")
        return jsonify({'error': str(e)}), 500


@app.route('/api/data-quality/<int:good_id>', methods=['GET'])
def data_quality_endpoint(good_id):
    """获取某商品的数据质量报告"""
    try:
        item_lock = CACHE_MANAGER.get_lock(good_id)
        with item_lock:
            model = CACHE_MANAGER.get(good_id)
            if model is None:
                model = PredictionModel(good_id)
                if not model.load_model():
                    return jsonify({'error': '模型不存在或无质量报告'}), 404
                CACHE_MANAGER.put(good_id, model)

            quality_report = model.metrics.get('quality_report')
            drift_report = model.metrics.get('drift_report')

            if not quality_report or not drift_report:
                return jsonify({'error': '尚无质量和漂移报告'}), 404

            return jsonify({
                'good_id': good_id,
                'quality_report': quality_report,
                'drift_report': drift_report,
                'timestamp': datetime.now().isoformat()
            }), 200

    except Exception as e:
        logger.error(f"获取数据质量报告异常: {e}")
        return jsonify({'error': str(e)}), 500


@app.route('/api/clear-cache', methods=['POST'])
def clear_cache_endpoint():
    """清空模型缓存"""
    try:
        size = CACHE_MANAGER.size()
        CACHE_MANAGER.clear()
        return jsonify({
            'status': 'success',
            'message': f'清空了 {size} 个模型缓存'
        }), 200
    except Exception as e:
        logger.error(f"清空缓存异常: {e}")
        return jsonify({'error': str(e)}), 500


@app.route('/api/cache-status', methods=['GET'])
def cache_status_endpoint():
    """查看缓存状态"""
    try:
        status = {
            'cached_models': CACHE_MANAGER.size(),
            'max_size': CACHE_MANAGER.max_size,
            'model_dir': str(MODEL_DIR),
            'metrics_dir': str(METRICS_DIR),
            'timestamp': datetime.now().isoformat()
        }
        return jsonify(status), 200
    except Exception as e:
        logger.error(f"查看缓存状态异常: {e}")
        return jsonify({'error': str(e)}), 500


# ============================================================================
# 启动
# ============================================================================

if __name__ == '__main__':
    logger.info("=" * 60)
    logger.info("CSGO 预测服务 v2.1 (性能优化版)")
    logger.info("特性: 并发优化 | 动态权重 | 交易手续费=1%")
    logger.info("=" * 60)
    logger.info(f"数据库: {DB_CONFIG['host']}")
    logger.info(f"模型目录: {MODEL_DIR}")
    logger.info(f"指标目录: {METRICS_DIR}")
    logger.info("=" * 60)

    app.run(debug=False, host='0.0.0.0', port=5000, threaded=True)
