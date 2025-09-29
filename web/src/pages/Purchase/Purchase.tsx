import React, { useState, useEffect } from 'react';
import {
  Card,
  Input,
  Button,
  List,
  Modal,
  Form,
  InputNumber,
  Select,
  Tag,
  Row,
  Col,
  Statistic,
  Divider,
  Space,
  message,
  Spin,
  Tabs,
  Image,
  Tooltip,
  Typography,
  Pagination
} from 'antd';
import {
  SearchOutlined,
  ShoppingCartOutlined,
  DollarOutlined,
  InfoCircleOutlined,
  StarOutlined
} from '@ant-design/icons';
import axios from 'axios';

const { Search } = Input;
const { Option } = Select;
const { TabPane } = Tabs;
const { Text, Title } = Typography;

interface SearchResult {
  id: number;
  gameId: number;
  gameName: string;
  gameIcon: string;
  commodityName: string;
  commodityHashName: string;
  iconUrl: string;
  iconUrlLarge: string;
  onSaleCount: number;
  onLeaseCount: number;
  leaseUnitPrice: string;
  longLeaseUnitPrice: string;
  leaseDeposit: string;
  price: string;
  steamPrice: string;
  steamUsdPrice: string;
  typeName: string;
  exterior: string;
  exteriorColor: string;
  rarity: string;
  rarityColor: string;
  quality: string;
  qualityColor: string;
  sortId: number;
  haveLease: number;
  stickersIsSort: boolean;
  subsidyPurchase: number;
  stickers: any;
  label: any;
  rent: string;
  minLeaseDeposit: any;
  listType: any;
  templatePurchaseCountText: any;
  templateTags: any;
}

interface WearLevel {
  WearName: string;
  WearCode: string;
  MinAbrade: number;
  MaxAbrade: number;
  MarketCount: number;
  MinPrice: number;
}

interface CommodityDetail {
  TemplateId: string;
  TemplateHashName: string;
  CommodityName: string;
  IconUrl: string;
  Description: string;
  Category: string;
  Rarity: string;
  Quality: string;
  WearLevels: WearLevel[];
  MarketSummary: {
    TotalMarketCount: number;
    TotalPurchaseCount: number;
    LowestPrice: number;
    HighestPurchase: number;
  };
}

interface MarketItem {
  CommodityId: string;
  Price: number;
  Abrade: number;
  WearName: string;
  StickerInfo: string;
  SellerNickname: string;
  SellTime: string;
  CanBuy: boolean;
}

interface PurchaseOrder {
  OrderId: string;
  PurchasePrice: number;
  PurchaseNum: number;
  SupplyQuantity: number;
  MinAbrade: number;
  MaxAbrade: number;
  WearName: string;
  BuyerNickname: string;
  CreateTime: string;
  CanSell: boolean;
}

interface PaginationInfo {
  page_index: number;
  page_size: number;
  total_count: number;
  total_pages: number;
}

