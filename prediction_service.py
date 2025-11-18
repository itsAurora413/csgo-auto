#!/usr/bin/env python3
"""
CSGO 饰品市场 - 集成预测服务
支持: XGBoost + Prophet + 线性回归集成预测
"""

import sys
import json
import warnings
import pickle
import logging
from datetime import datetime, timedelta
from pathlib import Path
from threading import Lock
import time

import numpy as np
import pandas as pd
from sklearn.linear_model import LinearRegression
from sklearn.metrics import mean_absolute_percentage_error, mean_squared_error, mean_absolute_error
from prophet import Prophet
from xgboost import XGBRegressor
import pymysql
from flask import Flask, jsonify, request, Response
from flask_cors import CORS

warnings.filterwarnings('ignore')

# ============================================================================
# 配置
# ============================================================================

LOG_FORMAT = '%(asctime)s - %(name)s - %(levelname)s - %(message)s'
logging.basicConfig(level=logging.INFO, format=LOG_FORMAT)
logger = logging.getLogger(__name__)

DB_CONFIG = {
    'host': '23.254.215.66',
    'user': 'root',
    'password': 'Wyj250413.',
    'database': 'csgo_trader',
    'charset': 'utf8mb4'
}

CACHE_DIR = Path('/Users/user/Downloads/csgoAuto/.cache')
CACHE_DIR.mkdir(exist_ok=True)

# Flask 应用
app = Flask(__name__)
CORS(app)

# 全局模型缓存 (内存中)
MODEL_CACHE = {}
MODEL_LOCK = Lock()


# ============================================================================
# 数据库操作
# ============================================================================

def get_db_connection():
    """获取数据库连接"""
    try:
        return pymysql.connect(**DB_CONFIG)
    except Exception as e:
        logger.error(f"数据库连接失败: {e}")
        return None


def fetch_historical_data(good_id, days=30):
    """从数据库获取历史价格数据"""
    conn = get_db_connection()
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
    finally:
        conn.close()


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

    # 处理缺失值
    df_features = df_features.fillna(method='ffill').fillna(method='bfill')

    return df_features


# ============================================================================
# 模型训练
# ============================================================================

