#!/usr/bin/env python3
"""
漂移告警系统
监控CSGO市场风格变化，自动触发告警和模型重训练
"""

import json
import logging
from datetime import datetime, timedelta
from pathlib import Path
from dataclasses import dataclass, asdict
from typing import Dict, List, Optional, Tuple
from enum import Enum

logger = logging.getLogger(__name__)


class AlertLevel(Enum):
    """告警级别"""
    INFO = "info"        # 信息
    WARNING = "warning"  # 警告
    CRITICAL = "critical"  # 严重


@dataclass
class Alert:
    """告警对象"""
    alert_id: str
    timestamp: str
    good_id: int
    alert_type: str  # 'data_quality', 'drift', 'performance'
    alert_level: str
    title: str
    message: str
    metrics: Dict
    recommended_action: str
    acknowledged: bool = False
    acknowledged_by: Optional[str] = None
    acknowledged_at: Optional[str] = None


class AlertRule:
    """告警规则"""

    def __init__(self, rule_id: str, condition_func, alert_level: AlertLevel, title: str, message_template: str):
        self.rule_id = rule_id
        self.condition_func = condition_func  # 返回 (True/False, message_dict)
        self.alert_level = alert_level
        self.title = title
        self.message_template = message_template

    def check(self, data: Dict) -> Optional[Tuple[bool, str]]:
        """检查规则"""
        triggered, msg_dict = self.condition_func(data)
        if triggered:
            message = self.message_template.format(**msg_dict)
            return True, message
        return False, None


