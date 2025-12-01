#!/usr/bin/env python3
"""
CSGO市场 - 数据质量与漂移检测模块

应对CSGO市场风格高漂移的情况:
1. 版本更新导致皮肤价值变化
2. 赛事季节性影响
3. 流动性变化
4. 短期炒作导致价格异常
"""

import json
import logging
from datetime import datetime, timedelta
from pathlib import Path
from dataclasses import dataclass, asdict
from typing import Dict, List, Tuple, Optional

import numpy as np
import pandas as pd
from scipy import stats
from scipy.stats import entropy

logger = logging.getLogger(__name__)


# ============================================================================
# 数据质量检测
# ============================================================================

@dataclass
class DataQualityReport:
    """数据质量报告"""
    good_id: int
    timestamp: str

    # 完整性检查
    total_points: int
    missing_values: int
    missing_ratio: float

    # 异常值检查
    outlier_count: int
    outlier_ratio: float
    outlier_indices: List[int]

    # 统计特性
    price_mean: float
    price_std: float
    price_min: float
    price_max: float
    price_range: float
    price_cv: float  # 变异系数 (std/mean)

    # 时间序列特性
    volatility: float  # 波动率
    trend: str  # 'increasing', 'decreasing', 'stable'
    trend_strength: float

    # 异常时段检查
    suspicious_days: int
    consecutive_same: int  # 连续相同价格数

    # 质量评分 (0-100)
    quality_score: float
    quality_level: str  # 'good', 'warning', 'critical'
    warnings: List[str]


