import React, { useEffect, useRef, useState } from 'react';
import { init, dispose, Chart } from 'klinecharts';
import axios from 'axios';
import { Card, Segmented, Spin, Checkbox, Row, Col, Statistic, Empty, Space, Button, Tabs } from 'antd';

interface KPoint { 
  t: number; 
  o: number; 
  h: number; 
  l: number; 
  c: number; 
  v: number;
  // Technical indicators
  ma5?: number;
  ma10?: number;
  ma20?: number;
  ma60?: number;
  ma120?: number;
  ema12?: number;
  ema26?: number;
  macd?: number;
  macd_signal?: number;
  macd_histogram?: number;
  rsi14?: number;
  bb_upper?: number;
  bb_middle?: number;
  bb_lower?: number;
  kdj_k?: number;
  kdj_d?: number;
  kdj_j?: number;
  atr14?: number;
}

interface Props {
  goodId: string | number;
  height?: number;
}

// 指标分类和配置
const INDICATOR_GROUPS = {
  MA: { label: '移动平均线', indicators: ['ma5', 'ma10', 'ma20', 'ma60', 'ma120'] },
  EMA: { label: '指数平均线', indicators: ['ema12', 'ema26'] },
  MACD: { label: 'MACD', indicators: ['macd', 'macd_signal', 'macd_histogram'] },
  RSI: { label: 'RSI', indicators: ['rsi14'] },
  BB: { label: '布林带', indicators: ['bb_upper', 'bb_middle', 'bb_lower'] },
  KDJ: { label: 'KDJ', indicators: ['kdj_k', 'kdj_d', 'kdj_j'] },
  ATR: { label: 'ATR', indicators: ['atr14'] },
};

const INDICATOR_LABELS: Record<string, string> = {
  ma5: 'MA5', ma10: 'MA10', ma20: 'MA20', ma60: 'MA60', ma120: 'MA120',
  ema12: 'EMA12', ema26: 'EMA26',
  macd: 'MACD', macd_signal: 'Signal', macd_histogram: 'Histogram',
  rsi14: 'RSI14',
  bb_upper: 'BB上', bb_middle: 'BB中', bb_lower: 'BB下',
  kdj_k: 'K值', kdj_d: 'D值', kdj_j: 'J值',
  atr14: 'ATR14',
};

