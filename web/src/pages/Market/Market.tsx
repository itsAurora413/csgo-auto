import React, { useMemo, useState } from 'react';
import { Card, Input, Row, Col, Button, Space, InputNumber, Table, Typography, message } from 'antd';
import KlineChart from '../../components/KlineChart';
import { marketApiService } from '../../services/marketApiService';

const { Text } = Typography;

const Market: React.FC = () => {
  const [indexId, setIndexId] = useState<string>('1');
  const [refreshToken, setRefreshToken] = useState<number>(0);
  const [budget, setBudget] = useState<number>(5000);
  const [minPrice, setMinPrice] = useState<number>(100);
  const [maxPrice, setMaxPrice] = useState<number>(200);
  const [minUnits, setMinUnits] = useState<number>(2);
  const [maxUnits, setMaxUnits] = useState<number>(5);
  const [planLoading, setPlanLoading] = useState<boolean>(false);
  const [planRows, setPlanRows] = useState<Array<any>>([]);
  const [planTotal, setPlanTotal] = useState<number>(0);
  const [forecastInfo, setForecastInfo] = useState<{
    currentPrice: number | null;
    forecasts: Record<number, { price: number; pct: number }>;
    minGainPct?: number;
    useLog?: boolean;
    earliestProfitDays: number | null;
    earliestProfitTarget: number | null;
    earliestProfitPct: number | null;
    bestEntryDay: number | null;
    bestEntryPrice: number | null;
  } | null>(null);
  const [maxEarliestDays, setMaxEarliestDays] = useState<number>(14);

  const estPct = useMemo(() => {
    // choose 7/14/30 closest horizon based on earliestProfitDays if available
    if (!forecastInfo) return 0;
    const { forecasts, earliestProfitDays } = forecastInfo;
    const keys = Object.keys(forecasts).map(k => parseInt(k, 10));
    if (!keys.length) return 0;
    let chosen = 7;
    if (earliestProfitDays != null) {
      const options = [7,14,30];
      chosen = options.reduce((a,b)=> Math.abs(b-earliestProfitDays!)<Math.abs(a-earliestProfitDays!)?b:a, 7);
    }
    return forecasts[chosen]?.pct ?? 0;
  }, [forecastInfo]);
  const netPct = useMemo(() => {
    if (!forecastInfo) return 0;
    const minGain = forecastInfo.minGainPct ?? 0; // 阈值+成本
    const gross = estPct;
    return gross - minGain; // 简化为扣除成本后净收益
  }, [forecastInfo, estPct]);

  const generatePlan = async () => {
    try {
      setPlanLoading(true);
      setPlanRows([]);
      setPlanTotal(0);

      // 使用现有的购买推荐API来获取适合的饰品
      const response = await fetch('/api/v1/csqaq/recommendations?limit=50&risk_level=all&type=all');
      const recommendationsData = await response.json();

      if (recommendationsData.code !== 200 || !recommendationsData.data || recommendationsData.data.length === 0) {
        message.warning('暂无推荐数据，请确保系统已收集足够的价格数据');
        return;
      }

      const recommendations = recommendationsData.data;
      const selected: Array<any> = [];
      let remaining = budget;

      for (const rec of recommendations) {
        try {
          // 使用推荐中的当前价格（YYYP价格作为参考）
          const price = rec.current_yyyp_price || rec.current_buff_price || 0;

          if (price >= minPrice && price <= maxPrice && price > 0) {
            // 计算可购买数量
            let units = Math.min(maxUnits, Math.max(minUnits, Math.floor(remaining / price)));
            if (units < minUnits) continue;

            const cost = units * price;
            if (cost > remaining) continue;

            remaining -= cost;

            selected.push({
              key: rec.good.good_id,
              name: rec.good.name || rec.good.market_hash_name,
              price: price,
              units,
              subtotal: cost,
              recommendation_type: rec.recommendation_type,
              confidence_score: rec.confidence_score,
              risk_level: rec.risk_level,
              potential_profit: rec.potential_profit,
              profit_percentage: rec.profit_percentage
            });

            // 检查是否已达到预算限制
            if (remaining < minPrice * minUnits) break;
          }
        } catch (error) {
          console.error('处理推荐数据时出错:', error);
        }
      }

      const total = selected.reduce((a,b)=>a+b.subtotal,0);
      setPlanRows(selected);
      setPlanTotal(total);

      if (!selected.length) {
        message.warning('未找到符合条件的饰品或预算不足。请尝试调整价格范围或预算设置。');
      } else {
        message.success(`成功生成购买方案：${selected.length}种饰品，总计${total.toFixed(2)}元`);
      }
    } catch (error) {
      console.error('生成购买方案失败:', error);
      message.error('生成购买方案失败，请检查网络连接或稍后重试');
    } finally {
      setPlanLoading(false);
    }
  };

  return (
    <div>
      <Row gutter={[16, 16]} style={{ marginBottom: 16 }}>
        <Col span={24}>
          <h1>市场分析 - 指数K线</h1>
        </Col>
      </Row>

      <Row gutter={[16, 16]}>
        <Col span={24}>
          <Card
            title={
              <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
                <span>指数ID：</span>
                <Input
                  style={{ width: 240 }}
                  value={indexId}
                  onChange={(e) => setIndexId(e.target.value)}
                  placeholder="请输入 CSQAQ 子指数ID，例如 1"
                />
                <Button type="primary" onClick={() => setRefreshToken((v) => v + 1)}>刷新</Button>
              </div>
            }
          >
            <KlineChart
              indexId={indexId}
              height={480}
              refreshToken={refreshToken}
              onIntervalChange={() => {}}
              onForecastUpdate={(p)=> setForecastInfo(p)}
            />
          </Card>
        </Col>
      </Row>

      <Row gutter={[16, 16]} style={{ marginTop: 16 }}>
        <Col span={24}>
          <Card title="智能购买方案（基于50-300价格区间推荐数据）">
            <Space size={12} wrap style={{ marginBottom: 8 }}>
              <span>预算</span>
              <InputNumber value={budget} onChange={(v)=> setBudget(Number(v))} min={0} step={100} />
              <span>单价范围</span>
              <InputNumber value={minPrice} onChange={(v)=> setMinPrice(Number(v))} min={0} step={10} />
              <span>~</span>
              <InputNumber value={maxPrice} onChange={(v)=> setMaxPrice(Number(v))} min={0} step={10} />
              <span>每件数量</span>
              <InputNumber value={minUnits} onChange={(v)=> setMinUnits(Number(v))} min={1} max={maxUnits} />
              <span>~</span>
              <InputNumber value={maxUnits} onChange={(v)=> setMaxUnits(Number(v))} min={minUnits} />
              <span>最早盈利上限(天)</span>
              <InputNumber value={maxEarliestDays} onChange={(v)=> setMaxEarliestDays(Number(v))} min={1} max={30} />
              <Button type="primary" onClick={generatePlan} loading={planLoading}>生成方案</Button>
              {forecastInfo && (
                <>
                  <Text type="secondary">指数预测：{estPct>0?'+':''}{estPct.toFixed(2)}%</Text>
                  <Text type="secondary" style={{ marginLeft: 8 }}>净收益(扣成本/阈值)：{netPct>0?'+':''}{netPct.toFixed(2)}%</Text>
                </>
              )}
            </Space>
            {forecastInfo && forecastInfo.earliestProfitDays != null && forecastInfo.earliestProfitDays > maxEarliestDays && (
              <div style={{ marginBottom: 8 }}>
                <Text type="danger">提示：指数预测在 {maxEarliestDays} 天内难以达到设定阈值，请谨慎入场或调整参数。</Text>
              </div>
            )}
            <Table
              size="small"
              pagination={false}
              rowKey="key"
              dataSource={planRows}
              scroll={{ x: 800 }}
              columns={[
                {
                  title: '饰品',
                  dataIndex: 'name',
                  width: 200,
                  render: (text: string, record: any) => (
                    <div>
                      <div style={{ fontWeight: 'bold' }}>{text}</div>
                      <div style={{ fontSize: '12px', color: '#999' }}>
                        ID: {record.key}
                      </div>
                    </div>
                  )
                },
                {
                  title: '推荐类型',
                  dataIndex: 'recommendation_type',
                  width: 100,
                  render: (type: string) => {
                    const typeMap: Record<string, { text: string; color: string }> = {
                      'arbitrage': { text: '套利', color: '#52c41a' },
                      'price_drop': { text: '抄底', color: '#1890ff' },
                      'undervalued': { text: '低估', color: '#faad14' },
                      'trending_up': { text: '上涨', color: '#722ed1' }
                    };
                    const config = typeMap[type] || { text: type, color: '#666' };
                    return <span style={{ color: config.color, fontSize: '12px' }}>{config.text}</span>;
                  }
                },
                {
                  title: '置信度',
                  dataIndex: 'confidence_score',
                  width: 80,
                  render: (score: number) => score ? `${(score * 100).toFixed(0)}%` : '-'
                },
                {
                  title: '风险',
                  dataIndex: 'risk_level',
                  width: 60,
                  render: (risk: string) => {
                    const colorMap: Record<string, string> = {
                      'low': '#52c41a',
                      'medium': '#faad14',
                      'high': '#ff4d4f'
                    };
                    return <span style={{ color: colorMap[risk] || '#666', fontSize: '12px' }}>
                      {risk === 'low' ? '低' : risk === 'medium' ? '中' : '高'}
                    </span>;
                  }
                },
                {
                  title: '单价',
                  dataIndex: 'price',
                  width: 80,
                  render: (v: number) => `¥${v?.toFixed?.(2) || 0}`
                },
                {
                  title: '数量',
                  dataIndex: 'units',
                  width: 60
                },
                {
                  title: '小计',
                  dataIndex: 'subtotal',
                  width: 80,
                  render: (v: number) => `¥${v?.toFixed?.(2) || 0}`
                },
                {
                  title: '预期收益%',
                  dataIndex: 'profit_percentage',
                  width: 100,
                  render: (pct: number, record: any) => {
                    if (!pct) return '-';
                    const color = pct > 0 ? '#52c41a' : '#ff4d4f';
                    return (
                      <div>
                        <div style={{ color, fontSize: '12px' }}>
                          {pct > 0 ? '+' : ''}{pct.toFixed(1)}%
                        </div>
                        {record.potential_profit && (
                          <div style={{ fontSize: '10px', color: '#999' }}>
                            ¥{(record.potential_profit * record.units).toFixed(2)}
                          </div>
                        )}
                      </div>
                    );
                  }
                },
              ]}
            />
            <div style={{ marginTop: 8 }}>
              <Text strong>合计：</Text> <Text>{planTotal.toFixed(2)} 元</Text>
              {forecastInfo && (
                <>
                  <Text style={{ marginLeft: 16 }}>
                    估算毛收益：{(planTotal * estPct / 100).toFixed(2)} 元（{estPct>0?'+':''}{estPct.toFixed(2)}%）
                  </Text>
                  <Text style={{ marginLeft: 16 }}>
                    估算净收益：{(planTotal * netPct / 100).toFixed(2)} 元（净{netPct>0?'+':''}{netPct.toFixed(2)}%）
                  </Text>
                </>
              )}
            </div>
          </Card>
        </Col>
      </Row>
    </div>
  );
};

export default Market;
