import React, { useState, useEffect } from 'react';
import {
  Card,
  Row,
  Col,
  Statistic,
  Table,
  Tag,
  Progress,
  List,
  Avatar,
  Spin,
  Alert,
  Button
} from 'antd';
import {
  DollarOutlined,
  TrophyOutlined,
  RiseOutlined,
  FallOutlined,
  SwapOutlined,
  ReloadOutlined
} from '@ant-design/icons';
import { LineChart, Line, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer } from 'recharts';
import { apiService, Trade, ArbitrageOpportunity, MarketTrend } from '../../services/apiService';
import { websocketService } from '../../services/websocketService';
import moment from 'moment';

interface DashboardData {
  recent_trades: Trade[];
  opportunities: ArbitrageOpportunity[];
  top_movers: MarketTrend[];
  timestamp: string;
}

interface PerformanceData {
  total_profit: number;
  total_trades: number;
  success_rate: number;
  roi: number;
}

const Dashboard: React.FC = () => {
  const [loading, setLoading] = useState(true);
  const [dashboardData, setDashboardData] = useState<DashboardData | null>(null);
  const [performanceData, setPerformanceData] = useState<PerformanceData | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    loadDashboardData();
    
    // Subscribe to real-time updates
    websocketService.subscribeToTradeUpdates(handleTradeUpdate);
    websocketService.subscribeToArbitrageOpportunities(handleArbitrageUpdate);
    websocketService.subscribeToMarketTrends(handleTrendUpdate);
    
    return () => {
      // Cleanup subscriptions
    };
  }, []);

  const loadDashboardData = async () => {
    try {
      setLoading(true);
      setError(null);
      
      const [dashboard, performance] = await Promise.all([
        apiService.getDashboard(),
        apiService.getPerformance()
      ]);
      
      setDashboardData(dashboard);
      setPerformanceData(performance);
    } catch (err: any) {
      setError(err.message || '加载数据失败');
    } finally {
      setLoading(false);
    }
  };

  const handleTradeUpdate = (data: any) => {
    // Update recent trades
    if (dashboardData) {
      setDashboardData({
        ...dashboardData,
        recent_trades: [data, ...dashboardData.recent_trades.slice(0, 9)]
      });
    }
  };

  const handleArbitrageUpdate = (data: ArbitrageOpportunity[]) => {
    if (dashboardData) {
      setDashboardData({
        ...dashboardData,
        opportunities: data
      });
    }
  };

  const handleTrendUpdate = (data: MarketTrend[]) => {
    if (dashboardData) {
      setDashboardData({
        ...dashboardData,
        top_movers: data
      });
    }
  };

  const tradeColumns = [
    {
      title: '物品',
      dataIndex: 'item',
      key: 'item',
      render: (item: any) => (
        <div style={{ display: 'flex', alignItems: 'center' }}>
          <Avatar src={item.icon_url} size="small" />
          <span style={{ marginLeft: 8 }}>{item.name}</span>
        </div>
      )
    },
    {
      title: '平台',
      dataIndex: 'platform',
      key: 'platform',
      render: (platform: string) => (
        <Tag className={`${platform}-tag`}>{platform.toUpperCase()}</Tag>
      )
    },
    {
      title: '类型',
      dataIndex: 'type',
      key: 'type',
      render: (type: string) => (
        <Tag color={type === 'buy' ? 'green' : 'red'}>
          {type === 'buy' ? '买入' : '卖出'}
        </Tag>
      )
    },
    {
      title: '价格',
      dataIndex: 'price',
      key: 'price',
      render: (price: number) => `$${price.toFixed(2)}`
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      render: (status: string) => (
        <Tag className={`status-${status}`}>
          {status === 'completed' ? '已完成' : 
           status === 'pending' ? '进行中' : '失败'}
        </Tag>
      )
    },
    {
      title: '时间',
      dataIndex: 'created_at',
      key: 'created_at',
      render: (time: string) => moment(time).fromNow()
    }
  ];

  if (error) {
    return (
      <Alert
        message="数据加载失败"
        description={error}
        type="error"
        action={
          <Button size="small" danger onClick={loadDashboardData}>
            重试
          </Button>
        }
      />
    );
  }

  return (
    <div>
      <Row gutter={[16, 16]} style={{ marginBottom: 24 }}>
        <Col span={24}>
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
            <h1>仪表盘</h1>
            <Button 
              icon={<ReloadOutlined />} 
              onClick={loadDashboardData}
              loading={loading}
            >
              刷新
            </Button>
          </div>
        </Col>
      </Row>

      {loading ? (
        <div className="loading-container">
          <Spin size="large" />
        </div>
      ) : (
        <>
          {/* Performance Stats */}
          <Row gutter={[16, 16]} style={{ marginBottom: 24 }}>
            <Col span={6}>
              <Card>
                <Statistic
                  title="总利润"
                  value={performanceData?.total_profit || 0}
                  precision={2}
                  prefix={<DollarOutlined />}
                  valueStyle={{ color: '#3f8600' }}
                />
              </Card>
            </Col>
            <Col span={6}>
              <Card>
                <Statistic
                  title="总交易数"
                  value={performanceData?.total_trades || 0}
                  prefix={<SwapOutlined />}
                />
              </Card>
            </Col>
            <Col span={6}>
              <Card>
                <Statistic
                  title="成功率"
                  value={performanceData?.success_rate || 0}
                  precision={1}
                  suffix="%"
                  prefix={<TrophyOutlined />}
                />
                <Progress 
                  percent={performanceData?.success_rate || 0} 
                  showInfo={false} 
                  strokeColor="#52c41a"
                />
              </Card>
            </Col>
            <Col span={6}>
              <Card>
                <Statistic
                  title="投资回报率"
                  value={performanceData?.roi || 0}
                  precision={1}
                  suffix="%"
                  prefix={<RiseOutlined />}
                  valueStyle={{ 
                    color: (performanceData?.roi || 0) > 0 ? '#3f8600' : '#cf1322' 
                  }}
                />
              </Card>
            </Col>
          </Row>

          <Row gutter={[16, 16]}>
            {/* Recent Trades */}
            <Col span={16}>
              <Card title="最近交易" className="dashboard-card">
                <Table
                  columns={tradeColumns}
                  dataSource={dashboardData?.recent_trades || []}
                  pagination={false}
                  size="small"
                  rowKey="id"
                />
              </Card>
            </Col>

            {/* Top Movers */}
            <Col span={8}>
              <Card title="价格变动" className="dashboard-card">
                <List
                  size="small"
                  dataSource={dashboardData?.top_movers || []}
                  renderItem={(item: MarketTrend) => (
                    <List.Item>
                      <div style={{ display: 'flex', alignItems: 'center', width: '100%' }}>
                        <Avatar src={item.item.icon_url} size="small" />
                        <div style={{ marginLeft: 8, flex: 1 }}>
                          <div>{item.item.name}</div>
                          <div style={{ fontSize: '12px', color: '#999' }}>
                            {item.platform.toUpperCase()}
                          </div>
                        </div>
                        <div className={`trend-${item.trend_direction}`}>
                          {item.trend_direction === 'up' ? <RiseOutlined /> : 
                           item.trend_direction === 'down' ? <FallOutlined /> : null}
                          {item.price_change > 0 ? '+' : ''}{item.price_change.toFixed(1)}%
                        </div>
                      </div>
                    </List.Item>
                  )}
                />
              </Card>
            </Col>
          </Row>

          {/* Arbitrage Opportunities */}
          <Row gutter={[16, 16]} style={{ marginTop: 16 }}>
            <Col span={24}>
              <Card title="套利机会" className="dashboard-card">
                <List
                  grid={{ gutter: 16, column: 3 }}
                  dataSource={dashboardData?.opportunities || []}
                  renderItem={(item: ArbitrageOpportunity) => (
                    <List.Item>
                      <Card className="arbitrage-card" size="small">
                        <div style={{ display: 'flex', alignItems: 'center', marginBottom: 8, justifyContent: 'space-between' }}>
                          <div style={{ display: 'flex', alignItems: 'center' }}>
                            <Avatar src={item.item.icon_url} size="small" />
                            <span style={{ marginLeft: 8, fontWeight: 'bold' }}>
                              {item.item.name}
                            </span>
                          </div>
                          {typeof item.score === 'number' && (
                            <span
                              title="综合评分（0-100）"
                              style={{
                                fontSize: 12,
                                fontWeight: 600,
                                color:
                                  item.score >= 80 ? '#52c41a' : item.score >= 60 ? '#1890ff' : item.score >= 40 ? '#faad14' : '#ff4d4f'
                              }}
                            >
                              评分 {item.score.toFixed(0)}
                            </span>
                          )}
                        </div>
                        <div>
                          <div>买入: {item.buy_platform.toUpperCase()} - ${item.buy_price.toFixed(2)}</div>
                          <div>卖出: {item.sell_platform.toUpperCase()} - ${item.sell_price.toFixed(2)}</div>
                          <div style={{ fontWeight: 'bold', marginTop: 4 }}>
                            利润: ${item.profit.toFixed(2)} ({item.profit_percent.toFixed(1)}%)
                          </div>
                        </div>
                      </Card>
                    </List.Item>
                  )}
                />
              </Card>
            </Col>
          </Row>
        </>
      )}
    </div>
  );
};

export default Dashboard;