class DataQualityChecker:
    """数据质量检查器"""

    def __init__(self, outlier_method='iqr', outlier_threshold=1.5):
        """
        初始化

        Args:
            outlier_method: 'iqr' (四分位距) 或 'zscore' (z分数)
            outlier_threshold: IQR方法的倍数 (默认1.5) 或 zscore的标准差 (默认3)
        """
        self.outlier_method = outlier_method
        self.outlier_threshold = outlier_threshold

    def detect_outliers_iqr(self, prices: np.ndarray) -> Tuple[List[int], float]:
        """使用四分位距方法检测异常值"""
        Q1 = np.percentile(prices, 25)
        Q3 = np.percentile(prices, 75)
        IQR = Q3 - Q1

        lower_bound = Q1 - self.outlier_threshold * IQR
        upper_bound = Q3 + self.outlier_threshold * IQR

        outliers = np.where((prices < lower_bound) | (prices > upper_bound))[0]
        outlier_ratio = len(outliers) / len(prices)

        return list(outliers), outlier_ratio

    def detect_outliers_zscore(self, prices: np.ndarray) -> Tuple[List[int], float]:
        """使用z分数方法检测异常值"""
        z_scores = np.abs(stats.zscore(prices))
        outliers = np.where(z_scores > self.outlier_threshold)[0]
        outlier_ratio = len(outliers) / len(prices)

        return list(outliers), outlier_ratio

    def detect_outliers(self, prices: np.ndarray) -> Tuple[List[int], float]:
        """检测异常值"""
        if self.outlier_method == 'iqr':
            return self.detect_outliers_iqr(prices)
        else:
            return self.detect_outliers_zscore(prices)

    def calculate_trend(self, prices: np.ndarray) -> Tuple[str, float]:
        """计算价格趋势"""
        # 使用线性回归计算趋势
        x = np.arange(len(prices))
        z = np.polyfit(x, prices, 1)
        slope = z[0]

        # 计算R平方（趋势强度）
        p = np.poly1d(z)
        y_pred = p(x)
        ss_res = np.sum((prices - y_pred) ** 2)
        ss_tot = np.sum((prices - np.mean(prices)) ** 2)
        r_squared = 1 - (ss_res / ss_tot) if ss_tot > 0 else 0

        # 判断趋势方向
        if slope > 0.0001:  # 上升
            trend = 'increasing'
        elif slope < -0.0001:  # 下降
            trend = 'decreasing'
        else:  # 稳定
            trend = 'stable'

        return trend, abs(r_squared)

    def check_quality(self, df: pd.DataFrame, good_id: int) -> DataQualityReport:
        """
        检查数据质量

        Args:
            df: 包含 timestamp, buy_price, sell_price 的DataFrame
            good_id: 商品ID

        Returns:
            DataQualityReport
        """
        warnings = []

        # 基本统计
        total_points = len(df)
        missing_values = df.isnull().sum().sum()
        missing_ratio = missing_values / (len(df) * len(df.columns))

        # 使用sell_price作为主要价格
        prices = df['sell_price'].values

        # 异常值检测
        outlier_indices, outlier_ratio = self.detect_outliers(prices)

        # 统计特性
        price_mean = float(np.mean(prices))
        price_std = float(np.std(prices))
        price_min = float(np.min(prices))
        price_max = float(np.max(prices))
        price_range = price_max - price_min
        price_cv = float(price_std / price_mean) if price_mean > 0 else 0  # 变异系数

        # 波动率（日收益率的标准差）
        returns = np.diff(prices) / prices[:-1]
        volatility = float(np.std(returns)) if len(returns) > 0 else 0

        # 趋势
        trend, trend_strength = self.calculate_trend(prices)

        # 检查异常情况
        suspicious_days = 0
        consecutive_same = 0

        # 检查价格跳跃 (一天内变化超过10%)
        for i in range(1, len(prices)):
            change = abs(prices[i] - prices[i-1]) / prices[i-1]
            if change > 0.10:  # 10%的跳跃
                suspicious_days += 1

            # 检查连续相同价格
            if prices[i] == prices[i-1]:
                consecutive_same += 1

        # 评分逻辑
        quality_score = 100.0

        # 缺失值扣分
        if missing_ratio > 0.05:
            quality_score -= 20
            warnings.append(f"缺失值过多: {missing_ratio*100:.1f}%")

        # 异常值扣分
        if outlier_ratio > 0.1:
            quality_score -= 15
            warnings.append(f"异常值过多: {outlier_ratio*100:.1f}%")
        elif outlier_ratio > 0.05:
            quality_score -= 5
            warnings.append(f"存在少量异常值: {outlier_ratio*100:.1f}%")

        # 波动率过高扣分 (可能是市场剧烈波动)
        if volatility > 0.05:  # 日波动率 > 5%
            quality_score -= 10
            warnings.append(f"波动率过高: {volatility*100:.2f}%")

        # 连续相同价格 (可能是数据问题)
        if consecutive_same > 5:
            quality_score -= 15
            warnings.append(f"连续相同价格点: {consecutive_same}个")

        # 价格跳跃过多 (市场异常)
        if suspicious_days > total_points * 0.1:
            quality_score -= 10
            warnings.append(f"异常价格跳跃: {suspicious_days}天")

        # 数据点过少
        if total_points < 10:
            quality_score -= 20
            warnings.append(f"数据点不足: {total_points}个")
        elif total_points < 20:
            quality_score -= 10
            warnings.append(f"数据点较少: {total_points}个")

        quality_score = max(0, min(100, quality_score))

        # 质量级别
        if quality_score >= 80:
            quality_level = 'good'
        elif quality_score >= 60:
            quality_level = 'warning'
        else:
            quality_level = 'critical'

        return DataQualityReport(
            good_id=good_id,
            timestamp=datetime.now().isoformat(),
            total_points=total_points,
            missing_values=int(missing_values),
            missing_ratio=float(missing_ratio),
            outlier_count=len(outlier_indices),
            outlier_ratio=float(outlier_ratio),
            outlier_indices=outlier_indices,
            price_mean=price_mean,
            price_std=price_std,
            price_min=price_min,
            price_max=price_max,
            price_range=float(price_range),
            price_cv=price_cv,
            volatility=volatility,
            trend=trend,
            trend_strength=float(trend_strength),
            suspicious_days=suspicious_days,
            consecutive_same=consecutive_same,
            quality_score=float(quality_score),
            quality_level=quality_level,
            warnings=warnings
        )


# ============================================================================
# 数据漂移检测
# ============================================================================

@dataclass
class DataDriftReport:
    """数据漂移报告"""
    good_id: int
    timestamp: str

    # 统计漂移
    mean_shift: float
    mean_shift_pct: float
    std_shift: float
    std_shift_pct: float

    # 分布漂移
    ks_statistic: float
    ks_pvalue: float
    kl_divergence: float
    wasserstein_distance: float

    # 范围变化
    range_shift: float
    range_shift_pct: float

    # 综合漂移分数 (0-100)
    drift_score: float
    drift_level: str  # 'none', 'mild', 'moderate', 'severe'

    # 检测结果
    has_drift: bool
    drift_reason: Optional[str]


