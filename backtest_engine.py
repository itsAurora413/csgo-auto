#!/usr/bin/env python3
"""
CSGO 饰品套利策略 - 增强回测引擎
功能：多维度评估、可视化报告、策略对比、参数优化
"""

import sys
import json
import warnings
import logging
from datetime import datetime, timedelta
from pathlib import Path
from typing import Dict, List, Tuple, Optional
import traceback

import numpy as np
import pandas as pd
import pymysql
from scipy import stats
import plotly.graph_objects as go
from plotly.subplots import make_subplots
import plotly.express as px
from flask import Flask, jsonify, request, send_file
from flask_cors import CORS

warnings.filterwarnings('ignore')

# ============================================================================
# 配置
# ============================================================================

LOG_FORMAT = '%(asctime)s - %(name)s - %(levelname)s - %(message)s'
logging.basicConfig(level=logging.INFO, format=LOG_FORMAT)
logger = logging.getLogger(__name__)

DB_CONFIG = {
    'host': '192.3.81.194',
    'user': 'root',
    'password': 'Wyj250413.',
    'database': 'csgo_trader',
    'charset': 'utf8mb4'
}

OUTPUT_DIR = Path('/root/backtest_reports')
OUTPUT_DIR.mkdir(exist_ok=True)

# Flask 应用
app = Flask(__name__)
CORS(app)

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


def fetch_historical_opportunities(days_ago: int, limit: int = 100) -> pd.DataFrame:
    """
    获取N天前的套利推荐记录
    """
    conn = get_db_connection()
    if not conn:
        return None
    
    try:
        # 计算目标日期范围
        target_date = datetime.now() - timedelta(days=days_ago)
        start_time = target_date - timedelta(hours=12)
        end_time = target_date + timedelta(hours=12)
        
        query = """
        SELECT 
            good_id, good_name, batch_id,
            current_buy_price, current_sell_price,
            profit_rate, estimated_profit,
            recommended_buy_price, recommended_quantity,
            risk_level, score,
            analysis_time, created_at
        FROM arbitrage_opportunities_history
        WHERE analysis_time >= %s AND analysis_time <= %s
        AND recommended_quantity > 0
        ORDER BY score DESC
        LIMIT %s
        """
        
        # 使用 cursor 执行查询，避免 _sqlite3 依赖
        cursor = conn.cursor(pymysql.cursors.DictCursor)
        cursor.execute(query, (start_time, end_time, limit))
        results = cursor.fetchall()
        cursor.close()
        
        df = pd.DataFrame(results)
        logger.info(f"获取到 {len(df)} 条历史推荐记录 ({start_time.date()} ~ {end_time.date()})")
        return df
        
    except Exception as e:
        logger.error(f"查询历史推荐记录失败: {e}")
        return None
    finally:
        conn.close()


def fetch_current_prices(good_ids: List[int]) -> pd.DataFrame:
    """
    获取商品当前价格
    """
    conn = get_db_connection()
    if not conn:
        return None
    
    try:
        # 获取每个商品的最新快照
        placeholders = ','.join(['%s'] * len(good_ids))
        query = f"""
        SELECT 
            good_id, 
            yyyp_buy_price, 
            yyyp_sell_price,
            yyyp_buy_count,
            yyyp_sell_count,
            created_at
        FROM (
            SELECT *,
                   ROW_NUMBER() OVER (PARTITION BY good_id ORDER BY created_at DESC) as rn
            FROM csqaq_good_snapshots
            WHERE good_id IN ({placeholders})
            AND yyyp_buy_price > 0 AND yyyp_sell_price > 0
        ) t
        WHERE rn = 1
        """
        
        # 使用 cursor 执行查询，避免 _sqlite3 依赖
        cursor = conn.cursor(pymysql.cursors.DictCursor)
        cursor.execute(query, tuple(good_ids))
        results = cursor.fetchall()
        cursor.close()
        
        df = pd.DataFrame(results)
        logger.info(f"获取到 {len(df)} 个商品的当前价格")
        return df
        
    except Exception as e:
        logger.error(f"查询当前价格失败: {e}")
        return None
    finally:
        conn.close()