const GoodKlineChart: React.FC<Props> = ({ goodId, height = 360 }) => {
  const ref = useRef<HTMLDivElement>(null);
  const chartRef = useRef<Chart | null>(null);
  const [loading, setLoading] = useState(false);
  const [interval, setInterval] = useState<'20m' | '1h' | '1d'>('1d');
  const [hasData, setHasData] = useState(false);
  const [klineData, setKlineData] = useState<KPoint[]>([]);
  const [selectedIndicators, setSelectedIndicators] = useState<string[]>(['ma5', 'ma20']);
  const [lastKline, setLastKline] = useState<KPoint | null>(null);
  const [activeTab, setActiveTab] = useState<string>('price');

  useEffect(() => {
    if (ref.current && !chartRef.current) {
      chartRef.current = init(ref.current);
      try { chartRef.current?.setBarSpace?.(8 as any); } catch {}
    }
    return () => {
      if (chartRef.current && ref.current) {
        dispose(ref.current);
        chartRef.current = null;
      }
    };
  }, []);

  // 刷新图表上的指标显示
  const applyIndicators = () => {
    if (!chartRef.current) return;
    
    // 移除所有现有指标
    try {
      const names = ['MA', 'EMA', 'MACD', 'RSI', 'BOLL', 'KDJ', 'ATR'];
      names.forEach(name => {
        while (chartRef.current?.removeIndicator?.({ name } as any)) {
          // loop until removed
        }
      });
    } catch {}

    // 根据选择添加指标
    try {
      if (selectedIndicators.some(i => i.startsWith('ma'))) {
        const maPeriods = selectedIndicators
          .filter(i => i.startsWith('ma') && i.length === 3)
          .map(i => parseInt(i.slice(2)))
          .sort((a, b) => a - b);
        
        if (maPeriods.length > 0) {
          chartRef.current?.overrideIndicator?.({ 
            name: 'MA', 
            calcParams: maPeriods 
          } as any);
          chartRef.current?.createIndicator('MA', false);
        }
      }

      if (selectedIndicators.some(i => i.startsWith('ema'))) {
        const emaPeriods = selectedIndicators
          .filter(i => i.startsWith('ema'))
          .map(i => parseInt(i.slice(3)))
          .sort((a, b) => a - b);
        
        if (emaPeriods.length > 0) {
          chartRef.current?.overrideIndicator?.({ 
            name: 'EMA', 
            calcParams: emaPeriods 
          } as any);
          chartRef.current?.createIndicator('EMA', false);
        }
      }

      if (selectedIndicators.includes('macd') || selectedIndicators.includes('macd_signal')) {
        chartRef.current?.overrideIndicator?.({ 
          name: 'MACD', 
          calcParams: [12, 26, 9] 
        } as any);
        chartRef.current?.createIndicator('MACD', false, { height: 100 } as any);
      }

      if (selectedIndicators.includes('rsi14')) {
        chartRef.current?.overrideIndicator?.({ 
          name: 'RSI', 
          calcParams: [14] 
        } as any);
        chartRef.current?.createIndicator('RSI', false, { height: 100 } as any);
      }

      if (selectedIndicators.some(i => i.startsWith('bb'))) {
        chartRef.current?.overrideIndicator?.({ 
          name: 'BOLL', 
          calcParams: [20, 2] 
        } as any);
        chartRef.current?.createIndicator('BOLL', false);
      }

      if (selectedIndicators.some(i => i.startsWith('kdj'))) {
        chartRef.current?.overrideIndicator?.({ 
          name: 'KDJ', 
          calcParams: [9, 3, 3] 
        } as any);
        chartRef.current?.createIndicator('KDJ', false, { height: 100 } as any);
      }

      if (selectedIndicators.includes('atr14')) {
        chartRef.current?.overrideIndicator?.({ 
          name: 'ATR', 
          calcParams: [14] 
        } as any);
        chartRef.current?.createIndicator('ATR', false, { height: 100 } as any);
      }
    } catch (e) {
      console.log('指标添加错误:', e);
    }
  };

  useEffect(() => {
    if (!chartRef.current || !goodId) return;
    let cancelled = false;
    (async () => {
      setLoading(true);
      try {
        // 构建indicators查询参数
        const indicatorsParam = selectedIndicators.length > 0 ? selectedIndicators.join(',') : '';
        const params: any = { id: goodId, interval, limit: 200 };
        if (indicatorsParam) {
          params.indicators = indicatorsParam;
        }
        
        const real = await axios.get('/csqaq/good/kline', { params });
        let rows: KPoint[] = real.data?.data || [];
        
        if (!rows.length) {
          const derived = await axios.get('/csqaq/good/derived_kline', { params: { id: goodId, days: 30 } });
          rows = derived.data?.data || [];
        }
        
        if (cancelled) return;
        
        setKlineData(rows);
        if (rows.length > 0) {
          setLastKline(rows[rows.length - 1]);
        }
        
        const data = rows.map((p: any) => ({ 
          timestamp: Number(p.t), 
          open: p.o, 
          high: p.h, 
          low: p.l, 
          close: p.c, 
          volume: p.v 
        }));
        
        chartRef.current!.applyNewData(data);
        setHasData(!!data.length);
        
        // 刷新图表上的指标
        setTimeout(() => applyIndicators(), 0);
      } finally { if (!cancelled) setLoading(false); }
    })();
    return () => { cancelled = true; };
  }, [goodId, interval, selectedIndicators]);

  const toggleIndicator = (indicator: string) => {
    setSelectedIndicators(prev => 
      prev.includes(indicator) 
        ? prev.filter(i => i !== indicator)
        : [...prev, indicator]
    );
  };

  const toggleGroup = (group: string) => {
    const groupIndicators = INDICATOR_GROUPS[group as keyof typeof INDICATOR_GROUPS]?.indicators || [];
    const allSelected = groupIndicators.every(i => selectedIndicators.includes(i));
    
    if (allSelected) {
      setSelectedIndicators(prev => prev.filter(i => !groupIndicators.includes(i)));
    } else {
      setSelectedIndicators(prev => {
        // Use concat and filter instead of Set to avoid TypeScript compilation issues
        const combined = prev.concat(groupIndicators);
        return combined.filter((item, index) => combined.indexOf(item) === index);
      });
    }
  };

  const formatIndicatorValue = (value: number | undefined, indicator: string) => {
    if (value === undefined || value === null) return '-';
    
    if (['rsi14', 'kdj_k', 'kdj_d', 'kdj_j'].includes(indicator)) {
      return value.toFixed(2);
    } else if (['macd', 'macd_signal', 'macd_histogram'].includes(indicator)) {
      return value.toFixed(4);
    } else {
      return value.toFixed(2);
    }
  };

  return (
    <Card 
      title="价格K线" 
      extra={
        <Segmented 
          options={[
            {label:'20分', value:'20m'}, 
            {label:'1小时', value:'1h'}, 
            {label:'1天', value:'1d'}
          ]} 
          value={interval} 
          onChange={(v)=> setInterval(v as any)} 
        />
      }
    >
      <Tabs 
        activeKey={activeTab} 
        onChange={setActiveTab}
        items={[
          {
            key: 'price',
            label: '📊 价格K线',
            children: (
              <>
                <div ref={ref} style={{ height }} />
                {loading && <Spin style={{ position:'absolute', right: 16, top: 16 }} />}
                {!hasData && !loading && <div style={{ padding: 8, color: '#999' }}>暂无数据</div>}
              </>
            )
          },
          {
            key: 'indicators',
            label: '🎯 指标管理',
            children: (
              <Card 
                type="inner" 
                title="技术指标选择" 
                size="small"
              >
                <Space direction="vertical" style={{ width: '100%' }} size="middle">
                  <div style={{ fontSize: '12px', color: '#666' }}>
                    💡 选择的指标将直接显示在K线图表上
                  </div>
                  {Object.entries(INDICATOR_GROUPS).map(([group, config]) => {
                    const groupIndicators = config.indicators;
                    const allSelected = groupIndicators.every(i => selectedIndicators.includes(i));
                    const someSelected = groupIndicators.some(i => selectedIndicators.includes(i));
                    
                    return (
                      <div key={group}>
                        <Checkbox 
                          indeterminate={someSelected && !allSelected}
                          checked={allSelected}
                          onChange={() => toggleGroup(group)}
                          style={{ fontWeight: 'bold' }}
                        >
                          {config.label}
                        </Checkbox>
                        <div style={{ marginLeft: 24, display: 'flex', flexWrap: 'wrap', gap: 8 }}>
                          {groupIndicators.map(indicator => (
                            <Checkbox 
                              key={indicator}
                              checked={selectedIndicators.includes(indicator)}
                              onChange={() => toggleIndicator(indicator)}
                            >
                              {INDICATOR_LABELS[indicator]}
                            </Checkbox>
                          ))}
                        </div>
                      </div>
                    );
                  })}
                </Space>
              </Card>
            )
          },
          {
            key: 'values',
            label: '📈 指标值',
            children: lastKline && selectedIndicators.length > 0 ? (
              <Card 
                type="inner" 
                title="最新指标值" 
                size="small"
              >
                <Row gutter={[16, 16]}>
                  <Col span={24}>
                    <Space>
                      <Statistic 
                        title="价格" 
                        value={lastKline.c} 
                        precision={2}
                      />
                      <Statistic 
                        title="最高" 
                        value={lastKline.h} 
                        precision={2}
                      />
                      <Statistic 
                        title="最低" 
                        value={lastKline.l} 
                        precision={2}
                      />
                    </Space>
                  </Col>
                  
                  {selectedIndicators.map(indicator => (
                    <Col key={indicator} xs={12} sm={8} md={6}>
                      <Statistic 
                        title={INDICATOR_LABELS[indicator]} 
                        value={formatIndicatorValue((lastKline as any)[indicator], indicator)}
                      />
                    </Col>
                  ))}
                </Row>
              </Card>
            ) : (
              <Empty description="请先选择指标" />
            )
          }
        ]}
      />
    </Card>
  );
};

export default GoodKlineChart;

