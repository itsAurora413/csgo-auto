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
  Pagination,
  Checkbox
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

interface PurchaseOrderItem {
  purchaseNo: string;
  isNew: number;
  headPicUrl: string;
  userName: string;
  userId: number;
  iconUrl: string;
  purchasePrice: number;
  purchasePriceDesc: string;
  commodityName: string;
  surplusQuantity: number;
  abradeText: any;
  fadeText: any;
  specialStyle: any;
  autoReceived: number;
  rankFirstPrice: any;
  rankFirstPriceText: any;
  isRankFirst: any;
  templateId: number;
  type: number;
  typeId: number;
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
  const [purchaseOrderItems, setPurchaseOrderItems] = useState<PurchaseOrderItem[]>([]);
  const [loading, setLoading] = useState(false);
  const [searchLoading, setSearchLoading] = useState(false);
  const [purchaseModalVisible, setPurchaseModalVisible] = useState(false);
  const [selectedPurchaseOrder, setSelectedPurchaseOrder] = useState<PurchaseOrderItem | null>(null);

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

  // 获取求购列表
  const fetchPurchaseOrderList = async (templateId: number) => {
    setLoading(true);
    try {
      console.log('正在请求求购列表, templateId:', templateId);
      const response = await axios.post('/youpin/purchase/list', {
        template_id: templateId,
        page_index: 1,
        page_size: 20
      });

      console.log('API 响应完整对象:', response);
      console.log('API 响应 data:', response.data);
      console.log('API 响应 success:', response.data.success);
      console.log('API 响应 data.data:', response.data.data);
      console.log('data.data 类型:', typeof response.data.data);
      console.log('data.data 是否为数组:', Array.isArray(response.data.data));

      if (response.data.success && response.data.data) {
        console.log('设置求购订单数据, 长度:', response.data.data.length);
        setPurchaseOrderItems(response.data.data);
        message.success(`成功获取 ${response.data.data.length} 条求购订单`);
      } else {
        console.log('API 响应格式不符合预期');
        message.warning('未获取到求购订单数据');
      }
    } catch (error: any) {
      console.error('获取求购列表失败:', error);
      message.error('获取求购列表失败: ' + (error.response?.data?.error || error.message));
    } finally {
      setLoading(false);
    }
  };

  // 获取商品求购信息
  const fetchPurchaseInfo = async (templateId: string) => {
    try {
      const response = await axios.post('/youpin/purchase/info', {
        template_id: templateId
      });
      return response.data;
    } catch (error: any) {
      console.error('获取求购信息失败:', error);
      return null;
    }
  };

  // 选择商品（获取求购列表）
  const handleSelectItem = async (item: SearchResult) => {
    setSelectedItem(item);
    setPurchaseOrderItems([]);

    // 先设置一个基本的 commodityDetail，确保界面能显示
    setCommodityDetail({
      TemplateId: item.id.toString(),
      TemplateHashName: item.commodityHashName || '',
      CommodityName: item.commodityName,
      IconUrl: item.iconUrl,
      Description: '',
      Category: 'CSGO',
      Rarity: item.rarity,
      Quality: item.quality,
      WearLevels: [],
      MarketSummary: {
        TotalMarketCount: 0,
        TotalPurchaseCount: 0,
        LowestPrice: parseFloat(item.price || '0'),
        HighestPurchase: 0
      }
    });

    // 获取求购信息并更新详细信息
    const purchaseInfo = await fetchPurchaseInfo(item.id.toString());

    if (purchaseInfo && purchaseInfo.templateInfo) {
      // 更新更详细的信息
      setCommodityDetail({
        TemplateId: item.id.toString(),
        TemplateHashName: purchaseInfo.templateInfo.templateHashName,
        CommodityName: purchaseInfo.templateInfo.commodityName,
        IconUrl: purchaseInfo.templateInfo.iconUrl,
        Description: '',
        Category: 'CSGO',
        Rarity: item.rarity,
        Quality: item.quality,
        WearLevels: [],
        MarketSummary: {
          TotalMarketCount: 0,
          TotalPurchaseCount: 0,
          LowestPrice: parseFloat(purchaseInfo.templateInfo.minSellPrice || '0'),
          HighestPurchase: parseFloat(purchaseInfo.templateInfo.maxPurchasePrice || '0')
        }
      });
    }

    // 获取求购列表
    await fetchPurchaseOrderList(item.id);
  };

  // 创建求购订单
  const handleCreatePurchaseOrder = () => {
    if (!selectedItem) {
      message.warning('请先选择商品');
      return;
    }
    setPurchaseModalVisible(true);
  };