def fetch_price_history(good_id: int, days: int = 30) -> pd.DataFrame:
    """
    获取商品的历史价格数据
    """
    conn = get_db_connection()
    if not conn:
        return None
    
    try:
        query = """
        SELECT 
            created_at as timestamp,
            yyyp_buy_price as buy_price,
            yyyp_sell_price as sell_price,
            yyyp_buy_count as buy_orders,
            yyyp_sell_count as sell_orders
        FROM csqaq_good_snapshots
        WHERE good_id = %s
        AND created_at >= DATE_SUB(NOW(), INTERVAL %s DAY)
        AND yyyp_buy_price > 0 AND yyyp_sell_price > 0
        ORDER BY created_at ASC
        """
        
        # 使用 cursor 执行查询，避免 _sqlite3 依赖
        cursor = conn.cursor(pymysql.cursors.DictCursor)
        cursor.execute(query, (good_id, days))
        results = cursor.fetchall()
        cursor.close()
        
        df = pd.DataFrame(results)
        return df
        
    except Exception as e:
        logger.error(f"查询商品 {good_id} 历史价格失败: {e}")
        return None
    finally:
        conn.close()


# ============================================================================
# 回测核心逻辑
# ============================================================================

class BacktestEngine:
    """回测引擎"""
    
    def __init__(self, backtest_days: int = 7, commission_rate: float = 0.01):
        """
        初始化回测引擎
        
        Args:
            backtest_days: 回测天数（N天前的推荐，看N天后的结果）
            commission_rate: 交易手续费率（默认1%，即0.99倍）
        """
        self.backtest_days = backtest_days
        self.commission_rate = commission_rate
        self.results = []
        
    def run(self, limit: int = 100) -> Dict:
        """
        执行回测
        
        Returns:
            回测结果字典
        """
        logger.info(f"=" * 80)
        logger.info(f"开始回测分析 - 回测周期: {self.backtest_days} 天")
        logger.info(f"=" * 80)
        
        # 1. 获取历史推荐
        hist_df = fetch_historical_opportunities(self.backtest_days, limit)
        if hist_df is None or len(hist_df) == 0:
            logger.error(f"未找到 {self.backtest_days} 天前的推荐数据")
            return None
        
        analysis_time = hist_df.iloc[0]['analysis_time']
        logger.info(f"原始分析时间: {analysis_time}")
        
        # 2. 获取当前价格
        good_ids = hist_df['good_id'].tolist()
        current_df = fetch_current_prices(good_ids)
        if current_df is None or len(current_df) == 0:
            logger.error("无法获取当前价格数据")
            return None
        
        # 创建价格查询字典
        current_prices = {
            row['good_id']: {
                'buy_price': row['yyyp_buy_price'],
                'sell_price': row['yyyp_sell_price'],
                'timestamp': row['created_at']
            }
            for _, row in current_df.iterrows()
        }
        
        # 3. 逐个计算回测结果
        results = []
        for _, hist in hist_df.iterrows():
            good_id = hist['good_id']
            if good_id not in current_prices:
                continue
            
            current = current_prices[good_id]
            
            # 计算预测值和实际值
            result = self._calculate_backtest_result(hist, current)
            if result:
                results.append(result)
        
        self.results = results
        logger.info(f"成功计算 {len(results)} 个商品的回测结果")
        
        # 4. 计算统计指标
        metrics = self._calculate_metrics(results, analysis_time)
        
        # 5. 生成报告数据
        report_data = {
            'backtest_config': {
                'backtest_days': self.backtest_days,
                'commission_rate': self.commission_rate,
                'analysis_time': str(analysis_time),
                'current_time': str(datetime.now()),
                'sample_count': len(results)
            },
            'metrics': metrics,
            'results': results
        }
        
        return report_data
    
    def _calculate_backtest_result(self, hist: pd.Series, current: Dict) -> Dict:
        """
        计算单个商品的回测结果
        """
        # 预测值（N天前的推荐）
        predicted_buy_price = hist['recommended_buy_price']
        predicted_sell_price = hist['current_sell_price']
        quantity = hist['recommended_quantity']
        
        # 实际值（今天的价格）
        actual_buy_price = predicted_buy_price  # 假设按推荐价格买入
        actual_sell_price = current['sell_price']
        
        # 计算利润（扣除手续费）
        predicted_profit = (predicted_sell_price * (1 - self.commission_rate) - predicted_buy_price) * quantity
        actual_profit = (actual_sell_price * (1 - self.commission_rate) - actual_buy_price) * quantity
        
        # 计算利润率
        predicted_profit_rate = (predicted_sell_price * (1 - self.commission_rate) - predicted_buy_price) / predicted_buy_price
        actual_profit_rate = (actual_sell_price * (1 - self.commission_rate) - actual_buy_price) / actual_buy_price
        
        # 价格变化率
        price_change_rate = (actual_sell_price - predicted_sell_price) / predicted_sell_price
        
        # 利润准确度
        profit_accuracy = actual_profit / predicted_profit if predicted_profit > 0 else 0
        
        return {
            'good_id': int(hist['good_id']),
            'good_name': hist['good_name'],
            'predicted_buy_price': float(predicted_buy_price),
            'predicted_sell_price': float(predicted_sell_price),
            'predicted_profit': float(predicted_profit),
            'predicted_profit_rate': float(predicted_profit_rate),
            'actual_buy_price': float(actual_buy_price),
            'actual_sell_price': float(actual_sell_price),
            'actual_profit': float(actual_profit),
            'actual_profit_rate': float(actual_profit_rate),
            'price_change_rate': float(price_change_rate),
            'profit_accuracy': float(profit_accuracy),
            'quantity': int(quantity),
            'investment': float(actual_buy_price * quantity),
            'is_successful': actual_profit > 0,
            'risk_level': hist['risk_level'],
            'score': float(hist['score']) if pd.notna(hist['score']) else 0.0
        }
    
    def _calculate_metrics(self, results: List[Dict], analysis_time) -> Dict:
        """
        计算回测统计指标
        """
        if not results:
            return {}
        
        df = pd.DataFrame(results)
        
        # 基础统计
        total_samples = len(df)
        total_investment = df['investment'].sum()
        total_predicted_profit = df['predicted_profit'].sum()
        total_actual_profit = df['actual_profit'].sum()
        
        # 成功率
        success_count = df['is_successful'].sum()
        success_rate = success_count / total_samples
        
        # ROI
        predicted_roi = total_predicted_profit / total_investment if total_investment > 0 else 0
        actual_roi = total_actual_profit / total_investment if total_investment > 0 else 0
        
        # 利润准确度
        avg_profit_accuracy = df['profit_accuracy'].mean()
        
        # 夏普比率（简化版：收益率/波动率）
        returns = df['actual_profit_rate'].values
        sharpe_ratio = np.mean(returns) / np.std(returns) if np.std(returns) > 0 else 0
        sharpe_ratio_annualized = sharpe_ratio * np.sqrt(365 / self.backtest_days)
        
        # 最大回撤
        cumulative_returns = np.cumsum(df['actual_profit'].values)
        running_max = np.maximum.accumulate(cumulative_returns)
        drawdown = (cumulative_returns - running_max)
        max_drawdown = np.min(drawdown) if len(drawdown) > 0 else 0
        max_drawdown_pct = max_drawdown / total_investment if total_investment > 0 else 0
        
        # 盈亏比（平均盈利/平均亏损）
        winning_trades = df[df['actual_profit'] > 0]
        losing_trades = df[df['actual_profit'] < 0]
        avg_win = winning_trades['actual_profit'].mean() if len(winning_trades) > 0 else 0
        avg_loss = abs(losing_trades['actual_profit'].mean()) if len(losing_trades) > 0 else 1
        profit_factor = avg_win / avg_loss if avg_loss > 0 else 0
        
        # 风险价值 VaR (95%置信度)
        var_95 = np.percentile(df['actual_profit'], 5)
        
        # 条件风险价值 CVaR (95%置信度)
        cvar_95 = df[df['actual_profit'] <= var_95]['actual_profit'].mean()
        
        # 按风险等级分组统计
        risk_stats = df.groupby('risk_level').agg({
            'actual_profit': ['sum', 'mean', 'count'],
            'is_successful': 'sum'
        }).to_dict()
        
        # 价格区间分析
        df['price_range'] = pd.cut(df['actual_buy_price'], 
                                    bins=[0, 10, 30, 50, 100, float('inf')],
                                    labels=['0-10元', '10-30元', '30-50元', '50-100元', '100元以上'])
        price_range_stats = df.groupby('price_range').agg({
            'actual_profit': ['sum', 'mean'],
            'is_successful': ['sum', 'count']
        }).to_dict()
        
        metrics = {
            '基础统计': {
                '样本数量': total_samples,
                '总投资金额': round(total_investment, 2),
                '成功交易数': int(success_count),
                '失败交易数': int(total_samples - success_count),
                '成功率': round(success_rate * 100, 2)
            },
            '收益指标': {
                '预测总利润': round(total_predicted_profit, 2),
                '实际总利润': round(total_actual_profit, 2),
                '利润差异': round(total_actual_profit - total_predicted_profit, 2),
                '利润差异百分比': round((total_actual_profit - total_predicted_profit) / total_predicted_profit * 100, 2) if total_predicted_profit != 0 else 0,
                '预测ROI': round(predicted_roi * 100, 2),
                '实际ROI': round(actual_roi * 100, 2),
                '平均利润准确度': round(avg_profit_accuracy * 100, 2)
            },
            '风险指标': {
                '夏普比率': round(sharpe_ratio, 3),
                '年化夏普比率': round(sharpe_ratio_annualized, 3),
                '最大回撤(元)': round(max_drawdown, 2),
                '最大回撤比例': round(max_drawdown_pct * 100, 2),
                '波动率': round(np.std(returns) * 100, 2),
                'VaR_95': round(var_95, 2),
                'CVaR_95': round(cvar_95, 2)
            },
            '交易指标': {
                '平均单笔盈利': round(avg_win, 2),
                '平均单笔亏损': round(avg_loss, 2),
                '盈亏比': round(profit_factor, 2),
                '最大单笔盈利': round(df['actual_profit'].max(), 2),
                '最大单笔亏损': round(df['actual_profit'].min(), 2),
                '中位数利润': round(df['actual_profit'].median(), 2)
            },
            '详细分析': {
                '按风险等级': self._format_risk_stats(df),
                '按价格区间': self._format_price_range_stats(df)
            }
        }
        
        return metrics
    
    def _format_risk_stats(self, df: pd.DataFrame) -> Dict:
        """格式化风险等级统计"""
        stats = {}
        for risk_level in ['low', 'medium', 'high']:
            subset = df[df['risk_level'] == risk_level]
            if len(subset) > 0:
                stats[risk_level] = {
                    '数量': len(subset),
                    '总利润': round(subset['actual_profit'].sum(), 2),
                    '平均利润': round(subset['actual_profit'].mean(), 2),
                    '成功率': round(subset['is_successful'].sum() / len(subset) * 100, 2)
                }
        return stats
    
    def _format_price_range_stats(self, df: pd.DataFrame) -> Dict:
        """格式化价格区间统计"""
        df['price_range'] = pd.cut(df['actual_buy_price'], 
                                    bins=[0, 10, 30, 50, 100, float('inf')],
                                    labels=['0-10元', '10-30元', '30-50元', '50-100元', '100元以上'])
        stats = {}
        for price_range in df['price_range'].unique():
            if pd.isna(price_range):
                continue
            subset = df[df['price_range'] == price_range]
            stats[str(price_range)] = {
                '数量': len(subset),
                '总利润': round(subset['actual_profit'].sum(), 2),
                '平均利润': round(subset['actual_profit'].mean(), 2),
                '成功率': round(subset['is_successful'].sum() / len(subset) * 100, 2)
            }
        return stats


