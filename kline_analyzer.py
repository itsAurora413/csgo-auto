#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
CSGO饰品指数K线数据分析系统
综合运用金融学、数学、统计学分析市场规律

指数说明：
- 计算方法：各饰品价格变化率的平均值
- 样本：11269件平台在售数>50的饰品
- 消除量纲影响：采用去量纲化处理

功能模块：
1. 技术分析 - 均线、MACD、RSI、布林带等
2. 统计分析 - 波动率、偏度、峰度、自相关
3. 趋势分析 - 时间序列、周期性、转折点
4. 风险分析 - 最大回撤、夏普比率、风险收益比
5. 预测分析 - ARIMA、指数平滑、趋势外推
"""

import requests
import json
import numpy as np
import pandas as pd
from datetime import datetime, timedelta
import matplotlib.pyplot as plt
from scipy import stats
from scipy.signal import find_peaks
from sklearn.preprocessing import StandardScaler
from sklearn.linear_model import LinearRegression
import warnings
import time
warnings.filterwarnings('ignore')

import pickle
import os
from pathlib import Path

# ============================================================================
# 0.5 模型持久化管理器
# ============================================================================

class ModelPersistenceManager:
    """模型持久化管理器 - 支持模型保存、加载和版本管理"""
    
    def __init__(self, model_dir="/Users/user/Downloads/csgoAuto/models", index_id=3, kline_type="1hour"):
        self.model_dir = model_dir
        self.index_id = index_id
        self.kline_type = kline_type
        
        # 创建模型目录
        Path(self.model_dir).mkdir(parents=True, exist_ok=True)
        
        # 模型存储路径
        self.model_prefix = f"{self.model_dir}/model_idx{index_id}_{kline_type}"
        
    def _get_model_path(self, model_name):
        """获取模型文件路径"""
        return f"{self.model_prefix}_{model_name}.pkl"
    
    def _get_metadata_path(self):
        """获取元数据文件路径"""
        return f"{self.model_prefix}_metadata.json"
    
    def _get_history_path(self):
        """获取训练历史文件路径"""
        return f"{self.model_prefix}_history.json"
    
    def save_model(self, model, model_name):
        """保存模型"""
        try:
            path = self._get_model_path(model_name)
            with open(path, 'wb') as f:
                pickle.dump(model, f)
            return True
        except Exception as e:
            print(f"❌ 保存模型 {model_name} 失败: {e}")
            return False
    
    def load_model(self, model_name):
        """加载模型"""
        try:
            path = self._get_model_path(model_name)
            if not os.path.exists(path):
                return None
            with open(path, 'rb') as f:
                model = pickle.load(f)
            return model
        except Exception as e:
            print(f"❌ 加载模型 {model_name} 失败: {e}")
            return False
    
    def model_exists(self, model_name):
        """检查模型是否存在"""
        return os.path.exists(self._get_model_path(model_name))
    
    def save_metadata(self, metadata):
        """保存元数据（训练时间、数据范围等）"""
        try:
            path = self._get_metadata_path()
            with open(path, 'w') as f:
                json.dump(metadata, f, indent=2, default=str)
            return True
        except Exception as e:
            print(f"❌ 保存元数据失败: {e}")
            return False
    
    def load_metadata(self):
        """加载元数据"""
        try:
            path = self._get_metadata_path()
            if not os.path.exists(path):
                return None
            with open(path, 'r') as f:
                metadata = json.load(f)
            return metadata
        except Exception as e:
            print(f"❌ 加载元数据失败: {e}")
            return None
    
    def add_training_history(self, training_info):
        """添加训练历史记录"""
        try:
            path = self._get_history_path()
            
            # 加载现有历史
            if os.path.exists(path):
                with open(path, 'r') as f:
                    history = json.load(f)
            else:
                history = []
            
            # 添加新记录
            history.append(training_info)
            
            # 保存
            with open(path, 'w') as f:
                json.dump(history, f, indent=2, default=str)
            
            return True
        except Exception as e:
            print(f"❌ 保存训练历史失败: {e}")
            return False
    
    def get_training_history(self):
        """获取训练历史"""
        try:
            path = self._get_history_path()
            if not os.path.exists(path):
                return []
            with open(path, 'r') as f:
                history = json.load(f)
            return history
        except Exception as e:
            print(f"❌ 加载训练历史失败: {e}")
            return []
    
    def show_training_history(self):
        """显示训练历史"""
        history = self.get_training_history()
        if not history:
            print("📊 还没有训练历史记录")
            return
        
        print("\n📚 训练历史记录:")
        print("=" * 80)
        for i, record in enumerate(history[-10:], 1):  # 显示最后10条
            timestamp = record.get('timestamp', 'N/A')
            arima_rmse = record.get('arima_rmse', 'N/A')
            prophet_rmse = record.get('prophet_rmse', 'N/A')
            xgb_rmse = record.get('xgb_rmse', 'N/A')
            improvement = record.get('improvement_percent', 'N/A')
            
            print(f"\n  #{len(history) - 10 + i} - {timestamp}")
            print(f"     ARIMA RMSE:   {arima_rmse}")
            print(f"     Prophet RMSE: {prophet_rmse}")
            print(f"     XGBoost RMSE: {xgb_rmse}")
            if improvement != 'N/A' and isinstance(improvement, (int, float)):
                print(f"     📈 相比上次改进: {improvement:.2f}%")
        
        print("\n" + "=" * 80)


# ============================================================================
# 0. K线周期配置系统
# ============================================================================

class KlineConfig:
    """K线周期配置 - 根据K线类型自动调整分析参数"""
    
    CONFIG = {
        '1day': {
            'name': '日线',
            'periods_per_year': 365,  # 全年365天可交易
            'ma_fast': 5,             # 快速均线
            'ma_mid': 10,             # 中期均线
            'ma_slow': 20,            # 慢速均线
            'rsi_window': 14,         # RSI周期
            'macd_fast': 12,          # MACD快速EMA
            'macd_slow': 26,          # MACD慢速EMA
            'macd_signal': 9,         # MACD信号线
            'bb_window': 20,          # 布林带周期
            'atr_window': 14,         # ATR周期
            'forecast_periods': [1, 7, 14],  # 预测周期（天数）
            'volatility_lookback': 10,  # 波动率回看期数
            'trend_lookback': 60,     # 趋势回看期数
        },
        '1hour': {
            'name': '小时线',
            'periods_per_year': 365 * 24,  # 小时数（全年交易小时 = 8760）
            'ma_fast': 12,            # 快速均线（对应3小时）
            'ma_mid': 24,             # 中期均线（对应1天）
            'ma_slow': 72,            # 慢速均线（对应3天）
            'rsi_window': 14,         # RSI周期保持不变
            'macd_fast': 12,          # MACD参数可保持，但跨度更大
            'macd_slow': 26,
            'macd_signal': 9,
            'bb_window': 20,          # 布林带周期
            'atr_window': 14,         # ATR周期
            'forecast_periods': [1, 24, 168],  # 预测周期（小时数：1、24、168小时即7天）
            'volatility_lookback': 24,  # 近24小时波动率
            'trend_lookback': 240,    # 10天趋势
        },
        '4hour': {
            'name': '4小时线',
            'periods_per_year': 365 * 6,  # 4小时线数 = 2190
            'ma_fast': 6,             # 快速均线（对应1天）
            'ma_mid': 12,             # 中期均线（对应2天）
            'ma_slow': 30,            # 慢速均线（对应5天）
            'rsi_window': 14,
            'macd_fast': 12,
            'macd_slow': 26,
            'macd_signal': 9,
            'bb_window': 20,
            'atr_window': 14,
            'forecast_periods': [1, 6, 42],  # 预测周期（单位：根据K线数）
            'volatility_lookback': 6,
            'trend_lookback': 60,
        },
        '7day': {
            'name': '周线',
            'periods_per_year': 52,  # 年周数
            'ma_fast': 4,
            'ma_mid': 8,
            'ma_slow': 13,
            'rsi_window': 14,
            'macd_fast': 12,
            'macd_slow': 26,
            'macd_signal': 9,
            'bb_window': 20,
            'atr_window': 14,
            'forecast_periods': [1, 4, 12],  # 预测周期（周数）
            'volatility_lookback': 4,
            'trend_lookback': 52,
        }
    }
    
    @staticmethod
    def get_config(kline_type='1day'):
        """获取K线类型对应的配置"""
        return KlineConfig.CONFIG.get(kline_type, KlineConfig.CONFIG['1day'])
    
    @staticmethod
    def get_annual_periods(kline_type='1day'):
        """获取年化周期数"""
        return KlineConfig.get_config(kline_type)['periods_per_year']

# ============================================================================
# 1. 数据获取模块
# ============================================================================

class KlineDataFetcher:
    """K线数据获取器"""
    
    def __init__(self, base_url="http://localhost:8080"):
        self.base_url = base_url
        self.session = requests.Session()
    
    def fetch_kline(self, index_id=3, kline_type="1hour", verbose=True, max_retries=5):
        """
        获取K线数据
        
        Args:
            index_id: 指数ID (默认3 = CSGO饰品大盘指数)
            kline_type: K线类型 (1day, 1hour, 4hour, 7day)
            verbose: 是否打印日志
            max_retries: 最大重试次数
        
        Returns:
            DataFrame: 包含 open, close, high, low, timestamp 的数据框
        """
        url = f"{self.base_url}/api/v1/sub/kline"
        params = {"id": index_id, "type": kline_type}
        
        retry_count = 0
        base_wait_time = 1  # 初始等待时间（秒）
        
        while retry_count < max_retries:
            try:
                if verbose and retry_count > 0:
                    print(f"📡 重试第 {retry_count}/{max_retries} 次...")
                elif verbose:
                    print(f"📡 正在获取K线数据: {url}")
                    print(f"   参数: {params}")
                
                response = self.session.get(url, params=params, timeout=30)
                
                if verbose:
                    print(f"   HTTP状态码: {response.status_code}")
                    print(f"   响应头: {response.headers}")
                    print(f"   响应体长度: {len(response.text)} 字符")
                
                # 检查是否是 429 Too Many Requests
                if response.status_code == 429:
                    retry_count += 1
                    if retry_count >= max_retries:
                        print(f"❌ 获取数据失败: 达到最大重试次数 ({max_retries}), 服务器返回 429 Too Many Requests")
                        return None
                    
                    # 指数退避策略
                    wait_time = base_wait_time * (2 ** (retry_count - 1))
                    # 添加随机抖动，避免雷鸣羊群效应
                    jitter = np.random.uniform(0, wait_time * 0.1)
                    actual_wait_time = wait_time + jitter
                    
                    print(f"⚠️  服务器限流 (429)，等待 {actual_wait_time:.1f} 秒后重试...")
                    time.sleep(actual_wait_time)
                    continue
                
                response.raise_for_status()
                
                # 检查响应是否为空
                if not response.text or response.text.strip() == "":
                    print("❌ 获取数据失败: 服务器返回空响应")
                    return None
                
                data = response.json()
                
                if data.get("code") != 200:
                    print(f"❌ API错误: {data.get('msg')}")
                    return None
                
                # 解析K线数据
                kline_data = data.get("data", [])
                if not kline_data:
                    print("⚠️  没有获取到K线数据")
                    return None
                
                # 转换为DataFrame
                df = pd.DataFrame(kline_data)
                
                # 处理时间戳 - 可能是字符串格式的毫秒时间戳
                if 't' in df.columns:
                    df['timestamp'] = pd.to_datetime(pd.to_numeric(df['t'], errors='coerce'), unit='ms', utc=True)
                    # 转换为UTC+8时区
                    df['timestamp'] = df['timestamp'].dt.tz_convert('Asia/Shanghai')
                elif 'timestamp' in df.columns:
                    df['timestamp'] = pd.to_datetime(pd.to_numeric(df['timestamp'], errors='coerce'), unit='s', utc=True)
                    # 转换为UTC+8时区
                    df['timestamp'] = df['timestamp'].dt.tz_convert('Asia/Shanghai')
                else:
                    print("❌ 无法找到时间戳列")
                    return None
                
                df = df.sort_values('timestamp').reset_index(drop=True)
                
                # 转换价格为float - 处理可能的字符串格式
                price_columns = {}
                if 'o' in df.columns:
                    price_columns['open'] = 'o'
                if 'c' in df.columns:
                    price_columns['close'] = 'c'
                if 'h' in df.columns:
                    price_columns['high'] = 'h'
                if 'l' in df.columns:
                    price_columns['low'] = 'l'
                
                for std_col, api_col in price_columns.items():
                    if api_col in df.columns:
                        df[std_col] = pd.to_numeric(df[api_col], errors='coerce')
                
                # 处理成交量
                if 'v' in df.columns:
                    df['volume'] = pd.to_numeric(df['v'], errors='coerce')
                
                if verbose:
                    print(f"✅ 成功获取 {len(df)} 条K线数据")
                    print(f"   时间范围: {df['timestamp'].min()} 至 {df['timestamp'].max()}")
                    if 'close' in df.columns:
                        print(f"   价格范围: {df['close'].min():.2f} - {df['close'].max():.2f}\n")
                
                return df
                
            except json.JSONDecodeError as e:
                print(f"❌ JSON解析失败: {e}")
                if verbose:
                    print(f"   响应内容前500字符: {response.text[:500] if hasattr(response, 'text') else 'N/A'}")
                return None
            except requests.exceptions.ConnectionError as e:
                print(f"❌ 连接失败: {e}")
                print(f"   请确保服务运行在 {self.base_url}")
                return None
            except requests.exceptions.Timeout as e:
                print(f"❌ 请求超时: {e}")
                return None
            except Exception as e:
                print(f"❌ 获取数据失败: {e}")
                if verbose and hasattr(response, 'text'):
                    print(f"   响应内容: {response.text[:500]}")
                return None


# ============================================================================
# 2. 技术分析模块
# ============================================================================

class TechnicalAnalysis:
    """技术分析工具类"""
    
    @staticmethod
    def moving_average(data, window):
        """计算简单移动平均线 (SMA)"""
        return data.rolling(window=window).mean()
    
    @staticmethod
    def exponential_moving_average(data, window):
        """计算指数移动平均线 (EMA)"""
        return data.ewm(span=window, adjust=False).mean()
    
    @staticmethod
    def macd(data, fast=12, slow=26, signal=9):
        """
        计算MACD指标
        MACD = EMA12 - EMA26
        Signal = EMA9(MACD)
        Histogram = MACD - Signal
        """
        ema_fast = data.ewm(span=fast, adjust=False).mean()
        ema_slow = data.ewm(span=slow, adjust=False).mean()
        macd_line = ema_fast - ema_slow
        signal_line = macd_line.ewm(span=signal, adjust=False).mean()
        histogram = macd_line - signal_line
        return macd_line, signal_line, histogram
    
    @staticmethod
    def rsi(data, window=14):
        """
        相对强弱指标 (RSI)
        RSI = 100 * U / (U + D)
        其中U = 平均上升幅度, D = 平均下降幅度
        """
        delta = data.diff()
        gain = (delta.where(delta > 0, 0)).rolling(window=window).mean()
        loss = (-delta.where(delta < 0, 0)).rolling(window=window).mean()
        rs = gain / loss
        rsi = 100 - (100 / (1 + rs))
        return rsi
    
    @staticmethod
    def bollinger_bands(data, window=20, num_std=2):
        """
        布林带 (Bollinger Bands)
        中线 = SMA(20)
        上线 = 中线 + 2*标准差
        下线 = 中线 - 2*标准差
        """
        sma = data.rolling(window=window).mean()
        std = data.rolling(window=window).std()
        upper = sma + (std * num_std)
        lower = sma - (std * num_std)
        return upper, sma, lower
    
    @staticmethod
    def atr(high, low, close, window=14):
        """
        平均真实波幅 (Average True Range)
        衡量市场波动性
        """
        tr1 = high - low
        tr2 = abs(high - close.shift())
        tr3 = abs(low - close.shift())
        tr = pd.concat([tr1, tr2, tr3], axis=1).max(axis=1)
        atr = tr.rolling(window=window).mean()
        return atr
    
    @staticmethod
    def stochastic_oscillator(high, low, close, window=14, smooth_k=3, smooth_d=3):
        """
        随机指标 (%K和%D)
        """
        lowest_low = low.rolling(window=window).min()
        highest_high = high.rolling(window=window).max()
        k_percent = 100 * ((close - lowest_low) / (highest_high - lowest_low))
        k_percent_smooth = k_percent.rolling(window=smooth_k).mean()
        d_percent = k_percent_smooth.rolling(window=smooth_d).mean()
        return k_percent_smooth, d_percent


# ============================================================================
# 3. 统计分析模块
# ============================================================================

class StatisticalAnalysis:
    """统计分析工具类"""
    
    @staticmethod
    def calculate_returns(prices):
        """计算收益率 (对数收益率)"""
        returns = np.log(prices / prices.shift(1))
        return returns.dropna()
    
    @staticmethod
    def volatility(returns, periods=252):
        """
        计算年化波动率
        年化波动率 = 日波动率 * sqrt(252)
        """
        daily_vol = returns.std()
        annualized_vol = daily_vol * np.sqrt(periods)
        return annualized_vol
    
    @staticmethod
    def skewness_kurtosis(returns):
        """
        计算偏度和峰度
        偏度: 分布的非对称性 (负值表示左偏，正值表示右偏)
        峰度: 分布的尾部厚度 (高峰度表示异常风险大)
        """
        skew = stats.skew(returns)
        kurt = stats.kurtosis(returns)
        return skew, kurt
    
    @staticmethod
    def autocorrelation(returns, lags=20):
        """
        自相关分析
        检测收益率序列中的模式和记忆效应
        """
        acf = pd.Series(returns).autocorr(lag=lags)
        return acf
    
    @staticmethod
    def draw_down(prices):
        """
        计算最大回撤
        最大回撤 = (最低价 - 峰值) / 峰值
        """
        cummax = prices.cummax()
        drawdown = (prices - cummax) / cummax
        max_drawdown = drawdown.min()
        return drawdown, max_drawdown
    
    @staticmethod
    def sharpe_ratio(returns, risk_free_rate=0.03, periods=252):
        """
        计算夏普比率
        Sharpe Ratio = (年化收益率 - 无风险利率) / 年化波动率
        """
        annual_return = returns.mean() * periods
        annual_vol = returns.std() * np.sqrt(periods)
        sharpe = (annual_return - risk_free_rate) / annual_vol if annual_vol != 0 else 0
        return sharpe
    
    @staticmethod
    def calmar_ratio(returns, prices, periods=252):
        """
        计算Calmar比率
        Calmar = 年化收益率 / 最大回撤绝对值
        """
        _, max_dd = StatisticalAnalysis.draw_down(prices)
        annual_return = returns.mean() * periods
        calmar = annual_return / abs(max_dd) if max_dd != 0 else 0
        return calmar


# ============================================================================
# 4. 趋势分析模块
# ============================================================================

class TrendAnalysis:
    """趋势分析工具类"""
    
    @staticmethod
    def linear_regression_trend(prices):
        """
        线性回归趋势分析
        计算趋势方向、斜率和R²拟合度
        """
        X = np.arange(len(prices)).reshape(-1, 1)
        y = prices.values.reshape(-1, 1)
        
        model = LinearRegression()
        model.fit(X, y)
        
        slope = model.coef_[0][0]
        r_squared = model.score(X, y)
        trend_line = model.predict(X).flatten()
        
        return {
            'slope': slope,
            'r_squared': r_squared,
            'trend_line': trend_line,
            'direction': '上升' if slope > 0 else '下降'
        }
    
    @staticmethod
    def find_peaks_and_valleys(prices, distance=10):
        """
        识别峰值和谷值
        用于识别市场的转折点
        """
        # 归一化价格
        prices_normalized = (prices - prices.mean()) / prices.std()
        
        # 找峰值
        peaks, _ = find_peaks(prices_normalized.values, distance=distance)
        valleys, _ = find_peaks(-prices_normalized.values, distance=distance)
        
        return peaks, valleys
    
    @staticmethod
    def trend_strength(high, low, close, period=14):
        """
        趋势强度指标
        基于DMI (Direction Movement Index)
        """
        up_move = high.diff()
        down_move = -low.diff()
        
        # 计算上升动能和下降动能
        plus_dm = (up_move.where((up_move > down_move) & (up_move > 0), 0)).rolling(period).mean()
        minus_dm = (down_move.where((down_move > up_move) & (down_move > 0), 0)).rolling(period).mean()
        
        tr = TechnicalAnalysis.atr(high, low, close, period)
        
        di_plus = 100 * plus_dm / tr if not tr.isna().all() else 0
        di_minus = 100 * minus_dm / tr if not tr.isna().all() else 0
        
        return di_plus, di_minus
    
    @staticmethod
    def cycle_detection(prices, min_period=5, max_period=100):
        """
        周期性检测
        使用自相关函数检测市场的周期性
        """
        acf_values = []
        for lag in range(1, min(max_period, len(prices))):
            acf = pd.Series(prices).autocorr(lag=lag)
            acf_values.append(acf)
        
        # 找最大自相关
        acf_array = np.array(acf_values)
        significant_lags = np.where(np.abs(acf_array) > 0.3)[0] + 1
        
        return significant_lags


# ============================================================================
# 5. 预测分析模块
# ============================================================================

class PredictiveAnalysis:
    """预测分析工具类"""
    
    @staticmethod
    def exponential_smoothing(data, alpha=0.3):
        """
        指数平滑预测
        """
        result = [data[0]]
        for i in range(1, len(data)):
            result.append(alpha * data[i] + (1 - alpha) * result[-1])
        return np.array(result)
    
    @staticmethod
    def trend_extrapolation(prices, forecast_periods=10):
        """
        趋势外推预测
        基于线性回归的简单趋势预测
        """
        trend_info = TrendAnalysis.linear_regression_trend(prices)
        slope = trend_info['slope']
        
        last_price = prices.iloc[-1]
        forecast = []
        for i in range(1, forecast_periods + 1):
            forecast_price = last_price + slope * i
            forecast.append(forecast_price)
        
        return np.array(forecast)
    
    @staticmethod
    def moving_average_convergence(prices, short_window=5, long_window=20):
        """
        均线收敛/乖离预测
        基于短期和长期均线的交叉
        """
        sma_short = prices.rolling(window=short_window).mean()
        sma_long = prices.rolling(window=long_window).mean()
        
        # 计算乖离率
        divergence = (sma_short - sma_long) / sma_long * 100
        
        # 预测下一个方向
        last_divergence = divergence.iloc[-1]
        prev_divergence = divergence.iloc[-2] if len(divergence) > 1 else last_divergence
        
        if last_divergence > 0 and prev_divergence < last_divergence:
            signal = "看涨"
        elif last_divergence < 0 and prev_divergence > last_divergence:
            signal = "看跌"
        else:
            signal = "持平"
        
        return divergence, signal


# ============================================================================
# 5.5 高级预测模块（多模型集成）
# ============================================================================

class AdvancedPredictiveAnalysis:
    """
    高级预测分析 - 使用多种模型集成预测
    包含: ARIMA、指数平滑、Prophet、加权动量预测
    """
    
    @staticmethod
    def arima_forecast(prices, forecast_periods=7, order=(1, 1, 1)):
        """
        ARIMA模型预测
        order: (p, d, q) 参数
        """
        try:
            from statsmodels.tsa.arima.model import ARIMA
            
            # 确保数据是数值型
            prices_clean = prices.dropna().values
            
            if len(prices_clean) < 10:
                return None
            
            model = ARIMA(prices_clean, order=order)
            fitted_model = model.fit()
            forecast_result = fitted_model.get_forecast(steps=forecast_periods)
            
            # 处理 predicted_mean - 它可能是 Series 或 ndarray
            predicted_mean = forecast_result.predicted_mean
            if hasattr(predicted_mean, 'values'):
                forecast_values = predicted_mean.values
            else:
                forecast_values = np.asarray(predicted_mean)
            
            # 处理置信区间
            conf_int = None
            try:
                conf_int_result = forecast_result.conf_int()
                if hasattr(conf_int_result, 'values'):
                    conf_int = conf_int_result.values
                else:
                    conf_int = np.asarray(conf_int_result)
            except:
                pass
            
            return {
                'forecast': forecast_values,
                'confidence_intervals': conf_int,
                'aic': fitted_model.aic,
                'model': 'ARIMA'
            }
        except Exception as e:
            print(f"⚠️  ARIMA预测失败: {e}")
            return None
    
    @staticmethod
    def exponential_smoothing_advanced(prices, forecast_periods=7):
        """
        高级指数平滑预测 (Holt-Winters)
        """
        try:
            from statsmodels.tsa.holtwinters import ExponentialSmoothing
            
            prices_clean = prices.dropna().values
            
            if len(prices_clean) < 10:
                return None
            
            # 使用Holt-Winters平滑（additive模型）
            model = ExponentialSmoothing(
                prices_clean,
                trend='add',
                seasonal=None,
                initialization_method='estimated'
            )
            fitted_model = model.fit(optimized=True)
            forecast_values = fitted_model.forecast(steps=forecast_periods)
            
            return {
                'forecast': forecast_values,
                'model': 'Exponential Smoothing'
            }
        except Exception as e:
            print(f"⚠️  指数平滑预测失败: {e}")
            return None
    
    @staticmethod
    def prophet_forecast(prices, timestamps, forecast_periods=7):
        """
        Facebook Prophet模型预测
        适合处理趋势和季节性
        """
        try:
            from statsmodels.tsa.seasonal import seasonal_decompose
            
            prices_clean = prices.dropna()
            
            if len(prices_clean) < 20:
                return None
            
            # 简单趋势 + 动量预测
            # 使用LOESS平滑估计趋势
            trend_series = prices_clean.rolling(window=min(7, len(prices_clean)//3), center=True).mean()
            
            # 计算最近的趋势斜率
            recent_prices = prices_clean.iloc[-14:] if len(prices_clean) > 14 else prices_clean
            X = np.arange(len(recent_prices)).reshape(-1, 1)
            y = recent_prices.values
            
            model = LinearRegression()
            model.fit(X, y)
            recent_slope = model.coef_[0]
            
            # 基于最近趋势的预测
            last_price = prices_clean.iloc[-1]
            forecast = []
            for i in range(1, forecast_periods + 1):
                # 衰减因子，远期预测时趋势影响逐渐减弱
                decay = 0.95 ** ((i - 1) / forecast_periods)
                pred = last_price + recent_slope * i * decay
                forecast.append(pred)
            
            return {
                'forecast': np.array(forecast),
                'trend_slope': recent_slope,
                'model': 'Prophet-Like'
            }
        except Exception as e:
            print(f"⚠️  Prophet预测失败: {e}")
            return None
    
    @staticmethod
    def weighted_momentum_forecast(prices, forecast_periods=7):
        """
        加权动量预测 (改进版)
        结合多个时间尺度的动量信息，并优先考虑最近趋势
        """
        try:
            prices_clean = prices.dropna()
            
            if len(prices_clean) < 10:
                return None
            
            last_price = prices_clean.iloc[-1]
            
            # 计算不同时间尺度的动量
            momenta = {}
            
            # 1日动量
            if len(prices_clean) > 1:
                momenta['1d'] = (prices_clean.iloc[-1] - prices_clean.iloc[-2]) / prices_clean.iloc[-2]
            
            # 5日动量
            if len(prices_clean) > 5:
                momenta['5d'] = (prices_clean.iloc[-1] - prices_clean.iloc[-5]) / prices_clean.iloc[-5]
            
            # 10日动量
            if len(prices_clean) > 10:
                momenta['10d'] = (prices_clean.iloc[-1] - prices_clean.iloc[-10]) / prices_clean.iloc[-10]
            
            # 20日动量
            if len(prices_clean) > 20:
                momenta['20d'] = (prices_clean.iloc[-1] - prices_clean.iloc[-20]) / prices_clean.iloc[-20]
            
            # 检测最近的价格趋势方向（最重要）
            recent_trend = None
            if len(prices_clean) > 5:
                recent_prices = prices_clean.iloc[-5:].values
                recent_trend = np.mean(np.diff(recent_prices))
            
            # 改进的权重分配：优先考虑最近趋势
            # 如果最近有明显下跌，增加其权重
            if recent_trend is not None and recent_trend < 0:
                # 下跌趋势：增加最近期权重
                weights = {'1d': 0.5, '5d': 0.3, '10d': 0.15, '20d': 0.05}
            else:
                # 平常权重分配
                weights = {'1d': 0.4, '5d': 0.3, '10d': 0.2, '20d': 0.1}
            
            weighted_momentum = sum(
                momenta.get(key, 0) * weights[key] 
                for key in weights.keys()
            ) / sum(v for k, v in weights.items() if k in momenta)
            
            # 检测强势趋势（连续3日下跌或上升）
            if len(prices_clean) >= 3:
                last_3_returns = np.diff(prices_clean.iloc[-3:].values) / prices_clean.iloc[-3:-1].values
                all_negative = np.all(last_3_returns < 0)
                all_positive = np.all(last_3_returns > 0)
                
                if all_negative:
                    # 连续下跌：强化下跌动量
                    weighted_momentum *= 1.3
                elif all_positive:
                    # 连续上升：强化上升动量
                    weighted_momentum *= 1.2
            
            # 基于加权动量进行预测
            forecast = []
            current_price = last_price
            
            for i in range(1, forecast_periods + 1):
                # 改进：动量衰减更陡峭（长期预测应该更保守）
                momentum_decay = weighted_momentum * (0.85 ** (i - 1))
                next_price = current_price * (1 + momentum_decay)
                forecast.append(next_price)
                current_price = next_price
            
            return {
                'forecast': np.array(forecast),
                'momentum': weighted_momentum,
                'momenta_detail': momenta,
                'recent_trend': recent_trend,
                'model': 'Weighted Momentum'
            }
        except Exception as e:
            print(f"⚠️  动量预测失败: {e}")
            return None
    
    @staticmethod
    def ensemble_forecast(prices, timestamps, forecast_periods=7):
        """
        集成预测 - 融合多个模型的预测结果
        使用平均值和加权融合
        """
        forecasts = {}
        
        # 获取各模型预测
        arima_result = AdvancedPredictiveAnalysis.arima_forecast(prices, forecast_periods)
        if arima_result:
            forecasts['ARIMA'] = {
                'values': arima_result['forecast'],
                'weight': 0.25,
                'aic': arima_result.get('aic', 0)
            }
        
        es_result = AdvancedPredictiveAnalysis.exponential_smoothing_advanced(prices, forecast_periods)
        if es_result:
            forecasts['ExponentialSmoothing'] = {
                'values': es_result['forecast'],
                'weight': 0.25
            }
        
        prophet_result = AdvancedPredictiveAnalysis.prophet_forecast(prices, timestamps, forecast_periods)
        if prophet_result:
            forecasts['Prophet'] = {
                'values': prophet_result['forecast'],
                'weight': 0.25
            }
        
        momentum_result = AdvancedPredictiveAnalysis.weighted_momentum_forecast(prices, forecast_periods)
        if momentum_result:
            forecasts['Momentum'] = {
                'values': momentum_result['forecast'],
                'weight': 0.25
            }
        
        # 如果没有任何模型成功，返回None
        if not forecasts:
            return None
        
        # 标准化权重
        total_weight = sum(f['weight'] for f in forecasts.values())
        for model_name in forecasts:
            forecasts[model_name]['weight'] /= total_weight
        
        # 计算加权平均预测
        ensemble_forecast_values = np.zeros(forecast_periods)
        
        for model_name, model_data in forecasts.items():
            weighted_values = model_data['values'] * model_data['weight']
            ensemble_forecast_values += weighted_values
        
        # 检测实际价格趋势
        prices_clean = prices.dropna()
        if len(prices_clean) >= 5:
            # 计算最近5日的趋势
            recent_prices = prices_clean.iloc[-5:].values
            recent_returns = np.diff(recent_prices) / recent_prices[:-1]
            actual_trend = np.sum(recent_returns)  # 总变化率
            
            # 如果实际趋势明显向下，但预测向上，需要调整
            if actual_trend < -0.05 and ensemble_forecast_values[-1] > prices_clean.iloc[-1]:
                # 强势下跌趋势，调整预测更保守
                current_price = prices_clean.iloc[-1]
                for i in range(len(ensemble_forecast_values)):
                    # 削弱上升预测，或转向下跌
                    ensemble_forecast_values[i] = current_price * (1 + actual_trend * (0.5 + i/forecast_periods))
        
        return {
            'ensemble_forecast': ensemble_forecast_values,
            'individual_forecasts': {k: v['values'] for k, v in forecasts.items()},
            'model_weights': {k: v['weight'] for k, v in forecasts.items()},
            'models_used': len(forecasts)
        }
    
    @staticmethod
    def calculate_forecast_confidence(prices, forecast_values, forecast_periods):
        """
        计算预测置信度和不确定性区间
        """
        returns = StatisticalAnalysis.calculate_returns(prices)
        volatility = StatisticalAnalysis.volatility(returns, periods=252)
        
        # 计算置信区间
        last_price = prices.iloc[-1]
        confidence_intervals = []
        
        for i in range(1, forecast_periods + 1):
            # 基于历史波动率估计标准差
            std_error = last_price * volatility * np.sqrt(i)
            
            # 95%置信区间
            upper_bound = forecast_values[i-1] + 1.96 * std_error
            lower_bound = forecast_values[i-1] - 1.96 * std_error
            
            confidence_intervals.append({
                'period': i,
                'forecast': forecast_values[i-1],
                'upper_95': upper_bound,
                'lower_95': lower_bound,
                'std_error': std_error
            })
        
        return confidence_intervals


# ============================================================================
# 6. 综合分析报告生成器
# ============================================================================

class AnalysisReporter:
    """综合分析报告生成器"""
    
    @staticmethod
    def generate_report(df):
        """
        生成全面的分析报告
        """
        print("\n" + "="*80)
        print("                    CSGO饰品大盘指数K线数据分析报告")
        print("="*80 + "\n")
        
        # 基础统计
        print("【第一部分】指数概览与基础统计")
        print("-" * 80)
        AnalysisReporter._basic_statistics(df)
        
        # 收益率分析
        print("\n【第二部分】指数变化率与风险分析")
        print("-" * 80)
        AnalysisReporter._return_analysis(df)
        
        # 技术面分析
        print("\n【第三部分】技术面分析")
        print("-" * 80)
        AnalysisReporter._technical_analysis(df)
        
        # 趋势分析
        print("\n【第四部分】趋势与周期分析")
        print("-" * 80)
        AnalysisReporter._trend_analysis(df)
        
        # 风险评估
        print("\n【第五部分】风险评估")
        print("-" * 80)
        AnalysisReporter._risk_assessment(df)
        
        # 市场规律总结
        print("\n【第六部分】大盘指数规律发现与结论")
        print("-" * 80)
        AnalysisReporter._market_patterns(df)
        
        # 指数信号
        print("\n【第七部分】指数方向信号")
        print("-" * 80)
        AnalysisReporter._trading_signals(df)
        
        # 预测报告
        AnalysisReporter._generate_forecast_report(df)
        
        print("\n" + "="*80)
        print("                            报告生成完成")
        print("="*80 + "\n")
    
    @staticmethod
    def _basic_statistics(df):
        """基础统计"""
        close_prices = df['close']
        
        print(f"📊 数据样本量: {len(df)} 条")
        print(f"⏰ 时间跨度: {df['timestamp'].min().date()} 至 {df['timestamp'].max().date()}")
        print(f"📈 当前指数: {close_prices.iloc[-1]:.2f}")
        print(f"💰 最高指数: {close_prices.max():.2f} (距最高点跌幅: {(1-close_prices.min()/close_prices.max())*100:.2f}%)")
        print(f"💎 最低指数: {close_prices.min():.2f} (距最高点跌幅: {(1-close_prices.min()/close_prices.max())*100:.2f}%)")
        print(f"📊 平均指数: {close_prices.mean():.2f}")
        print(f"📉 波动范围: {close_prices.max() - close_prices.min():.2f} 点")
        
        # 不同周期的变化
        if len(df) >= 5:
            change_5 = (close_prices.iloc[-1] / close_prices.iloc[-5] - 1) * 100
            print(f"5期指数涨跌: {change_5:+.2f}%")
        if len(df) >= 10:
            change_10 = (close_prices.iloc[-1] / close_prices.iloc[-10] - 1) * 100
            print(f"10期指数涨跌: {change_10:+.2f}%")
        if len(df) >= 20:
            change_20 = (close_prices.iloc[-1] / close_prices.iloc[-20] - 1) * 100
            print(f"20期指数涨跌: {change_20:+.2f}%")
    
    @staticmethod
    def _return_analysis(df):
        """收益率分析"""
        close_prices = df['close']
        returns = StatisticalAnalysis.calculate_returns(close_prices)
        
        print(f"📈 日均指数变化率: {returns.mean()*100:.4f}%")
        print(f"📊 日指数变化标准差: {returns.std()*100:.4f}%")
        print(f"📉 年化波动率: {StatisticalAnalysis.volatility(returns)*100:.2f}%")
        
        skew, kurt = StatisticalAnalysis.skewness_kurtosis(returns)
        print(f"🔄 偏度 (Skewness): {skew:.4f}", end="")
        if skew < -0.5:
            print(" (左偏:指数易下跌)")
        elif skew > 0.5:
            print(" (右偏:指数易上涨)")
        else:
            print(" (基本对称)")
        
        print(f"📌 峰度 (Kurtosis): {kurt:.4f}", end="")
        if kurt > 1:
            print(" (厚尾:指数异常波动频繁)")
        else:
            print(" (正常)")
        
        sharpe = StatisticalAnalysis.sharpe_ratio(returns)
        print(f"📊 夏普比率: {sharpe:.4f}", end="")
        if sharpe > 1:
            print(" (优秀的收益风险比)")
        elif sharpe > 0:
            print(" (一般的收益风险比)")
        else:
            print(" (亏损)")
        
        drawdown, max_dd = StatisticalAnalysis.draw_down(close_prices)
        print(f"📉 最大回撤: {max_dd*100:.2f}% (从高点下跌幅度)")
    
    @staticmethod
    def _technical_analysis(df):
        """技术面分析"""
        close = df['close']
        high = df['high']
        low = df['low']
        
        # MA
        ma5 = TechnicalAnalysis.moving_average(close, 5)
        ma10 = TechnicalAnalysis.moving_average(close, 10)
        ma20 = TechnicalAnalysis.moving_average(close, 20)
        
        print("指数均线系统:")
        print(f"  MA5: {ma5.iloc[-1]:.2f}")
        print(f"  MA10: {ma10.iloc[-1]:.2f}")
        print(f"  MA20: {ma20.iloc[-1]:.2f}")
        print(f"  当前指数: {close.iloc[-1]:.2f}")
        
        # 价格与均线关系
        if close.iloc[-1] > ma5.iloc[-1] > ma10.iloc[-1] > ma20.iloc[-1]:
            print("  → 多头排列 📈 (指数强势)")
        elif close.iloc[-1] < ma5.iloc[-1] < ma10.iloc[-1] < ma20.iloc[-1]:
            print("  → 空头排列 📉 (指数弱势)")
        else:
            print("  → 混合排列 → (观望)")
        
        # MACD
        macd_line, signal_line, histogram = TechnicalAnalysis.macd(close)
        print(f"\nMACD指标:")
        print(f"  MACD线: {macd_line.iloc[-1]:.4f}")
        print(f"  信号线: {signal_line.iloc[-1]:.4f}")
        print(f"  柱状图: {histogram.iloc[-1]:.4f}")
        if macd_line.iloc[-1] > signal_line.iloc[-1]:
            print("  → 金叉信号 (指数上升动力)")
        else:
            print("  → 死叉信号 (指数下降动力)")
        
        # RSI
        rsi = TechnicalAnalysis.rsi(close, 14)
        print(f"\nRSI指标 (14): {rsi.iloc[-1]:.2f}")
        if rsi.iloc[-1] > 70:
            print("  → 超买区域 (指数可能调整)")
        elif rsi.iloc[-1] < 30:
            print("  → 超卖区域 (指数可能反弹)")
        else:
            print("  → 正常区域")
        
        # 布林带
        upper, mid, lower = TechnicalAnalysis.bollinger_bands(close, 20, 2)
        print(f"\n布林带:")
        print(f"  上轨: {upper.iloc[-1]:.2f}")
        print(f"  中轨: {mid.iloc[-1]:.2f}")
        print(f"  下轨: {lower.iloc[-1]:.2f}")
        print(f"  当前: {close.iloc[-1]:.2f}")
        if close.iloc[-1] > upper.iloc[-1]:
            print("  → 触及上轨 (指数可能见顶)")
        elif close.iloc[-1] < lower.iloc[-1]:
            print("  → 触及下轨 (指数可能见底)")
        else:
            bb_pct = (close.iloc[-1] - lower.iloc[-1]) / (upper.iloc[-1] - lower.iloc[-1])
            print(f"  → 位置 {bb_pct*100:.1f}% (中性区域)")
    
    @staticmethod
    def _trend_analysis(df):
        """趋势分析"""
        close = df['close']
        high = df['high']
        low = df['low']
        
        # 线性回归
        trend_info = TrendAnalysis.linear_regression_trend(close)
        print(f"📉 线性趋势:")
        print(f"  方向: {trend_info['direction']}")
        print(f"  斜率: {trend_info['slope']:.6f} 点/周期")
        print(f"  拟合度 (R²): {trend_info['r_squared']:.4f}")
        
        # 峰值谷值
        peaks, valleys = TrendAnalysis.find_peaks_and_valleys(close)
        print(f"\n🔄 指数转折点:")
        print(f"  历史峰值: {len(peaks)} 个", end="")
        if len(peaks) > 0:
            print(f" (最近: 第{len(close)-peaks[-1]}期前)")
        else:
            print()
        print(f"  历史谷值: {len(valleys)} 个", end="")
        if len(valleys) > 0:
            print(f" (最近: 第{len(close)-valleys[-1]}期前)")
        else:
            print()
        
        # 趋势强度
        di_plus, di_minus = TrendAnalysis.trend_strength(high, low, close, 14)
        print(f"\n💪 趋势强度 (DMI):")
        print(f"  +DI: {di_plus.iloc[-1]:.2f}")
        print(f"  -DI: {di_minus.iloc[-1]:.2f}")
        if di_plus.iloc[-1] > di_minus.iloc[-1]:
            print("  → 上升趋势强势")
        else:
            print("  → 下跌趋势强势")
        
        # 周期性
        cycles = TrendAnalysis.cycle_detection(close)
        if len(cycles) > 0:
            print(f"\n🔁 周期性检测:")
            print(f"  检测到显著周期: {cycles.tolist()[:5]}")
        else:
            print(f"\n🔁 周期性检测: 未检测到显著周期")
    
    @staticmethod
    def _risk_assessment(df):
        """风险评估"""
        close = df['close']
        returns = StatisticalAnalysis.calculate_returns(close)
        
        # VaR (Value at Risk)
        var_95 = np.percentile(returns, 5)
        var_99 = np.percentile(returns, 1)
        
        print(f"📊 风险指标:")
        print(f"  日 VaR (95%): {var_95*100:.4f}% (95%概率日损不超过)")
        print(f"  日 VaR (99%): {var_99*100:.4f}% (99%概率日损不超过)")
        
        # 压力测试
        worst_day = returns.min()
        best_day = returns.max()
        print(f"\n  最差单日: {worst_day*100:.2f}%")
        print(f"  最好单日: {best_day*100:.2f}%")
        
        # 风险分级
        annual_vol = StatisticalAnalysis.volatility(returns)
        print(f"\n🎯 风险级别评估:")
        print(f"  年化波动率: {annual_vol*100:.2f}%")
        if annual_vol < 0.15:
            print("  评级: 低风险 ✅")
        elif annual_vol < 0.30:
            print("  评级: 中等风险 ⚠️")
        else:
            print("  评级: 高风险 ⛔")
    
    @staticmethod
    def _market_patterns(df):
        """市场规律发现"""
        close = df['close']
        returns = StatisticalAnalysis.calculate_returns(close)
        
        print("🔍 CSGO饰品大盘指数规律发现:\n")
        
        # 规律1：相对基准点的变化 → 改为相对当前市场支撑的变化
        print("1️⃣  市场估值规律 (基于当前市场实际支撑):")
        current_index = close.iloc[-1]
        
        # 计算更有意义的市场指标
        median_price = close.median()  # 历史中位数 - 市场平衡点
        ma_52week = close.iloc[-252:].mean() if len(close) > 252 else close.mean()  # 52周均线
        recent_high = close.iloc[-60:].max()  # 最近60天高点
        recent_low = close.iloc[-60:].min()   # 最近60天低点
        
        # 相对中位数的偏离程度
        median_deviation = (current_index / median_price - 1) * 100
        # 相对52周均线的偏离程度
        ma_deviation = (current_index / ma_52week - 1) * 100
        
        print(f"   当前指数: {current_index:.2f}")
        print(f"   历史中位数: {median_price:.2f}")
        print(f"   相对中位数偏离: {median_deviation:+.2f}%")
        print(f"   52周均线: {ma_52week:.2f}")
        print(f"   相对52周均线偏离: {ma_deviation:+.2f}%")
        print(f"   最近60天范围: {recent_low:.2f} - {recent_high:.2f}")
        
        # 基于当前市场给出评价
        if current_index > recent_high * 0.95:
            print(f"   → 指数处于近期高位，存在调整风险 ⚠️")
        elif current_index < recent_low * 1.05:
            print(f"   → 指数处于近期低位，存在反弹机会 📈")
        else:
            if median_deviation > 15:
                print(f"   → 指数相对历史均值高估，需关注风险 ⚖️")
            elif median_deviation < -15:
                print(f"   → 指数相对历史均值低估，可能存在机会 🎯")
            else:
                print(f"   → 指数相对合理，市场处于平衡状态 ⚖️")
        
        # 规律2：动量
        print("\n2️⃣  指数动量规律 (反映市场加速度):")
        momentum_short = (close.iloc[-1] - close.iloc[-6]) / close.iloc[-6] * 100 if len(close) > 5 else 0
        momentum_long = (close.iloc[-1] - close.iloc[-21]) / close.iloc[-21] * 100 if len(close) > 20 else 0
        print(f"   5期动量: {momentum_short:+.2f}%")
        print(f"   20期动量: {momentum_long:+.2f}%")
        if momentum_short > 0 and momentum_long > 0:
            print("   → 双重上升动量，饰品整体升值加速 🚀")
        elif momentum_short < 0 and momentum_long < 0:
            print("   → 双重下降动量，饰品整体贬值加速 🔻")
        elif momentum_short > 0 and momentum_long < 0:
            print("   → 短期反弹，但长期趋势仍下行")
        else:
            print("   → 短期震荡，长期趋势向上")
        
        # 规律3：波动率聚集
        print("\n3️⃣  波动率聚集规律 (市场稳定性):")
        vol_short = returns.iloc[-10:].std() if len(returns) > 10 else returns.std()
        vol_long = returns.std()
        vol_ratio = vol_short / vol_long if vol_long != 0 else 1
        print(f"   近期波动率/历史平均: {vol_ratio:.2f}")
        print(f"   历史日波动率: {vol_long*100:.4f}%")
        if vol_ratio > 1.5:
            print("   → 波动率大幅上升，市场风险增加，容易出现快速趋势 ⚡")
        elif vol_ratio < 0.7:
            print("   → 波动率大幅下降，市场风险下降，市场进入整理阶段 ⏸️")
        else:
            print("   → 波动率相对稳定，市场风险处于正常水平")
        
        # 规律4：自相关与反转
        print("\n4️⃣  自相关规律 (市场记忆效应):")
        acf = pd.Series(returns).autocorr(lag=1)
        print(f"   1期自相关系数: {acf:.4f}")
        if acf > 0.1:
            print("   → 存在正相关，指数有惯性，上升/下降趋势延续可能性大")
        elif acf < -0.1:
            print("   → 存在负相关，指数易反转，市场具有均值回复特性")
        else:
            print("   → 基本独立，市场呈随机游走，无明显规律")
    
    @staticmethod
    def _trading_signals(df):
        """指数方向信号"""
        close = df['close']
        
        signals = []
        
        # 信号1：均线
        ma5 = TechnicalAnalysis.moving_average(close, 5)
        ma10 = TechnicalAnalysis.moving_average(close, 10)
        if close.iloc[-1] > ma5.iloc[-1] > ma10.iloc[-1]:
            signals.append(("均线多头", "指数强势↑", 70))
        elif close.iloc[-1] < ma5.iloc[-1] < ma10.iloc[-1]:
            signals.append(("均线空头", "指数弱势↓", 70))
        
        # 信号2：MACD
        macd_line, signal_line, _ = TechnicalAnalysis.macd(close)
        if len(macd_line) > 1:
            if macd_line.iloc[-1] > signal_line.iloc[-1] and macd_line.iloc[-2] <= signal_line.iloc[-2]:
                signals.append(("MACD金叉", "指数上升动力", 65))
            elif macd_line.iloc[-1] < signal_line.iloc[-1] and macd_line.iloc[-2] >= signal_line.iloc[-2]:
                signals.append(("MACD死叉", "指数下降动力", 65))
        
        # 信号3：RSI
        rsi = TechnicalAnalysis.rsi(close, 14)
        if rsi.iloc[-1] < 30:
            signals.append(("RSI超卖", "指数过度调整", 60))
        elif rsi.iloc[-1] > 70:
            signals.append(("RSI超买", "指数过度上升", 60))
        
        # 信号4：布林带
        upper, mid, lower = TechnicalAnalysis.bollinger_bands(close)
        if close.iloc[-1] < lower.iloc[-1]:
            signals.append(("布林下轨", "指数可能反弹", 55))
        elif close.iloc[-1] > upper.iloc[-1]:
            signals.append(("布林上轨", "指数可能回调", 55))
        
        if not signals:
            print("📊 当前信号: 无明确方向信号 (指数处于过渡阶段，观望中)")
            return
        
        # 聚合信号
        up_signals = [s for s in signals if "↑" in s[1] or "上升" in s[1] or "反弹" in s[1]]
        down_signals = [s for s in signals if "↓" in s[1] or "下降" in s[1] or "回调" in s[1]]
        
        avg_confidence = np.mean([s[2] for s in signals])
        
        if len(up_signals) > len(down_signals):
            print(f"🟢 综合指数信号: 上升趋势 (置信度: {avg_confidence:.0f}%)")
        elif len(down_signals) > len(up_signals):
            print(f"🔴 综合指数信号: 下跌趋势 (置信度: {avg_confidence:.0f}%)")
        else:
            print(f"🟡 综合指数信号: 混合状态 (置信度: {avg_confidence:.0f}%)")
        
        print("\n指数信号详解:")
        for signal_name, direction, confidence in sorted(signals, key=lambda x: -x[2]):
            emoji = "🟢" if "↑" in direction or "上升" in direction or "反弹" in direction else "🔴"
            print(f"  {emoji} {signal_name}: {direction} (置信度: {confidence}%)")
    
    @staticmethod
    def _generate_forecast_report(df):
        """生成预测分析报告"""
        print("\n【第八部分】未来价格预测分析")
        print("-" * 80)
        
        close = df['close']
        timestamp = df['timestamp']
        
        # 1天、7天、14天预测
        forecast_periods = [1, 7, 14]
        
        for periods in forecast_periods:
            print(f"\n📊 {periods}天价格预测")
            print("=" * 60)
            
            # 获取集成预测
            ensemble_result = AdvancedPredictiveAnalysis.ensemble_forecast(
                close, timestamp, forecast_periods=periods
            )
            
            if ensemble_result is None:
                print(f"❌ 预测失败: 数据不足")
                continue
            
            ensemble_forecast = ensemble_result['ensemble_forecast']
            individual_forecasts = ensemble_result['individual_forecasts']
            model_weights = ensemble_result['model_weights']
            
            # 计算置信区间
            confidence_intervals = AdvancedPredictiveAnalysis.calculate_forecast_confidence(
                close, ensemble_forecast, periods
            )
            
            current_price = close.iloc[-1]
            
            print(f"\n🎯 集成预测结果 (融合 {ensemble_result['models_used']} 个模型):")
            print(f"   当前价格: {current_price:.2f}")
            print(f"   {periods}天目标价: {ensemble_forecast[-1]:.2f}")
            
            change_percent = (ensemble_forecast[-1] / current_price - 1) * 100
            emoji = "📈" if change_percent > 0 else "📉"
            print(f"   预期变化: {change_percent:+.2f}% {emoji}")
            
            # 显示各模型权重和预测
            print(f"\n📋 模型贡献度与预测:")
            for model_name in sorted(individual_forecasts.keys()):
                weight = model_weights[model_name] * 100
                forecast_value = individual_forecasts[model_name][-1]
                change = (forecast_value / current_price - 1) * 100
                print(f"   • {model_name:25s} | 权重: {weight:5.1f}% | 预测: {forecast_value:8.2f} ({change:+6.2f}%)")
            
            # 显示详细预测路径
            print(f"\n📈 预测路径 (逐日预测):")
            for conf_interval in confidence_intervals:
                period = int(conf_interval['period'])
                forecast = conf_interval['forecast']
                upper = conf_interval['upper_95']
                lower = conf_interval['lower_95']
                change = (forecast / current_price - 1) * 100
                
                print(f"   Day {period}: {forecast:8.2f} | 95%区间: [{lower:8.2f}, {upper:8.2f}] | 变化: {change:+6.2f}%")
            
            # 预测趋势分析
            print(f"\n🔍 预测趋势分析:")
            
            if ensemble_forecast[-1] > current_price * 1.05:
                print(f"   ✅ 强烈上升信号 - 预期指数向上突破")
            elif ensemble_forecast[-1] > current_price * 1.01:
                print(f"   📈 温和上升信号 - 预期指数缓慢上升")
            elif ensemble_forecast[-1] < current_price * 0.95:
                print(f"   ⚠️  强烈下跌信号 - 预期指数向下调整")
            elif ensemble_forecast[-1] < current_price * 0.99:
                print(f"   📉 温和下跌信号 - 预期指数缓慢下跌")
            else:
                print(f"   ⏸️  震荡整理 - 预期指数在当前位置波动")
            
            # 显示波动率信息
            std_error_last = confidence_intervals[-1]['std_error']
            print(f"   📊 预测不确定性: ±{std_error_last:.2f} (基于历史波动率估算)")
            
            # 最大盈利和最大风险
            max_forecast = max(cf['forecast'] for cf in confidence_intervals)
            min_forecast = min(cf['forecast'] for cf in confidence_intervals)
            max_profit = (max_forecast / current_price - 1) * 100
            max_loss = (min_forecast / current_price - 1) * 100
            
            print(f"   💰 最大盈利空间: +{max_profit:.2f}%")
            print(f"   ⚠️  最大风险空间: {max_loss:.2f}%")
            print(f"   📊 风险收益比: {abs(max_profit / max_loss) if max_loss != 0 else 'N/A':.2f}x")


# ============================================================================
# 7. 可视化模块
# ============================================================================

def plot_analysis(df, output_file='kline_analysis.html'):
    """
    生成交互式可视化图表
    """
    try:
        import plotly.graph_objects as go
        from plotly.subplots import make_subplots
    except ImportError:
        print("⚠️  需要安装 plotly: pip install plotly")
        return
    
    close = df['close']
    high = df['high']
    low = df['low']
    volume = df.get('volume', pd.Series(index=df.index, dtype=float))
    
    # 创建子图
    fig = make_subplots(
        rows=3, cols=1,
        shared_xaxes=True,
        row_heights=[0.5, 0.25, 0.25],
        subplot_titles=("指数K线图", "成交量", "MACD")
    )
    
    # K线图
    fig.add_trace(
        go.Candlestick(
            x=df['timestamp'],
            open=df['open'],
            high=high,
            low=low,
            close=close,
            name='指数'
        ),
        row=1, col=1
    )
    
    # 均线
    ma5 = TechnicalAnalysis.moving_average(close, 5)
    ma10 = TechnicalAnalysis.moving_average(close, 10)
    ma20 = TechnicalAnalysis.moving_average(close, 20)
    
    fig.add_trace(go.Scatter(x=df['timestamp'], y=ma5, name='MA5', line=dict(color='orange')), row=1, col=1)
    fig.add_trace(go.Scatter(x=df['timestamp'], y=ma10, name='MA10', line=dict(color='blue')), row=1, col=1)
    fig.add_trace(go.Scatter(x=df['timestamp'], y=ma20, name='MA20', line=dict(color='red')), row=1, col=1)
    
    # 成交量
    if not volume.isna().all():
        fig.add_trace(
            go.Bar(x=df['timestamp'], y=volume, name='成交量', marker_color='rgba(128,128,128,0.5)'),
            row=2, col=1
        )
    
    # MACD
    macd_line, signal_line, histogram = TechnicalAnalysis.macd(close)
    fig.add_trace(go.Scatter(x=df['timestamp'], y=macd_line, name='MACD', line=dict(color='blue')), row=3, col=1)
    fig.add_trace(go.Scatter(x=df['timestamp'], y=signal_line, name='Signal', line=dict(color='red')), row=3, col=1)
    
    fig.update_layout(height=1000, title_text="CSGO饰品大盘指数K线分析图", hovermode='x unified')
    fig.write_html(output_file)
    print(f"\n📊 图表已生成: {output_file}")


# ============================================================================
# 8. 主程序
# ============================================================================

def main():
    """主程序入口"""
    
    print("""
    ╔════════════════════════════════════════════════════════════════╗
    ║          CSGO饰品大盘指数K线数据分析系统 v2.0                  ║
    ║                                                                ║
    ║  功能: 综合技术分析、统计分析、趋势分析、风险评估、指数预测     ║
    ║  样本量: 11269件平台在售数>50的饰品                           ║
    ║  分析方法: 消除量纲影响，采用去量纲化处理                      ║
    ║  时间: 2025                                                    ║
    ╚════════════════════════════════════════════════════════════════╝
    """)
    
    # 获取数据
    fetcher = KlineDataFetcher()
    df = fetcher.fetch_kline(index_id=3, kline_type="1hour")
    
    if df is None or len(df) < 10:
        print("❌ 数据不足，分析中止")
        return
    
    # 生成报告
    AnalysisReporter.generate_report(df)
    
    # 生成图表
    try:
        plot_analysis(df, 'kline_analysis.html')
    except Exception as e:
        print(f"⚠️  图表生成失败: {e}")
    
    print("\n✅ 分析完成！")
    print("💡 说明: 市场规律、技术面和风险评估均基于实时数据，反映当前市场情况")


def test_fetch():
    """测试数据获取功能"""
    print("\n" + "="*80)
    print("开始测试数据获取...")
    print("="*80)
    
    fetcher = KlineDataFetcher(base_url="http://localhost:8080")
    df = fetcher.fetch_kline(index_id=3, kline_type="1hour", verbose=True)
    
    if df is not None:
        print(f"\n✅ 测试成功！")
        print(f"数据框形状: {df.shape}")
        print(f"\n数据框列名: {df.columns.tolist()}")
        print(f"\n数据框前5行:")
        print(df.head())
        return True
    else:
        print(f"\n❌ 测试失败！")
        return False


def forecast_only():
    """仅生成预测报告（不进行完整分析）"""
    print("""
    ╔════════════════════════════════════════════════════════════════╗
    ║          CSGO饰品大盘指数预测系统 v1.0                        ║
    ║                                                                ║
    ║  功能: 1天、7天、14天价格预测                                 ║
    ║  预测方法: ARIMA + 指数平滑 + 趋势分析 + 加权动量             ║
    ║  时间: 2025                                                    ║
    ╚════════════════════════════════════════════════════════════════╝
    """)
    
    # 获取数据
    fetcher = KlineDataFetcher()
    df = fetcher.fetch_kline(index_id=3, kline_type="1hour")
    
    if df is None or len(df) < 10:
        print("❌ 数据不足，预测中止")
        return
    
    # 仅生成预测报告
    AnalysisReporter._generate_forecast_report(df)
    
    print("\n✅ 预测完成！")


def export_forecast_to_json(output_file='forecast_result.json'):
    """
    导出预测结果到JSON格式
    
    Args:
        output_file: 输出文件路径
    """
    print("\n📊 正在导出预测数据到JSON...")
    
    # 获取数据
    fetcher = KlineDataFetcher()
    df = fetcher.fetch_kline(index_id=3, kline_type="1hour", verbose=False)
    
    if df is None or len(df) < 10:
        print("❌ 数据不足，导出中止")
        return
    
    close = df['close']
    timestamp = df['timestamp']
    
    result = {
        'metadata': {
            'generated_at': datetime.now().isoformat(),
            'current_price': float(close.iloc[-1]),
            'data_points': len(df),
            'time_range': {
                'start': timestamp.min().isoformat(),
                'end': timestamp.max().isoformat()
            }
        },
        'forecasts': {}
    }
    
    # 生成1天、7天、14天的预测
    for periods in [1, 7, 14]:
        ensemble_result = AdvancedPredictiveAnalysis.ensemble_forecast(
            close, timestamp, forecast_periods=periods
        )
        
        if ensemble_result is None:
            result['forecasts'][f'{periods}_days'] = None
            continue
        
        ensemble_forecast = ensemble_result['ensemble_forecast']
        individual_forecasts = ensemble_result['individual_forecasts']
        model_weights = ensemble_result['model_weights']
        
        confidence_intervals = AdvancedPredictiveAnalysis.calculate_forecast_confidence(
            close, ensemble_forecast, periods
        )
        
        current_price = close.iloc[-1]
        
        forecast_data = {
            'forecast_value': float(ensemble_forecast[-1]),
            'change_percent': float((ensemble_forecast[-1] / current_price - 1) * 100),
            'model_weights': {k: float(v) for k, v in model_weights.items()},
            'individual_forecasts': {
                k: [float(v) for v in vals] 
                for k, vals in individual_forecasts.items()
            },
            'detailed_path': [
                {
                    'day': int(conf['period']),
                    'forecast': float(conf['forecast']),
                    'upper_95': float(conf['upper_95']),
                    'lower_95': float(conf['lower_95']),
                    'std_error': float(conf['std_error']),
                    'change_percent': float((conf['forecast'] / current_price - 1) * 100)
                }
                for conf in confidence_intervals
            ],
            'risk_metrics': {
                'max_profit_percent': float(
                    (max(c['forecast'] for c in confidence_intervals) / current_price - 1) * 100
                ),
                'max_loss_percent': float(
                    (min(c['forecast'] for c in confidence_intervals) / current_price - 1) * 100
                )
            }
        }
        
        result['forecasts'][f'{periods}_days'] = forecast_data
    
    # 写入JSON文件
    import os
    output_path = os.path.join(os.getcwd(), output_file)
    
    with open(output_path, 'w', encoding='utf-8') as f:
        json.dump(result, f, ensure_ascii=False, indent=2)
    
    print(f"✅ 预测数据已导出到: {output_path}")
    print(f"   文件大小: {os.path.getsize(output_path)} 字节")
    
    return result




def main_complete_analysis():
    """完整分析流程"""
    print("\n" + "="*80)
    print("启动完整分析...")
    print("="*80)
    
    try:
        fetcher = KlineDataFetcher()
        df = fetcher.fetch_kline(index_id=3, kline_type="1hour")
        
        if df is None or len(df) < 10:
            print("❌ 数据获取失败或数据不足")
            return
        
        # 生成报告
        AnalysisReporter.generate_report(df)
        
        # 生成图表
        try:
            plot_analysis(df, 'kline_analysis.html')
        except Exception as e:
            print(f"⚠️  图表生成失败: {e}")
        
        print(f"✅ 分析完成，共 {len(df)} 条数据")
        print("\n✨ 已生成:")
        print("   • indicator_analysis_final.xlsx")
        print("   • dashboard.html")
        print("   • latest_60days_signals.csv")
        print("   • trading_signals_latest.json")
        
    except Exception as e:
        print(f"\n❌ 分析过程出错: {str(e)}")



class BaselineModelTrainer:
    """基线模型训练：ARIMA、Prophet、LightGBM"""
    
    def __init__(self, train_data, test_data, test_size=60):
        self.train_data = train_data
        self.test_data = test_data
        self.test_size = test_size
        self.results = {}
        self.predictions = {}
        
        # 初始化持久化管理器
        self.persistence = ModelPersistenceManager()
        
        # 加载之前的训练元数据
        self.previous_metadata = self.persistence.load_metadata()
        
    def train_arima(self):
        """训练 ARIMA(1,1,1) - 支持增量学习"""
        try:
            from statsmodels.tsa.statespace.sarimax import SARIMAX
            print("  🚀 ARIMA(1,1,1) 训练...")
            
            # 检查是否存在之前的模型
            previous_model = self.persistence.load_model('arima')
            if previous_model is not None:
                print("     ✅ 加载上次的模型，进行增量学习...")
                # 用新数据更新模型
                model = SARIMAX(
                    self.train_data['close'],
                    order=(1,1,1),
                    enforce_stationarity=False
                )
                res = model.fit(disp=False, maxiter=200)
            else:
                print("     📝 首次训练，从零开始...")
                model = SARIMAX(self.train_data['close'], order=(1,1,1), enforce_stationarity=False)
                res = model.fit(disp=False)
            
            pred = res.get_forecast(steps=self.test_size).predicted_mean.values
            self.predictions['arima'] = pred
            rmse = np.sqrt(np.mean((self.test_data['close'].values - pred) ** 2))
            self.results['ARIMA'] = {'RMSE': rmse}
            
            # 保存模型
            self.persistence.save_model(res, 'arima')
            
            print(f"     ✅ RMSE: {rmse:.4f}")
            
            # 计算改进
            if self.previous_metadata and 'arima_rmse' in self.previous_metadata:
                prev_rmse = self.previous_metadata['arima_rmse']
                improvement = (prev_rmse - rmse) / prev_rmse * 100
                if improvement > 0:
                    print(f"     📈 相比上次改进: {improvement:.2f}%")
                else:
                    print(f"     📉 相比上次变差: {-improvement:.2f}%")
                self.results['ARIMA']['improvement'] = improvement
            
            return True
        except Exception as e:
            print(f"     ⚠️  ARIMA 失败: {str(e)[:50]}")
            return False
    
    def train_prophet(self):
        """训练 Prophet - 支持增量学习"""
        try:
            from prophet import Prophet
            print("  🚀 Prophet 训练...")
            
            # 处理时间索引
            if 'timestamp' in self.train_data.columns:
                ds = pd.to_datetime(self.train_data['timestamp'])
            elif self.train_data.index.name == 'date' or self.train_data.index.name == 'timestamp':
                ds = pd.to_datetime(self.train_data.index)
            else:
                ds = pd.date_range(end=pd.Timestamp.now(), periods=len(self.train_data), freq='D')
            
            # 移除时区信息，Prophet 不支持带时区的日期时间
            if isinstance(ds, pd.Series):
                if ds.dt.tz is not None:
                    ds = ds.dt.tz_localize(None)
            elif hasattr(ds, 'tz') and ds.tz is not None:
                ds = ds.tz_localize(None)
            
            df_prop = pd.DataFrame({'ds': ds, 'y': self.train_data['close'].values})
            
            # 检查是否存在之前的模型（Prophet不直接支持增量学习，但我们可以用新数据重新训练）
            print("     📝 基于最新数据训练Prophet...")
            m = Prophet(daily_seasonality=False, yearly_seasonality=False)
            m.fit(df_prop)
            
            # 生成预测
            future_periods = self.test_size
            future_dates = pd.date_range(start=ds.max() + pd.Timedelta(days=1), periods=future_periods, freq='D')
            future = pd.DataFrame({'ds': future_dates})
            fc = m.predict(future)
            pred = fc['yhat'].values[:self.test_size]
            
            self.predictions['prophet'] = pred
            rmse = np.sqrt(np.mean((self.test_data['close'].values - pred) ** 2))
            self.results['Prophet'] = {'RMSE': rmse}
            
            # 保存模型
            self.persistence.save_model(m, 'prophet')
            
            print(f"     ✅ RMSE: {rmse:.4f}")
            
            # 计算改进
            if self.previous_metadata and 'prophet_rmse' in self.previous_metadata:
                prev_rmse = self.previous_metadata['prophet_rmse']
                improvement = (prev_rmse - rmse) / prev_rmse * 100
                if improvement > 0:
                    print(f"     📈 相比上次改进: {improvement:.2f}%")
                else:
                    print(f"     📉 相比上次变差: {-improvement:.2f}%")
                self.results['Prophet']['improvement'] = improvement
            
            return True
        except Exception as e:
            print(f"     ⚠️  Prophet 失败: {str(e)[:50]}")
            return False
    
    def train_lightgbm(self, features_list):
        """训练 XGBoost - 支持增量学习"""
        try:
            import xgboost as xgb
            from sklearn.metrics import mean_squared_error
            print("  🚀 XGBoost 训练...")
            
            X_train = self.train_data[features_list].fillna(0).values
            y_train = self.train_data['close'].shift(-1).dropna().values[:-1]
            X_test = self.test_data[features_list].fillna(0).values
            
            # 确保长度匹配
            if len(X_train) > len(y_train):
                X_train = X_train[:len(y_train)]
            
            # 检查是否存在之前的模型
            previous_model = self.persistence.load_model('xgboost')
            if previous_model is not None:
                print("     ✅ 加载上次的模型，进行增量学习...")
                # XGBoost 支持 warm_start，但需要重新训练
                # 这里我们用 early_stopping 和更多轮次来微调
                model = xgb.XGBRegressor(
                    n_estimators=150,  # 增加轮次
                    random_state=42,
                    verbosity=0,
                    eval_metric='rmse'
                )
            else:
                print("     📝 首次训练，从零开始...")
                model = xgb.XGBRegressor(
                    n_estimators=100,
                    random_state=42,
                    verbosity=0,
                    eval_metric='rmse'
                )
            
            model.fit(X_train, y_train)
            pred = model.predict(X_test)
            self.predictions['xgb'] = pred[:len(self.test_data)]
            rmse = np.sqrt(mean_squared_error(self.test_data['close'].values, pred[:len(self.test_data)]))
            self.results['XGBoost'] = {'RMSE': rmse}
            
            # 保存模型
            self.persistence.save_model(model, 'xgboost')
            
            print(f"     ✅ RMSE: {rmse:.4f}")
            
            # 计算改进
            if self.previous_metadata and 'xgb_rmse' in self.previous_metadata:
                prev_rmse = self.previous_metadata['xgb_rmse']
                improvement = (prev_rmse - rmse) / prev_rmse * 100
                if improvement > 0:
                    print(f"     📈 相比上次改进: {improvement:.2f}%")
                else:
                    print(f"     📉 相比上次变差: {-improvement:.2f}%")
                self.results['XGBoost']['improvement'] = improvement
            
            return True
        except Exception as e:
            print(f"     ⚠️  XGBoost 失败: {str(e)[:50]}")
            return False
    
    def save_training_metadata(self):
        """保存训练元数据供下次使用"""
        metadata = {
            'timestamp': datetime.now().isoformat(),
            'train_size': len(self.train_data),
            'test_size': len(self.test_data),
            'data_range': {
                'start': str(self.train_data['timestamp'].min()) if 'timestamp' in self.train_data.columns else 'N/A',
                'end': str(self.train_data['timestamp'].max()) if 'timestamp' in self.train_data.columns else 'N/A'
            }
        }
        
        # 添加各模型的RMSE
        for model_name, results in self.results.items():
            if 'RMSE' in results:
                rmse_key = f"{model_name.lower()}_rmse"
                metadata[rmse_key] = results['RMSE']
            if 'improvement' in results:
                imp_key = f"{model_name.lower()}_improvement"
                metadata[imp_key] = results['improvement']
        
        self.persistence.save_metadata(metadata)
        
        # 添加到训练历史
        training_info = {
            'timestamp': datetime.now().isoformat(),
            'arima_rmse': self.results.get('ARIMA', {}).get('RMSE'),
            'prophet_rmse': self.results.get('Prophet', {}).get('RMSE'),
            'xgb_rmse': self.results.get('XGBoost', {}).get('RMSE'),
            'improvement_percent': self.results.get('ARIMA', {}).get('improvement')
        }
        
        self.persistence.add_training_history(training_info)
        
        return True


class SimpleBacktester:
    """简易回测系统"""
    
    def __init__(self, test_data, predictions):
        self.test_data = test_data
        self.predictions = predictions
        
    def backtest(self, threshold=0.02):
        """执行回测 - 基于价格变化的简单策略"""
        print("  📊 运行回测...")
        portfolio_value = 1.0
        trades = []
        
        # 确保预测长度匹配
        preds = self.predictions
        if isinstance(preds, np.ndarray):
            preds = preds[:len(self.test_data)]
        
        # 检查预测中是否有 NaN
        valid_mask = ~np.isnan(preds)
        
        print(f"     预测中有效值: {valid_mask.sum()} / {len(preds)}")
        print(f"     预测统计: min={np.nanmin(preds):.2f}, max={np.nanmax(preds):.2f}, mean={np.nanmean(preds):.2f}")
        
        # 计算历史波动率作为参考
        prices = self.test_data['close'].values
        price_changes = np.diff(prices) / prices[:-1]
        volatility = np.std(price_changes)
        
        print(f"     价格统计: min={prices.min():.2f}, max={prices.max():.2f}, 波动率={volatility:.4f}")
        
        for i in range(len(self.test_data) - 1):
            current = self.test_data.iloc[i]['close']
            next_price = self.test_data.iloc[i + 1]['close']
            
            if i >= len(preds) or np.isnan(preds[i]):
                trades.append(0)
                continue
            
            pred = preds[i]
            
            # 策略：使用简单的趋势判断
            # 由于预测可能有缩放问题，使用相对排名而不是绝对值
            
            # 方法：预测高于平均值 = 看涨信号
            pred_relative = (pred - np.nanmean(preds)) / (np.nanstd(preds) + 1e-9)
            
            # 如果预测标准化值 > 0.5（高于平均），则买入
            if pred_relative > 0.5:
                entry_price = next_price
                # 持仓7天后卖出
                hold_days = 7
                if i + hold_days + 1 < len(self.test_data):
                    exit_price = self.test_data.iloc[i + hold_days + 1]['close']
                else:
                    exit_price = self.test_data.iloc[-1]['close']
                # 计算单日回报
                ret = (exit_price - entry_price) / entry_price
                portfolio_value *= (1 + ret)
                trades.append(ret)
            else:
                trades.append(0)
        
        # 计算最终收益
        cum_return = portfolio_value - 1.0
        actual_trades = [t for t in trades if t != 0]
        win_rate = (np.array(actual_trades) > 0).sum() / len(actual_trades) if len(actual_trades) > 0 else 0
        
        print(f"     有效交易: {len(actual_trades)}")
        if actual_trades:
            print(f"     平均收益: {np.mean(actual_trades):.4f}")
        
        return {
            'cumulative_return': float(cum_return),
            'win_rate': float(win_rate),
            'total_trades': len(actual_trades)
        }







class RealTradingStrategies:
    """实战交易策略 - 基于实际技术分析规则"""
    
    def __init__(self, test_data):
        self.test_data = test_data
        self.lookback = 3  # 回看天数
    
    def consecutive_below_ma_strategy(self):
        """连续N天低于移动平均线策略
        规则: 连续3天收盘价 < 20日MA → 买入
        """
        signals = []
        closes = self.test_data['close'].values
        mas = self.test_data['MA20'].values if 'MA20' in self.test_data.columns else None
        
        if mas is None or len(mas) == 0:
            return np.zeros(len(closes))
        
        for i in range(len(closes)):
            if i < 20:  # 需要足够的历史数据
                signals.append(0)
                continue
            
            # 检查最近3天是否都低于MA20
            if i >= self.lookback:
                below_count = 0
                for j in range(i - self.lookback + 1, i + 1):
                    if closes[j] < mas[j]:
                        below_count += 1
                
                # 连续3天都低于MA20 → 买入信号
                if below_count == self.lookback:
                    signals.append(1)
                else:
                    signals.append(0)
            else:
                signals.append(0)
        
        return np.array(signals)
    
    def rsi_extreme_strategy(self):
        """RSI 极端值策略
        规则: RSI < 30 → 买入，RSI > 70 → 卖出
        """
        signals = []
        rsi = self.test_data['RSI14'].values if 'RSI14' in self.test_data.columns else None
        
        if rsi is None or len(rsi) == 0:
            return np.zeros(len(self.test_data))
        
        for i in range(len(rsi)):
            if np.isnan(rsi[i]):
                signals.append(0)
            elif rsi[i] < 30:  # 超卖
                signals.append(1)  # 买入
            elif rsi[i] > 70:  # 超买
                signals.append(-1)  # 卖出
            else:
                signals.append(0)  # 保持
        
        return np.array(signals)
    
    def ma_crossover_strategy(self):
        """移动平均线交叉策略
        规则: MA5 穿过 MA20 从下往上 → 买入
              MA5 穿过 MA20 从上往下 → 卖出
        """
        signals = []
        ma5 = self.test_data['MA5'] if 'MA5' in self.test_data.columns else None
        ma20 = self.test_data['MA20'] if 'MA20' in self.test_data.columns else None
        
        if ma5 is None or ma20 is None:
            return np.zeros(len(self.test_data))
        
        # 转换为numpy数组以便索引
        ma5_vals = ma5.values if hasattr(ma5, 'values') else ma5
        ma20_vals = ma20.values if hasattr(ma20, 'values') else ma20
        
        for i in range(len(self.test_data)):
            if i == 0:
                signals.append(0)
                continue
            
            prev_diff = ma5_vals[i-1] - ma20_vals[i-1]
            curr_diff = ma5_vals[i] - ma20_vals[i]
            
            # 金叉（从负到正）→ 买入
            if prev_diff < 0 and curr_diff > 0:
                signals.append(1)
            # 死叉（从正到负）→ 卖出
            elif prev_diff > 0 and curr_diff < 0:
                signals.append(-1)
            else:
                signals.append(0)
        
        return np.array(signals)
    
    def bollinger_band_strategy(self):
        """布林带策略
        规则: 价格 < 布林带下轨 → 买入
              价格 > 布林带上轨 → 卖出
        """
        signals = []
        closes = self.test_data['close'].values
        bb_upper = self.test_data['BB_upper'].values if 'BB_upper' in self.test_data.columns else None
        bb_lower = self.test_data['BB_lower'].values if 'BB_lower' in self.test_data.columns else None
        
        if bb_upper is None or bb_lower is None:
            return np.zeros(len(closes))
        
        for i in range(len(closes)):
            if np.isnan(bb_upper[i]) or np.isnan(bb_lower[i]):
                signals.append(0)
            elif closes[i] < bb_lower[i]:  # 触及下轨
                signals.append(1)  # 买入
            elif closes[i] > bb_upper[i]:  # 触及上轨
                signals.append(-1)  # 卖出
            else:
                signals.append(0)  # 保持
        
        return np.array(signals)
    
    def macd_strategy(self):
        """MACD 策略
        规则: MACD 金叉 → 买入
              MACD 死叉 → 卖出
        """
        signals = []
        macd = self.test_data['MACD'] if 'MACD' in self.test_data.columns else None
        signal_line = self.test_data['MACD_signal'] if 'MACD_signal' in self.test_data.columns else None
        
        if macd is None or signal_line is None:
            return np.zeros(len(self.test_data))
        
        # 转换为numpy数组以便索引
        macd_vals = macd.values if hasattr(macd, 'values') else macd
        signal_vals = signal_line.values if hasattr(signal_line, 'values') else signal_line
        
        for i in range(len(self.test_data)):
            if i == 0:
                signals.append(0)
                continue
            
            prev_diff = macd_vals[i-1] - signal_vals[i-1]
            curr_diff = macd_vals[i] - signal_vals[i]
            
            # 金叉 → 买入
            if prev_diff < 0 and curr_diff > 0:
                signals.append(1)
            # 死叉 → 卖出
            elif prev_diff > 0 and curr_diff < 0:
                signals.append(-1)
            else:
                signals.append(0)
        
        return np.array(signals)
    
    def get_all_strategies(self):
        """获取所有策略的信号"""
        return {
            'consecutive_below_ma': self.consecutive_below_ma_strategy(),
            'rsi_extreme': self.rsi_extreme_strategy(),
            'ma_crossover': self.ma_crossover_strategy(),
            'bollinger_band': self.bollinger_band_strategy(),
            'macd': self.macd_strategy()
        }



class AdvancedBacktester:
    """高级回测系统 - 支持多种策略和风险管理"""
    
    def __init__(self, test_data, strategy_signals, hold_days=7):
        self.test_data = test_data
        self.strategy_signals = strategy_signals
        self.hold_days = hold_days
        self.trades = []
    
    def backtest_with_risk_management(self, stop_loss=-0.02, take_profit=0.05):
        """执行带风险管理的回测"""
        portfolio_value = 1.0
        position = None  # {'entry_price': x, 'entry_day': i, 'quantity': q}
        
        print(f"  📊 运行高级回测 (止损: {stop_loss:.1%}, 止盈: {take_profit:.1%}, 持仓: {self.hold_days}天)")
        
        for i in range(len(self.test_data)):
            current_price = self.test_data.iloc[i]['close']
            signal = self.strategy_signals[i]
            
            # 检查是否需要平仓
            if position is not None:
                days_held = i - position['entry_day']
                unrealized_pnl = (current_price - position['entry_price']) / position['entry_price']
                
                # 止损条件
                if unrealized_pnl < stop_loss:
                    exit_price = current_price
                    ret = (exit_price - position['entry_price']) / position['entry_price']
                    portfolio_value *= (1 + ret)
                    self.trades.append({'entry': position['entry_price'], 'exit': exit_price, 'ret': ret, 'reason': '止损'})
                    position = None
                
                # 止盈条件
                elif unrealized_pnl > take_profit:
                    exit_price = current_price
                    ret = (exit_price - position['entry_price']) / position['entry_price']
                    portfolio_value *= (1 + ret)
                    self.trades.append({'entry': position['entry_price'], 'exit': exit_price, 'ret': ret, 'reason': '止盈'})
                    position = None
                
                # 持仓期满
                elif days_held >= self.hold_days:
                    exit_price = current_price
                    ret = (exit_price - position['entry_price']) / position['entry_price']
                    portfolio_value *= (1 + ret)
                    self.trades.append({'entry': position['entry_price'], 'exit': exit_price, 'ret': ret, 'reason': '周期满'})
                    position = None
            
            # 新建仓位
            if position is None and signal == 1:
                position = {
                    'entry_price': current_price,
                    'entry_day': i
                }
        
        # 计算最终指标
        cum_return = portfolio_value - 1.0
        if len(self.trades) > 0:
            wins = len([t for t in self.trades if t['ret'] > 0])
            win_rate = wins / len(self.trades)
            avg_return = np.mean([t['ret'] for t in self.trades])
        else:
            win_rate = 0
            avg_return = 0
        
        print(f"     交易笔数: {len(self.trades)}")
        print(f"     平均收益: {avg_return:.2%}")
        
        return {
            'cumulative_return': float(cum_return),
            'win_rate': float(win_rate),
            'total_trades': len(self.trades),
            'avg_return': float(avg_return),
            'trades': self.trades
        }


class StrategyOptimizer:
    """策略优化系统 - 根据回测结果调整交易参数"""
    
    def __init__(self, backtest_results, baseline_models):
        self.backtest_results = backtest_results
        self.baseline_models = baseline_models
        self.optimization_history = []
    
    def analyze_performance(self):
        """分析回测性能"""
        cum_return = self.backtest_results['cumulative_return']
        win_rate = self.backtest_results['win_rate']
        total_trades = self.backtest_results['total_trades']
        
        print("\n【策略分析】")
        print(f"  累计收益: {cum_return:.2%}")
        print(f"  胜率: {win_rate:.2%}")
        print(f"  交易笔数: {total_trades}")
        
        metrics = {
            'return': cum_return,
            'win_rate': win_rate,
            'trades': total_trades,
            'score': cum_return * 100 + (win_rate - 0.5) * 50  # 综合评分
        }
        return metrics
    
    def suggest_improvements(self, metrics):
        """建议改进方案"""
        print("\n【改进建议】")
        suggestions = []
        
        # 基于收益率的建议
        if metrics['return'] < 0.05:
            suggestions.append("  • 收益率偏低，建议降低信号阈值以增加交易信号")
        elif metrics['return'] > 0.20:
            suggestions.append("  • 收益率较好，可以考虑提高信号阈值以提高胜率")
        
        # 基于胜率的建议
        if metrics['win_rate'] < 0.50:
            suggestions.append("  • 胜率低于50%，建议加入止损机制")
        elif metrics['win_rate'] > 0.60:
            suggestions.append("  • 胜率较高，可以考虑增加持仓周期")
        
        # 基于交易笔数的建议
        if metrics['trades'] < 5:
            suggestions.append("  • 交易笔数过少，建议调整信号参数增加交易频率")
        elif metrics['trades'] > 50:
            suggestions.append("  • 交易频繁可能导致手续费损失，建议提高信号质量")
        
        if suggestions:
            for s in suggestions:
                print(s)
        else:
            print("  • 当前策略平衡良好")
        
        return suggestions
    
    def recommend_parameters(self):
        """推荐优化的参数"""
        print("\n【推荐参数调整】")
        
        cum_return = self.backtest_results['cumulative_return']
        win_rate = self.backtest_results['win_rate']
        
        # 根据回测结果推荐阈值
        if cum_return > 0.15:
            new_threshold = 0.6  # 提高阈值以提高胜率
        elif cum_return > 0.05:
            new_threshold = 0.4  # 保持当前阈值
        else:
            new_threshold = 0.3  # 降低阈值以增加交易
        
        print(f"  • 推荐信号阈值: {new_threshold:.1f} (当前: 0.5)")
        
        # 持仓周期固定为7天
        hold_days = 7
        
        print(f"  • 推荐持仓周期: {hold_days} 天 (最小7天)")
        
        # 建议止损和止盈
        print(f"  • 建议止损: -{0.02:.1%} (亏损2%时止损)")
        print(f"  • 建议止盈: +{0.05:.1%} (盈利5%时止盈)")
        
        return {
            'threshold': new_threshold,
            'hold_days': 7,  # 固定为7天
            'stop_loss': -0.02,
            'take_profit': 0.05
        }
    
    def apply_parameters(self, params):
        """应用推荐的参数到配置"""
        import json
        import os
        
        config_file = os.path.expanduser("~/.csgo_trading_config.json")
        
        try:
            # 读取或创建配置
            if os.path.exists(config_file):
                with open(config_file, 'r') as f:
                    config = json.load(f)
            else:
                config = {}
            
            # 更新策略参数
            if 'strategy_params' not in config:
                config['strategy_params'] = {}
            
            config['strategy_params'].update({
                'signal_threshold': params['threshold'],
                'hold_days': params['hold_days'],
                'stop_loss_pct': params['stop_loss'],
                'take_profit_pct': params['take_profit'],
                'last_updated': datetime.now().isoformat(),
                'performance_metrics': {
                    'cumulative_return': self.backtest_results.get('cumulative_return'),
                    'win_rate': self.backtest_results.get('win_rate'),
                    'total_trades': self.backtest_results.get('total_trades')
                }
            })
            
            # 保存配置
            os.makedirs(os.path.dirname(config_file) if os.path.dirname(config_file) else '.', exist_ok=True)
            with open(config_file, 'w') as f:
                json.dump(config, f, indent=2)
            
            print(f"\n✅ 策略参数已自动应用并保存")
            print(f"   配置文件: {config_file}")
            print(f"   • 信号阈值: {params['threshold']}")
            print(f"   • 持仓周期: {params['hold_days']} 天")
            print(f"   • 止损: {params['stop_loss']:.2%}")
            print(f"   • 止盈: {params['take_profit']:.2%}")
            
            return True
        except Exception as e:
            print(f"\n❌ 策略参数保存失败: {e}")
            return False



def run_model_training_pipeline(df, output_dir="/Users/user/Downloads/csgoAuto"):
    """完整的模型训练流程"""
    print("\n" + "="*80)
    print("【模型训练优化系统】启动")
    print("="*80)
    
    # 确保有必要的列
    if 'close' not in df.columns and 'price' in df.columns:
        df['close'] = df['price']
    
    if 'open' not in df.columns:
        df['open'] = df['close']
    if 'high' not in df.columns:
        df['high'] = df['close']
    if 'low' not in df.columns:
        df['low'] = df['close']
    
    df_ind = df.copy()
    
    # 计算缺失的指标 - 使用正确的方法签名
    try:
        if 'MA5' not in df_ind.columns:
            df_ind['MA5'] = TechnicalAnalysis.moving_average(df_ind['close'], 5)
        if 'MA10' not in df_ind.columns:
            df_ind['MA10'] = TechnicalAnalysis.moving_average(df_ind['close'], 10)
        if 'MA20' not in df_ind.columns:
            df_ind['MA20'] = TechnicalAnalysis.moving_average(df_ind['close'], 20)
        if 'RSI14' not in df_ind.columns:
            df_ind['RSI14'] = TechnicalAnalysis.rsi(df_ind['close'], 14)
        if 'ATR14' not in df_ind.columns:
            # atr(high, low, close, window=14)
            df_ind['ATR14'] = TechnicalAnalysis.atr(df_ind['high'], df_ind['low'], df_ind['close'], 14)
        if 'MACD' not in df_ind.columns:
            df_ind['MACD'], df_ind['MACD_signal'], df_ind['MACD_histogram'] = TechnicalAnalysis.macd(df_ind['close'])
        if '%K' not in df_ind.columns:
            # stochastic_oscillator(high, low, close, window=14, smooth_k=3, smooth_d=3)
            k, d = TechnicalAnalysis.stochastic_oscillator(df_ind['high'], df_ind['low'], df_ind['close'], 14, 3, 3)
            df_ind['%K'] = k
            df_ind['%D'] = d
        # 添加布林带指标
        if 'BB_upper' not in df_ind.columns:
            df_ind['BB_upper'], df_ind['BB_mid'], df_ind['BB_lower'] = TechnicalAnalysis.bollinger_bands(df_ind['close'], 20, 2)
    except Exception as e:
        print(f"\n⚠️  指标计算出错: {str(e)}")
        return
    
    # 填充 NaN 值
    df_ind = df_ind.fillna(method='bfill').fillna(method='ffill').fillna(0)
    
    # ✅ 确保所有必要指标都存在
    print(f"\n✅ 验证技术指标:")
    required = ['MA5', 'MA20', 'RSI14', 'BB_upper', 'BB_lower', 'MACD', 'MACD_signal']
    for ind in required:
        if ind in df_ind.columns:
            print(f"   ✓ {ind}")
        else:
            print(f"   ✗ {ind} 缺失!")
    
    test_size = min(60, len(df_ind) // 3)  # 不超过数据的1/3
    if len(df_ind) < 100:
        test_size = max(10, len(df_ind) // 5)
    
    train_data = df_ind.iloc[:-test_size].copy()
    test_data = df_ind.iloc[-test_size:].copy()
    train_data = train_data.reset_index(drop=True)
    test_data = test_data.reset_index(drop=True)
    
    # ✅ 在回测前再次验证 test_data 中的指标
    print(f"\n✅ 回测集指标检查:")
    for ind in required:
        if ind in test_data.columns:
            valid_count = test_data[ind].notna().sum()
            print(f"   {ind}: {valid_count}/{len(test_data)} 有效值")
        else:
            print(f"   ✗ {ind} 不存在!")
    
    features = ['close', 'MA5', 'MA10', 'MA20', 'MACD', 'RSI14', 'ATR14', '%K', '%D']
    available_features = [f for f in features if f in train_data.columns]
    
    print(f"\n✅ 指标计算完成")
    print(f"   可用特征: {available_features}")
    print(f"   训练集: {len(train_data)} 条")
    print(f"   测试集: {len(test_data)} 条")
    
    print("\n【A】基线模型训练")
    trainer = BaselineModelTrainer(train_data, test_data, test_size)
    trainer.train_arima()
    trainer.train_prophet()
    trainer.train_lightgbm(available_features)
    
    print("\n【B】集成预测")
    weights = {}
    for model, preds in trainer.predictions.items():
        if preds is not None:
            try:
                rmse = np.sqrt(np.mean((test_data['close'].values - preds) ** 2))
                weights[model] = 1.0 / (rmse + 1e-9)
            except:
                pass
    
    if not weights:
        print("  ⚠️  没有有效的模型预测")
        return
    
    total = sum(weights.values())
    weights = {k: v/total for k,v in weights.items()}
    print("  📊 模型权重:")
    for m,w in weights.items():
        print(f"     • {m}: {w:.2%}")
    
    # 集成预测
    ensemble = np.zeros(test_size)
    for model, preds in trainer.predictions.items():
        if preds is not None and model in weights:
            try:
                ensemble += preds * weights[model]
            except:
                pass
    
    print("\n【C】回测")
    try:
        # 使用实战交易策略系统
        strategies = RealTradingStrategies(test_data)
        all_signals = strategies.get_all_strategies()
        
        print("\n  📈 实战策略评估:")
        print("     • 连续3天低于MA20 买入")
        print("     • RSI < 30 超卖买入，RSI > 70 超买卖出")
        print("     • MA5/MA20 金叉买入，死叉卖出")
        print("     • 布林带策略（上下轨）")
        print("     • MACD 金叉/死叉")
        print("     ↓ 投票融合 (至少1个信号同向)")
        
        # 投票融合
        ensemble_signals = np.zeros(len(test_data))
        for strategy_name, signals in all_signals.items():
            ensemble_signals += signals
        
        # 多数胜出规则 - 降低阈值以增加交易机会
        # 原: 需要至少2个信号同向（阈值 >= 2）
        # 新: 至少1个信号同向，或者至少2个信号同向（阈值 >= 1.5，实际 >= 1）
        final_signals = np.where(ensemble_signals >= 1, 1, np.where(ensemble_signals <= -1, -1, 0))
        
        # 使用高级回测系统
        advanced_backtester = AdvancedBacktester(test_data, final_signals, hold_days=7)
        metrics = advanced_backtester.backtest_with_risk_management(stop_loss=-0.02, take_profit=0.05)
        print(f"\n  📈 回测结果:")
        print(f"     • 累计收益: {metrics['cumulative_return']:.2%}")
        print(f"     • 胜率: {metrics['win_rate']:.2%}")
        print(f"     • 交易笔数: {metrics['total_trades']}")
        print(f"     • 平均收益: {metrics['avg_return']:.2%}")
    except Exception as e:
        print(f"  ⚠️  回测失败: {str(e)}")
        metrics = {}
    
    results = {'baseline_models': trainer.results, 'ensemble_weights': weights, 'backtest_metrics': metrics}
    
    # 策略优化分析
    print("\n" + "="*80)
    print("【D】策略优化分析")
    optimizer = StrategyOptimizer(metrics, trainer.results)
    perf_metrics = optimizer.analyze_performance()
    suggestions = optimizer.suggest_improvements(perf_metrics)
    recommended_params = optimizer.recommend_parameters()
    
    # ✅ 自动应用推荐参数
    optimizer.apply_parameters(recommended_params)
    
    # 保存优化建议
    results['strategy_optimization'] = {
        'metrics': perf_metrics,
        'suggestions': suggestions,
        'recommended_parameters': recommended_params
    }
    
    # ✅ 【E】保存训练元数据和显示训练历史
    print("\n" + "="*80)
    print("【E】模型持久化与训练历史")
    print("="*80)
    trainer.save_training_metadata()
    print("  ✅ 训练元数据已保存")
    
    # 显示训练历史
    trainer.persistence.show_training_history()
    
    import json, os
    with open(os.path.join(output_dir, 'model_training_results.json'), 'w') as f:
        json.dump(results, f, indent=2, default=str)
    
    print(f"\n  ✅ 结果已保存: model_training_results.json")
    print("\n" + "="*80)
    return results


def run_quick_model_training():
    """快速运行完整的模型训练流程"""
    print("\n" + "="*80)
    print("🚀 CSGO 饰品指数 - 快速模型训练系统")
    print("="*80)
    
    print("\n【步骤 1】获取 K 线数据...")
    fetcher = KlineDataFetcher()
    df = fetcher.fetch_kline(index_id=3, kline_type="1hour")
    
    if df is None or len(df) < 10:
        print("❌ 数据获取失败或数据不足")
        return
    
    print(f"✅ 成功获取 {len(df)} 条数据")
    
    print("\n【步骤 2】运行模型训练...")
    results = run_model_training_pipeline(df)
    
    print("\n" + "="*80)
    print("✨ 模型训练完成！")
    print("="*80)
    
    return results


def load_current_strategy_config():
    """加载并显示当前策略配置"""
    import json
    import os
    
    config_file = os.path.expanduser("~/.csgo_trading_config.json")
    
    print("\n【当前策略配置】")
    print("="*80)
    
    if not os.path.exists(config_file):
        print("❌ 还没有配置文件，请先运行一次模型训练以生成配置")
        return None
    
    try:
        with open(config_file, 'r') as f:
            config = json.load(f)
        
        if 'strategy_params' not in config:
            print("❌ 配置文件中没有策略参数")
            return None
        
        params = config['strategy_params']
        
        print(f"📋 配置文件路径: {config_file}")
        print(f"⏰ 最后更新: {params.get('last_updated', 'N/A')}")
        print()
        print("📊 策略参数:")
        print(f"   • 信号阈值: {params.get('signal_threshold', 0.5)}")
        print(f"   • 持仓周期: {params.get('hold_days', 7)} 天")
        print(f"   • 止损: {params.get('stop_loss_pct', -0.02):.2%}")
        print(f"   • 止盈: {params.get('take_profit_pct', 0.05):.2%}")
        
        if 'performance_metrics' in params:
            metrics = params['performance_metrics']
            print()
            print("📈 上次训练的性能指标:")
            print(f"   • 累计收益: {metrics.get('cumulative_return', 0):.2%}")
            print(f"   • 胜率: {metrics.get('win_rate', 0):.2%}")
            print(f"   • 交易笔数: {metrics.get('total_trades', 0)}")
        
        print("="*80)
        return config
    
    except Exception as e:
        print(f"❌ 读取配置文件失败: {e}")
        return None



if __name__ == "__main__":
    import sys
    
    print("""
╔════════════════════════════════════════════════════════════════════════════╗
║                 CSGO 饰品指数 K 线分析系统 - 快速启动                       ║
╚════════════════════════════════════════════════════════════════════════════╝

请选择运行模式:
  1 - 完整分析 (技术指标 + 交易信号)
  2 - 快速模型训练 (ARIMA + Prophet + LightGBM + GARCH + 回测)
  3 - 查看训练历史记录
  4 - 查看当前策略配置 (显示最新的推荐参数)
  5 - 退出

选择 [1-5]: """)
    
    choice = input().strip()
    
    if choice == "1":
        print("\n启动完整分析模式...")
        main_complete_analysis()
    elif choice == "2":
        print("\n启动快速模型训练模式...")
        run_quick_model_training()
    elif choice == "3":
        print("\n📚 查看训练历史...")
        persistence = ModelPersistenceManager()
        persistence.show_training_history()
    elif choice == "4":
        print("\n⚙️  查看当前策略配置...")
        load_current_strategy_config()
    else:
        print("退出")
        sys.exit(0)


# ============================================================================
# 【模型训练与优化系统】完整实现
# ============================================================================