  // 执行求购
  const executePurchaseOrder = async (values: any) => {
    if (!selectedItem || !commodityDetail) return;

    try {
      const response = await axios.post('/youpin/purchase', {
        template_id: selectedItem.id.toString(),
        template_hash_name: commodityDetail.TemplateHashName,
        commodity_name: commodityDetail.CommodityName,
        purchase_price: values.price,
        purchase_num: values.quantity,
        reference_price: commodityDetail.MarketSummary.LowestPrice.toString(),
        min_sell_price: commodityDetail.MarketSummary.LowestPrice.toString(),
        max_purchase_price: commodityDetail.MarketSummary.HighestPurchase.toString(),
        auto_received: values.autoReceived || false
      });

      message.success('求购订单创建成功！');
      setPurchaseModalVisible(false);
      form.resetFields();

      // 刷新求购订单列表
      await fetchPurchaseOrderList(selectedItem.id);
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
          <DollarOutlined /> 悠悠有品求购中心
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

        {/* 商品详情和求购列表 */}
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
                  <Statistic title="在售最低价" value={commodityDetail.MarketSummary.LowestPrice} prefix="¥" />
                  <Statistic title="最高求购价" value={commodityDetail.MarketSummary.HighestPurchase} prefix="¥" />
                  <Button type="primary" icon={<DollarOutlined />} onClick={handleCreatePurchaseOrder}>
                    发布求购
                  </Button>
                </Space>
              }
            >
              {/* 显示求购列表 */}
              <div>
                <Title level={4}>求购列表 ({purchaseOrderItems.length}条)</Title>
                <Spin spinning={loading}>
                  {purchaseOrderItems.length === 0 ? (
                    <div style={{ textAlign: 'center', padding: '40px', color: '#999' }}>
                      <InfoCircleOutlined style={{ fontSize: '24px', marginBottom: '8px' }} />
                      <div>暂无求购订单</div>
                    </div>
                  ) : (
                    <List
                      dataSource={purchaseOrderItems}
                      renderItem={(item: PurchaseOrderItem) => (
                          <List.Item>
                            <List.Item.Meta
                              avatar={<Image src={item.headPicUrl} width={48} height={48} style={{ borderRadius: '50%' }} />}
                              title={
                                <Space>
                                  <Text strong>¥{item.purchasePrice}</Text>
                                  <Tag color="green">剩余 {item.surplusQuantity} 件</Tag>
                                  {item.autoReceived === 1 && <Tag color="blue">自动收货</Tag>}
                                  {item.isNew === 1 && <Tag color="orange">新</Tag>}
                                </Space>
                              }
                              description={
                                <Space direction="vertical" size="small">
                                  <Text>求购者: {item.userName}</Text>
                                  {item.abradeText && <Text type="secondary">磨损: {item.abradeText}</Text>}
                                  {item.fadeText && <Text type="secondary">渐变: {item.fadeText}</Text>}
                                  {item.specialStyle && <Text type="secondary">特殊样式: {item.specialStyle}</Text>}
                                </Space>
                              }
                            />
                          </List.Item>
                      )}
                    />
                  )}
                </Spin>
              </div>
            </Card>
          ) : (
            <Card style={{ height: '600px', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
              <div style={{ textAlign: 'center', color: '#999' }}>
                <InfoCircleOutlined style={{ fontSize: '48px', marginBottom: '16px' }} />
                <div>请先搜索并选择一个饰品查看求购列表</div>
              </div>
            </Card>
          )}
        </Col>
      </Row>

      {/* 求购订单对话框 */}
      <Modal
        title="发布求购订单"
        visible={purchaseModalVisible}
        onCancel={() => setPurchaseModalVisible(false)}
        footer={null}
      >
        <Form
          form={form}
          onFinish={executePurchaseOrder}
          layout="vertical"
        >
          {commodityDetail && (
            <div style={{ marginBottom: '16px', padding: '12px', background: '#f5f5f5', borderRadius: '6px' }}>
              <Space>
                <Image src={commodityDetail.IconUrl} width={64} height={48} />
                <div>
                  <Text strong>{commodityDetail.CommodityName}</Text>
                  <br />
                  <Text type="secondary">
                    参考价: ¥{commodityDetail.MarketSummary.LowestPrice} - ¥{commodityDetail.MarketSummary.HighestPurchase}
                  </Text>
                </div>
              </Space>
            </div>
          )}

          <Form.Item
            label="求购单价"
            name="price"
            rules={[{ required: true, message: '请输入求购单价' }]}
            tooltip="建议价格不要低于在售最低价的80%，否则可能无人愿意出售"
          >
            <InputNumber
              style={{ width: '100%' }}
              placeholder="输入求购单价"
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
              max={999}
            />
          </Form.Item>

          <Form.Item
            name="autoReceived"
            valuePropName="checked"
            initialValue={true}
          >
            <Checkbox>
              开启自动收货（推荐，收到饰品后自动确认收货）
            </Checkbox>
          </Form.Item>

          <Form.Item>
            <Space>
              <Button type="primary" htmlType="submit">
                发布求购
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