class PredictionModel:
    """预测模型集合"""

    FEATURE_COLS = [
        'day_of_week', 'day_of_month', 'days_since_start',
        'price_range', 'total_orders', 'order_ratio',
        'buy_price_ma3', 'sell_price_ma3', 'price_change_ma'
    ]

    def __init__(self):
        self.lr = None
        self.prophet = None
        self.xgb = None
        self.last_price = None
        self.last_timestamp = None
        self.train_size = 0

    def train(self, df):
        """训练所有模型"""
        if len(df) < 10:
            return False

        try:
            split_point = int(len(df) * 0.7)
            df_train = df[:split_point].copy()
            self.train_size = len(df_train)

            # 保存最后一个价格和时间戳
            self.last_price = df.iloc[-1]['sell_price']
            self.last_timestamp = df.iloc[-1]['timestamp']

            # ===== 线性回归 =====
            y_train = df_train['sell_price'].values
            X_train = np.arange(len(y_train)).reshape(-1, 1)
            self.lr = LinearRegression()
            self.lr.fit(X_train, y_train)

            # ===== Prophet =====
            df_prophet = df_train[['timestamp', 'sell_price']].copy()
            df_prophet.columns = ['ds', 'y']
            self.prophet = Prophet(
                yearly_seasonality=False,
                weekly_seasonality=True,
                daily_seasonality=False,
                interval_width=0.95
            )
            self.prophet.fit(df_prophet)

            # ===== XGBoost =====
            df_features = prepare_features(df_train)
            X_train_xgb = df_features[self.FEATURE_COLS].values
            y_train_xgb = df_features['sell_price'].values

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

            logger.info(f"模型训练成功: {len(df_train)} 条训练数据")
            return True

        except Exception as e:
            logger.error(f"模型训练失败: {e}")
            return False

    def predict(self, days=7):
        """预测未来N天的价格"""
        if self.lr is None or self.prophet is None or self.xgb is None:
            return None

        predictions = {
            'current_price': float(self.last_price),
            'last_timestamp': self.last_timestamp.isoformat(),
            'forecast_days': days,
            'predictions': {}
        }

        try:
            # ===== 线性回归预测 =====
            X_future = np.arange(self.train_size, self.train_size + days).reshape(-1, 1)
            lr_pred = np.maximum(self.lr.predict(X_future), 0)
            predictions['predictions']['lr'] = {
                'forecast': [float(p) for p in lr_pred],
                'model': 'LinearRegression'
            }

            # ===== Prophet 预测 =====
            future = self.prophet.make_future_dataframe(periods=days)
            forecast = self.prophet.predict(future)
            prophet_pred = np.maximum(forecast['yhat'].values[-days:], 0)
            prophet_lower = np.maximum(forecast['yhat_lower'].values[-days:], 0)
            prophet_upper = forecast['yhat_upper'].values[-days:]

            predictions['predictions']['prophet'] = {
                'forecast': [float(p) for p in prophet_pred],
                'lower': [float(l) for l in prophet_lower],
                'upper': [float(u) for u in prophet_upper],
                'model': 'Facebook Prophet'
            }

            # ===== XGBoost 预测 (简化: 使用最后一行特征) =====
            # 在生产环境中应该生成完整的未来特征
            last_features = self._generate_future_features(days)
            X_future_xgb = last_features[self.FEATURE_COLS].values
            xgb_pred = np.maximum(self.xgb.predict(X_future_xgb), 0)

            predictions['predictions']['xgb'] = {
                'forecast': [float(p) for p in xgb_pred],
                'model': 'XGBoost'
            }

            # ===== 集成预测 (加权平均) =====
            weights = {'lr': 0.2, 'prophet': 0.3, 'xgb': 0.5}
            ensemble_pred = (
                np.array(predictions['predictions']['lr']['forecast']) * weights['lr'] +
                np.array(predictions['predictions']['prophet']['forecast']) * weights['prophet'] +
                np.array(predictions['predictions']['xgb']['forecast']) * weights['xgb']
            )

            predictions['ensemble'] = {
                'forecast': [float(p) for p in ensemble_pred],
                'weights': weights,
                'model': 'Weighted Ensemble'
            }

            # ===== 生成推荐 =====
            next_price = ensemble_pred[0]
            price_change_pct = ((next_price - self.last_price) / self.last_price) * 100

            if price_change_pct > 5:
                recommendation = 'sell'  # 价格上升明显，考虑卖出
                reason = '价格预测上升 > 5%'
            elif price_change_pct < -5:
                recommendation = 'buy'   # 价格下降明显，考虑持有或买入
                reason = '价格预测下降 > 5%'
            else:
                recommendation = 'hold'  # 价格相对稳定
                reason = '价格变化不明显'

            predictions['recommendation'] = {
                'action': recommendation,
                'next_price': float(next_price),
                'price_change_pct': float(price_change_pct),
                'reason': reason,
                'confidence': 0.95
            }

            return predictions

        except Exception as e:
            logger.error(f"预测失败: {e}")
            return None

    def _generate_future_features(self, days):
        """生成未来特征（简化版）"""
        # 这是一个简化的实现，实际应该基于时间序列特征
        future_dates = pd.date_range(
            start=self.last_timestamp + timedelta(days=1),
            periods=days
        )

        future_df = pd.DataFrame({
            'timestamp': future_dates,
            'day_of_week': future_dates.dayofweek,
            'day_of_month': future_dates.day,
            'days_since_start': [(d - self.last_timestamp).days for d in future_dates],
            'price_range': 0.5,  # 假设价格范围保持不变
            'total_orders': 100,  # 假设订单数保持不变
            'order_ratio': 0.5,
            'buy_price_ma3': self.last_price,
            'sell_price_ma3': self.last_price,
            'price_change_ma': 0.0
        })

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
        'cached_models': len(MODEL_CACHE),
        'version': '1.0.0'
    }), 200


