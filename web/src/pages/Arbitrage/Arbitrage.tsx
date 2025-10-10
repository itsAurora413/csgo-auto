import React, { useState, useEffect } from 'react';
import {
  Card,
  Table,
  Button,
  Input,
  message,
  Space,
  Tag,
  Statistic,
  Row,
  Col,
  InputNumber,
  Form,
  Modal,
  Select,
  Tooltip,
  Progress,
  Checkbox,
  Radio,
  Tabs
} from 'antd';
import {
  RiseOutlined,
  FallOutlined,
  DollarOutlined,
  ReloadOutlined,
  InfoCircleOutlined,
  LineChartOutlined,
  ShoppingCartOutlined,
  DownloadOutlined
} from '@ant-design/icons';
import type { ColumnsType } from 'antd/es/table';

const { Option } = Select;
const { TabPane } = Tabs;

interface ArbitrageOpportunity {
  good_id: number;
  good_name: string;
  current_buy_price: number;
  current_sell_price: number;
  profit_rate: number;
  estimated_profit: number;
  avg_buy_price_7d: number;
  avg_sell_price_7d: number;
  price_trend: string;
  days_of_data: number;
  last_update_time: string;
  risk_level: string;
  score: number;
}

interface PurchasePlan {
  id: number;
  budget: number;
  total_items: number;
  total_cost: number;
  status: string;
  created_at: string;
  updated_at: string;
  items?: PurchasePlanItem[];
}

interface PurchasePlanItem {
  id: number;
  plan_id: number;
  good_id: number;
  good_name: string;
  buy_price: number;
  quantity: number;
  subtotal: number;
  profit_rate: number;
  risk_level: string;
  created_at: string;
}

