import React, { useEffect, useState } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import { Card, Descriptions, Typography, Spin, message, Button, Space } from 'antd';
import { marketApiService } from '../../services/marketApiService';
import GoodKlineChart from '../../components/GoodKlineChart';

const { Title, Text } = Typography;

const GoodDetail: React.FC = () => {
  const { id } = useParams();
  const navigate = useNavigate();
  const [loading, setLoading] = useState(false);
  const [data, setData] = useState<any>(null);

  useEffect(() => {
    const run = async () => {
      if (!id) return;
      setLoading(true);
      try {
        const resp = await marketApiService.getGoodDetail(id);
        if (resp?.code !== 200) {
          message.error(resp?.msg || '加载详情失败');
        } else {
          setData(resp.data);
        }
      } catch (e: any) {
        message.error(e?.message || '加载详情失败');
      } finally { setLoading(false); }
    };
    run();
  }, [id]);

  if (loading) return <Spin />;
  if (!data) return null;

  const gi = data?.goods_info || {};
  const fmt = (v: any, suffix = '') => (v === null || v === undefined || v === '' ? '-' : `${v}${suffix}`);
  const toNumber = (v: any): number | null => {
    const n = Number(v);
    return isNaN(n) ? null : n;
  };
  const baseSell = toNumber(gi?.yyyp_sell_price) ?? toNumber(gi?.buff_sell_price);
  const rate1 = toNumber(gi?.sell_price_rate_1);
  const rate7 = toNumber(gi?.sell_price_rate_7);
  const rate30 = toNumber(gi?.sell_price_rate_30);
  const forecast = (base: number | null, ratePct: number | null) => {
    if (base == null || ratePct == null) return null;
    return base * (1 + ratePct / 100);
  };
  const pred7 = forecast(baseSell, rate7);
  const pred30 = forecast(baseSell, rate30);
  const trendShort = rate7 == null ? '-' : rate7 > 0 ? '短期上行' : rate7 < 0 ? '短期下行' : '短期震荡';
  const spread = (toNumber(gi?.yyyp_sell_price) != null && toNumber(gi?.yyyp_buy_price) != null)
    ? (toNumber(gi?.yyyp_sell_price)! - toNumber(gi?.yyyp_buy_price)!) : null;

  return (
    <div>
      <Space align="center" style={{ marginBottom: 12 }}>
        <Button onClick={() => navigate(-1)}>返回</Button>
        <Title level={4} style={{ margin: 0 }}>饰品详情 #{id}</Title>
      </Space>

      <Card style={{ marginBottom: 16 }}>
        <Descriptions bordered column={2} size="small">
          <Descriptions.Item label="中文名称" span={1}>{gi?.name}</Descriptions.Item>
          <Descriptions.Item label="英文名称" span={1}>{gi?.market_hash_name}</Descriptions.Item>
          <Descriptions.Item label="类型" span={2}>{gi?.type_localized_name} / {gi?.quality_localized_name}</Descriptions.Item>
          <Descriptions.Item label="磨损" span={1}>{gi?.exterior_localized_name}</Descriptions.Item>
          <Descriptions.Item label="图片" span={1}>{gi?.img ? <img src={gi.img} alt="img" style={{ maxHeight: 100 }} /> : '-'}</Descriptions.Item>
          <Descriptions.Item label="热度排名" span={1}>{fmt(gi?.rank_num)}</Descriptions.Item>
          <Descriptions.Item label="排名变化" span={1}>{fmt(gi?.rank_num_change)}</Descriptions.Item>
          <Descriptions.Item label="更新时间" span={2}>{gi?.updated_at}</Descriptions.Item>
        </Descriptions>
      </Card>

      {/* K线：优先使用真实采样数据，若不足则自动回退推导走势 */}
      <GoodKlineChart goodId={id!} height={360} />

      <Card title="YYYP 信息" style={{ marginBottom: 16 }}>
        <Descriptions bordered column={2} size="small">
          <Descriptions.Item label="yyyp_id">{gi?.yyyp_id}</Descriptions.Item>
          <Descriptions.Item label="yyyp指导价">{gi?.yyyp_steam_price}</Descriptions.Item>
          <Descriptions.Item label="求购数量">{gi?.yyyp_buy_num}</Descriptions.Item>
          <Descriptions.Item label="求购价">{gi?.yyyp_buy_price}</Descriptions.Item>
          <Descriptions.Item label="在售数量">{gi?.yyyp_sell_num}</Descriptions.Item>
          <Descriptions.Item label="在售价">{gi?.yyyp_sell_price}</Descriptions.Item>
          <Descriptions.Item label="短租数量">{gi?.yyyp_lease_num}</Descriptions.Item>
          <Descriptions.Item label="短租价格">{gi?.yyyp_lease_price}</Descriptions.Item>
          <Descriptions.Item label="短租年化">{gi?.yyyp_lease_annual}</Descriptions.Item>
          <Descriptions.Item label="长租价格">{gi?.yyyp_long_lease_price}</Descriptions.Item>
          <Descriptions.Item label="长租年化">{gi?.yyyp_long_lease_annual}</Descriptions.Item>
          <Descriptions.Item label="过户底价">{gi?.yyyp_transfer_price}</Descriptions.Item>
        </Descriptions>
      </Card>

      <Card title="BUFF 信息" style={{ marginBottom: 16 }}>
        <Descriptions bordered column={2} size="small">
          <Descriptions.Item label="buff_id">{gi?.buff_id}</Descriptions.Item>
          <Descriptions.Item label="在售价">{gi?.buff_sell_price}</Descriptions.Item>
          <Descriptions.Item label="在售数量">{gi?.buff_sell_num}</Descriptions.Item>
          <Descriptions.Item label="求购价">{gi?.buff_buy_price}</Descriptions.Item>
          <Descriptions.Item label="求购数量">{gi?.buff_buy_num}</Descriptions.Item>
          <Descriptions.Item label="求购套现比例">{gi?.buff_steam_buy_conversion}</Descriptions.Item>
          <Descriptions.Item label="售价套现比例">{gi?.buff_steam_sell_conversion}</Descriptions.Item>
        </Descriptions>
      </Card>

      <Card title="近期涨跌" style={{ marginBottom: 16 }}>
        <Descriptions bordered column={2} size="small">
          <Descriptions.Item label="近1日涨跌量">{fmt(gi?.sell_price_1)}</Descriptions.Item>
          <Descriptions.Item label="近1日涨跌幅">{fmt(gi?.sell_price_rate_1, '%')}</Descriptions.Item>
          <Descriptions.Item label="近7日涨跌量">{fmt(gi?.sell_price_7)}</Descriptions.Item>
          <Descriptions.Item label="近7日涨跌幅">{fmt(gi?.sell_price_rate_7, '%')}</Descriptions.Item>
          <Descriptions.Item label="近30日涨跌量">{fmt(gi?.sell_price_30)}</Descriptions.Item>
          <Descriptions.Item label="近30日涨跌幅">{fmt(gi?.sell_price_rate_30, '%')}</Descriptions.Item>
          <Descriptions.Item label="近180日涨跌量">{fmt(gi?.sell_price_180)}</Descriptions.Item>
          <Descriptions.Item label="近180日涨跌幅">{fmt(gi?.sell_price_rate_180, '%')}</Descriptions.Item>
        </Descriptions>
      </Card>

      <Card title="价格预测（基于YYYP在售价与历史涨跌幅）" style={{ marginBottom: 16 }}>
        <Descriptions bordered column={2} size="small">
          <Descriptions.Item label="当前基准价(YYYP在售)">{baseSell == null ? '-' : baseSell.toFixed(2)}</Descriptions.Item>
          <Descriptions.Item label="买卖价差(YYYP)">{spread == null ? '-' : spread.toFixed(2)}</Descriptions.Item>
          <Descriptions.Item label="短期走势判断">{trendShort}</Descriptions.Item>
          <Descriptions.Item label="近1日涨跌幅">{fmt(rate1, '%')}</Descriptions.Item>
          <Descriptions.Item label="预测7日价格">{pred7 == null ? '-' : pred7.toFixed(2)}</Descriptions.Item>
          <Descriptions.Item label="预测30日价格">{pred30 == null ? '-' : pred30.toFixed(2)}</Descriptions.Item>
        </Descriptions>
        <Text type="secondary">说明：预测采用最近周期累计涨跌幅对当前YYYP在售价进行比例推算，仅作参考。</Text>
      </Card>

      {/* 多维度预测与建议 */}
      <Card title="预测/走势（多维度）" style={{ marginBottom: 16 }}>
        {(() => {
          // 将周期涨跌幅转为“日均涨跌幅(%)”
          const d1  = rate1 ?? null;
          const d7  = rate7 != null ? rate7 / 7 : null;
          const d30 = rate30 != null ? rate30 / 30 : null;
          const d180= rate30 != null && gi?.sell_price_rate_180 != null ? Number(gi.sell_price_rate_180)/180 : (gi?.sell_price_rate_180 != null ? Number(gi.sell_price_rate_180)/180 : null);

          const exists = (v: number | null): v is number => v !== null && !isNaN(v);
          const vals = [d7, d30, d180].filter(exists);
          const nonNeg = vals.filter(v => v > 0);

          // 三种日均增速
          const conservativeDaily = nonNeg.length ? Math.min(...nonNeg) : (vals.length ? Math.min(...vals) : 0);
          const baseDaily = (
            (exists(d7) ? d7 * 0.6 : 0) +
            (exists(d30) ? d30 * 0.3 : 0) +
            (exists(d180) ? d180 * 0.1 : 0)
          ) / ( (exists(d7)?0.6:0) + (exists(d30)?0.3:0) + (exists(d180)?0.1:0) || 1);
          const aggressiveDaily = vals.length ? Math.max(...vals) : 0;

          // 幂次复利预测函数（百分比 -> 增长因子）
          const powPredict = (price: number, dailyPct: number, days: number) => price * Math.pow(1 + dailyPct/100, days);

          // 求解“最早盈利天数”和“180天内最大收益日”
          const grid = [1,3,5,7,10,14,21,30,45,60,90,120,150,180];
          const solveEarliest = (dailyPct: number): number | null => {
            if (baseSell == null) return null;
            for (const d of grid) { if (powPredict(baseSell, dailyPct, d) > baseSell) return d; }
            return null;
          };
          const solveBest = (dailyPct: number): number | null => {
            if (baseSell == null) return null;
            let bestDay: number | null = null;
            let bestVal = -Infinity;
            for (const d of grid) {
              const val = powPredict(baseSell, dailyPct, d) - baseSell;
              if (val > bestVal) { bestVal = val; bestDay = d; }
            }
            return bestDay;
          };

          const cEarliest = solveEarliest(conservativeDaily);
          const bEarliest = solveEarliest(baseDaily);
          const aEarliest = solveEarliest(aggressiveDaily);
          const cBest = solveBest(conservativeDaily);
          const bBest = solveBest(baseDaily);
          const aBest = solveBest(aggressiveDaily);

          // 购买时机建议
          let advice = '建议观望';
          if (exists(rate1) && exists(rate7)) {
            if (rate1 < 0 && rate7 > 0) advice = '短线回调中，建议1-3天后逢低布局';
            else if (rate1 >= 0 && rate7 >= 0) advice = '短中期同步上行，建议尽快买入';
            else if (rate7 < 0 && exists(rate30) && rate30 >= 0) advice = '中期向上短期承压，建议一周后视情况入场';
            else if ((rate1 < 0) && (rate7 < 0) && (exists(rate30) && rate30 < 0)) advice = '多周期走弱，建议暂缓买入';
          }

          return (
            <>
              <Descriptions bordered column={2} size="small" style={{ marginBottom: 12 }}>
                <Descriptions.Item label="基准日均涨跌(保守)">{fmt(conservativeDaily?.toFixed?.(3), '%/天')}</Descriptions.Item>
                <Descriptions.Item label="基准日均涨跌(基准)">{fmt(baseDaily?.toFixed?.(3), '%/天')}</Descriptions.Item>
                <Descriptions.Item label="基准日均涨跌(激进)">{fmt(aggressiveDaily?.toFixed?.(3), '%/天')}</Descriptions.Item>
                <Descriptions.Item label="买入建议" span={1}>{advice}</Descriptions.Item>
              </Descriptions>

              <Descriptions bordered column={3} size="small" title="最早盈利与最佳卖出日">
                <Descriptions.Item label="保守-最早盈利">{cEarliest ?? '-'}</Descriptions.Item>
                <Descriptions.Item label="基准-最早盈利">{bEarliest ?? '-'}</Descriptions.Item>
                <Descriptions.Item label="激进-最早盈利">{aEarliest ?? '-'}</Descriptions.Item>
                <Descriptions.Item label="保守-最佳卖出日">{cBest ?? '-'}</Descriptions.Item>
                <Descriptions.Item label="基准-最佳卖出日">{bBest ?? '-'}</Descriptions.Item>
                <Descriptions.Item label="激进-最佳卖出日">{aBest ?? '-'}</Descriptions.Item>
              </Descriptions>

              <Descriptions bordered column={3} size="small" title="价格预测(¥)">
                <Descriptions.Item label="7日(保守)">{baseSell==null?'-':powPredict(baseSell, conservativeDaily, 7).toFixed(2)}</Descriptions.Item>
                <Descriptions.Item label="7日(基准)">{baseSell==null?'-':powPredict(baseSell, baseDaily, 7).toFixed(2)}</Descriptions.Item>
                <Descriptions.Item label="7日(激进)">{baseSell==null?'-':powPredict(baseSell, aggressiveDaily, 7).toFixed(2)}</Descriptions.Item>
                <Descriptions.Item label="30日(保守)">{baseSell==null?'-':powPredict(baseSell, conservativeDaily, 30).toFixed(2)}</Descriptions.Item>
                <Descriptions.Item label="30日(基准)">{baseSell==null?'-':powPredict(baseSell, baseDaily, 30).toFixed(2)}</Descriptions.Item>
                <Descriptions.Item label="30日(激进)">{baseSell==null?'-':powPredict(baseSell, aggressiveDaily, 30).toFixed(2)}</Descriptions.Item>
              </Descriptions>
              <Text type="secondary">注：多维度预测基于分周期日均涨跌幅组合生成，仅用于辅助判断，不构成投资建议。</Text>
            </>
          );
        })()}
      </Card>
    </div>
  );
};

export default GoodDetail;