@app.route('/api/predict/<int:good_id>', methods=['GET'])
def predict_endpoint(good_id):
    """
    预测商品未来价格
    参数: days (默认 7) - 预测天数
    """
    try:
        days = request.args.get('days', default=7, type=int)
        if days < 1 or days > 30:
            return jsonify({'error': '预测天数必须在 1-30 之间'}), 400

        logger.info(f"预测请求: good_id={good_id}, days={days}")

        # 检查缓存
        with MODEL_LOCK:
            if good_id not in MODEL_CACHE:
                # 加载数据和训练模型
                df = fetch_historical_data(good_id, days=30)
                if df is None or len(df) < 10:
                    return jsonify({'error': '数据不足 (< 10 条记录)'}), 404

                model = PredictionModel()
                if not model.train(df):
                    return jsonify({'error': '模型训练失败'}), 500

                MODEL_CACHE[good_id] = model
            else:
                model = MODEL_CACHE[good_id]

        # 进行预测
        result = model.predict(days=days)
        if result is None:
            return jsonify({'error': '预测失败'}), 500

        result['good_id'] = good_id
        return jsonify(result), 200

    except Exception as e:
        logger.error(f"预测端点异常: {e}")
        return jsonify({'error': str(e)}), 500


@app.route('/api/batch-predict', methods=['POST'])
def batch_predict_endpoint():
    """
    批量预测多个商品
    请求体: {"good_ids": [24026, 24028, ...], "days": 7}
    """
    try:
        data = request.get_json()
        good_ids = data.get('good_ids', [])
        days = data.get('days', 7)

        if not good_ids or len(good_ids) > 100:
            return jsonify({'error': '商品数必须在 1-100 之间'}), 400

        logger.info(f"批量预测: {len(good_ids)} 个商品")

        results = []
        for good_id in good_ids:
            try:
                with MODEL_LOCK:
                    if good_id not in MODEL_CACHE:
                        df = fetch_historical_data(good_id, days=30)
                        if df is None or len(df) < 10:
                            continue

                        model = PredictionModel()
                        if not model.train(df):
                            continue

                        MODEL_CACHE[good_id] = model
                    else:
                        model = MODEL_CACHE[good_id]

                result = model.predict(days=days)
                if result:
                    result['good_id'] = good_id
                    results.append(result)
            except Exception as e:
                logger.warning(f"商品 {good_id} 预测失败: {e}")
                continue

        return jsonify({
            'total_requested': len(good_ids),
            'total_success': len(results),
            'results': results
        }), 200

    except Exception as e:
        logger.error(f"批量预测异常: {e}")
        return jsonify({'error': str(e)}), 500


@app.route('/api/clear-cache', methods=['POST'])
def clear_cache_endpoint():
    """清空模型缓存"""
    try:
        with MODEL_LOCK:
            count = len(MODEL_CACHE)
            MODEL_CACHE.clear()

        logger.info(f"缓存已清空 ({count} 个模型)")
        return jsonify({
            'status': 'success',
            'cleared_models': count,
            'timestamp': datetime.now().isoformat()
        }), 200

    except Exception as e:
        logger.error(f"清空缓存异常: {e}")
        return jsonify({'error': str(e)}), 500


@app.route('/api/cache-status', methods=['GET'])
def cache_status_endpoint():
    """获取缓存状态"""
    try:
        with MODEL_LOCK:
            cache_info = {
                'total_cached_models': len(MODEL_CACHE),
                'cached_good_ids': list(MODEL_CACHE.keys()),
                'timestamp': datetime.now().isoformat()
            }

        return jsonify(cache_info), 200

    except Exception as e:
        logger.error(f"获取缓存状态异常: {e}")
        return jsonify({'error': str(e)}), 500


@app.errorhandler(404)
def not_found(e):
    """404 处理"""
    return jsonify({'error': '端点不存在'}), 404


@app.errorhandler(500)
def internal_error(e):
    """500 处理"""
    logger.error(f"内部服务器错误: {e}")
    return jsonify({'error': '内部服务器错误'}), 500


# ============================================================================
# 启动
# ============================================================================

if __name__ == '__main__':
    import sys

    # 命令行参数支持
    port = 5000
    for arg in sys.argv[1:]:
        if arg.startswith('--port='):
            port = int(arg.split('=')[1])

    logger.info("启动 CSGO 预测服务...")
    logger.info(f"数据库: {DB_CONFIG['host']}:{DB_CONFIG['user']}")
    logger.info(f"缓存目录: {CACHE_DIR}")
    logger.info(f"监听端口: {port}")

    app.run(
        host='0.0.0.0',
        port=port,
        debug=False,
        threaded=True
    )