const Arbitrage: React.FC = () => {
  const [opportunities, setOpportunities] = useState<ArbitrageOpportunity[]>([]);
  const [loading, setLoading] = useState(false);
  const [collecting, setCollecting] = useState(false);
  const [minProfitRate, setMinProfitRate] = useState(0.05);
  const [minDaysHistory, setMinDaysHistory] = useState(7);
  const [collectModalVisible, setCollectModalVisible] = useState(false);
  const [templateIds, setTemplateIds] = useState<string>('');
  const [form] = Form.useForm();

  // 自动求购相关状态
  const [selectedGoodIds, setSelectedGoodIds] = useState<number[]>([]);
  const [autoPurchaseModalVisible, setAutoPurchaseModalVisible] = useState(false);
  const [purchasing, setPurchasing] = useState(false);
  const [purchaseForm] = Form.useForm();
  const [purchaseResultVisible, setPurchaseResultVisible] = useState(false);
  const [purchaseResult, setPurchaseResult] = useState<any>(null);
  const [purchaseMode, setPurchaseMode] = useState<'manual' | 'smart'>('manual');

  // 求购清单相关状态
  const [activeTab, setActiveTab] = useState<string>('opportunities');
  const [purchasePlans, setPurchasePlans] = useState<PurchasePlan[]>([]);
  const [plansLoading, setPlansLoading] = useState(false);
  const [selectedPlan, setSelectedPlan] = useState<PurchasePlan | null>(null);
  const [planDetailVisible, setPlanDetailVisible] = useState(false);
  const [executePlanModalVisible, setExecutePlanModalVisible] = useState(false);
  const [executePlanForm] = Form.useForm();

  // 获取套利机会
  const fetchOpportunities = async () => {
    setLoading(true);
    try {
      const response = await fetch(
        `/api/v1/youpin/arbitrage/opportunities?min_profit_rate=${minProfitRate}&min_days_history=${minDaysHistory}&limit=100`
      );
      const data = await response.json();

      if (data.success) {
        setOpportunities(data.opportunities || []);
        message.success(`加载了 ${data.count} 个套利机会`);
      } else {
        throw new Error(data.error || '获取套利机会失败');
      }
    } catch (error) {
      message.error(`获取套利机会失败: ${error}`);
    } finally {
      setLoading(false);
    }
  };

  const handleExport = () => {
    const url = `/api/v1/youpin/arbitrage/opportunities/export?min_profit_rate=${minProfitRate}&min_days_history=${minDaysHistory}&limit=1000`;
    window.open(url, '_blank');
  };

  // 收集价格快照
  const collectPrices = async () => {
    const ids = templateIds.split(',').map(id => parseInt(id.trim())).filter(id => !isNaN(id));

    if (ids.length === 0) {
      message.error('请输入有效的商品模板ID（用逗号分隔）');
      return;
    }

    setCollecting(true);
    try {
      const response = await fetch('/api/v1/youpin/arbitrage/collect-prices', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json'
        },
        body: JSON.stringify({ good_ids: ids })
      });

      const data = await response.json();

      if (data.success) {
        message.success(`成功收集 ${data.collected} 个商品的价格快照，失败 ${data.failed} 个`);
        setCollectModalVisible(false);
        setTemplateIds('');
        // 收集完成后自动刷新套利机会列表
        fetchOpportunities();
      } else {
        throw new Error(data.error || '收集价格快照失败');
      }
    } catch (error) {
      message.error(`收集价格快照失败: ${error}`);
    } finally {
      setCollecting(false);
    }
  };

  // 自动求购
  const handleAutoPurchase = async () => {
    try {
      const values = await purchaseForm.validateFields();

      setPurchasing(true);

      // 构建请求体
      const requestBody: any = {
        mode: values.mode || 'manual',
        max_total: values.max_total,
        auto_receive: values.auto_receive,
        dry_run: values.dry_run
      };

      // 根据模式添加不同的参数
      if (values.mode === 'smart') {
        // 前端百分比转换为小数（8% -> 0.08）
        requestBody.min_profit_rate = (values.min_profit_rate || 8) / 100;
        requestBody.risk_level = values.risk_level || 'low';
        requestBody.top_n = values.top_n || 10;
      } else {
        requestBody.good_ids = selectedGoodIds;
      }

      const response = await fetch('/api/v1/youpin/arbitrage/auto-purchase', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json'
        },
        body: JSON.stringify(requestBody)
      });

      const data = await response.json();

      if (data.success) {
        setPurchaseResult(data);
        setAutoPurchaseModalVisible(false);
        setPurchaseResultVisible(true);
        setSelectedGoodIds([]);

        if (values.dry_run) {
          message.success(`模拟运行完成：成功 ${data.success_count} 个，失败 ${data.failed_count} 个`);
        } else {
          message.success(`求购完成：成功 ${data.success_count} 个，失败 ${data.failed_count} 个`);
        }
      } else {
        throw new Error(data.error || '自动求购失败');
      }
    } catch (error: any) {
      if (error.errorFields) {
        message.error('请填写完整的求购配置');
      } else {
        message.error(`自动求购失败: ${error}`);
      }
    } finally {
      setPurchasing(false);
    }
  };

  // 选择商品
  const handleSelectChange = (goodId: number, checked: boolean) => {
    if (checked) {
      setSelectedGoodIds([...selectedGoodIds, goodId]);
    } else {
      setSelectedGoodIds(selectedGoodIds.filter(id => id !== goodId));
    }
  };

  // 全选/取消全选
  const handleSelectAll = (checked: boolean) => {
    if (checked) {
      setSelectedGoodIds(opportunities.map(o => o.good_id));
    } else {
      setSelectedGoodIds([]);
    }
  };

  // 打开自动求购对话框（手动模式）
  const openAutoPurchaseModal = () => {
    if (selectedGoodIds.length === 0) {
      message.warning('请先选择要求购的商品');
      return;
    }

    setPurchaseMode('manual');

    // 设置默认值（手动模式）
    purchaseForm.setFieldsValue({
      mode: 'manual',
      max_total: 500,
      auto_receive: false,
      dry_run: true
    });

    setAutoPurchaseModalVisible(true);
  };

  // 打开智能推荐求购对话框
  const openSmartPurchaseModal = () => {
    setPurchaseMode('smart');

    // 设置默认值（智能模式）
    purchaseForm.setFieldsValue({
      mode: 'smart',
      max_total: 500,
      min_profit_rate: 8, // 8% (前端显示为百分比)
      risk_level: 'low',
      top_n: 10,
      auto_receive: false,
      dry_run: true
    });

    setAutoPurchaseModalVisible(true);
  };

  // 获取求购计划列表
  const fetchPurchasePlans = async () => {
    setPlansLoading(true);
    try {
      const response = await fetch('/api/v1/youpin/purchase-plans?limit=50');
      const data = await response.json();

      if (data.success) {
        setPurchasePlans(data.plans || []);
      } else {
        throw new Error(data.error || '获取求购计划失败');
      }
    } catch (error) {
      message.error(`获取求购计划失败: ${error}`);
    } finally {
      setPlansLoading(false);
    }
  };

  // 查看计划详情
  const viewPlanDetail = async (planId: number) => {
    try {
      const response = await fetch(`/api/v1/youpin/purchase-plans/${planId}`);
      const data = await response.json();

      if (data.success && data.plan) {
        setSelectedPlan(data.plan);
        setPlanDetailVisible(true);
      } else {
        throw new Error(data.error || '获取计划详情失败');
      }
    } catch (error) {
      message.error(`获取计划详情失败: ${error}`);
    }
  };

  // 打开执行计划对话框
  const openExecutePlanModal = (plan: PurchasePlan) => {
    setSelectedPlan(plan);
    executePlanForm.setFieldsValue({
      auto_receive: false,
      dry_run: true
    });
    setExecutePlanModalVisible(true);
  };

  // 执行求购计划
  const executePurchasePlan = async () => {
    if (!selectedPlan) return;

    try {
      const values = await executePlanForm.validateFields();
      setPurchasing(true);

      const response = await fetch(`/api/v1/youpin/purchase-plans/${selectedPlan.id}/execute`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json'
        },
        body: JSON.stringify(values)
      });

      const data = await response.json();

      if (data.success) {
        setPurchaseResult(data);
        setExecutePlanModalVisible(false);
        setPurchaseResultVisible(true);

        // 以服务端返回为准，避免仅依赖前端提交值
        if (data.dry_run) {
          message.success(`模拟运行完成：成功 ${data.success_count} 个，失败 ${data.failed_count} 个`);
        } else {
          message.success(`执行完成：成功 ${data.success_count} 个，失败 ${data.failed_count} 个`);
          // 刷新计划列表
          fetchPurchasePlans();
        }
      } else {
        throw new Error(data.error || '执行求购计划失败');
      }
    } catch (error: any) {
      if (error.errorFields) {
        message.error('请填写完整的执行配置');
      } else {
        message.error(`执行求购计划失败: ${error}`);
      }
    } finally {
      setPurchasing(false);
    }
  };

  // 重置清单状态
  const resetPurchasePlan = async (planId: number) => {
    try {
      const response = await fetch(`/api/v1/youpin/purchase-plans/${planId}/reset`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json'
        }
      });

      const data = await response.json();

      if (data.success) {
        message.success('清单状态已重置为待执行');
        fetchPurchasePlans(); // 刷新列表
      } else {
        throw new Error(data.error || '重置失败');
      }
    } catch (error) {
      message.error(`重置清单失败: ${error}`);
    }
  };

  // 页面加载时获取套利机会
  useEffect(() => {
    fetchOpportunities();
  }, []);

  // 切换到清单标签页时获取清单列表
  useEffect(() => {
    if (activeTab === 'plans') {
      fetchPurchasePlans();
    }
  }, [activeTab]);

  // 风险等级颜色
  const getRiskColor = (risk: string) => {
    switch (risk) {
      case 'low':
        return 'green';
      case 'medium':
        return 'orange';
      case 'high':
        return 'red';
      default:
        return 'default';
    }
  };

  // 风险等级文本
  const getRiskText = (risk: string) => {
    switch (risk) {
      case 'low':
        return '低风险';
      case 'medium':
        return '中风险';
      case 'high':
        return '高风险';
      default:
        return '未知';
    }
  };

  // 价格趋势图标
  const getTrendIcon = (trend: string) => {
    switch (trend) {
      case 'up':
        return <RiseOutlined style={{ color: '#52c41a' }} />;
      case 'down':
        return <FallOutlined style={{ color: '#ff4d4f' }} />;
      default:
        return <LineChartOutlined style={{ color: '#1890ff' }} />;
    }
  };

  // 价格趋势文本
  const getTrendText = (trend: string) => {
    switch (trend) {
      case 'up':
        return '上涨';
      case 'down':
        return '下跌';
      default:
        return '稳定';
    }
  };

  const columns: ColumnsType<ArbitrageOpportunity> = [
    {
      title: (
        <Checkbox
          checked={selectedGoodIds.length === opportunities.length && opportunities.length > 0}
          indeterminate={selectedGoodIds.length > 0 && selectedGoodIds.length < opportunities.length}
          onChange={(e) => handleSelectAll(e.target.checked)}
        >
          选择
        </Checkbox>
      ),
      key: 'select',
      width: 80,
      fixed: 'left',
      render: (_, record) => (
        <Checkbox
          checked={selectedGoodIds.includes(record.good_id)}
          onChange={(e) => handleSelectChange(record.good_id, e.target.checked)}
        />
      )
    },
    {
      title: '商品名称',
      dataIndex: 'good_name',
      key: 'good_name',
      width: 250,
      fixed: 'left',
      render: (text: string, record: ArbitrageOpportunity) => (
        <div>
          <div style={{ fontWeight: 500 }}>{text}</div>
          <div style={{ fontSize: 12, color: '#999' }}>ID: {record.good_id}</div>
        </div>
      )
    },
    {
      title: (
        <Tooltip title="综合评分：基于利润率、风险、流动性等多维度量化评估（0-100分）">
          评分 <InfoCircleOutlined />
        </Tooltip>
      ),
      dataIndex: 'score',
      key: 'score',
      width: 100,
      sorter: (a, b) => (a.score || 0) - (b.score || 0),
      defaultSortOrder: 'descend',
      render: (score: number) => {
        const color = score >= 80 ? '#52c41a' : score >= 60 ? '#1890ff' : score >= 40 ? '#faad14' : '#ff4d4f';
        return (
          <div style={{ display: 'flex', alignItems: 'center', gap: '4px' }}>
            <Progress
              type="circle"
              percent={score}
              width={40}
              strokeColor={color}
              format={(percent) => `${percent?.toFixed(0)}`}
            />
            <span style={{ color, fontWeight: 500 }}>{score?.toFixed(1)}</span>
          </div>
        );
      }
    },
    {
      title: '当前求购价',
      dataIndex: 'current_buy_price',
      key: 'current_buy_price',
      width: 120,
      sorter: (a, b) => a.current_buy_price - b.current_buy_price,
      render: (price: number) => `¥${price.toFixed(2)}`
    },
    {
      title: '当前售价',
      dataIndex: 'current_sell_price',
      key: 'current_sell_price',
      width: 120,
      sorter: (a, b) => a.current_sell_price - b.current_sell_price,
      render: (price: number) => `¥${price.toFixed(2)}`
    },
    {
      title: (
        <Tooltip title="扣除1%手续费后的预期利润率">
          利润率 <InfoCircleOutlined />
        </Tooltip>
      ),
      dataIndex: 'profit_rate',
      key: 'profit_rate',
      width: 120,
      sorter: (a, b) => a.profit_rate - b.profit_rate,
      render: (rate: number) => (
        <Tag color={rate > 0.1 ? 'green' : rate > 0.05 ? 'blue' : 'default'}>
          {(rate * 100).toFixed(2)}%
        </Tag>
      )
    },
    {
      title: '预期利润',
      dataIndex: 'estimated_profit',
      key: 'estimated_profit',
      width: 120,
      sorter: (a, b) => a.estimated_profit - b.estimated_profit,
      render: (profit: number) => (
        <span style={{ color: profit > 0 ? '#52c41a' : '#ff4d4f', fontWeight: 500 }}>
          ¥{profit.toFixed(2)}
        </span>
      )
    },
    {
      title: '价格趋势',
      dataIndex: 'price_trend',
      key: 'price_trend',
      width: 100,
      filters: [
        { text: '上涨', value: 'up' },
        { text: '稳定', value: 'stable' },
        { text: '下跌', value: 'down' }
      ],
      onFilter: (value, record) => record.price_trend === value,
      render: (trend: string) => (
        <Space>
          {getTrendIcon(trend)}
          <span>{getTrendText(trend)}</span>
        </Space>
      )
    },
    {
      title: '风险等级',
      dataIndex: 'risk_level',
      key: 'risk_level',
      width: 100,
      filters: [
        { text: '低风险', value: 'low' },
        { text: '中风险', value: 'medium' },
        { text: '高风险', value: 'high' }
      ],
      onFilter: (value, record) => record.risk_level === value,
      render: (risk: string) => (
        <Tag color={getRiskColor(risk)}>{getRiskText(risk)}</Tag>
      )
    },
    {
      title: '7天均价',
      key: 'avg_prices',
      width: 150,
      render: (_, record) => (
        <div style={{ fontSize: 12 }}>
          <div>求购: ¥{record.avg_buy_price_7d.toFixed(2)}</div>
          <div>售价: ¥{record.avg_sell_price_7d.toFixed(2)}</div>
        </div>
      )
    },
    {
      title: '数据天数',
      dataIndex: 'days_of_data',
      key: 'days_of_data',
      width: 100,
      sorter: (a, b) => a.days_of_data - b.days_of_data,
      render: (days: number) => `${days} 天`
    }
  ];

  return (
    <div style={{ padding: '24px' }}>
      <Card
        title={
          <Space>
            <DollarOutlined />
            <span>套利分析 - 求购持有策略</span>
          </Space>
        }
      >
        <Tabs activeKey={activeTab} onChange={setActiveTab}>
          <TabPane tab="套利机会" key="opportunities">
            <div style={{ marginBottom: 16 }}>
              <Space>
                <Button
                  type="primary"
                  icon={<ShoppingCartOutlined />}
                  onClick={openSmartPurchaseModal}
                >
                  智能推荐求购
                </Button>
                <Button
                  icon={<ShoppingCartOutlined />}
                  onClick={openAutoPurchaseModal}
                  disabled={selectedGoodIds.length === 0}
                >
                  手动求购 {selectedGoodIds.length > 0 ? `(${selectedGoodIds.length})` : ''}
                </Button>
                <Button
                  icon={<ReloadOutlined />}
                  onClick={fetchOpportunities}
                  loading={loading}
                >
                  刷新
                </Button>
                <Button
                  icon={<LineChartOutlined />}
                  onClick={() => setCollectModalVisible(true)}
                >
                  收集价格数据
                </Button>
                <Button
                  icon={<DownloadOutlined />}
                  onClick={handleExport}
                >
                  导出Excel
                </Button>
              </Space>
            </div>
        {/* 统计信息 */}
        <Row gutter={16} style={{ marginBottom: 24 }}>
          <Col span={6}>
            <Card>
              <Statistic
                title="总机会数"
                value={opportunities.length}
                prefix={<DollarOutlined />}
              />
            </Card>
          </Col>
          <Col span={6}>
            <Card>
              <Statistic
                title="平均利润率"
                value={
                  opportunities.length > 0
                    ? (opportunities.reduce((sum, o) => sum + o.profit_rate, 0) / opportunities.length * 100).toFixed(2)
                    : 0
                }
                suffix="%"
                prefix={<RiseOutlined />}
              />
            </Card>
          </Col>
          <Col span={6}>
            <Card>
              <Statistic
                title="低风险机会"
                value={opportunities.filter(o => o.risk_level === 'low').length}
                valueStyle={{ color: '#52c41a' }}
              />
            </Card>
          </Col>
          <Col span={6}>
            <Card>
              <Statistic
                title="上涨趋势"
                value={opportunities.filter(o => o.price_trend === 'up').length}
                valueStyle={{ color: '#52c41a' }}
              />
            </Card>
          </Col>
        </Row>

        {/* 筛选条件 */}
        <Card size="small" style={{ marginBottom: 16 }}>
          <Form layout="inline">
            <Form.Item label="最小利润率">
              <InputNumber
                min={0}
                max={1}
                step={0.01}
                value={minProfitRate}
                onChange={(value) => setMinProfitRate(value || 0.05)}
                formatter={value => `${(Number(value) * 100).toFixed(0)}%`}
                parser={value => (Number(value?.replace('%', '')) || 0) / 100}
              />
            </Form.Item>
            <Form.Item label="最少历史天数">
              <InputNumber
                min={1}
                max={30}
                value={minDaysHistory}
                onChange={(value) => setMinDaysHistory(value || 7)}
              />
            </Form.Item>
            <Form.Item>
              <Button type="primary" onClick={fetchOpportunities}>
                应用筛选
              </Button>
            </Form.Item>
          </Form>
        </Card>

        {/* 说明 */}
        <Card size="small" type="inner" style={{ marginBottom: 16, background: '#f0f5ff' }}>
          <Space direction="vertical" size="small">
            <div><InfoCircleOutlined /> <strong>策略说明：</strong></div>
            <div>1. 通过求购订单以最高求购价买入饰品</div>
            <div>2. 等待7天或更久的交易冷却期</div>
            <div>3. 以当前最低售价或更高价格上架出售</div>
            <div>4. 利润率已扣除1%的平台手续费</div>
            <div>5. 评分越高越推荐（综合考虑利润率、风险、流动性、价格趋势等因素）</div>
            <div>6. 建议选择高评分、低风险、价格稳定或上涨趋势的商品</div>
          </Space>
        </Card>

        {/* 数据表格 */}
        <Table
          columns={columns}
          dataSource={opportunities}
          rowKey="good_id"
          loading={loading}
          scroll={{ x: 1500 }}
          pagination={{
            pageSize: 20,
            showSizeChanger: true,
            showTotal: (total) => `共 ${total} 条记录`
          }}
        />
          </TabPane>

          <TabPane tab="求购清单" key="plans">
            <div style={{ marginBottom: 16 }}>
              <Space>
                <Button
                  icon={<ReloadOutlined />}
                  onClick={fetchPurchasePlans}
                  loading={plansLoading}
                >
                  刷新清单
                </Button>
              </Space>
            </div>

            <Table
              dataSource={purchasePlans}
              rowKey="id"
              loading={plansLoading}
              pagination={{
                pageSize: 10,
                showSizeChanger: true,
                showTotal: (total) => `共 ${total} 条清单`
              }}
              columns={[
                {
                  title: '清单ID',
                  dataIndex: 'id',
                  key: 'id',
                  width: 80
                },
                {
                  title: '预算',
                  dataIndex: 'budget',
                  key: 'budget',
                  width: 120,
                  render: (budget: number) => `¥${budget.toFixed(2)}`
                },
                {
                  title: '总件数',
                  dataIndex: 'total_items',
                  key: 'total_items',
                  width: 100,
                  render: (items: number) => `${items} 件`
                },
                {
                  title: '实际花费',
                  dataIndex: 'total_cost',
                  key: 'total_cost',
                  width: 120,
                  render: (cost: number) => (
                    <span style={{ color: '#52c41a', fontWeight: 500 }}>
                      ¥{cost.toFixed(2)}
                    </span>
                  )
                },
                {
                  title: '预算使用率',
                  key: 'usage_rate',
                  width: 120,
                  render: (_: any, record: PurchasePlan) => {
                    const rate = (record.total_cost / record.budget * 100);
                    return (
                      <Progress
                        percent={parseFloat(rate.toFixed(1))}
                        size="small"
                        status={rate > 95 ? 'success' : 'active'}
                      />
                    );
                  }
                },
                {
                  title: '状态',
                  dataIndex: 'status',
                  key: 'status',
                  width: 100,
                  render: (status: string) => {
                    const statusMap: { [key: string]: { text: string; color: string } } = {
                      pending: { text: '待执行', color: 'blue' },
                      partial: { text: '部分完成', color: 'orange' },
                      completed: { text: '已完成', color: 'green' },
                      cancelled: { text: '已取消', color: 'red' }
                    };
                    const statusInfo = statusMap[status] || { text: status, color: 'default' };
                    return <Tag color={statusInfo.color}>{statusInfo.text}</Tag>;
                  }
                },
                {
                  title: '创建时间',
                  dataIndex: 'created_at',
                  key: 'created_at',
                  width: 180,
                  render: (time: string) => new Date(time).toLocaleString('zh-CN')
                },
                {
                  title: '操作',
                  key: 'action',
                  width: 240,
                  fixed: 'right',
                  render: (_: any, record: PurchasePlan) => (
                    <Space>
                      <Button
                        size="small"
                        onClick={() => viewPlanDetail(record.id)}
                      >
                        查看详情
                      </Button>
                      <Button
                        size="small"
                        type="primary"
                        disabled={record.status === 'completed'}
                        onClick={() => openExecutePlanModal(record)}
                      >
                        {record.status === 'partial' ? '重新执行' : '执行'}
                      </Button>
                      {(record.status === 'completed' || record.status === 'partial') && (
                        <Button
                          size="small"
                          onClick={() => resetPurchasePlan(record.id)}
                        >
                          重置
                        </Button>
                      )}
                    </Space>
                  )
                }
              ]}
            />
          </TabPane>
        </Tabs>
      </Card>

      {/* 收集价格数据模态框 */}
      <Modal
        title="收集价格快照"
        open={collectModalVisible}
        onOk={collectPrices}
        onCancel={() => setCollectModalVisible(false)}
        confirmLoading={collecting}
      >
        <Form form={form} layout="vertical">
          <Form.Item
            label="商品ID列表"
            help="输入要收集价格的CSQAQ商品ID，多个ID用逗号分隔，例如：110,111,112"
          >
            <Input.TextArea
              rows={4}
              placeholder="例如：110,111,112,113,114"
              value={templateIds}
              onChange={(e) => setTemplateIds(e.target.value)}
            />
          </Form.Item>
          <Form.Item>
            <div style={{ fontSize: 12, color: '#999' }}>
              <InfoCircleOutlined /> 系统会收集每个商品的最高求购价和最低售价，用于套利分析。建议每天定时收集价格数据以获得更准确的趋势分析。
            </div>
          </Form.Item>
        </Form>
      </Modal>

      {/* 自动求购配置模态框 */}
      <Modal
        title={purchaseMode === 'smart' ? '智能推荐求购' : '手动选择求购'}
        open={autoPurchaseModalVisible}
        onOk={handleAutoPurchase}
        onCancel={() => setAutoPurchaseModalVisible(false)}
        confirmLoading={purchasing}
        width={600}
      >
        <Form form={purchaseForm} layout="vertical">
          <Form.Item name="mode" hidden>
            <Input />
          </Form.Item>

          <Form.Item>
            <div style={{ padding: '12px', background: purchaseMode === 'smart' ? '#f6ffed' : '#f0f5ff', borderRadius: '4px', marginBottom: '16px' }}>
              <Space direction="vertical" size="small" style={{ width: '100%' }}>
                {purchaseMode === 'smart' ? (
                  <>
                    <div><strong>🤖 智能推荐模式</strong></div>
                    <div style={{ fontSize: 12, color: '#666' }}>
                      系统会根据套利分析结果，自动选择评分最高、风险最低的商品进行求购
                    </div>
                  </>
                ) : (
                  <>
                    <div><strong>👆 手动选择模式 - 已选择 {selectedGoodIds.length} 个商品</strong></div>
                    <div style={{ fontSize: 12, color: '#666' }}>
                      系统会按照您选择的商品实时查询最新价格，并根据价格策略自动计算最优求购价
                    </div>
                  </>
                )}
              </Space>
            </div>
          </Form.Item>

          {/* 智能模式的专属参数 */}
          {purchaseMode === 'smart' && (
            <>
              <Form.Item
                label="最小利润率（%）"
                name="min_profit_rate"
                help="只求购利润率大于此值的商品"
              >
                <InputNumber
                  min={0}
                  max={100}
                  step={1}
                  style={{ width: '100%' }}
                  addonAfter="%"
                />
              </Form.Item>

              <Form.Item
                label="风险等级"
                name="risk_level"
                help="选择可接受的风险等级"
              >
                <Radio.Group>
                  <Radio value="low">低风险</Radio>
                  <Radio value="medium">中风险</Radio>
                  <Radio value="high">高风险</Radio>
                </Radio.Group>
              </Form.Item>

              <Form.Item
                label="商品数量"
                name="top_n"
                help="从推荐列表中取前N个商品"
              >
                <InputNumber
                  min={1}
                  max={50}
                  style={{ width: '100%' }}
                />
              </Form.Item>
            </>
          )}

          <Form.Item
            label="最大总金额"
            name="max_total"
            rules={[{ required: true, message: '请输入最大总金额' }]}
            help="求购的总金额上限（单位：元）"
          >
            <InputNumber
              min={1}
              max={100000}
              step={100}
              style={{ width: '100%' }}
              prefix="¥"
            />
          </Form.Item>

          {/* 价格策略不再需要，已由后端根据步进规则与市场最高价自动计算最终出价 */}

          <Form.Item
            label="收货设置"
            name="auto_receive"
            valuePropName="checked"
          >
            <Checkbox>自动收货（交易冷却期到达后自动收货到库存）</Checkbox>
          </Form.Item>

          <Form.Item
            label="运行模式"
            name="dry_run"
            valuePropName="checked"
          >
            <Checkbox>模拟运行（不会实际发起求购，仅用于测试）</Checkbox>
          </Form.Item>

          <Form.Item>
            <div style={{ fontSize: 12, color: '#999', background: '#fff7e6', padding: '8px', borderRadius: '4px' }}>
              <InfoCircleOutlined /> <strong>价格规则说明：</strong>
              <div>• 0～1元：增量为0.01的倍数</div>
              <div>• 1～50元：增量为0.1的倍数</div>
              <div>• 50～1000元：增量为1的倍数</div>
              <div>• 1000元以上：增量为10的倍数</div>
            </div>
          </Form.Item>
        </Form>
      </Modal>

      {/* 求购结果模态框 */}
      <Modal
        title="求购结果"
        open={purchaseResultVisible}
        onOk={() => setPurchaseResultVisible(false)}
        onCancel={() => setPurchaseResultVisible(false)}
        width={700}
        footer={[
          <Button key="close" type="primary" onClick={() => setPurchaseResultVisible(false)}>
            关闭
          </Button>
        ]}
      >
        {purchaseResult && (
          <div>
            <Row gutter={16} style={{ marginBottom: 24 }}>
              <Col span={8}>
                <Card>
                  <Statistic
                    title="总处理数"
                    value={purchaseResult.success_count + purchaseResult.failed_count}
                    prefix={<ShoppingCartOutlined />}
                  />
                </Card>
              </Col>
              <Col span={8}>
                <Card>
                  <Statistic
                    title="成功数"
                    value={purchaseResult.success_count}
                    valueStyle={{ color: '#52c41a' }}
                  />
                </Card>
              </Col>
              <Col span={8}>
                <Card>
                  <Statistic
                    title="失败数"
                    value={purchaseResult.failed_count}
                    valueStyle={{ color: '#ff4d4f' }}
                  />
                </Card>
              </Col>
            </Row>

            <Row gutter={16} style={{ marginBottom: 24 }}>
              <Col span={12}>
                <Card>
                  <Statistic
                    title="总花费"
                    value={purchaseResult.total_cost?.toFixed(2) || '0.00'}
                    prefix="¥"
                  />
                </Card>
              </Col>
              <Col span={12}>
                <Card>
                  <Statistic
                    title="预算使用率"
                    value={purchaseResult.budget_used_rate?.toFixed(1) || '0.0'}
                    suffix="%"
                  />
                </Card>
              </Col>
            </Row>

            {purchaseResult.failed_count > 0 && (
              <Card
                style={{ marginBottom: 16, background: '#fff7e6', borderColor: '#ffa940' }}
                size="small"
              >
                <Space direction="vertical" style={{ width: '100%' }}>
                  <div style={{ color: '#d46b08', fontWeight: 500 }}>
                    ⚠️ {purchaseResult.all_success === false ? '执行未完全成功' : '部分商品执行失败'}
                  </div>
                  <div style={{ fontSize: 12, color: '#666' }}>
                    有 {purchaseResult.failed_count} 个商品求购失败，您可以查看下方详细信息了解失败原因，并重新执行此清单
                  </div>
                </Space>
              </Card>
            )}

            {purchaseResult.details && purchaseResult.details.length > 0 && (
              <div>
                <h4>详细信息</h4>
                <Table
                  dataSource={purchaseResult.details}
                  rowKey="good_id"
                  pagination={false}
                  size="small"
                  rowClassName={(record: any) => record.success ? '' : 'row-error'}
                  columns={[
                    {
                      title: '商品名称',
                      dataIndex: 'good_name',
                      key: 'good_name',
                      width: 200,
                      ellipsis: true
                    },
                    {
                      title: '状态',
                      dataIndex: 'success',
                      key: 'success',
                      width: 80,
                      render: (success: boolean) => (
                        <Tag color={success ? 'green' : 'red'}>
                          {success ? '成功' : '失败'}
                        </Tag>
                      )
                    },
                    {
                      title: '价格',
                      dataIndex: 'price',
                      key: 'price',
                      width: 100,
                      render: (price: number) => price ? `¥${price.toFixed(2)}` : '-'
                    },
                    {
                      title: '数量',
                      dataIndex: 'quantity',
                      key: 'quantity',
                      width: 80
                    },
                    {
                      title: '订单号',
                      dataIndex: 'order_no',
                      key: 'order_no',
                      width: 150,
                      ellipsis: true
                    },
                    {
                      title: '信息',
                      dataIndex: 'message',
                      key: 'message',
                      render: (text: string, record: any) => (
                        <Tooltip title={text} placement="topLeft">
                          <span style={{ color: record.success ? '#52c41a' : '#ff4d4f' }}>
                            {text}
                          </span>
                        </Tooltip>
                      )
                    }
                  ]}
                />
              </div>
            )}
          </div>
        )}
      </Modal>

      {/* 清单详情模态框 */}
      <Modal
        title={`求购清单详情 #${selectedPlan?.id || ''}`}
        open={planDetailVisible}
        onCancel={() => setPlanDetailVisible(false)}
        width={900}
        footer={[
          <Button key="close" onClick={() => setPlanDetailVisible(false)}>
            关闭
          </Button>,
          <Button
            key="execute"
            type="primary"
            disabled={selectedPlan?.status === 'completed'}
            onClick={() => {
              setPlanDetailVisible(false);
              if (selectedPlan) openExecutePlanModal(selectedPlan);
            }}
          >
            执行清单
          </Button>
        ]}
      >
        {selectedPlan && (
          <div>
            <Row gutter={16} style={{ marginBottom: 24 }}>
              <Col span={8}>
                <Card>
                  <Statistic
                    title="预算"
                    value={selectedPlan.budget}
                    prefix="¥"
                  />
                </Card>
              </Col>
              <Col span={8}>
                <Card>
                  <Statistic
                    title="总件数"
                    value={selectedPlan.total_items}
                    suffix="件"
                  />
                </Card>
              </Col>
              <Col span={8}>
                <Card>
                  <Statistic
                    title="实际花费"
                    value={selectedPlan.total_cost}
                    prefix="¥"
                    valueStyle={{ color: '#52c41a' }}
                  />
                </Card>
              </Col>
            </Row>

            {selectedPlan.items && selectedPlan.items.length > 0 && (
              <div>
                <h4>清单明细 ({selectedPlan.items.length} 件商品)</h4>
                <Table
                  dataSource={selectedPlan.items}
                  rowKey="id"
                  pagination={false}
                  size="small"
                  columns={[
                    {
                      title: '商品名称',
                      dataIndex: 'good_name',
                      key: 'good_name',
                      width: 300,
                      ellipsis: true
                    },
                    {
                      title: '求购价',
                      dataIndex: 'buy_price',
                      key: 'buy_price',
                      width: 100,
                      render: (price: number) => `¥${price.toFixed(2)}`
                    },
                    {
                      title: '数量',
                      dataIndex: 'quantity',
                      key: 'quantity',
                      width: 80,
                      render: (qty: number) => `${qty} 件`
                    },
                    {
                      title: '小计',
                      dataIndex: 'subtotal',
                      key: 'subtotal',
                      width: 100,
                      render: (subtotal: number) => (
                        <span style={{ color: '#52c41a', fontWeight: 500 }}>
                          ¥{subtotal.toFixed(2)}
                        </span>
                      )
                    },
                    {
                      title: '利润率',
                      dataIndex: 'profit_rate',
                      key: 'profit_rate',
                      width: 100,
                      render: (rate: number) => (
                        <Tag color={rate > 0.1 ? 'green' : rate > 0.05 ? 'blue' : 'default'}>
                          {(rate * 100).toFixed(2)}%
                        </Tag>
                      )
                    },
                    {
                      title: '风险',
                      dataIndex: 'risk_level',
                      key: 'risk_level',
                      width: 80,
                      render: (risk: string) => (
                        <Tag color={getRiskColor(risk)}>{getRiskText(risk)}</Tag>
                      )
                    }
                  ]}
                />
              </div>
            )}
          </div>
        )}
      </Modal>

      {/* 执行清单模态框 */}
      <Modal
        title={`执行求购清单 #${selectedPlan?.id || ''}`}
        open={executePlanModalVisible}
        onOk={executePurchasePlan}
        onCancel={() => setExecutePlanModalVisible(false)}
        confirmLoading={purchasing}
        width={600}
      >
        <Form form={executePlanForm} layout="vertical">
          <Form.Item>
            <div style={{ padding: '12px', background: '#f6ffed', borderRadius: '4px', marginBottom: '16px' }}>
              <Space direction="vertical" size="small" style={{ width: '100%' }}>
                <div><strong>📋 执行清单求购</strong></div>
                <div style={{ fontSize: 12, color: '#666' }}>
                  系统将按照清单中的商品和数量，实时查询最新价格并发起求购
                </div>
                {selectedPlan && (
                  <div style={{ fontSize: 12, color: '#666' }}>
                    清单包含 <strong>{selectedPlan.total_items}</strong> 件商品，预算 <strong>¥{selectedPlan.budget.toFixed(2)}</strong>
                  </div>
                )}
              </Space>
            </div>
          </Form.Item>

          {/* 价格策略不再需要，已由后端根据步进规则与市场最高价自动计算最终出价 */}

          <Form.Item
            label="收货设置"
            name="auto_receive"
            valuePropName="checked"
          >
            <Checkbox>自动收货（交易冷却期到达后自动收货到库存）</Checkbox>
          </Form.Item>

          <Form.Item
            label="运行模式"
            name="dry_run"
            valuePropName="checked"
          >
            <Checkbox>模拟运行（不会实际发起求购，仅用于测试）</Checkbox>
          </Form.Item>

          <Form.Item>
            <div style={{ fontSize: 12, color: '#999', background: '#fff7e6', padding: '8px', borderRadius: '4px' }}>
              <InfoCircleOutlined /> <strong>价格规则说明：</strong>
              <div>• 0～1元：增量为0.01的倍数</div>
              <div>• 1～50元：增量为0.1的倍数</div>
              <div>• 50～1000元：增量为1的倍数</div>
              <div>• 1000元以上：增量为10的倍数</div>
            </div>
          </Form.Item>
        </Form>
      </Modal>
    </div>
  );
};

export default Arbitrage;