class DriftAlertSystem:
    """漂移告警系统"""

    def __init__(self, alert_dir: Path = Path('.cache/alerts')):
        self.alert_dir = alert_dir
        self.alert_dir.mkdir(exist_ok=True)

        self.alerts: List[Alert] = []
        self.rules: List[AlertRule] = []

        # 初始化规则
        self._setup_rules()

    def _setup_rules(self):
        """设置告警规则"""

        # 规则1: 数据质量严重
        def rule_data_quality_critical(data):
            quality_score = data.get('quality_score', 100)
            if quality_score is not None and quality_score < 40:
                return True, {'score': quality_score, 'level': data.get('quality_level', 'unknown')}
            return False, {}

        self.add_rule(AlertRule(
            'data_quality_critical',
            rule_data_quality_critical,
            AlertLevel.CRITICAL,
            '数据质量严重下降',
            '商品 {good_id} 的数据质量评分为 {score:.0f}，等级为 {level}，建议进行数据清理或重新获取'
        ))

        # 规则2: 异常值过多
        def rule_too_many_outliers(data):
            outlier_ratio = data.get('outlier_ratio', 0)
            if outlier_ratio is not None and outlier_ratio > 0.15:
                return True, {'ratio': outlier_ratio * 100}
            return False, {}

        self.add_rule(AlertRule(
            'too_many_outliers',
            rule_too_many_outliers,
            AlertLevel.WARNING,
            '异常值过多',
            '检测到 {ratio:.1f}% 的异常值，可能存在市场异常或数据问题'
        ))

        # 规则3: 严重的数据漂移
        def rule_severe_drift(data):
            if data.get('drift_level') == 'severe':
                return True, {'drift_score': data['drift_score'], 'reason': data.get('drift_reason', 'Unknown')}
            return False, {}

        self.add_rule(AlertRule(
            'severe_drift',
            rule_severe_drift,
            AlertLevel.CRITICAL,
            '检测到严重数据漂移',
            '漂移分数 {drift_score:.0f} (原因: {reason})，强烈建议重新训练模型'
        ))

        # 规则4: 中等数据漂移
        def rule_moderate_drift(data):
            if data.get('drift_level') == 'moderate':
                return True, {'drift_score': data['drift_score']}
            return False, {}

        self.add_rule(AlertRule(
            'moderate_drift',
            rule_moderate_drift,
            AlertLevel.WARNING,
            '检测到中等数据漂移',
            '漂移分数 {drift_score:.0f}，建议监控模型性能，必要时考虑重训练'
        ))

        # 规则5: 高波动率 (市场剧烈波动)
        def rule_high_volatility(data):
            volatility = data.get('volatility', 0)
            if volatility is not None and volatility > 0.08:  # > 8%日波动率
                return True, {'volatility': volatility * 100}
            return False, {}

        self.add_rule(AlertRule(
            'high_volatility',
            rule_high_volatility,
            AlertLevel.WARNING,
            '市场波动率过高',
            '检测到 {volatility:.2f}% 的日波动率，市场可能存在重大事件'
        ))

        # 规则6: 模型准确性下降
        def rule_model_performance_drop(data):
            mape = data.get('ensemble_mape')
            if mape is not None and mape > 25:  # MAPE > 25%
                return True, {'mape': mape}
            return False, {}

        self.add_rule(AlertRule(
            'model_performance_drop',
            rule_model_performance_drop,
            AlertLevel.WARNING,
            '模型准确性明显下降',
            '集成模型MAPE为 {mape:.2f}%，超过可接受范围，需要重新训练'
        ))

        # 规则7: 连续相同价格过多 (数据问题)
        def rule_consecutive_same_price(data):
            consecutive_same = data.get('consecutive_same', 0)
            if consecutive_same is not None and consecutive_same > 10:
                return True, {'count': consecutive_same}
            return False, {}

        self.add_rule(AlertRule(
            'consecutive_same_price',
            rule_consecutive_same_price,
            AlertLevel.WARNING,
            '检测到连续相同价格',
            '存在 {count} 个连续相同价格点，可能是数据获取问题'
        ))

    def add_rule(self, rule: AlertRule):
        """添加规则"""
        self.rules.append(rule)

    def check_alerts(self, good_id: int, quality_report: Dict, drift_report: Dict,
                     performance_metrics: Dict) -> List[Alert]:
        """检查并生成告警"""
        new_alerts = []

        # 合并所有数据
        combined_data = {
            'good_id': good_id,
            **quality_report,
            **drift_report,
            **performance_metrics
        }

        # 检查所有规则
        for rule in self.rules:
            triggered, message = rule.check(combined_data)
            if triggered:
                alert = Alert(
                    alert_id=f"{good_id}_{rule.rule_id}_{datetime.now().timestamp()}",
                    timestamp=datetime.now().isoformat(),
                    good_id=good_id,
                    alert_type=rule.rule_id,
                    alert_level=rule.alert_level.value,
                    title=rule.title,
                    message=message,
                    metrics=combined_data,
                    recommended_action=self._get_recommended_action(rule.rule_id, combined_data)
                )
                new_alerts.append(alert)
                self.alerts.append(alert)

        return new_alerts

    def _get_recommended_action(self, rule_id: str, data: Dict) -> str:
        """获取推荐行动"""
        actions = {
            'data_quality_critical': '立即进行数据清理，检查数据源。如数据问题持续，考虑清空该商品的历史模型强制重训。',
            'too_many_outliers': '检查最近的市场事件，可能导致价格异常。考虑使用异常值移除进行数据清理。',
            'severe_drift': '市场风格发生重大变化。强烈建议删除该商品的历史模型 (rm .cache/models/model_{good_id}.pkl)，下次请求时将强制重训。',
            'moderate_drift': '市场风格有所变化。建议监控模型性能。若MAPE继续上升，则进行重训。',
            'high_volatility': '市场出现剧烈波动。检查是否有重大赛事、版本更新等事件。模型可能需要调整。',
            'model_performance_drop': '模型准确性下降。建议清空模型缓存强制重训 (curl -X POST http://localhost:5001/api/clear-cache)。',
            'consecutive_same_price': '检查数据获取源，可能存在通讯问题导致价格数据未更新。',
        }
        return actions.get(rule_id, '请查看具体告警信息，手动决策。')

    def save_alert(self, alert: Alert):
        """保存告警到文件"""
        alert_file = self.alert_dir / f"{alert.good_id}_{alert.alert_type}_{alert.timestamp.replace(':', '-')}.json"
        try:
            with open(alert_file, 'w') as f:
                json.dump(asdict(alert), f, indent=2, default=str)
        except Exception as e:
            logger.error(f"保存告警失败: {e}")

    def save_alerts(self, alerts: List[Alert]):
        """保存多个告警"""
        for alert in alerts:
            self.save_alert(alert)

    def get_active_alerts(self, good_id: Optional[int] = None) -> List[Alert]:
        """获取未确认的告警"""
        if good_id is None:
            return [a for a in self.alerts if not a.acknowledged]
        else:
            return [a for a in self.alerts if not a.acknowledged and a.good_id == good_id]

    def acknowledge_alert(self, alert_id: str, acknowledged_by: str) -> bool:
        """确认告警"""
        for alert in self.alerts:
            if alert.alert_id == alert_id:
                alert.acknowledged = True
                alert.acknowledged_by = acknowledged_by
                alert.acknowledged_at = datetime.now().isoformat()
                return True
        return False

    def get_alert_summary(self) -> Dict:
        """获取告警摘要"""
        total = len(self.alerts)
        unacknowledged = len([a for a in self.alerts if not a.acknowledged])
        by_level = {}
        for level in ['info', 'warning', 'critical']:
            by_level[level] = len([a for a in self.alerts if a.alert_level == level])

        return {
            'total_alerts': total,
            'unacknowledged': unacknowledged,
            'by_level': by_level,
            'recent_alerts': [asdict(a) for a in sorted(self.alerts, key=lambda x: x.timestamp, reverse=True)[:10]]
        }

    def export_alert_report(self, output_file: Path):
        """导出告警报告"""
        report = self.get_alert_summary()
        with open(output_file, 'w') as f:
            json.dump(report, f, indent=2, default=str)
        logger.info(f"告警报告已导出到 {output_file}")