# ============================================================================
# 可视化
# ============================================================================

class BacktestVisualizer:
    """回测可视化"""
    
    def __init__(self, report_data: Dict):
        self.data = report_data
        self.results_df = pd.DataFrame(report_data['results'])
        
    def generate_html_report(self, output_path: str = None) -> str:
        """
        生成完整的HTML报告
        """
        if output_path is None:
            timestamp = datetime.now().strftime('%Y%m%d_%H%M%S')
            output_path = OUTPUT_DIR / f'backtest_report_{timestamp}.html'
        
        # 创建子图
        fig = make_subplots(
            rows=3, cols=2,
            subplot_titles=(
                '累计收益曲线', '单笔利润分布',
                '预测vs实际对比', '按风险等级表现',
                '按价格区间表现', '利润准确度分布'
            ),
            specs=[
                [{'type': 'scatter'}, {'type': 'histogram'}],
                [{'type': 'scatter'}, {'type': 'bar'}],
                [{'type': 'bar'}, {'type': 'histogram'}]
            ],
            vertical_spacing=0.12,
            horizontal_spacing=0.1
        )
        
        # 1. 累计收益曲线
        cumulative_profit = np.cumsum(self.results_df['actual_profit'].values)
        fig.add_trace(
            go.Scatter(
                x=list(range(1, len(cumulative_profit) + 1)),
                y=cumulative_profit,
                mode='lines+markers',
                name='累计收益',
                line=dict(color='green', width=2),
                fill='tozeroy'
            ),
            row=1, col=1
        )
        
        # 2. 单笔利润分布
        fig.add_trace(
            go.Histogram(
                x=self.results_df['actual_profit'],
                name='利润分布',
                marker_color='steelblue',
                nbinsx=30
            ),
            row=1, col=2
        )
        
        # 3. 预测vs实际对比
        top_10 = self.results_df.nlargest(10, 'investment')
        fig.add_trace(
            go.Bar(
                x=top_10.index,
                y=top_10['predicted_profit'],
                name='预测利润',
                marker_color='lightblue'
            ),
            row=2, col=1
        )
        fig.add_trace(
            go.Bar(
                x=top_10.index,
                y=top_10['actual_profit'],
                name='实际利润',
                marker_color='orange'
            ),
            row=2, col=1
        )
        
        # 4. 按风险等级表现
        risk_stats = self.results_df.groupby('risk_level').agg({
            'actual_profit': 'sum',
            'is_successful': 'sum'
        }).reset_index()
        fig.add_trace(
            go.Bar(
                x=risk_stats['risk_level'],
                y=risk_stats['actual_profit'],
                name='按风险等级利润',
                marker_color=['green', 'yellow', 'red'],
                text=risk_stats['actual_profit'].round(2),
                textposition='auto'
            ),
            row=2, col=2
        )
        
        # 5. 按价格区间表现
        self.results_df['price_range'] = pd.cut(
            self.results_df['actual_buy_price'],
            bins=[0, 10, 30, 50, 100, float('inf')],
            labels=['0-10', '10-30', '30-50', '50-100', '100+']
        )
        price_stats = self.results_df.groupby('price_range')['actual_profit'].sum().reset_index()
        fig.add_trace(
            go.Bar(
                x=price_stats['price_range'].astype(str),
                y=price_stats['actual_profit'],
                name='按价格区间利润',
                marker_color='purple',
                text=price_stats['actual_profit'].round(2),
                textposition='auto'
            ),
            row=3, col=1
        )
        
        # 6. 利润准确度分布
        fig.add_trace(
            go.Histogram(
                x=self.results_df['profit_accuracy'],
                name='准确度分布',
                marker_color='teal',
                nbinsx=20
            ),
            row=3, col=2
        )
        
        # 更新布局
        fig.update_layout(
            title_text=f"回测报告 - {self.data['backtest_config']['backtest_days']}天周期",
            title_font_size=20,
            showlegend=True,
            height=1200,
            template='plotly_white'
        )
        
        # 更新坐标轴标签
        fig.update_xaxes(title_text="交易序号", row=1, col=1)
        fig.update_yaxes(title_text="累计收益(元)", row=1, col=1)
        
        fig.update_xaxes(title_text="利润(元)", row=1, col=2)
        fig.update_yaxes(title_text="频数", row=1, col=2)
        
        fig.update_xaxes(title_text="商品", row=2, col=1)
        fig.update_yaxes(title_text="利润(元)", row=2, col=1)
        
        fig.update_xaxes(title_text="风险等级", row=2, col=2)
        fig.update_yaxes(title_text="总利润(元)", row=2, col=2)
        
        fig.update_xaxes(title_text="价格区间(元)", row=3, col=1)
        fig.update_yaxes(title_text="总利润(元)", row=3, col=1)
        
        fig.update_xaxes(title_text="准确度", row=3, col=2)
        fig.update_yaxes(title_text="频数", row=3, col=2)
        
        # 生成完整HTML
        html_content = self._generate_full_html(fig)
        
        # 保存
        with open(output_path, 'w', encoding='utf-8') as f:
            f.write(html_content)
        
        logger.info(f"HTML报告已生成: {output_path}")
        return str(output_path)
    
    def _generate_full_html(self, fig) -> str:
        """生成完整的HTML内容"""
        metrics = self.data['metrics']
        config = self.data['backtest_config']
        
        # 图表HTML
        chart_html = fig.to_html(include_plotlyjs='cdn', div_id='charts')
        
        # 完整HTML
        html = f"""
<!DOCTYPE html>
<html lang="zh-CN">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>回测报告 - {config['backtest_days']}天周期</title>
    <style>
        * {{
            margin: 0;
            padding: 0;
            box-sizing: border-box;
        }}
        body {{
            font-family: 'Microsoft YaHei', 'SimHei', Arial, sans-serif;
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            padding: 20px;
            color: #333;
        }}
        .container {{
            max-width: 1400px;
            margin: 0 auto;
            background: white;
            border-radius: 15px;
            box-shadow: 0 10px 40px rgba(0,0,0,0.3);
            overflow: hidden;
        }}
        .header {{
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            color: white;
            padding: 30px;
            text-align: center;
        }}
        .header h1 {{
            font-size: 2.5em;
            margin-bottom: 10px;
            text-shadow: 2px 2px 4px rgba(0,0,0,0.3);
        }}
        .header p {{
            font-size: 1.1em;
            opacity: 0.9;
        }}
        .meta-info {{
            background: #f8f9fa;
            padding: 20px 30px;
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
            gap: 15px;
            border-bottom: 3px solid #667eea;
        }}
        .meta-item {{
            text-align: center;
        }}
        .meta-label {{
            font-size: 0.9em;
            color: #666;
            margin-bottom: 5px;
        }}
        .meta-value {{
            font-size: 1.3em;
            font-weight: bold;
            color: #667eea;
        }}
        .metrics-section {{
            padding: 30px;
        }}
        .metrics-grid {{
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(300px, 1fr));
            gap: 20px;
            margin-bottom: 30px;
        }}
        .metric-card {{
            background: linear-gradient(135deg, #f5f7fa 0%, #c3cfe2 100%);
            padding: 20px;
            border-radius: 10px;
            box-shadow: 0 4px 6px rgba(0,0,0,0.1);
            transition: transform 0.3s;
        }}
        .metric-card:hover {{
            transform: translateY(-5px);
            box-shadow: 0 6px 12px rgba(0,0,0,0.15);
        }}
        .metric-card h3 {{
            color: #667eea;
            margin-bottom: 15px;
            font-size: 1.3em;
            border-bottom: 2px solid #667eea;
            padding-bottom: 10px;
        }}
        .metric-item {{
            display: flex;
            justify-content: space-between;
            padding: 8px 0;
            border-bottom: 1px solid #ddd;
        }}
        .metric-item:last-child {{
            border-bottom: none;
        }}
        .metric-label {{
            color: #555;
            font-weight: 500;
        }}
        .metric-value {{
            color: #333;
            font-weight: bold;
        }}
        .metric-value.positive {{
            color: #28a745;
        }}
        .metric-value.negative {{
            color: #dc3545;
        }}
        .charts-section {{
            padding: 30px;
            background: #fafafa;
        }}
        .section-title {{
            font-size: 1.8em;
            color: #667eea;
            margin-bottom: 20px;
            text-align: center;
            font-weight: bold;
        }}
        .details-section {{
            padding: 30px;
        }}
        .details-table {{
            width: 100%;
            border-collapse: collapse;
            margin-top: 20px;
            font-size: 0.9em;
        }}
        .details-table th {{
            background: #667eea;
            color: white;
            padding: 12px;
            text-align: left;
            font-weight: bold;
        }}
        .details-table td {{
            padding: 10px;
            border-bottom: 1px solid #ddd;
        }}
        .details-table tr:hover {{
            background: #f5f5f5;
        }}
        .success-badge {{
            display: inline-block;
            padding: 4px 12px;
            border-radius: 12px;
            font-size: 0.85em;
            font-weight: bold;
        }}
        .success-badge.yes {{
            background: #d4edda;
            color: #155724;
        }}
        .success-badge.no {{
            background: #f8d7da;
            color: #721c24;
        }}
        .footer {{
            background: #2c3e50;
            color: white;
            text-align: center;
            padding: 20px;
            font-size: 0.9em;
        }}
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>🎯 CSGO饰品套利策略回测报告</h1>
            <p>数据驱动 · 科学决策 · 稳健盈利</p>
        </div>
        
        <div class="meta-info">
            <div class="meta-item">
                <div class="meta-label">回测周期</div>
                <div class="meta-value">{config['backtest_days']} 天</div>
            </div>
            <div class="meta-item">
                <div class="meta-label">分析时间</div>
                <div class="meta-value">{config['analysis_time'][:10]}</div>
            </div>
            <div class="meta-item">
                <div class="meta-label">当前时间</div>
                <div class="meta-value">{config['current_time'][:10]}</div>
            </div>
            <div class="meta-item">
                <div class="meta-label">样本数量</div>
                <div class="meta-value">{config['sample_count']}</div>
            </div>
        </div>
        
        <div class="metrics-section">
            <div class="section-title">📊 核心指标</div>
            <div class="metrics-grid">
                <div class="metric-card">
                    <h3>基础统计</h3>
                    {self._render_metric_items(metrics['基础统计'])}
                </div>
                <div class="metric-card">
                    <h3>收益指标</h3>
                    {self._render_metric_items(metrics['收益指标'])}
                </div>
                <div class="metric-card">
                    <h3>风险指标</h3>
                    {self._render_metric_items(metrics['风险指标'])}
                </div>
                <div class="metric-card">
                    <h3>交易指标</h3>
                    {self._render_metric_items(metrics['交易指标'])}
                </div>
            </div>
        </div>
        
        <div class="charts-section">
            <div class="section-title">📈 可视化分析</div>
            {chart_html}
        </div>
        
        <div class="details-section">
            <div class="section-title">📋 详细交易记录（Top 20）</div>
            {self._render_details_table()}
        </div>
        
        <div class="footer">
            <p>© 2024 CSGO饰品套利系统 | 生成时间: {datetime.now().strftime('%Y-%m-%d %H:%M:%S')}</p>
            <p>本报告由Python回测引擎自动生成 | 仅供参考，投资有风险</p>
        </div>
    </div>
</body>
</html>
"""
        return html
    
    def _render_metric_items(self, metrics: Dict) -> str:
        """渲染指标项"""
        html = ""
        for label, value in metrics.items():
            if isinstance(value, (int, float)):
                # 判断正负值
                css_class = ""
                if '利润' in label or 'ROI' in label or '准确度' in label:
                    if value > 0:
                        css_class = "positive"
                    elif value < 0:
                        css_class = "negative"
                
                # 格式化值
                if isinstance(value, float):
                    if abs(value) >= 1:
                        formatted_value = f"{value:.2f}"
                    else:
                        formatted_value = f"{value:.4f}"
                else:
                    formatted_value = str(value)
                
                # 添加单位
                if '%' not in label and 'ROI' in label:
                    formatted_value += '%'
                elif '率' in label and '%' not in formatted_value:
                    formatted_value += '%'
                elif '比率' in label or '比例' in label:
                    pass  # 不添加单位
                elif '金额' in label or '利润' in label or '投资' in label or '亏损' in label or '盈利' in label or 'VaR' in label or 'CVaR' in label:
                    formatted_value += '元'
                
                html += f"""
                <div class="metric-item">
                    <span class="metric-label">{label}</span>
                    <span class="metric-value {css_class}">{formatted_value}</span>
                </div>
                """
        return html
    
    def _render_details_table(self) -> str:
        """渲染详细表格"""
        # 取Top 20
        top_20 = self.results_df.nlargest(20, 'investment')
        
        html = """
        <table class="details-table">
            <thead>
                <tr>
                    <th>序号</th>
                    <th>商品名称</th>
                    <th>买入价</th>
                    <th>卖出价</th>
                    <th>数量</th>
                    <th>投资</th>
                    <th>预测利润</th>
                    <th>实际利润</th>
                    <th>利润率</th>
                    <th>成功</th>
                </tr>
            </thead>
            <tbody>
        """
        
        for idx, row in top_20.iterrows():
            success_class = 'yes' if row['is_successful'] else 'no'
            success_text = '✅ 是' if row['is_successful'] else '❌ 否'
            
            profit_class = 'positive' if row['actual_profit'] > 0 else 'negative'
            
            # 截断商品名称
            good_name = row['good_name']
            if len(good_name) > 40:
                good_name = good_name[:37] + '...'
            
            html += f"""
            <tr>
                <td>{idx + 1}</td>
                <td title="{row['good_name']}">{good_name}</td>
                <td>¥{row['actual_buy_price']:.2f}</td>
                <td>¥{row['actual_sell_price']:.2f}</td>
                <td>{row['quantity']}</td>
                <td>¥{row['investment']:.2f}</td>
                <td>¥{row['predicted_profit']:.2f}</td>
                <td class="metric-value {profit_class}">¥{row['actual_profit']:.2f}</td>
                <td class="metric-value {profit_class}">{row['actual_profit_rate']*100:.2f}%</td>
                <td><span class="success-badge {success_class}">{success_text}</span></td>
            </tr>
            """
        
        html += """
            </tbody>
        </table>
        """
        
        return html