class DataDriftDetector:
    """数据漂移检测器 - 检测市场风格变化"""

    def __init__(self, recent_ratio=0.3, drift_threshold=0.5):
        """
        初始化

        Args:
            recent_ratio: 划分新旧数据的比例 (默认最近30%)为新数据
            drift_threshold: 漂移阈值 (0-1)，超过则判定为漂移
        """
        self.recent_ratio = recent_ratio
        self.drift_threshold = drift_threshold

    def detect_drift(self, prices: np.ndarray) -> DataDriftReport:
        """
        检测数据漂移

        Args:
            prices: 价格序列

        Returns:
            DataDriftReport
        """
        # 划分新旧数据
        split_idx = int(len(prices) * (1 - self.recent_ratio))
        old_prices = prices[:split_idx]
        new_prices = prices[split_idx:]

        if len(new_prices) < 3 or len(old_prices) < 3:
            return DataDriftReport(
                good_id=0,
                timestamp=datetime.now().isoformat(),
                mean_shift=0,
                mean_shift_pct=0,
                std_shift=0,
                std_shift_pct=0,
                ks_statistic=0,
                ks_pvalue=1.0,
                kl_divergence=0,
                wasserstein_distance=0,
                range_shift=0,
                range_shift_pct=0,
                drift_score=0,
                drift_level='none',
                has_drift=False,
                drift_reason=None
            )

        # 1. 均值和标准差变化
        old_mean = np.mean(old_prices)
        new_mean = np.mean(new_prices)
        old_std = np.std(old_prices)
        new_std = np.std(new_prices)

        mean_shift = abs(new_mean - old_mean)
        mean_shift_pct = mean_shift / old_mean if old_mean > 0 else 0
        std_shift = abs(new_std - old_std)
        std_shift_pct = std_shift / old_std if old_std > 0 else 0

        # 2. 分布检验 (KS检验)
        ks_statistic, ks_pvalue = stats.ks_2samp(old_prices, new_prices)

        # 3. KL散度 (分布距离)
        # 使用直方图估计概率分布
        hist_old, bin_edges = np.histogram(old_prices, bins=10, density=True)
        hist_new, _ = np.histogram(new_prices, bins=bin_edges, density=True)

        # 避免log(0)
        hist_old = hist_old + 1e-10
        hist_new = hist_new + 1e-10
        hist_old /= hist_old.sum()
        hist_new /= hist_new.sum()

        kl_divergence = float(entropy(hist_old, hist_new))

        # 4. Wasserstein距离 (最优传输距离)
        wasserstein_distance = float(stats.wasserstein_distance(old_prices, new_prices))

        # 5. 范围变化
        old_range = np.max(old_prices) - np.min(old_prices)
        new_range = np.max(new_prices) - np.min(new_prices)
        range_shift = abs(new_range - old_range)
        range_shift_pct = range_shift / old_range if old_range > 0 else 0

        # 综合漂移分数计算
        drift_score = 0
        drift_reasons = []

        # 均值变化权重 (30%)
        if mean_shift_pct > 0.10:  # 均值变化>10%
            drift_score += 30
            drift_reasons.append(f"均值变化{mean_shift_pct*100:.1f}%")
        elif mean_shift_pct > 0.05:
            drift_score += 15

        # 标准差变化权重 (20%)
        if std_shift_pct > 0.20:  # 波动率变化>20%
            drift_score += 20
            drift_reasons.append(f"波动率变化{std_shift_pct*100:.1f}%")
        elif std_shift_pct > 0.10:
            drift_score += 10

        # KS检验权重 (30%)
        if ks_pvalue < 0.01:  # p值<0.01，统计显著
            drift_score += 30
            drift_reasons.append(f"KS检验p值={ks_pvalue:.4f}(显著)")
        elif ks_pvalue < 0.05:
            drift_score += 15

        # KL散度权重 (10%)
        if kl_divergence > 0.5:  # KL散度>0.5
            drift_score += 10
            drift_reasons.append(f"KL散度={kl_divergence:.3f}")
        elif kl_divergence > 0.2:
            drift_score += 5

        # 范围变化权重 (10%)
        if range_shift_pct > 0.30:
            drift_score += 10
            drift_reasons.append(f"范围变化{range_shift_pct*100:.1f}%")

        drift_score = min(100, drift_score)

        # 判定漂移级别
        if drift_score < 20:
            drift_level = 'none'
            has_drift = False
        elif drift_score < 40:
            drift_level = 'mild'
            has_drift = drift_score >= self.drift_threshold * 100 * 0.4
        elif drift_score < 60:
            drift_level = 'moderate'
            has_drift = drift_score >= self.drift_threshold * 100 * 0.6
        else:
            drift_level = 'severe'
            has_drift = True

        drift_reason = "; ".join(drift_reasons) if drift_reasons else None

        return DataDriftReport(
            good_id=0,
            timestamp=datetime.now().isoformat(),
            mean_shift=float(mean_shift),
            mean_shift_pct=float(mean_shift_pct),
            std_shift=float(std_shift),
            std_shift_pct=float(std_shift_pct),
            ks_statistic=float(ks_statistic),
            ks_pvalue=float(ks_pvalue),
            kl_divergence=kl_divergence,
            wasserstein_distance=wasserstein_distance,
            range_shift=float(range_shift),
            range_shift_pct=float(range_shift_pct),
            drift_score=float(drift_score),
            drift_level=drift_level,
            has_drift=has_drift,
            drift_reason=drift_reason
        )