class RetrainingTrigger:
    """自动重训练触发器"""

    def __init__(self, models_dir: Path = Path('.cache/models')):
        self.models_dir = models_dir

    def should_retrain(self, alerts: List[Alert]) -> Tuple[bool, Optional[str]]:
        """判断是否需要重训练"""
        for alert in alerts:
            if alert.alert_type in ['severe_drift', 'data_quality_critical']:
                return True, alert.alert_type
            if alert.alert_type == 'model_performance_drop' and alert.alert_level == 'critical':
                return True, alert.alert_type

        return False, None

    def trigger_retrain(self, good_id: int) -> bool:
        """触发重训练（删除缓存的模型）"""
        model_file = self.models_dir / f"model_{good_id}.pkl"
        try:
            if model_file.exists():
                model_file.unlink()
                logger.info(f"已删除商品 {good_id} 的缓存模型，下次请求将强制重训练")
                return True
        except Exception as e:
            logger.error(f"删除模型文件失败: {e}")
            return False

    def trigger_retrain_batch(self, good_ids: List[int]) -> Dict[int, bool]:
        """批量触发重训练"""
        results = {}
        for good_id in good_ids:
            results[good_id] = self.trigger_retrain(good_id)
        return results


# 示例: 集成到预测服务中
class AlertIntegration:
    """与预测服务的集成示例"""

    @staticmethod
    def integrate_with_predictor(quality_report, drift_report, performance_metrics):
        """
        在预测服务中集成告警系统

        使用示例:
        ```python
        # 在 prediction_service_v2.py 的 predict 函数中添加:
        from drift_alert_system import AlertIntegration

        alert_system = DriftAlertSystem()
        alerts = alert_system.check_alerts(
            good_id,
            quality_report=asdict(quality_report),
            drift_report=asdict(drift_report),
            performance_metrics=model.metrics
        )

        if alerts:
            alert_system.save_alerts(alerts)

            # 检查是否需要重训练
            trigger = RetrainingTrigger()
            should_retrain, reason = trigger.should_retrain(alerts)
            if should_retrain:
                trigger.trigger_retrain(good_id)
                logger.warning(f"商品{good_id}已触发重训练，原因: {reason}")
        ```
        """
        pass