const Purchase: React.FC = () => {
  console.log('Purchase component rendering...');
  const [searchResults, setSearchResults] = useState<SearchResult[]>([]);
  const [selectedItem, setSelectedItem] = useState<SearchResult | null>(null);
  const [commodityDetail, setCommodityDetail] = useState<CommodityDetail | null>(null);
  const [marketItems, setMarketItems] = useState<MarketItem[]>([]);
  const [purchaseOrders, setPurchaseOrders] = useState<PurchaseOrder[]>([]);
  const [loading, setLoading] = useState(false);
  const [searchLoading, setSearchLoading] = useState(false);
  const [selectedWear, setSelectedWear] = useState<WearLevel | null>(null);
  const [buyModalVisible, setBuyModalVisible] = useState(false);
  const [purchaseModalVisible, setPurchaseModalVisible] = useState(false);
  const [selectedMarketItem, setSelectedMarketItem] = useState<MarketItem | null>(null);

  // 分页相关状态
  const [currentKeyword, setCurrentKeyword] = useState<string>('');
  const [pagination, setPagination] = useState<PaginationInfo>({
    page_index: 1,
    page_size: 50,
    total_count: 0,
    total_pages: 0
  });

  const [form] = Form.useForm();

  // 搜索商品
  const performSearch = async (keyword: string, pageIndex: number = 1, pageSize: number = 50) => {
    if (!keyword.trim()) {
      message.warning('请输入搜索关键词');
      return;
    }

    setSearchLoading(true);
    try {
      const response = await axios.post('/youpin/search', {
        keyword: keyword.trim(),
        page_index: pageIndex,
        page_size: pageSize
      });

      if (response.data.success && response.data.data) {
        setSearchResults(response.data.data);
        setCurrentKeyword(keyword.trim());

        // 更新分页信息
        if (response.data.pagination) {
          setPagination(response.data.pagination);
        }

        if (response.data.data.length === 0) {
          message.info('未找到相关商品');
        }
      }
    } catch (error: any) {
      const errorData = error.response?.data;
      if (errorData?.redirect === '/youpin') {
        message.error({
          content: errorData.message || '需要先配置悠悠有品账户',
          duration: 5
        });
      } else {
        message.error('搜索失败: ' + (errorData?.error || error.message));
      }
    } finally {
      setSearchLoading(false);
    }
  };

  // 处理搜索框的搜索事件
  const handleSearch = (keyword: string) => {
    performSearch(keyword, 1, pagination.page_size);
  };

  // 处理分页变化
  const handlePageChange = (page: number, pageSize?: number) => {
    if (currentKeyword) {
      performSearch(currentKeyword, page, pageSize || pagination.page_size);
    }
  };

  // 获取商品详情（直接获取在售商品列表）
  const fetchCommodityDetail = async (templateId: string) => {
    setLoading(true);
    try {
      const response = await axios.get(`/youpin/commodity/${templateId}`);
      // 直接使用返回的商品列表作为市场物品
      if (response.data && response.data.Data && response.data.Data.commodityList) {
        setMarketItems(response.data.Data.commodityList);
        // 构造一个简单的商品详情用于显示
        const firstItem = response.data.Data.commodityList[0];
        if (firstItem) {
          setCommodityDetail({
            TemplateId: templateId,
            TemplateHashName: firstItem.commodityName,
            CommodityName: firstItem.commodityName,
            IconUrl: firstItem.iconUrl,
            Description: '',
            Category: 'CSGO',
            Rarity: 'Classified',
            Quality: 'Field-Tested',
            WearLevels: [], // 不需要磨损选择
            MarketSummary: {
              TotalMarketCount: response.data.Data.commodityList.length,
              TotalPurchaseCount: 0,
              LowestPrice: Math.min(...response.data.Data.commodityList.map((item: any) => parseFloat(item.price) || 0)),
              HighestPurchase: 0
            }
          });
        }
      }
    } catch (error: any) {
      message.error('获取商品详情失败: ' + (error.response?.data?.error || error.message));
    } finally {
      setLoading(false);
    }
  };

  // 获取市场物品
  const fetchMarketItems = async (templateId: string, minAbrade = 0, maxAbrade = 1) => {
    setLoading(true);
    try {
      const response = await axios.post('/youpin/market/items', {
        template_id: templateId,
        page_index: 1,
        page_size: 50,
        min_abrade: minAbrade,
        max_abrade: maxAbrade
      });

      if (response.data.items) {
        setMarketItems(response.data.items);
      }
    } catch (error: any) {
      message.error('获取市场物品失败: ' + (error.response?.data?.error || error.message));
    } finally {
      setLoading(false);
    }
  };

  // 获取求购订单
  const fetchPurchaseOrders = async (templateId: string, minAbrade = 0, maxAbrade = 1) => {
    setLoading(true);
    try {
      const response = await axios.post('/youpin/purchase/orders', {
        template_id: templateId,
        page_index: 1,
        page_size: 50,
        min_abrade: minAbrade,
        max_abrade: maxAbrade
      });

      if (response.data.orders) {
        setPurchaseOrders(response.data.orders);
      }
    } catch (error: any) {
      message.error('获取求购订单失败: ' + (error.response?.data?.error || error.message));
    } finally {
      setLoading(false);
    }
  };

  // 选择商品（直接获取在售列表）
  const handleSelectItem = async (item: SearchResult) => {
    setSelectedItem(item);
    setSelectedWear(null);
    setMarketItems([]);
    setPurchaseOrders([]);
    await fetchCommodityDetail(item.id.toString());
  };

  // 选择磨损等级
  const handleSelectWear = async (wear: WearLevel) => {
    setSelectedWear(wear);
    if (selectedItem) {
      await Promise.all([
        fetchMarketItems(selectedItem.id.toString(), wear.MinAbrade, wear.MaxAbrade),
        fetchPurchaseOrders(selectedItem.id.toString(), wear.MinAbrade, wear.MaxAbrade)
      ]);
    }
  };

  // 直接购买
  const handleBuyFromMarket = async (item: MarketItem) => {
    setSelectedMarketItem(item);
    setBuyModalVisible(true);
  };

  // 执行购买（使用余额支付）
  const executeBuy = async () => {
    console.log('executeBuy called, selectedMarketItem:', selectedMarketItem);

    if (!selectedMarketItem) {
      console.log('No selectedMarketItem, returning early');
      return;
    }

    try {
      console.log('Making API call to buy-with-balance...');
      const priceValue = (selectedMarketItem as any).price;
      const commodityId = (selectedMarketItem as any).commodityId;

      console.log('Purchase data:', {
        commodity_id: commodityId,
        price: typeof priceValue === 'string' ? parseFloat(priceValue) : priceValue,
        payment_method: 'balance'
      });

      const response = await axios.post('/youpin/buy-with-balance', {
        commodity_id: commodityId,
        price: typeof priceValue === 'string' ? parseFloat(priceValue) : priceValue,
        payment_method: 'balance'
      });

      console.log('API call successful:', response.data);
      message.success('余额购买成功！');
      setBuyModalVisible(false);

      // 刷新市场物品列表
      if (selectedItem && selectedWear) {
        await fetchMarketItems(selectedItem.id.toString(), selectedWear.MinAbrade, selectedWear.MaxAbrade);
      }
    } catch (error: any) {
      console.error('Purchase failed:', error);
      message.error('购买失败: ' + (error.response?.data?.error || error.message));
    }
  };

  // 创建求购订单
  const handleCreatePurchaseOrder = () => {
    if (!selectedItem || !selectedWear) {
      message.warning('请先选择商品和磨损等级');
      return;
    }
    setPurchaseModalVisible(true);
  };

  // 执行求购
  const executePurchaseOrder = async (values: any) => {
    if (!selectedItem || !selectedWear) return;

    try {
      await axios.post('/youpin/purchase', {
        template_id: selectedItem.id.toString(),
        template_hash_name: selectedItem.commodityHashName,
        commodity_name: selectedItem.commodityName,
        purchase_price: values.price,
        purchase_num: values.quantity,
        min_abrade: selectedWear.MinAbrade,
        max_abrade: selectedWear.MaxAbrade
      });

      message.success('求购订单创建成功！');
      setPurchaseModalVisible(false);
      form.resetFields();

      // 刷新求购订单列表
      await fetchPurchaseOrders(selectedItem.id.toString(), selectedWear.MinAbrade, selectedWear.MaxAbrade);
    } catch (error: any) {
      message.error('创建求购订单失败: ' + (error.response?.data?.error || error.message));
    }
  };

  // 获取稀有度颜色
  const getRarityColor = (rarity: string) => {
    const rarityColors: { [key: string]: string } = {
      'Consumer Grade': '#b0c3d9',
      'Industrial Grade': '#5e98d9',
      'Mil-Spec Grade': '#4b69ff',
      'Restricted': '#8847ff',
      'Classified': '#d32ce6',
      'Covert': '#eb4b4b',
      'Contraband': '#e4ae39'
    };
    return rarityColors[rarity] || '#888';
  };

  try {
    return (
      <div style={{ padding: '24px', maxWidth: '1400px', margin: '0 auto' }}>
        <Title level={2}>
          <ShoppingCartOutlined /> 悠悠有品购买中心
        </Title>

      {/* 搜索区域 */}
      <Card style={{ marginBottom: '16px' }}>
        <Search
          placeholder="输入饰品名称进行搜索，如：AK-47"
          enterButton={<Button type="primary" icon={<SearchOutlined />}>搜索</Button>}
          size="large"
          onSearch={handleSearch}
          loading={searchLoading}
        />
      </Card>

      <Row gutter={[16, 16]}>
        {/* 搜索结果 */}
        <Col xs={24} lg={8}>
          <Card
            title={
              <Space>
                <span>搜索结果</span>
                {pagination.total_count > 0 && (
                  <Text type="secondary">共 {pagination.total_count} 条</Text>
                )}
              </Space>
            }
            style={{ height: '600px', display: 'flex', flexDirection: 'column' }}
            bodyStyle={{ flex: 1, display: 'flex', flexDirection: 'column', overflow: 'hidden' }}
          >
            <div style={{ flex: 1, overflow: 'auto', marginBottom: '16px' }}>
              <List
                dataSource={searchResults}
                renderItem={(item) => (
                  <List.Item
                    onClick={() => handleSelectItem(item)}
                    style={{
                      cursor: 'pointer',
                      background: selectedItem?.id === item.id ? '#f0f0f0' : 'transparent',
                      padding: '12px',
                      borderRadius: '8px',
                      marginBottom: '8px'
                    }}
                  >
                    <List.Item.Meta
                      avatar={<Image src={item.iconUrl} width={64} height={48} />}
                      title={
                        <Text style={{ color: getRarityColor(item.rarity) }}>
                          {item.commodityName}
                        </Text>
                      }
                      description={
                        <Space direction="vertical" size="small">
                          <Text type="secondary">{item.typeName}</Text>
                          <Space>
                            <Tag color="blue">市场: {item.onSaleCount}</Tag>
                            <Tag color="orange">租赁: {item.onLeaseCount}</Tag>
                          </Space>
                          <Text strong>¥{item.price}</Text>
                        </Space>
                      }
                    />
                  </List.Item>
                )}
              />
            </div>

            {/* 分页组件 */}
            {pagination.total_count > 0 && (
              <div style={{ textAlign: 'center', borderTop: '1px solid #f0f0f0', paddingTop: '16px' }}>
                <Pagination
                  current={pagination.page_index}
                  pageSize={pagination.page_size}
                  total={pagination.total_count}
                  showSizeChanger={true}
                  showQuickJumper={true}
                  showTotal={(total, range) => `第 ${range[0]}-${range[1]} 条，共 ${total} 条`}
                  onChange={handlePageChange}
                  onShowSizeChange={handlePageChange}
                  pageSizeOptions={['10', '20', '50', '100']}
                  size="small"
                />
              </div>
            )}
          </Card>
        </Col>

        {/* 商品详情和磨损选择 */}
        <Col xs={24} lg={16}>
          {commodityDetail ? (
            <Card
              title={
                <Space>
                  <Image src={commodityDetail.IconUrl} width={64} height={48} />
                  <div>
                    <div style={{ color: getRarityColor(commodityDetail.Rarity), fontWeight: 'bold' }}>
                      {commodityDetail.CommodityName}
                    </div>
                    <Text type="secondary">{commodityDetail.Category}</Text>
                  </div>
                </Space>
              }
              extra={
                <Space>
                  <Statistic title="最低价" value={commodityDetail.MarketSummary.LowestPrice} prefix="¥" />
                  <Statistic title="最高求购" value={commodityDetail.MarketSummary.HighestPurchase} prefix="¥" />
                </Space>
              }
            >
              {/* 直接显示在售商品列表 */}
              <div>
                <Title level={4}>在售商品 ({marketItems.length}件)</Title>
                <Spin spinning={loading}>
                  <List
                    dataSource={marketItems}
                    renderItem={(item: any) => (
                      <List.Item
                        actions={[
                          <Button
                            type="primary"
                            icon={<DollarOutlined />}
                            onClick={() => handleBuyFromMarket(item)}
                            disabled={item.canSold !== 1}
                          >
                            余额购买
                          </Button>
                        ]}
                      >
                        <List.Item.Meta
                          title={
                            <Space>
                              <Text strong>¥{item.price}</Text>
                              <Tag color="blue">{item.exteriorName}</Tag>
                              <Text type="secondary">磨损: {item.abrade ? parseFloat(item.abrade).toFixed(4) : 'N/A'}</Text>
                            </Space>
                          }
                          description={
                            <Space direction="vertical" size="small">
                              <Text>卖家: {item.sellerNickname}</Text>
                              <Text type="secondary">上架时间: {item.sellTime}</Text>
                            </Space>
                          }
                        />
                      </List.Item>
                    )}
                  />
                </Spin>
              </div>
            </Card>
          ) : (
            <Card style={{ height: '600px', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
              <div style={{ textAlign: 'center', color: '#999' }}>
                <InfoCircleOutlined style={{ fontSize: '48px', marginBottom: '16px' }} />
                <div>请先搜索并选择一个饰品</div>
              </div>
            </Card>
          )}
        </Col>
      </Row>

      {/* 购买确认对话框 */}
      <Modal
        title="确认购买（余额支付）"
        visible={buyModalVisible}
        onOk={executeBuy}
        onCancel={() => setBuyModalVisible(false)}
        okText="确认余额购买"
        cancelText="取消"
      >
        {selectedMarketItem && (
          <div>
            <p><strong>商品:</strong> {selectedItem?.commodityName}</p>
            <p><strong>磨损:</strong> {(selectedMarketItem as any).exteriorName} ({(selectedMarketItem as any).abrade ? parseFloat((selectedMarketItem as any).abrade).toFixed(4) : 'N/A'})</p>
            <p><strong>价格:</strong> ¥{(selectedMarketItem as any).price}</p>
            <p><strong>卖家:</strong> {(selectedMarketItem as any).sellerNickname}</p>
            <p><strong>支付方式:</strong> <Tag color="green">钱包余额</Tag></p>
          </div>
        )}
      </Modal>

      {/* 求购订单对话框 */}
      <Modal
        title="创建求购订单"
        visible={purchaseModalVisible}
        onCancel={() => setPurchaseModalVisible(false)}
        footer={null}
      >
        <Form
          form={form}
          onFinish={executePurchaseOrder}
          layout="vertical"
        >
          <Form.Item
            label="求购价格"
            name="price"
            rules={[{ required: true, message: '请输入求购价格' }]}
          >
            <InputNumber
              style={{ width: '100%' }}
              placeholder="输入求购价格"
              min={0.01}
              step={0.01}
              precision={2}
              addonBefore="¥"
            />
          </Form.Item>

          <Form.Item
            label="求购数量"
            name="quantity"
            rules={[{ required: true, message: '请输入求购数量' }]}
          >
            <InputNumber
              style={{ width: '100%' }}
              placeholder="输入求购数量"
              min={1}
              max={100}
            />
          </Form.Item>

          {selectedWear && (
            <div style={{ marginBottom: '16px', padding: '12px', background: '#f5f5f5', borderRadius: '6px' }}>
              <p><strong>磨损等级:</strong> {selectedWear.WearName}</p>
              <p><strong>磨损范围:</strong> {selectedWear.MinAbrade ? selectedWear.MinAbrade.toFixed(4) : 'N/A'} - {selectedWear.MaxAbrade ? selectedWear.MaxAbrade.toFixed(4) : 'N/A'}</p>
            </div>
          )}

          <Form.Item>
            <Space>
              <Button type="primary" htmlType="submit">
                创建求购订单
              </Button>
              <Button onClick={() => setPurchaseModalVisible(false)}>
                取消
              </Button>
            </Space>
          </Form.Item>
        </Form>
      </Modal>
    </div>
  );
  } catch (error) {
    console.error('Purchase component render error:', error);
    return (
      <div style={{ padding: '24px', textAlign: 'center' }}>
        <h2>页面渲染错误</h2>
        <p>购买页面遇到了一些问题，请刷新页面重试。</p>
        <p>错误信息: {String(error)}</p>
      </div>
    );
  }
};

export default Purchase;