# ============================================================================
# API 端点
# ============================================================================

@app.route('/api/health', methods=['GET'])
def health_check():
    """健康检查"""
    return jsonify({
        'status': 'ok',
        'service': 'backtest_engine',
        'timestamp': datetime.now().isoformat(),
        'version': '1.0.0'
    }), 200


@app.route('/api/backtest/run', methods=['POST'])
def run_backtest():
    """
    运行回测
    POST /api/backtest/run
    Body: {
        "backtest_days": 7,
        "commission_rate": 0.01,
        "limit": 100
    }
    """
    try:
        data = request.get_json() or {}
        backtest_days = data.get('backtest_days', 7)
        commission_rate = data.get('commission_rate', 0.01)
        limit = data.get('limit', 100)
        
        logger.info(f"收到回测请求: days={backtest_days}, rate={commission_rate}, limit={limit}")
        
        # 执行回测
        engine = BacktestEngine(backtest_days, commission_rate)
        report_data = engine.run(limit)
        
        if report_data is None:
            return jsonify({'error': '回测执行失败，请检查日志'}), 500
        
        # 生成可视化报告
        visualizer = BacktestVisualizer(report_data)
        html_path = visualizer.generate_html_report()
        
        # 保存JSON结果
        timestamp = datetime.now().strftime('%Y%m%d_%H%M%S')
        json_path = OUTPUT_DIR / f'backtest_result_{timestamp}.json'
        with open(json_path, 'w', encoding='utf-8') as f:
            json.dump(report_data, f, ensure_ascii=False, indent=2)
        
        return jsonify({
            'status': 'success',
            'html_report': str(html_path),
            'json_result': str(json_path),
            'summary': report_data['metrics']['基础统计'],
            'timestamp': datetime.now().isoformat()
        }), 200
        
    except Exception as e:
        logger.error(f"回测API异常: {e}")
        logger.error(traceback.format_exc())
        return jsonify({'error': str(e)}), 500


