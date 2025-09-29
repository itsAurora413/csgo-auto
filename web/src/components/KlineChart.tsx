import React, { useEffect, useRef, useState, useCallback } from 'react';
import { init, dispose, Chart } from 'klinecharts';
import axios from 'axios';
import { Segmented, Spin, Typography, Space, Button, Tabs, Card, InputNumber, Select, Tooltip, Table } from 'antd';
import { QuestionCircleOutlined } from '@ant-design/icons';
import { IndexKlineResponse, IndexKlineType } from '../services/marketApiService';

const { Text } = Typography;

interface KlineChartProps {
  // 改为指数ID，以匹配 CSQAQ 指数K线接口
  indexId: string;
  height?: number;
  onIntervalChange?: (interval: string) => void;
  // 刷新触发器：父组件改变该值将强制重新拉取
  refreshToken?: number;
  onForecastUpdate?: (payload: {
    currentPrice: number | null;
    forecasts: Record<number, { price: number; pct: number }>;
    earliestProfitDays: number | null;
    earliestProfitTarget: number | null;
    earliestProfitPct: number | null;
    bestEntryDay: number | null;
    bestEntryPrice: number | null;
  }) => void;
}

const KlineChart: React.FC<KlineChartProps> = ({
  indexId,
  height = 400,
  onIntervalChange,
  refreshToken,
  onForecastUpdate
}) => {
  const chartRef = useRef<HTMLDivElement>(null);
  const chartInstance = useRef<Chart | null>(null);
  const [loading, setLoading] = useState(false);
  const [hasData, setHasData] = useState<boolean>(false);
  const [interval, setInterval] = useState<IndexKlineType>('1day');
  const [lastPrice, setLastPrice] = useState<number | null>(null);
  const [prevClose, setPrevClose] = useState<number | null>(null);
  const [lastOpen, setLastOpen] = useState<number | null>(null);
  const [lastHigh, setLastHigh] = useState<number | null>(null);
  const [lastLow, setLastLow] = useState<number | null>(null);
  const [lastVol, setLastVol] = useState<number | null>(null);
  const [lastTs, setLastTs] = useState<number | null>(null);
  const [avgVol14, setAvgVol14] = useState<number | null>(null);
  const [activeTab, setActiveTab] = useState<string>('MA');
  const [dataList, setDataList] = useState<Array<{ timestamp: number; open: number; high: number; low: number; close: number; volume: number }>>([]);
  const [forecast, setForecast] = useState<{ d7?: { price: number; pct: number }, d14?: { price: number; pct: number }, d30?: { price: number; pct: number } }>({});
  const [btMetrics, setBtMetrics] = useState<Record<string, { points: number; mape: number; mae: number; rmse: number }> | null>(null);
  const [fMethod, setFMethod] = useState<'linreg' | 'ema' | 'holt'>('linreg');
  const [fWindow, setFWindow] = useState<number>(200);
  const [fAlpha, setFAlpha] = useState<number>(0.3);
  const [fBeta, setFBeta] = useState<number>(0.1);
  const [fUseLog, setFUseLog] = useState<boolean>(true);
  const [fThreshPct, setFThreshPct] = useState<number>(0);
  const [fCostPct, setFCostPct] = useState<number>(0);
  const [earliestProfitDays, setEarliestProfitDays] = useState<number | null>(null);
  const [earliestProfitTarget, setEarliestProfitTarget] = useState<number | null>(null);
  const [earliestProfitPct, setEarliestProfitPct] = useState<number | null>(null);
  const [bestEntryDay, setBestEntryDay] = useState<number | null>(null);
  const [bestEntryDate, setBestEntryDate] = useState<string>('');
  const [bestEntryPrice, setBestEntryPrice] = useState<number | null>(null);
  const [curve, setCurve] = useState<Array<{ day: number; pct: number; netPct: number; hit: boolean }>>([]);
  // help moved to Help Center page

  // Initialize chart then load data once mounted
  useEffect(() => {
    if (chartRef.current && !chartInstance.current) {
      chartInstance.current = init(chartRef.current);

      // Apply light theme styles for cleaner visuals
      try {
        chartInstance.current.setStyles({
          grid: {
            show: true,
            horizontal: { show: true, style: 'solid', size: 1, color: '#eceff3', dashedValue: [3, 3] },
            vertical: { show: true, style: 'solid', size: 1, color: '#eceff3', dashedValue: [3, 3] }
          },
          candle: {
            type: 'candle_solid',
            bar: {
              compareRule: 'previous_close',
              upColor: '#16a34a',
              downColor: '#ef4444',
              noChangeColor: '#64748b',
              upBorderColor: '#16a34a',
              downBorderColor: '#ef4444',
              noChangeBorderColor: '#64748b',
              upWickColor: '#16a34a',
              downWickColor: '#ef4444',
              noChangeWickColor: '#64748b'
            }
          },
          xAxis: {
            show: true,
            size: 'auto',
            axisLine: { show: false, style: 'solid', size: 1, color: '#e2e8f0', dashedValue: [0, 0] },
            tickLine: { show: false, style: 'solid', size: 1, color: '#e2e8f0', dashedValue: [0, 0], length: 3 },
            tickText: { show: true, color: '#475569', weight: 400, family: 'Inter, system-ui, -apple-system, Segoe UI, Roboto, Helvetica, Arial', size: 10, marginStart: 0, marginEnd: 0 }
          },
          yAxis: {
            show: true,
            size: 'auto',
            axisLine: { show: false, style: 'solid', size: 1, color: '#e2e8f0', dashedValue: [0, 0] },
            tickLine: { show: false, style: 'solid', size: 1, color: '#e2e8f0', dashedValue: [0, 0], length: 3 },
            tickText: { show: true, color: '#475569', weight: 400, family: 'Inter, system-ui, -apple-system, Segoe UI, Roboto, Helvetica, Arial', size: 10, marginStart: 0, marginEnd: 0 }
          },
          crosshair: {
            show: true,
            horizontal: {
              show: true,
              line: { show: true, style: 'dashed', size: 1, color: '#94a3b8', dashedValue: [2, 3] },
              text: {
                show: true,
                color: '#0f172a',
                size: 10,
                family: 'Inter, system-ui, -apple-system, Segoe UI, Roboto, Helvetica, Arial',
                weight: 500,
                paddingLeft: 6, paddingRight: 6, paddingTop: 2, paddingBottom: 2,
                borderRadius: 4,
                backgroundColor: '#f8fafc',
                borderStyle: 'solid', borderDashedValue: [0, 0], borderSize: 1, borderColor: '#e2e8f0'
              }
            },
            vertical: {
              show: true,
              line: { show: true, style: 'dashed', size: 1, color: '#94a3b8', dashedValue: [2, 3] },
              text: {
                show: true,
                color: '#0f172a',
                size: 10,
                family: 'Inter, system-ui, -apple-system, Segoe UI, Roboto, Helvetica, Arial',
                weight: 500,
                paddingLeft: 6, paddingRight: 6, paddingTop: 2, paddingBottom: 2,
                borderRadius: 4,
                backgroundColor: '#f8fafc',
                borderStyle: 'solid', borderDashedValue: [0, 0], borderSize: 1, borderColor: '#e2e8f0'
              }
            }
          },
          separator: { size: 1, color: '#e2e8f0', fill: false, activeBackgroundColor: '#e5e7eb' }
        } as any);

        // Default indicators: none at init; will attach based on active tab below
      } catch (e) {
        // ignore style application errors to avoid breaking chart init
      }

      // Handle responsive resizing
      const handleResize = () => {
        if (chartInstance.current) {
          chartInstance.current.resize();
        }
      };
      window.addEventListener('resize', handleResize);
      // Slightly tighter bars for a modern look
      try { chartInstance.current?.setBarSpace?.(8 as any); } catch {}
      
      return () => {
        window.removeEventListener('resize', handleResize);
      };
    }

    return () => {
      if (chartInstance.current && chartRef.current) {
        const chartElement = chartRef.current;
        dispose(chartElement);
        chartInstance.current = null;
      }
    };
  }, []);

  // First load after init and on changes
  useEffect(() => {
    if (indexId && chartInstance.current) {
      loadKlineData(interval);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [indexId, interval]);

  // Load kline data
  const loadKlineData = useCallback(async (selectedInterval: string) => {
    if (!chartInstance.current) return;

    setLoading(true);

    try {
      // Import marketApiService dynamically to avoid circular imports
      const { marketApiService } = await import('../services/marketApiService');
      const resp: IndexKlineResponse = await marketApiService.getKlineDataByIndex(indexId, selectedInterval as IndexKlineType);

      const points = Array.isArray(resp?.data) ? resp.data : [];
      if (!points.length) {
        setHasData(false);
        chartInstance.current.applyNewData([]);
        return;
      }

      const chartData = points.map(p => ({
        timestamp: Number(p.t),
        open: p.o,
        high: p.h,
        low: p.l,
        close: p.c,
        volume: p.v
      }));

      // Clear existing data and add new data
      chartInstance.current.applyNewData(chartData);
      chartInstance.current.scrollToRealTime?.(0);
      setDataList(chartData);
      setHasData(true);

      // Update header metrics
      const len = chartData.length;
      if (len >= 2) {
        const last = chartData[len - 1];
        const prev = chartData[len - 2];
        setLastPrice(last.close);
        setPrevClose(prev.close);
        setLastOpen(last.open);
        setLastHigh(last.high);
        setLastLow(last.low);
        setLastVol(last.volume);
        setLastTs(last.timestamp);
        const start = Math.max(0, len - 14);
        const vols = chartData.slice(start, len).map(d => d.volume);
        setAvgVol14(vols.length ? vols.reduce((a, b) => a + b, 0) / vols.length : null);
      } else if (len === 1) {
        const last = chartData[0];
        setLastPrice(last.close);
        setPrevClose(last.close);
        setLastOpen(last.open);
        setLastHigh(last.high);
        setLastLow(last.low);
        setLastVol(last.volume);
        setLastTs(last.timestamp);
        setAvgVol14(last.volume);
      } else {
        setLastPrice(null);
        setPrevClose(null);
        setLastOpen(null);
        setLastHigh(null);
        setLastLow(null);
        setLastVol(null);
        setLastTs(null);
        setAvgVol14(null);
      }

      // Update forecast from backend strategy
      try {
        const EXT_HORIZONS = Array.from({ length: 30 }, (_, i) => i + 1);
        const res = await axios.post('/forecast/run', { id: indexId, type: interval, horizons: EXT_HORIZONS, method: fMethod, window: fWindow, params: { alpha: fAlpha, beta: fBeta, use_log: fUseLog ? 1 : 0 }, data: chartData.map(d => ({ t: String(d.timestamp), o: d.open, h: d.high, l: d.low, c: d.close, v: d.volume })) }, { baseURL: '/api/v1' });
        const preds = res.data?.predictions || {};
        const cur = chartData.length ? chartData[chartData.length - 1].close : null;
        const pVal = (d: number) => (preds[d] ?? preds[String(d)]);
        const build = (d: number) => {
          const p = pVal(d);
          if (p == null || cur == null) return undefined;
          const pct = cur ? ((p - cur) / cur) * 100 : 0;
          return { price: p, pct };
        };
        const forecastsObj: Record<number, { price: number; pct: number }> = {};
        [7,14,30].forEach((d) => { const v = build(d); if (v) forecastsObj[d] = v; });
        setForecast({ d7: forecastsObj[7], d14: forecastsObj[14], d30: forecastsObj[30] });
        let earliest: number | null = null;
        let earliestTarget: number | null = null;
        let earliestPct: number | null = null;
        let minDay: number | null = null;
        let minPrice: number | null = null;
        const minGain = fThreshPct + fCostPct;
        const daily: Array<{day:number;pct:number;netPct:number;hit:boolean}> = [];
        if (cur != null) {
          for (const d of EXT_HORIZONS) {
            const p = pVal(d);
            if (typeof p === 'number') {
              if (minPrice == null || p < minPrice) { minPrice = p; minDay = d; }
              const pct = ((p - cur) / cur) * 100;
              const net = pct - (fCostPct);
              const hit = pct >= minGain;
              daily.push({ day: d, pct, netPct: net, hit });
              if (earliest == null && pct >= minGain) {
                earliest = d; earliestTarget = p; earliestPct = pct;
              }
            }
          }
        }
        setEarliestProfitDays(earliest);
        setEarliestProfitTarget(earliestTarget);
        setEarliestProfitPct(earliestPct);
        setBestEntryDay(minDay);
        if (lastTs && minDay != null) {
          const dt = new Date(Number(lastTs) + minDay * 24 * 60 * 60 * 1000);
          setBestEntryDate(dt.toLocaleDateString());
        } else {
          setBestEntryDate('');
        }
        setBestEntryPrice(minPrice ?? null);
        setCurve(daily);

        onForecastUpdate && onForecastUpdate({
          currentPrice: cur,
          forecasts: forecastsObj,
          // @ts-ignore
          minGainPct: minGain,
          // @ts-ignore
          useLog: fUseLog,
          earliestProfitDays: earliest,
          earliestProfitTarget: earliestTarget,
          earliestProfitPct: earliestPct,
          bestEntryDay: minDay,
          bestEntryPrice: minPrice ?? null,
        });
      } catch {}

      // Fetch backtest metrics (1..30 days)
      try {
        const EXT_HORIZONS = Array.from({ length: 30 }, (_, i) => i + 1);
        const res = await axios.post('/forecast/backtest', { id: indexId, type: interval, horizons: EXT_HORIZONS, method: fMethod, window: fWindow, step: 5, params: { alpha: fAlpha, beta: fBeta, use_log: fUseLog ? 1 : 0 }, data: chartData.map(d => ({ t: String(d.timestamp), o: d.open, h: d.high, l: d.low, c: d.close, v: d.volume })) }, { baseURL: '/api/v1' });
        setBtMetrics(res.data?.metrics || null);
      } catch {}

    } catch (err: any) {
      console.error('K线数据加载错误:', err);
      setHasData(false);
    } finally {
      setLoading(false);
    }
  }, [indexId]);

  // Fetch forecasts only when settings change
  useEffect(() => {
    const run = async () => {
      if (!chartInstance.current || !dataList.length) return;
      try {
        const EXT_HORIZONS = Array.from({ length: 30 }, (_, i) => i + 1);
        const res = await axios.post('/forecast/run', { id: indexId, type: interval, horizons: EXT_HORIZONS, method: fMethod, window: fWindow, params: { alpha: fAlpha, beta: fBeta, use_log: fUseLog ? 1 : 0 }, data: dataList.map(d => ({ t: String(d.timestamp), o: d.open, h: d.high, l: d.low, c: d.close, v: d.volume })) }, { baseURL: '/api/v1' });
        const preds = res.data?.predictions || {};
        const cur = dataList[dataList.length - 1]?.close ?? null;
        const pVal = (d: number) => (preds[d] ?? preds[String(d)]);
        const build = (d: number) => {
          const p = pVal(d);
          if (p == null || cur == null) return undefined;
          const pct = cur ? ((p - cur) / cur) * 100 : 0;
          return { price: p, pct };
        };
        const forecastsObj: Record<number, { price: number; pct: number }> = {};
        [7,14,30].forEach((d) => { const v = build(d); if (v) forecastsObj[d] = v; });
        setForecast({ d7: forecastsObj[7], d14: forecastsObj[14], d30: forecastsObj[30] });
        let earliest: number | null = null;
        let earliestTarget: number | null = null;
        let earliestPct: number | null = null;
        let minDay: number | null = null;
        let minPrice: number | null = null;
        const minGain = fThreshPct + fCostPct;
        const daily: Array<{day:number;pct:number;netPct:number;hit:boolean}> = [];
        if (cur != null) {
          for (const d of EXT_HORIZONS) {
            const p = pVal(d);
            if (typeof p === 'number') {
              if (minPrice == null || p < minPrice) { minPrice = p; minDay = d; }
              const pct = ((p - cur) / cur) * 100;
              const net = pct - (fCostPct);
              const hit = pct >= minGain;
              daily.push({ day: d, pct, netPct: net, hit });
              if (earliest == null && pct >= minGain) {
                earliest = d; earliestTarget = p; earliestPct = pct;
              }
            }
          }
        }
        setEarliestProfitDays(earliest);
        setEarliestProfitTarget(earliestTarget);
        setEarliestProfitPct(earliestPct);
        setBestEntryDay(minDay);
        if (lastTs && minDay != null) {
          const dt = new Date(Number(lastTs) + minDay * 24 * 60 * 60 * 1000);
          setBestEntryDate(dt.toLocaleDateString());
        } else {
          setBestEntryDate('');
        }
        setBestEntryPrice(minPrice ?? null);
        setCurve(daily);

        onForecastUpdate && onForecastUpdate({
          currentPrice: cur,
          forecasts: forecastsObj,
          // @ts-ignore
          minGainPct: minGain,
          // @ts-ignore
          useLog: fUseLog,
          earliestProfitDays: earliest,
          earliestProfitTarget: earliestTarget,
          earliestProfitPct: earliestPct,
          bestEntryDay: minDay,
          bestEntryPrice: minPrice ?? null,
        });
      } catch {}
      try {
        const EXT_HORIZONS = Array.from({ length: 30 }, (_, i) => i + 1);
        const res = await axios.post('/forecast/backtest', { id: indexId, type: interval, horizons: EXT_HORIZONS, method: fMethod, window: fWindow, step: 5, params: { alpha: fAlpha, beta: fBeta, use_log: fUseLog ? 1 : 0 }, data: dataList.map(d => ({ t: String(d.timestamp), o: d.open, h: d.high, l: d.low, c: d.close, v: d.volume })) }, { baseURL: '/api/v1' });
        setBtMetrics(res.data?.metrics || null);
      } catch {}
    };
    run();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [fMethod, fWindow, fAlpha, fBeta]);

  // Manual refresh support
  useEffect(() => {
    if (indexId && chartInstance.current) {
      loadKlineData(interval);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [refreshToken]);

  const handleIntervalChange = (newInterval: IndexKlineType) => {
    setInterval(newInterval);
    if (onIntervalChange) {
      onIntervalChange(newInterval);
    }
  };

  // Compute change
  const changeAbs = lastPrice != null && prevClose != null ? lastPrice - prevClose : null;
  const changePct = changeAbs != null && prevClose ? (changeAbs / prevClose) * 100 : null;
  const up = (changeAbs ?? 0) > 0;
  const down = (changeAbs ?? 0) < 0;
  const formatNum = (n: number | null, digits = 2) => (n == null ? '--' : n.toFixed(digits));
  const formatTS = (ts: number | null) => (ts == null ? '' : new Date(ts).toLocaleString());

  // Remove all known indicators
  const removeAllIndicators = () => {
    if (!chartInstance.current) return;
    const names = ['MA', 'BOLL', 'MACD', 'VOL'];
    for (const name of names) {
      // keep removing until none left
      // @ts-ignore
      while (chartInstance.current.removeIndicator?.({ name })) {
        // loop
      }
    }
  };

  const applyIndicatorByTab = (tab: string) => {
    if (!chartInstance.current) return;
    removeAllIndicators();
    try {
      switch (tab) {
        case 'MA':
          chartInstance.current.overrideIndicator?.({ name: 'MA', calcParams: [5, 10, 20] } as any);
          chartInstance.current.createIndicator('MA', false);
          break;
        case 'BOLL':
          chartInstance.current.createIndicator('BOLL', false as any);
          break;
        case 'MACD':
          chartInstance.current.overrideIndicator?.({ name: 'MACD', calcParams: [12, 26, 9] } as any);
          chartInstance.current.createIndicator('MACD', false, { height: 120 } as any);
          break;
        case 'VOL':
          chartInstance.current.overrideIndicator?.({ name: 'VOL', calcParams: [5, 10] } as any);
          chartInstance.current.createIndicator('VOL', false, { height: 96 } as any);
          break;
        default:
          break;
      }
    } catch {}
  };

  useEffect(() => {
    applyIndicatorByTab(activeTab);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [activeTab, chartInstance.current]);


  return (
    <div style={{ position: 'relative' }}>
      <div style={{
        marginBottom: 12,
        display: 'flex',
        justifyContent: 'space-between',
        alignItems: 'center'
      }}>
        <Space size={12} wrap>
          <Text strong style={{ color: '#334155' }}>{`指数ID: ${indexId}`}</Text>
          <Segmented
            size="small"
            value={interval}
            options={[
              { label: '1小时', value: '1hour' },
              { label: '4小时', value: '4hour' },
              { label: '日线', value: '1day' },
              { label: '周线', value: '7day' }
            ]}
            onChange={(val) => handleIntervalChange(val as IndexKlineType)}
          />
        </Space>
        <Space size={10} wrap>
          {lastPrice != null && (
            <Text style={{ color: up ? '#2ECB7A' : down ? '#F64E4E' : '#cbd5e1', fontWeight: 600 }}>
              {formatNum(lastPrice)} {changeAbs != null && changePct != null && ((up ? '+' : '') + `${formatNum(changeAbs)} (${formatNum(changePct)}%)`)}
            </Text>
          )}
          <Text type="secondary" style={{ color: '#475569' }}>O {formatNum(lastOpen)}</Text>
          <Text type="secondary" style={{ color: '#475569' }}>H {formatNum(lastHigh)}</Text>
          <Text type="secondary" style={{ color: '#475569' }}>L {formatNum(lastLow)}</Text>
          <Text type="secondary" style={{ color: '#475569' }}>C {formatNum(lastPrice)}</Text>
          <Text type="secondary" style={{ color: '#64748b' }}>量 {formatNum(lastVol, 0)}{avgVol14 != null ? ` / 均量14 ${formatNum(avgVol14, 0)}` : ''}</Text>
          <Text type="secondary" style={{ color: '#94a3b8' }}>{formatTS(lastTs)}</Text>
          
          {loading && <Spin size="small" />}
        </Space>
      </div>

      {/* Forecast summary */}
      <div style={{ marginTop: 12 }}>
        <Card size="small" bordered style={{ borderRadius: 8 }}>
          <Space size={24} wrap>
            <div>
              <Text strong>7天预测</Text>
              <div>
                {forecast.d7 ? (
                  <Text style={{ color: (forecast.d7.pct > 0 ? '#16a34a' : forecast.d7.pct < 0 ? '#ef4444' : '#334155') }}>
                    {formatNum(forecast.d7.price)} ({formatNum(forecast.d7.pct)}%)
                  </Text>
                ) : <Text type="secondary">—</Text>}
              </div>
            </div>
            <div>
              <Text strong>14天预测</Text>
              <div>
                {forecast.d14 ? (
                  <Text style={{ color: (forecast.d14.pct > 0 ? '#16a34a' : forecast.d14.pct < 0 ? '#ef4444' : '#334155') }}>
                    {formatNum(forecast.d14.price)} ({formatNum(forecast.d14.pct)}%)
                  </Text>
                ) : <Text type="secondary">—</Text>}
              </div>
            </div>
            <div>
              <Text strong>30天预测</Text>
              <div>
                {forecast.d30 ? (
                  <Text style={{ color: (forecast.d30.pct > 0 ? '#16a34a' : forecast.d30.pct < 0 ? '#ef4444' : '#334155') }}>
                    {formatNum(forecast.d30.price)} ({formatNum(forecast.d30.pct)}%)
                  </Text>
                ) : <Text type="secondary">—</Text>}
              </div>
            </div>
            <div>
              <Text strong>最早盈利天数</Text>
              <div>
                {earliestProfitDays != null ? (
                  <>
                    <Text style={{ color: '#334155' }}>{earliestProfitDays} 天</Text>
                    {earliestProfitTarget != null && earliestProfitPct != null && (
                      <Text style={{ marginLeft: 8, color: earliestProfitPct > 0 ? '#16a34a' : earliestProfitPct < 0 ? '#ef4444' : '#334155' }}>
                        目标价 {formatNum(earliestProfitTarget)} ({formatNum(earliestProfitPct)}%)
                      </Text>
                    )}
                  </>
                ) : (
                  <Text type="secondary">暂无</Text>
                )}
              </div>
            </div>
            <div>
              <Text strong>最佳入手日期</Text>
              <div>
                {bestEntryDay != null && bestEntryPrice != null ? (
                  <>
                    <Text style={{ color: '#334155' }}>{bestEntryDate || `${bestEntryDay} 天后`}</Text>
                    <Text style={{ marginLeft: 8, color: '#334155' }}>预测价 {formatNum(bestEntryPrice)}</Text>
                  </>
                ) : (
                  <Text type="secondary">暂无</Text>
                )}
              </div>
            </div>
            
          </Space>
        </Card>
      </div>

      {/* Cumulative returns view */}
      <div style={{ marginTop: 12 }}>
        <Card size="small" bordered style={{ borderRadius: 8 }} title="累计收益（1~30天）">
          {curve.length ? (
            <Table
              size="small"
              pagination={false}
              rowKey={(r:any)=> String(r.day)}
              columns={[
                { title: '天数', dataIndex: 'day', width: 60 },
                { title: '预测%', dataIndex: 'pct', width: 100, render:(v:number)=> `${formatNum(v,2)}` },
                { title: '净收益%', dataIndex: 'netPct', width: 100, render:(v:number)=> `${formatNum(v,2)}` },
                { title: '达标', dataIndex: 'hit', width: 80, render:(v:boolean)=> v? '是':'否' },
              ] as any}
              dataSource={curve}
            />
          ) : (
            <Text type="secondary">暂无数据</Text>
          )}
        </Card>
      </div>

      {/* Backtest metrics */}
      <div style={{ marginTop: 12 }}>
        <Card size="small" bordered style={{ borderRadius: 8 }} title="预测准确率回溯（1~30天）">
          {btMetrics ? (
            <Table
              size="small"
              pagination={false}
              rowKey={(r: any) => String(r.day)}
              columns={[
                { title: '天数', dataIndex: 'day', width: 60 },
                { title: '样本', dataIndex: 'points', width: 80 },
                { title: 'MAPE%', dataIndex: 'mape', width: 100 },
                { title: 'MAE', dataIndex: 'mae', width: 100 },
                { title: 'RMSE', dataIndex: 'rmse', width: 100 },
                { title: '命中率%', dataIndex: 'hit', width: 100 },
              ] as any}
              dataSource={Array.from({ length: 30 }, (_, i) => i + 1).map((d) => {
                const k = String(d);
                const m = btMetrics[k] as any;
                return {
                  day: d,
                  points: m?.points ?? 0,
                  mape: m ? `${formatNum(m.mape, 2)}` : '--',
                  mae: m ? `${formatNum(m.mae, 2)}` : '--',
                  rmse: m ? `${formatNum(m.rmse, 2)}` : '--',
                  hit: m ? `${formatNum(m.hit_rate, 2)}` : '--',
                };
              })}
            />
          ) : (
            <Text type="secondary">正在加载回溯指标...</Text>
          )}
        </Card>
      </div>

      <div style={{ marginBottom: 8 }}>
        <Tabs
          size="small"
          activeKey={activeTab}
          onChange={setActiveTab}
          items={[
            { key: 'MA', label: 'MA 均线' },
            { key: 'BOLL', label: 'BOLL 布林带' },
            { key: 'MACD', label: 'MACD 动能' },
            { key: 'VOL', label: 'VOL 成交量' },
          ]}
        />
      </div>

      {/* Forecast settings */}
      <Card
        size="small"
        style={{ marginBottom: 8, borderRadius: 8 }}
        title={
          <Space size={6}>
            预测设置
            <Tooltip
              overlayStyle={{ maxWidth: 420 }}
              title={
                <div style={{ fontSize: 12, lineHeight: 1.6 }}>
                  <div><b>方法选择</b>：linreg（线性趋势），EMA（更灵敏），Holt（水平+趋势）。</div>
                  <div><b>窗口 window</b>：建议 100–200；越大越平滑，越小越敏感。</div>
                  <div><b>α（EMA/Holt）</b>：建议 0.2–0.5；越大越看重最新数据。</div>
                  <div><b>β（Holt）</b>：建议 0.05–0.2；越大趋势更新更快。</div>
                  <div><b>回测 step</b>：建议 3–10；越小样本更多但耗时更长。</div>
                  <div style={{ color: '#64748b' }}>以上为经验值，仅供量化参考，非投资建议。</div>
                </div>
              }
            >
              <QuestionCircleOutlined style={{ color: '#64748b' }} />
            </Tooltip>
          </Space>
        }
      >
        <Space size={12} wrap>
          <span>方法</span>
          <Segmented
            size="small"
            value={fMethod}
            onChange={(v) => setFMethod(v as any)}
            options={[{ label: '线性回归', value: 'linreg' }, { label: 'EMA', value: 'ema' }, { label: 'Holt', value: 'holt' }]}
          />

          <span>窗口</span>
          <Select size="small" value={fWindow} onChange={setFWindow} style={{ width: 120 }}
            options={[{ value: 50, label: '50' }, { value: 100, label: '100' }, { value: 200, label: '200' }, { value: 300, label: '300' }]} />

          {(fMethod === 'ema' || fMethod === 'holt') && (
            <>
              <span>α</span>
              <InputNumber size="small" value={fAlpha} min={0.01} max={0.99} step={0.05} onChange={(v) => setFAlpha(Number(v))} />
            </>
          )}
          {fMethod === 'holt' && (
            <>
              <span>β</span>
              <InputNumber size="small" value={fBeta} min={0.0} max={0.99} step={0.05} onChange={(v) => setFBeta(Number(v))} />
            </>
          )}

          <span>对数预测</span>
          <Segmented size="small" value={fUseLog ? 'on' : 'off'} onChange={(v)=> setFUseLog(v === 'on')} options={[{label:'开',value:'on'},{label:'关',value:'off'}]} />

          <span>收益阈值%</span>
          <InputNumber size="small" value={fThreshPct} step={0.5} onChange={(v)=> setFThreshPct(Number(v))} />
          <span>成本%</span>
          <InputNumber size="small" value={fCostPct} step={0.5} onChange={(v)=> setFCostPct(Number(v))} />
        </Space>
      </Card>

      <div
        ref={chartRef}
        style={{
          width: '100%',
          height: `${height}px`,
          backgroundColor: '#ffffff',
          borderRadius: 8,
          border: '1px solid #e2e8f0',
          position: 'relative'
        }}
      />

      {/* Forecast summary */}
      <div style={{ marginTop: 12 }}>
        <Card size="small" bordered style={{ borderRadius: 8 }}>
          <Space size={24} wrap>
            <div>
              <Text strong>7天预测</Text>
              <div>
                {forecast.d7 ? (
                  <Text style={{ color: (forecast.d7.pct > 0 ? '#16a34a' : forecast.d7.pct < 0 ? '#ef4444' : '#334155') }}>
                    {formatNum(forecast.d7.price)} ({formatNum(forecast.d7.pct)}%)
                  </Text>
                ) : <Text type="secondary">—</Text>}
              </div>
            </div>
            <div>
              <Text strong>14天预测</Text>
              <div>
                {forecast.d14 ? (
                  <Text style={{ color: (forecast.d14.pct > 0 ? '#16a34a' : forecast.d14.pct < 0 ? '#ef4444' : '#334155') }}>
                    {formatNum(forecast.d14.price)} ({formatNum(forecast.d14.pct)}%)
                  </Text>
                ) : <Text type="secondary">—</Text>}
              </div>
            </div>
            <div>
              <Text strong>30天预测</Text>
              <div>
                {forecast.d30 ? (
                  <Text style={{ color: (forecast.d30.pct > 0 ? '#16a34a' : forecast.d30.pct < 0 ? '#ef4444' : '#334155') }}>
                    {formatNum(forecast.d30.price)} ({formatNum(forecast.d30.pct)}%)
                  </Text>
                ) : <Text type="secondary">—</Text>}
              </div>
            </div>
            <div>
              <Text strong>最早盈利天数</Text>
              <div>
                {earliestProfitDays != null ? (
                  <>
                    <Text style={{ color: '#334155' }}>{earliestProfitDays} 天</Text>
                    {earliestProfitTarget != null && earliestProfitPct != null && (
                      <Text style={{ marginLeft: 8, color: earliestProfitPct > 0 ? '#16a34a' : earliestProfitPct < 0 ? '#ef4444' : '#334155' }}>
                        目标价 {formatNum(earliestProfitTarget)} ({formatNum(earliestProfitPct)}%)
                      </Text>
                    )}
                  </>
                ) : (
                  <Text type="secondary">暂无</Text>
                )}
              </div>
            </div>
            <div>
              <Text strong>最佳入手日期</Text>
              <div>
                {bestEntryDay != null && bestEntryPrice != null ? (
                  <>
                    <Text style={{ color: '#334155' }}>{bestEntryDate || `${bestEntryDay} 天后`}</Text>
                    <Text style={{ marginLeft: 8, color: '#334155' }}>预测价 {formatNum(bestEntryPrice)}</Text>
                  </>
                ) : (
                  <Text type="secondary">暂无</Text>
                )}
              </div>
            </div>
            
          </Space>
        </Card>
      </div>

      {/* help content moved to /help */}

      {loading && (
        <div
          style={{
            position: 'absolute',
            top: '50%',
            left: '50%',
            transform: 'translate(-50%, -50%)',
            backgroundColor: 'rgba(0, 0, 0, 0.7)',
            borderRadius: '4px',
            padding: '20px',
            zIndex: 1000
          }}
        >
          <Spin size="large" />
        </div>
      )}
    </div>
  );
};

export default KlineChart;
