import React, { useEffect, useRef, useState } from 'react';
import { init, dispose, Chart } from 'klinecharts';
import axios from 'axios';
import { Card, Segmented, Spin } from 'antd';

interface KPoint { t: number; o: number; h: number; l: number; c: number; v: number }

interface Props {
  goodId: string | number;
  height?: number;
}

const GoodKlineChart: React.FC<Props> = ({ goodId, height = 360 }) => {
  const ref = useRef<HTMLDivElement>(null);
  const chartRef = useRef<Chart | null>(null);
  const [loading, setLoading] = useState(false);
  const [interval, setInterval] = useState<'20m' | '1h' | '1d'>('20m');
  const [hasData, setHasData] = useState(false);

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

  useEffect(() => {
    if (!chartRef.current || !goodId) return;
    let cancelled = false;
    (async () => {
      setLoading(true);
      try {
        const real = await axios.get('/csqaq/good/kline', { params: { id: goodId, interval } });
        let rows: KPoint[] = real.data?.data || [];
        if (!rows.length) {
          const derived = await axios.get('/csqaq/good/derived_kline', { params: { id: goodId, days: 30 } });
          rows = derived.data?.data || [];
        }
        if (cancelled) return;
        const data = rows.map((p: any) => ({ timestamp: Number(p.t), open: p.o, high: p.h, low: p.l, close: p.c, volume: p.v }));
        chartRef.current!.applyNewData(data);
        setHasData(!!data.length);
      } finally { if (!cancelled) setLoading(false); }
    })();
    return () => { cancelled = true; };
  }, [goodId, interval]);

  return (
    <Card title="价格K线" extra={<Segmented options={[{label:'20分', value:'20m'}, {label:'1小时', value:'1h'}, {label:'1天', value:'1d'}]} value={interval} onChange={(v)=> setInterval(v as any)} />}>
      <div ref={ref} style={{ height }} />
      {loading && <Spin style={{ position:'absolute', right: 16, top: 16 }} />}
      {!hasData && !loading && <div style={{ padding: 8, color: '#999' }}>暂无数据，已回退到推导走势</div>}
    </Card>
  );
};

export default GoodKlineChart;

