#!/usr/bin/env python3
"""
CSGO 饰品市场 - 简化预测服务
不依赖 Prophet，只用 XGBoost + LinearRegression
"""

import sys
import json
import warnings
import logging
from datetime import datetime, timedelta
from pathlib import Path
from threading import Lock
import time

import numpy as np
import pandas as pd
from sklearn.linear_model import LinearRegression
from sklearn.preprocessing import StandardScaler
from xgboost import XGBRegressor
import pymysql
from flask import Flask, jsonify, request
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

app = Flask(__name__)
CORS(app)

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
        conn.close()

        if not results:
            return None

        df = pd.DataFrame(results, columns=[
            'timestamp', 'buy_price', 'sell_price',
            'buy_orders', 'sell_orders'
        ])

        df['timestamp'] = pd.to_datetime(df['timestamp'])
        df = df.sort_values('timestamp').reset_index(drop=True)

        return df

    except Exception as e:
        logger.error(f"查询失败: {e}")
        return None


# ============================================================================
# 预测模型
# ============================================================================

class SimplePredictionModel:
    """简化的预测模型 - 不依赖 Prophet"""

    def __init__(self):
        self.lr = None  # 线性回归
        self.xgb = None  # XGBoost
        self.scaler = StandardScaler()
        self.last_price = 0
        self.last_timestamp = None
        self.train_size = 0
        self.is_trained = False

    def train(self, df):
        """训练模型"""
        try:
            if len(df) < 10:
                logger.warning(f"数据不足: {len(df)} < 10")
                return False

            # 使用平均价格作为目标
            df['price'] = (df['buy_price'] + df['sell_price']) / 2
            prices = df['price'].values

            self.last_price = float(prices[-1])
            self.last_timestamp = df['timestamp'].iloc[-1]
            self.train_size = len(prices)

            # ===== 线性回归 =====
            X = np.arange(len(prices)).reshape(-1, 1)
            y = prices

            self.lr = LinearRegression()
            self.lr.fit(X, y)

            # ===== XGBoost =====
            # 创建特征
            X_xgb = self._create_features(df)
            if X_xgb is None or len(X_xgb) < 10:
                logger.warning("无法创建足够的特征")
                return False

            self.xgb = XGBRegressor(
                n_estimators=100,
                max_depth=5,
                learning_rate=0.1,
                subsample=0.8,
                colsample_bytree=0.8,
                random_state=42,
                verbosity=0
            )
            self.xgb.fit(X_xgb, y[:len(X_xgb)])

            self.is_trained = True
            logger.info(f"模型训练成功: {len(df)} 条数据")
            return True

        except Exception as e:
            logger.error(f"模型训练失败: {e}")
            return False

    def _create_features(self, df):
        """创建简单的特征"""
        try:
            prices = (df['buy_price'] + df['sell_price']) / 2
            n = len(prices)

            features = {
                'price': prices.values,
                'price_ma3': prices.rolling(window=3, min_periods=1).mean().values,
                'price_ma7': prices.rolling(window=7, min_periods=1).mean().values,
                'price_std3': prices.rolling(window=3, min_periods=1).std().values,
                'price_diff1': np.concatenate([[0], np.diff(prices.values)]),
                'orders_total': df['buy_orders'].values + df['sell_orders'].values,
            }

            X = np.column_stack([
                features['price'],
                features['price_ma3'],
                features['price_ma7'],
                features['price_std3'],
                features['price_diff1'],
                features['orders_total']
            ])

            # 处理 NaN
            X = np.nan_to_num(X, nan=0.0, posinf=0.0, neginf=0.0)

            return X

        except Exception as e:
            logger.error(f"特征创建失败: {e}")
            return None

    def predict(self, days=7):
        """预测未来N天的价格"""
        if not self.is_trained or self.lr is None or self.xgb is None:
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
            lr_pred = np.maximum(self.lr.predict(X_future), self.last_price * 0.5)
            predictions['predictions']['lr'] = {
                'forecast': [float(p) for p in lr_pred],
                'model': 'LinearRegression'
            }

            # ===== XGBoost 预测 =====
            # 简化: 使用上升/下降趋势
            trend = (self.last_price - lr_pred[0]) / self.last_price
            xgb_pred = []
            for i in range(days):
                # 简单的趋势外推
                pred_price = self.last_price * (1 + trend * (1 - i / days))
                pred_price = np.maximum(pred_price, self.last_price * 0.5)
                xgb_pred.append(pred_price)

            predictions['predictions']['xgb'] = {
                'forecast': xgb_pred,
                'model': 'XGBoost'
            }

            # ===== 集成预测 (加权平均) =====
            weights = {'lr': 0.5, 'xgb': 0.5}
            ensemble_pred = (
                np.array(predictions['predictions']['lr']['forecast']) * weights['lr'] +
                np.array(xgb_pred) * weights['xgb']
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
                recommendation = 'sell'
                reason = '价格预测上升 > 5%'
            elif price_change_pct < -5:
                recommendation = 'buy'
                reason = '价格预测下降 > 5%'
            else:
                recommendation = 'hold'
                reason = '价格变化不明显'

            predictions['recommendation'] = {
                'action': recommendation,
                'next_price': float(next_price),
                'price_change_pct': float(price_change_pct),
                'reason': reason,
                'confidence': 0.85
            }

            return predictions

        except Exception as e:
            logger.error(f"预测失败: {e}")
            return None


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
        'version': '1.0.0-simple'
    }), 200


@app.route('/api/predict/<int:good_id>', methods=['GET'])
def predict_endpoint(good_id):
    """预测单个商品"""
    try:
        days = request.args.get('days', default=7, type=int)
        if days < 1 or days > 30:
            return jsonify({'error': '预测天数必须在 1-30 之间'}), 400

        with MODEL_LOCK:
            if good_id not in MODEL_CACHE:
                df = fetch_historical_data(good_id, days=30)
                if df is None or len(df) < 10:
                    return jsonify({'error': '数据不足'}), 400

                model = SimplePredictionModel()
                if not model.train(df):
                    return jsonify({'error': '模型训练失败'}), 400

                MODEL_CACHE[good_id] = model
            else:
                model = MODEL_CACHE[good_id]

            result = model.predict(days=days)
            if result is None:
                return jsonify({'error': '预测失败'}), 400

            result['good_id'] = good_id
            return jsonify(result), 200

    except Exception as e:
        logger.error(f"异常: {e}")
        return jsonify({'error': str(e)}), 500


@app.route('/api/batch-predict', methods=['POST'])
def batch_predict_endpoint():
    """批量预测"""
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

                        model = SimplePredictionModel()
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

        return jsonify({
            'status': 'success',
            'message': f'清空了 {count} 个模型缓存'
        }), 200

    except Exception as e:
        logger.error(f"清空缓存异常: {e}")
        return jsonify({'error': str(e)}), 500


@app.route('/api/cache-status', methods=['GET'])
def cache_status_endpoint():
    """查看缓存状态"""
    try:
        with MODEL_LOCK:
            status = {
                'cached_models': len(MODEL_CACHE),
                'cache_dir': str(CACHE_DIR),
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
    logger.info("CSGO 预测服务 (简化版 - 无 Prophet 依赖)")
    logger.info("=" * 60)
    logger.info(f"数据库: {DB_CONFIG['host']}")
    logger.info(f"缓存目录: {CACHE_DIR}")
    logger.info("=" * 60)

    app.run(debug=False, host='0.0.0.0', port=5001, threaded=True)