@app.route('/api/backtest/report/<filename>', methods=['GET'])
def get_report(filename):
    """
    获取回测报告文件
    GET /api/backtest/report/{filename}
    """
    try:
        file_path = OUTPUT_DIR / filename
        if not file_path.exists():
            return jsonify({'error': '文件不存在'}), 404
        
        return send_file(file_path, mimetype='text/html')
        
    except Exception as e:
        logger.error(f"获取报告异常: {e}")
        return jsonify({'error': str(e)}), 500


@app.route('/api/backtest/list', methods=['GET'])
def list_reports():
    """
    列出所有回测报告
    GET /api/backtest/list
    """
    try:
        reports = []
        for file_path in OUTPUT_DIR.glob('backtest_report_*.html'):
            reports.append({
                'filename': file_path.name,
                'timestamp': file_path.stat().st_mtime,
                'size': file_path.stat().st_size
            })
        
        # 按时间降序排序
        reports.sort(key=lambda x: x['timestamp'], reverse=True)
        
        return jsonify({
            'reports': reports,
            'total': len(reports)
        }), 200
        
    except Exception as e:
        logger.error(f"列出报告异常: {e}")
        return jsonify({'error': str(e)}), 500


# ============================================================================
# CLI 命令行接口
# ============================================================================

if __name__ == '__main__':
    import argparse
    
    parser = argparse.ArgumentParser(description='CSGO饰品套利策略回测引擎')
    parser.add_argument('--cli', action='store_true', help='使用命令行模式（默认为API服务模式）')
    parser.add_argument('-d', '--days', type=int, default=7, help='回测天数（默认7天，仅CLI模式）')
    parser.add_argument('-c', '--commission', type=float, default=0.01, help='手续费率（默认0.01，仅CLI模式）')
    parser.add_argument('-l', '--limit', type=int, default=100, help='样本数量限制（默认100，仅CLI模式）')
    parser.add_argument('--no-html', action='store_true', help='不生成HTML报告（仅CLI模式）')
    parser.add_argument('--port', type=int, default=5002, help='服务器端口（默认5002）')
    
    args = parser.parse_args()
    
    if args.cli:
        # 命令行模式
        logger.info("=" * 80)
        logger.info("回测引擎 - 命令行模式")
        logger.info("=" * 80)
        
        # 执行回测
        engine = BacktestEngine(args.days, args.commission)
        report_data = engine.run(args.limit)
        
        if report_data is None:
            logger.error("回测失败")
            sys.exit(1)
        
        # 打印摘要
        print("\n" + "=" * 80)
        print("回测摘要")
        print("=" * 80)
        for category, metrics in report_data['metrics'].items():
            if category != '详细分析':
                print(f"\n【{category}】")
                for key, value in metrics.items():
                    print(f"  {key}: {value}")
        
        # 保存JSON
        timestamp = datetime.now().strftime('%Y%m%d_%H%M%S')
        json_path = OUTPUT_DIR / f'backtest_result_{timestamp}.json'
        with open(json_path, 'w', encoding='utf-8') as f:
            json.dump(report_data, f, ensure_ascii=False, indent=2)
        logger.info(f"\nJSON结果已保存: {json_path}")
        
        # 生成HTML
        if not args.no_html:
            visualizer = BacktestVisualizer(report_data)
            html_path = visualizer.generate_html_report()
            logger.info(f"HTML报告已保存: {html_path}")
            print(f"\n✅ 回测完成！请打开浏览器查看报告: {html_path}")
    else:
        # API 服务模式（默认）
        logger.info("=" * 80)
        logger.info("CSGO 饰品回测引擎 - API 服务模式")
        logger.info("=" * 80)
        logger.info(f"监听端口: {args.port}")
        logger.info(f"数据库: {DB_CONFIG['host']}")
        logger.info(f"报告目录: {OUTPUT_DIR}")
        logger.info("=" * 80)
        logger.info("可用API端点:")
        logger.info("  GET  /api/health              - 健康检查")
        logger.info("  POST /api/backtest/run        - 执行回测")
        logger.info("  GET  /api/backtest/list       - 列出所有报告")
        logger.info("  GET  /api/backtest/report/<filename> - 获取报告文件")
        logger.info("=" * 80)
        
        app.run(host='0.0.0.0', port=args.port, debug=False, threaded=True)