# ============================================================================
# 数据清理
# ============================================================================

class DataCleaner:
    """智能数据清理器"""

    @staticmethod
    def remove_outliers(df: pd.DataFrame, method='iqr', threshold=1.5) -> pd.DataFrame:
        """
        移除异常值

        Args:
            df: 输入DataFrame
            method: 'iqr' 或 'zscore'
            threshold: 阈值

        Returns:
            清理后的DataFrame
        """
        df_clean = df.copy()

        price_col = 'sell_price'

        if method == 'iqr':
            Q1 = df_clean[price_col].quantile(0.25)
            Q3 = df_clean[price_col].quantile(0.75)
            IQR = Q3 - Q1
            lower = Q1 - threshold * IQR
            upper = Q3 + threshold * IQR
            df_clean = df_clean[(df_clean[price_col] >= lower) & (df_clean[price_col] <= upper)]

        elif method == 'zscore':
            z_scores = np.abs(stats.zscore(df_clean[price_col]))
            df_clean = df_clean[z_scores < threshold]

        return df_clean.reset_index(drop=True)

    @staticmethod
    def fill_missing(df: pd.DataFrame, method='forward') -> pd.DataFrame:
        """
        填补缺失值

        Args:
            df: 输入DataFrame
            method: 'forward', 'backward', 'linear'

        Returns:
            填补后的DataFrame
        """
        df_clean = df.copy()

        if method == 'forward':
            df_clean = df_clean.fillna(method='ffill')
        elif method == 'backward':
            df_clean = df_clean.fillna(method='bfill')
        elif method == 'linear':
            df_clean = df_clean.interpolate(method='linear')

        df_clean = df_clean.fillna(method='bfill')  # 最后的缺失值使用后向填充

        return df_clean

    @staticmethod
    def remove_consecutive_duplicates(df: pd.DataFrame, price_col='sell_price', max_consecutive=10) -> pd.DataFrame:
        """
        移除过多的连续相同价格

        Args:
            df: 输入DataFrame
            price_col: 价格列名
            max_consecutive: 允许的最大连续相同个数（默认10个，保留更多数据）

        Returns:
            清理后的DataFrame
        """
        df_clean = df.copy()

        # 标记需要删除的行
        to_remove = []
        consecutive_count = 0
        last_price = None

        for idx, row in df_clean.iterrows():
            if row[price_col] == last_price:
                consecutive_count += 1
                if consecutive_count > max_consecutive:
                    to_remove.append(idx)
            else:
                consecutive_count = 0
                last_price = row[price_col]

        df_clean = df_clean.drop(to_remove)

        return df_clean.reset_index(drop=True)

    @staticmethod
    def clean_data(df: pd.DataFrame, remove_outliers=True, fill_missing=True,
                   remove_duplicates=True) -> Tuple[pd.DataFrame, Dict[str, int]]:
        """
        综合数据清理

        Returns:
            清理后的DataFrame 和 清理统计
        """
        stats_dict = {
            'original_count': len(df),
            'removed_outliers': 0,
            'filled_missing': 0,
            'removed_duplicates': 0
        }

        df_clean = df.copy()

        # 移除异常值
        if remove_outliers:
            original_len = len(df_clean)
            df_clean = DataCleaner.remove_outliers(df_clean)
            stats_dict['removed_outliers'] = original_len - len(df_clean)

        # 填补缺失值
        if fill_missing:
            original_len = len(df_clean)
            df_clean = DataCleaner.fill_missing(df_clean)

        # 移除连续重复
        if remove_duplicates:
            original_len = len(df_clean)
            df_clean = DataCleaner.remove_consecutive_duplicates(df_clean)
            stats_dict['removed_duplicates'] = original_len - len(df_clean)

        stats_dict['final_count'] = len(df_clean)

        return df_clean, stats_dict
