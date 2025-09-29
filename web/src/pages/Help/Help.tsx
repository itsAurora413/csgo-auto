import React from 'react';
import { Card, Typography, Divider } from 'antd';

const { Title, Paragraph, Text } = Typography;

const Help: React.FC = () => {
  return (
    <div>
      <Card>
        <Title level={3}>帮助中心 - K线与指标术语</Title>
        <Paragraph>
          本页解释 K 线图中常见的价格、成交量与技术指标术语，帮助你快速理解图表所表达的信息。
        </Paragraph>

        <Divider />

        <Title level={4}>基础数据</Title>
        <Paragraph>
          <Text strong>OHLC</Text>：开盘价 (Open)、最高价 (High)、最低价 (Low)、收盘价 (Close)，构成每根 K 线的基本数据。
        </Paragraph>
        <Paragraph>
          <Text strong>成交量 (Vol)</Text>：该周期内的成交数量，常与价格配合判断趋势有效性。通常“价涨量增”更健康，“价涨量减”需警惕。
        </Paragraph>

        <Divider />

        <Title level={4}>常用指标</Title>
        <Paragraph>
          <Text strong>MA 移动平均</Text>：按固定周期对收盘价取算术平均，平滑价格波动，常见参数如 MA(5/10/20)。
        </Paragraph>
        <Paragraph>
          <Text strong>EMA 指数移动平均</Text>：对最新数据权重更高的移动平均，响应更敏捷，适合捕捉较快的趋势变化。
        </Paragraph>
        <Paragraph>
          <Text strong>BOLL 布林带</Text>：以移动平均为中轨，上下轨为若干倍标准差，衡量价格波动区间与偏离程度，触及上轨/下轨常提示超买/超卖。
        </Paragraph>
        <Paragraph>
          <Text strong>MACD</Text>：由快慢两条 EMA 的差 (DIF) 及其信号线 (DEA) 构成，并配合柱体显示动能强弱与变化。柱体翻红/翻绿、DIF 与 DEA 的金叉/死叉，都可能提示拐点。
        </Paragraph>
        <Paragraph>
          <Text strong>RSI</Text>：相对强弱指数，衡量一段时间内上涨与下跌的力度对比。高位(&gt;70) 易超买，低位(&lt;30) 易超卖（本应用默认未展示，可按需开启）。
        </Paragraph>
        <Paragraph>
          <Text strong>KDJ</Text>：随机指标，反映超买超卖状态与拐点，常与趋势类指标结合使用（本应用默认未展示，可按需开启）。
        </Paragraph>

        <Divider />

        <Title level={4}>图表与配色</Title>
        <Paragraph>
          <Text strong>十字线</Text>：用于精确读取任意点位的价格与时间，便于比对历史位置与指标数值。
        </Paragraph>
        <Paragraph>
          <Text strong>配色</Text>：本应用采用 <Text code>绿涨 (#2ECB7A)</Text>、<Text code>红跌 (#F64E4E)</Text> 的配色，以便快速识别涨跌。
        </Paragraph>

        <Divider />

        <Title level={4}>预测方法说明</Title>
        <Paragraph>
          <Text strong>线性回归（linreg）</Text>：基于最近 <Text code>window</Text> 根收盘价拟合趋势线，外推到未来 <Text code>7/14/30</Text> 天。优点是简单直观，对线性趋势响应良好；缺点是对拐点与非线性形态不敏感。
        </Paragraph>
        <Paragraph>
          <Text strong>指数移动平均（EMA）</Text>：用 <Text code>α</Text> 控制对最新数据的权重，响应更敏捷。这里以 <Text code>EMA</Text> 的最新估计为基准，再按近段增量推算未来变化，适合较快节奏的市场。
        </Paragraph>
        <Paragraph>
          <Text strong>Holt 双指数平滑（holt）</Text>：分解为水平项与趋势项，分别用 <Text code>α</Text>（水平）与 <Text code>β</Text>（趋势）更新，能较好刻画线性趋势并随时间动态调整。
        </Paragraph>

        <Title level={4}>参数建议</Title>
        <Paragraph>
          <Text strong>window（训练窗口）</Text>：建议 100–200。窗口越大，趋势越平滑但响应更慢；越小响应更快但噪声更大。
          <br />
          <Text strong>α（EMA/Holt）</Text>：建议 0.2–0.5。越大越看重最新数据，预测更灵敏但可能更抖动。
          <br />
          <Text strong>β（Holt）</Text>：建议 0.05–0.2。越大趋势更新越快，适合趋势变化较快的标的。
          <br />
          <Text strong>horizons（预测天数）</Text>：默认 7/14/30 天，可按需求调整更短/更长的展望期。
          <br />
          <Text strong>step（回测步长）</Text>：建议 3–10，在回测中控制滑窗步进，步长越小，回测样本越多但耗时更长。
        </Paragraph>
        <Paragraph>
          <Text type="secondary">提示：</Text>以上方法均基于历史价格时间序列，无法捕捉突发事件及基本面变化，仅作量化参考，不构成投资建议。
        </Paragraph>

        <Divider />

        <Title level={4}>小贴士</Title>
        <Paragraph>
          - 趋势跟随：关注均线方向、均线排列，以及 MA 与 BOLL 中轨的支撑/压力作用。
          <br />
          - 量价配合：观察 VOL 与均量的配合，放量突破、缩量回踩更可靠。
          <br />
          - 动能变化：MACD 柱体由绿转红/红转绿、DIF 与 DEA 的交叉，常提示动能变化与潜在拐点。
        </Paragraph>
      </Card>
    </div>
  );
};

export default Help;
