import React, { useEffect, useState } from 'react';
import { Card, Table, Tag, Select, Button, Row, Col, Statistic, Alert, Spin, Space, Typography } from 'antd';
import { ReloadOutlined, TrophyOutlined, RiseOutlined, FallOutlined, FireOutlined } from '@ant-design/icons';
import axios from 'axios';

const { Title, Text } = Typography;

interface PurchaseRecommendation {
  good_id: number;
  name: string;
  market_hash_name: string;
  recommendation_type: 'arbitrage' | 'price_drop' | 'undervalued' | 'trending_up';
  confidence_score: number;
  risk_level: 'low' | 'medium' | 'high';
  current_price: number;
  current_yyyp_price?: number;
  current_buff_price?: number;
  target_price?: number;
  potential_profit?: number;
  profit_percentage?: number;
  price_change_7d?: number;
  price_change_30d?: number;
  volatility_score: number;
  reason: string;
  created_at: string;
}

interface RecommendationResponse {
  code: number;
  data: PurchaseRecommendation[];
  total: number;
  filters: {
    limit: number;
    risk_level: string;
    type: string;
  };
  generated_at: string;
  msg: string;
}

function Strategies() {
  const [loading, setLoading] = useState(false);
  const [recommendations, setRecommendations] = useState<PurchaseRecommendation[]>([]);
  const [total, setTotal] = useState(0);
  const [filters, setFilters] = useState({
    limit: 20,
    risk_level: 'all',
    type: 'all'
  });
  const [lastUpdate, setLastUpdate] = useState<string>('');

  const fetchRecommendations = async () => {
    setLoading(true);
    try {
      const params: any = { limit: filters.limit };
      if (filters.risk_level !== 'all') params.risk_level = filters.risk_level;
      if (filters.type !== 'all') params.type = filters.type;

      const response = await axios.get<RecommendationResponse>('/api/v1/csqaq/recommendations', { params });

      if (response.data.code === 200) {
        setRecommendations(response.data.data || []);
        setTotal(response.data.total);
        setLastUpdate(response.data.generated_at);
      }
    } catch (error) {
      console.error('获取推荐数据失败:', error);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchRecommendations();
  }, [filters]);

  const getTypeIcon = (type: string) => {
    switch (type) {
      case 'arbitrage': return <RiseOutlined />;
      case 'price_drop': return <FallOutlined />;
      case 'undervalued': return <TrophyOutlined />;
      case 'trending_up': return <FireOutlined />;
      default: return null;
    }
  };

  const getTypeColor = (type: string) => {
    switch (type) {
      case 'arbitrage': return 'green';
      case 'price_drop': return 'blue';
      case 'undervalued': return 'gold';
      case 'trending_up': return 'red';
      default: return 'default';
    }
  };

  const getTypeLabel = (type: string) => {
    switch (type) {
      case 'arbitrage': return '套利机会';
      case 'price_drop': return '价格下跌';
      case 'undervalued': return '被低估';
      case 'trending_up': return '上涨趋势';
      default: return type;
    }
  };

  const getRiskColor = (risk: string) => {
    switch (risk) {
      case 'low': return 'green';
      case 'medium': return 'orange';
      case 'high': return 'red';
      default: return 'default';
    }
  };

  const getRiskLabel = (risk: string) => {
    switch (risk) {
      case 'low': return '低风险';
      case 'medium': return '中等风险';
      case 'high': return '高风险';
      default: return risk;
    }
  };

  const columns = [
    {
      title: '饰品名称',
      dataIndex: 'name',
      key: 'name',
      width: 250,
      render: (text: string, record: PurchaseRecommendation) => (
        <div>
          <div style={{ fontWeight: 'bold', marginBottom: 4 }}>{text}</div>
          <div style={{ fontSize: '12px', color: '#999' }}>{record.market_hash_name}</div>
        </div>
      ),
    },
    {
      title: '推荐类型',
      dataIndex: 'recommendation_type',
      key: 'recommendation_type',
      width: 120,
      render: (type: string) => (
        <Tag icon={getTypeIcon(type)} color={getTypeColor(type)}>
          {getTypeLabel(type)}
        </Tag>
      ),
    },
    {
      title: '置信度',
      dataIndex: 'confidence_score',
      key: 'confidence_score',
      width: 100,
      render: (score: number) => (
        <div>
          <div>{(score * 100).toFixed(1)}%</div>
          <div style={{
            width: '60px',
            height: '4px',
            backgroundColor: '#f0f0f0',
            borderRadius: '2px',
            marginTop: '2px'
          }}>
            <div
              style={{
                width: `${score * 100}%`,
                height: '100%',
                backgroundColor: score > 0.7 ? '#52c41a' : score > 0.5 ? '#faad14' : '#ff4d4f',
                borderRadius: '2px'
              }}
            />
          </div>
        </div>
      ),
    },
    {
      title: '风险等级',
      dataIndex: 'risk_level',
      key: 'risk_level',
      width: 100,
      render: (risk: string) => (
        <Tag color={getRiskColor(risk)}>
          {getRiskLabel(risk)}
        </Tag>
      ),
    },
    {
      title: '当前价格',
      dataIndex: 'current_price',
      key: 'current_price',
      width: 120,
      render: (price: number, record: PurchaseRecommendation) => (
        <div>
          <div style={{ fontWeight: 'bold' }}>
            {record.current_yyyp_price ? `¥${record.current_yyyp_price.toFixed(2)}` : `¥${(price || 0).toFixed(2)}`}
          </div>
          {record.current_buff_price && (
            <div style={{ fontSize: '11px', color: '#999' }}>
              Buff: ¥{record.current_buff_price.toFixed(2)}
            </div>
          )}
        </div>
      ),
    },
    {
      title: '目标价格',
      dataIndex: 'target_price',
      key: 'target_price',
      width: 100,
      render: (price: number) => price ? `¥${price.toFixed(2)}` : '-',
    },
    {
      title: '预期收益',
      dataIndex: 'profit_percentage',
      key: 'profit_percentage',
      width: 120,
      render: (percentage: number, record: PurchaseRecommendation) => {
        if (!percentage && !record.potential_profit) return '-';
        return (
          <div>
            {percentage && <div style={{ color: percentage > 0 ? '#52c41a' : '#ff4d4f', fontWeight: 'bold' }}>
              {percentage > 0 ? '+' : ''}{percentage.toFixed(1)}%
            </div>}
            {record.potential_profit && (
              <div style={{ fontSize: '12px', color: '#666' }}>
                ¥{record.potential_profit.toFixed(2)}
              </div>
            )}
          </div>
        );
      },
    },
    {
      title: '价格变化',
      dataIndex: 'price_changes',
      key: 'price_changes',
      width: 100,
      render: (_: any, record: PurchaseRecommendation) => (
        <div>
          {record.price_change_7d !== undefined && (
            <div style={{
              color: record.price_change_7d > 0 ? '#52c41a' : record.price_change_7d < 0 ? '#ff4d4f' : '#666',
              fontSize: '12px'
            }}>
              7天: {record.price_change_7d > 0 ? '+' : ''}{record.price_change_7d.toFixed(1)}%
            </div>
          )}
          {record.price_change_30d !== undefined && (
            <div style={{
              color: record.price_change_30d > 0 ? '#52c41a' : record.price_change_30d < 0 ? '#ff4d4f' : '#666',
              fontSize: '11px'
            }}>
              30天: {record.price_change_30d > 0 ? '+' : ''}{record.price_change_30d.toFixed(1)}%
            </div>
          )}
        </div>
      ),
    },
    {
      title: '波动率',
      dataIndex: 'volatility_score',
      key: 'volatility_score',
      width: 80,
      render: (score: number) => {
        const getVolatilityColor = (vol: number) => {
          if (vol < 0.2) return '#52c41a';
          if (vol < 0.5) return '#faad14';
          return '#ff4d4f';
        };
        return (
          <div style={{ color: getVolatilityColor(score) }}>
            {(score * 100).toFixed(0)}%
          </div>
        );
      },
    },
    {
      title: '推荐理由',
      dataIndex: 'reason',
      key: 'reason',
      render: (reason: string) => (
        <Text style={{ fontSize: '12px' }} ellipsis={{ tooltip: reason }}>
          {reason}
        </Text>
      ),
    },
  ];

  return (
    <div style={{ padding: '24px' }}>
      <Title level={2}>购买推荐策略</Title>

      <Row gutter={16} style={{ marginBottom: '24px' }}>
        <Col span={6}>
          <Card>
            <Statistic
              title="总推荐数 (50-300价格区间)"
              value={total}
              prefix={<TrophyOutlined />}
            />
          </Card>
        </Col>
        <Col span={6}>
          <Card>
            <Statistic
              title="低风险推荐"
              value={recommendations.filter(r => r.risk_level === 'low').length}
              valueStyle={{ color: '#52c41a' }}
            />
          </Card>
        </Col>
        <Col span={6}>
          <Card>
            <Statistic
              title="套利机会"
              value={recommendations.filter(r => r.recommendation_type === 'arbitrage').length}
              valueStyle={{ color: '#1890ff' }}
            />
          </Card>
        </Col>
        <Col span={6}>
          <Card>
            <Statistic
              title="平均预期收益"
              value={recommendations.filter(r => r.profit_percentage).length > 0 ?
                (recommendations.filter(r => r.profit_percentage)
                  .reduce((sum, r) => sum + (r.profit_percentage || 0), 0) /
                 recommendations.filter(r => r.profit_percentage).length).toFixed(1) : 0}
              suffix="%"
              valueStyle={{ color: '#fa8c16' }}
            />
          </Card>
        </Col>
      </Row>

      <Row gutter={16} style={{ marginBottom: '24px' }}>
        <Col span={6}>
          <Card>
            <Statistic
              title="平均置信度"
              value={recommendations.length > 0 ? (recommendations.reduce((sum, r) => sum + r.confidence_score, 0) / recommendations.length * 100).toFixed(1) : 0}
              suffix="%"
            />
          </Card>
        </Col>
        <Col span={6}>
          <Card>
            <Statistic
              title="低波动率推荐"
              value={recommendations.filter(r => r.volatility_score < 0.3).length}
              valueStyle={{ color: '#52c41a' }}
            />
          </Card>
        </Col>
        <Col span={6}>
          <Card>
            <Statistic
              title="上涨趋势"
              value={recommendations.filter(r => r.recommendation_type === 'trending_up').length}
              valueStyle={{ color: '#722ed1' }}
            />
          </Card>
        </Col>
        <Col span={6}>
          <Card>
            <Statistic
              title="抄底机会"
              value={recommendations.filter(r => r.recommendation_type === 'price_drop' || r.recommendation_type === 'undervalued').length}
              valueStyle={{ color: '#13c2c2' }}
            />
          </Card>
        </Col>
      </Row>

      <Card
        title="推荐列表"
        extra={
          <Space>
            <Select
              value={filters.type}
              style={{ width: 120 }}
              onChange={(value) => setFilters({ ...filters, type: value })}
            >
              <Select.Option value="all">全部类型</Select.Option>
              <Select.Option value="arbitrage">套利机会</Select.Option>
              <Select.Option value="price_drop">价格下跌</Select.Option>
              <Select.Option value="undervalued">被低估</Select.Option>
              <Select.Option value="trending_up">上涨趋势</Select.Option>
            </Select>
            <Select
              value={filters.risk_level}
              style={{ width: 100 }}
              onChange={(value) => setFilters({ ...filters, risk_level: value })}
            >
              <Select.Option value="all">全部风险</Select.Option>
              <Select.Option value="low">低风险</Select.Option>
              <Select.Option value="medium">中等风险</Select.Option>
              <Select.Option value="high">高风险</Select.Option>
            </Select>
            <Select
              value={filters.limit}
              style={{ width: 80 }}
              onChange={(value) => setFilters({ ...filters, limit: value })}
            >
              <Select.Option value={10}>10条</Select.Option>
              <Select.Option value={20}>20条</Select.Option>
              <Select.Option value={50}>50条</Select.Option>
              <Select.Option value={100}>100条</Select.Option>
            </Select>
            <Button
              icon={<ReloadOutlined />}
              onClick={fetchRecommendations}
              loading={loading}
            >
              刷新
            </Button>
          </Space>
        }
      >
        {recommendations.length === 0 && !loading && (
          <Alert
            message="暂无推荐数据"
            description="系统正在收集50-300价格区间的饰品数据，请等待更多历史数据积累后再查看推荐。针对中低价饰品，系统需要至少3-5天的价格数据来生成有效的购买推荐，包括套利机会、抄底时机和趋势分析。"
            type="info"
            showIcon
            style={{ marginBottom: '16px' }}
          />
        )}

        <Spin spinning={loading}>
          <Table
            columns={columns}
            dataSource={recommendations}
            rowKey="good_id"
            pagination={false}
            size="middle"
          />
        </Spin>

        {lastUpdate && (
          <div style={{ textAlign: 'center', marginTop: '16px', color: '#999', fontSize: '12px' }}>
            数据更新时间: {new Date(lastUpdate).toLocaleString()}
          </div>
        )}
      </Card>
    </div>
  );
}

export default Strategies;


