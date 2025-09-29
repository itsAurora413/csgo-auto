import React, { useState, useEffect } from 'react';
import { 
  Card, 
  Tabs, 
  Button, 
  Input, 
  Form, 
  message, 
  List, 
  Badge, 
  Space, 
  Switch, 
  InputNumber,
  Select,
  Row,
  Col,
  Statistic,
  Alert,
  Modal,
  Table,
  Tag
} from 'antd';
import {
  ShopOutlined,
  UserOutlined,
  SettingOutlined,
  PlayCircleOutlined,
  AppstoreOutlined,
  UnorderedListOutlined,
  PlusOutlined,
  DeleteOutlined,
  ReloadOutlined,
  DollarOutlined,
  CheckOutlined
} from '@ant-design/icons';

const { TabPane } = Tabs;
const { TextArea } = Input;
const { Option } = Select;

interface YouPinAccount {
  id: number;
  nickname: string;
  balance: number;
  purchase_balance: number;
  is_active: boolean;
}

interface YouPinConfig {
  auto_sell_enabled: boolean;
  take_profile_enabled: boolean;
  use_price_adjustment: boolean;
  sell_item_names: string;
  blacklist_words: string;
  max_sale_price: number;
  take_profile_ratio: number;
  price_adjustment_threshold: number;
  run_time: string;
  interval: number;
}

const YouPin: React.FC = () => {
  const [accounts, setAccounts] = useState<YouPinAccount[]>([]);
  const [config, setConfig] = useState<YouPinConfig | null>(null);
  const [inventory, setInventory] = useState<any[]>([]);
  const [orders, setOrders] = useState<any[]>([]);
  const [sellList, setSellList] = useState<any[]>([]);
  const [loading, setLoading] = useState(false);
  const [activeTab, setActiveTab] = useState('accounts');
  const [form] = Form.useForm();
  const [configForm] = Form.useForm();
  const [selectedItems, setSelectedItems] = useState<number[]>([]);
  const [sellModalVisible, setSellModalVisible] = useState(false);
  const [currentSellItem, setCurrentSellItem] = useState<any>(null);
  // Steam凭据
  const [steamCreds, setSteamCreds] = useState<any>({
    shared_secret: '',
    identity_secret: '',
    steam_username: '',
    steam_password: '',
    api_key: ''
  });
  const [priceModalVisible, setPriceModalVisible] = useState(false);
  const [priceTargetItem, setPriceTargetItem] = useState<any>(null);
  const [newPrice, setNewPrice] = useState<number | null>(null);

  // API请求封装
  const apiRequest = async (url: string, options: RequestInit = {}) => {
    try {
      const response = await fetch(`/api/v1/youpin${url}`, {
        headers: {
          'Content-Type': 'application/json',
          ...options.headers
        },
        ...options
      });

      const data = await response.json();
      if (!response.ok) {
        throw new Error(data.error || `HTTP ${response.status}`);
      }
      return data;
    } catch (error) {
      console.error('API请求失败:', error);
      throw error;
    }
  };

  // 加载账户列表
  const loadAccounts = async () => {
    try {
      setLoading(true);
      const data = await apiRequest('/accounts');
      setAccounts(data.accounts || []);
    } catch (error: any) {
      message.error(`加载账户失败: ${error.message}`);
    } finally {
      setLoading(false);
    }
  };

  // 加载Steam凭据
  const loadSteamCreds = async () => {
    try {
      const resp = await fetch('/api/v1/steam/credentials');
      const data = await resp.json();
      setSteamCreds(data);
    } catch (e) {
      // ignore
    }
  };

  const saveSteamCreds = async (values: any) => {
    try {
      const resp = await fetch('/api/v1/steam/credentials', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(values)
      });
      if (!resp.ok) {
        const err = await resp.json();
        throw new Error(err.error || '保存失败');
      }
      // 保存成功后尝试登录并获取 API Key
      const loginResp = await fetch('/api/v1/steam/login', { method: 'POST' });
      const loginData = await loginResp.json();
      if (!loginResp.ok || !loginData.api_key) {
        throw new Error(loginData.error || '获取 API Key 失败，请检查输入');
      }
      message.success('凭据有效，已获取 API Key');
      setSteamCreds((prev: any) => ({ ...prev, api_key: loginData.api_key }));
    } catch (e: any) {
      message.error('保存失败: ' + e.message);
    }
  };

  // 加载配置
  const loadConfig = async () => {
    try {
      const data = await apiRequest('/config');
      setConfig(data);
      
      // 填充表单
      configForm.setFieldsValue({
        ...data,
        sell_item_names: JSON.parse(data.sell_item_names || '[]').join('\n'),
        blacklist_words: JSON.parse(data.blacklist_words || '[]').join('\n'),
        take_profile_ratio: (data.take_profile_ratio || 0) * 100
      });
    } catch (error: any) {
      message.error(`加载配置失败: ${error.message}`);
    }
  };

  // 添加账户
  const addAccount = async (values: { token: string }) => {
    try {
      setLoading(true);
      await apiRequest('/accounts', {
        method: 'POST',
        body: JSON.stringify(values)
      });
      message.success('账户添加成功！');
      form.resetFields();
      loadAccounts();
    } catch (error: any) {
      message.error(`添加账户失败: ${error.message}`);
    } finally {
      setLoading(false);
    }
  };

  // 删除账户
  const deleteAccount = async (accountId: number) => {
    try {
      await apiRequest(`/accounts/${accountId}`, {
        method: 'DELETE'
      });
      message.success('账户删除成功！');
      loadAccounts();
    } catch (error: any) {
      message.error(`删除账户失败: ${error.message}`);
    }
  };

  // 保存配置
  const saveConfig = async (values: any) => {
    try {
      setLoading(true);
      
      const sellItems = values.sell_item_names ? 
        values.sell_item_names.split('\n').filter((item: string) => item.trim()) : [];
      const blacklistWords = values.blacklist_words ? 
        values.blacklist_words.split('\n').filter((word: string) => word.trim()) : [];

      const configData = {
        ...values,
        sell_item_names: JSON.stringify(sellItems),
        blacklist_words: JSON.stringify(blacklistWords),
        take_profile_ratio: (values.take_profile_ratio || 0) / 100
      };

      await apiRequest('/config', {
        method: 'PUT',
        body: JSON.stringify(configData)
      });
      
      message.success('配置保存成功！');
      setConfig(configData);
    } catch (error: any) {
      message.error(`保存配置失败: ${error.message}`);
    } finally {
      setLoading(false);
    }
  };

  // 启动功能
  const startFunction = async (functionName: string, displayName: string) => {
    try {
      setLoading(true);
      await apiRequest(`/${functionName}/start`, {
        method: 'POST'
      });
      message.success(`${displayName}已启动！`);
    } catch (error: any) {
      message.error(`启动${displayName}失败: ${error.message}`);
    } finally {
      setLoading(false);
    }
  };

  // 加载库存
  const loadInventory = async () => {
    try {
      setLoading(true);
      const data = await apiRequest('/inventory');
      setInventory(data.items || []);
      message.success(`库存数据加载成功！共${data.items?.length || 0}件物品`);
    } catch (error: any) {
      message.error(`加载库存失败: ${error.message}`);
    } finally {
      setLoading(false);
    }
  };

  // 加载订单
  const loadOrders = async () => {
    try {
      setLoading(true);
      const data = await apiRequest('/orders');
      setOrders(data.orders || []);
      message.success(`订单数据加载成功！共${data.orders?.length || 0}条订单`);
    } catch (error: any) {
      message.error(`加载订单失败: ${error.message}`);
    } finally {
      setLoading(false);
    }
  };

  // 加载在售列表
  const loadSellList = async () => {
    try {
      setLoading(true);
      const data = await apiRequest('/sell-list');
      setSellList(data.data || []);
      message.success(`在售列表加载成功！共${data.count || 0}件在售物品`);
    } catch (error: any) {
      message.error(`加载在售列表失败: ${error.message}`);
    } finally {
      setLoading(false);
    }
  };

  // 改价（单个）
  const changePrice = async (record: any, price: number, remark: string = '') => {
    try {
      setLoading(true);
      const commodityId = String(record.id || record.commodityId || record.CommodityId);
      await apiRequest('/change-price', {
        method: 'POST',
        body: JSON.stringify({
          commodities: [{ commodity_id: commodityId, price, remark }]
        })
      });
      message.success('改价成功');
      await loadSellList();
    } catch (error: any) {
      message.error(`改价失败: ${error.message}`);
    } finally {
      setLoading(false);
    }
  };

  // 下架（单个）
  const offSale = async (record: any) => {
    try {
      setLoading(true);
      const commodityId = String(record.id || record.commodityId || record.CommodityId);
      await apiRequest('/off-sale', {
        method: 'POST',
        body: JSON.stringify({ commodity_ids: [commodityId] })
      });
      message.success('下架成功');
      await loadSellList();
    } catch (error: any) {
      message.error(`下架失败: ${error.message}`);
    } finally {
      setLoading(false);
    }
  };

  // 手动上架单个物品
  const sellItem = async (item: any, price: number, remark: string = '') => {
    try {
      setLoading(true);

      // 找到物品在库存中的索引
      const itemIndex = inventory.findIndex(invItem => invItem.SteamAssetID === item.SteamAssetID);
      if (itemIndex === -1) {
        message.error('未找到物品在库存中的位置');
        return;
      }

      const data = await apiRequest('/sell-by-index', {
        method: 'POST',
        body: JSON.stringify({
          item_indexes: [itemIndex],
          price: price,
          remark: remark
        })
      });

      if (data.success_count > 0) {
        message.success(`物品上架成功！`);
        loadInventory(); // 重新加载库存
      } else {
        message.error(`上架失败: ${data.message || '未知错误'}`);
      }
    } catch (error: any) {
      message.error(`上架失败: ${error.message}`);
    } finally {
      setLoading(false);
    }
  };

  // 批量上架选中物品
  const sellSelectedItems = async (price: number, remark: string = '') => {
    try {
      setLoading(true);
      const data = await apiRequest('/sell-by-index', {
        method: 'POST',
        body: JSON.stringify({
          item_indexes: selectedItems,
          price: price,
          remark: remark
        })
      });

      if (data.success_count > 0) {
        message.success(`成功上架 ${data.success_count} 件物品！`);
        if (data.fail_count > 0) {
          message.warning(`有 ${data.fail_count} 件物品上架失败`);
        }
        setSelectedItems([]);
        loadInventory(); // 重新加载库存
      } else {
        message.error(`批量上架失败: ${data.message || '所有物品都上架失败'}`);
      }
    } catch (error: any) {
      message.error(`批量上架失败: ${error.message}`);
    } finally {
      setLoading(false);
    }
  };

  // 打开上架弹窗
  const openSellModal = (item: any) => {
    setCurrentSellItem(item);
    setSellModalVisible(true);
  };

  // 处理批量选择
  const handleSelectItem = (index: number, checked: boolean) => {
    if (checked) {
      setSelectedItems([...selectedItems, index]);
    } else {
      setSelectedItems(selectedItems.filter(i => i !== index));
    }
  };

  // 全选/取消全选
  const handleSelectAll = (checked: boolean) => {
    if (checked) {
      setSelectedItems(inventory.map((_, index) => index));
    } else {
      setSelectedItems([]);
    }
  };

  useEffect(() => {
    loadAccounts();
    loadConfig();
    loadSteamCreds();
  }, []);

  return (
    <div style={{ padding: '24px' }}>
      <Card 
        title={
          <Space>
            <ShopOutlined />
            悠悠有品自动交易管理
          </Space>
        }
        style={{ marginBottom: '24px' }}
      >
        {/* removed marketing/info banner for self-use */}

        <Tabs activeKey={activeTab} onChange={setActiveTab}>
          {/* 账户管理 */}
          <TabPane 
            tab={
              <Space>
                <UserOutlined />
                账户管理
              </Space>
            } 
            key="accounts"
          >
            <Row gutter={[24, 24]}>
              <Col xs={24} lg={12}>
                <Card title="添加悠悠有品账户" size="small">
                  <Form form={form} onFinish={addAccount} layout="vertical">
                    <Form.Item
                      name="token"
                      label="悠悠有品Token"
                      rules={[{ required: true, message: '请输入Token' }]}
                    >
                      <Input.Password placeholder="请输入您的悠悠有品JWT Token" />
                    </Form.Item>
                    <Form.Item>
                      <Space>
                        <Button type="primary" htmlType="submit" loading={loading}>
                          <PlusOutlined />
                          添加账户
                        </Button>
                        <Button 
                          onClick={() => {
                            Modal.info({
                              title: 'Token获取教程',
                              width: 600,
                              content: (
                                <div>
                                  <p><strong>如何获取Token：</strong></p>
                                  <ol>
                                    <li>登录 youpin898.com</li>
                                    <li>按 F12 打开开发者工具</li>
                                    <li>切换到 Network 标签页</li>
                                    <li>刷新页面或进行任何操作</li>
                                    <li>找到 api.youpin898.com 的请求</li>
                                    <li>复制请求头中 authorization 字段的值</li>
                                  </ol>
                                </div>
                              )
                            });
                          }}
                        >
                          获取教程
                        </Button>
                      </Space>
                    </Form.Item>
                  </Form>
                </Card>
              </Col>

              <Col xs={24} lg={12}>
                <Card 
                  title="账户列表" 
                                   extra={
                    <Button 
                      icon={<ReloadOutlined />} 
                      onClick={loadAccounts}
                      loading={loading}
                    >
                      刷新
                    </Button>
                  }
                >
                  <List
                    dataSource={accounts}
                    renderItem={(account) => (
                      <List.Item
                        actions={[
                          <Button 
                            danger 
                                                       icon={<DeleteOutlined />}
                            onClick={() => {
                              Modal.confirm({
                                title: '确定删除此账户？',
                                onOk: () => deleteAccount(account.id)
                              });
                            }}
                          >
                            删除
                          </Button>
                        ]}
                      >
                        <List.Item.Meta
                          title={
                            <Space>
                              <Badge 
                                status={account.is_active ? 'success' : 'error'} 
                              />
                              {account.nickname || '未获取昵称'}
                            </Space>
                          }
                          description={
                            <div>
                              <div>钱包余额: ¥{(account as any).balance || '0.00'}</div>
                              <div>求购余额: ¥{(account as any).purchase_balance || '0.00'}</div>
                            </div>
                          }
                        />
                      </List.Item>
                    )}
                    locale={{ emptyText: '暂无账户，请先添加悠悠有品账户' }}
                  />
                </Card>
              </Col>
            </Row>
          </TabPane>

          {/* Steam 配置 */}
          <TabPane
            tab={
              <Space>
                <SettingOutlined />
                Steam 配置
              </Space>
            }
            key="steam"
          >
            <Card title="Steam 验证参数（参考Steamauto方式）" style={{ maxWidth: 720 }}>
              <Form
                layout="vertical"
                initialValues={steamCreds}
                onFinish={saveSteamCreds}
              >
                <Form.Item label="shared_secret" name="shared_secret" rules={[{ required: true, message: '请输入 shared_secret' }]}>
                  <Input placeholder="shared_secret" />
                </Form.Item>
                <Form.Item label="identity_secret" name="identity_secret" rules={[{ required: true, message: '请输入 identity_secret' }]}>
                  <Input placeholder="identity_secret" />
                </Form.Item>
                <Form.Item label="steam_username" name="steam_username" rules={[{ required: true, message: '请输入用户名' }]}>
                  <Input placeholder="Steam 用户名" />
                </Form.Item>
                <Form.Item label="steam_password" name="steam_password" rules={[{ required: true, message: '请输入密码' }]}>
                  <Input.Password placeholder="Steam 密码" autoComplete="new-password" />
                </Form.Item>
                <Form.Item>
                  <Space>
                    <Button type="primary" htmlType="submit">保存</Button>
                    <Button onClick={loadSteamCreds}>刷新</Button>
                  </Space>
                </Form.Item>
              </Form>
              {steamCreds.api_key && (
                <div style={{ marginTop: 8 }}>
                  <span style={{ marginRight: 8 }}>API Key:</span>
                  <code>{steamCreds.api_key}</code>
                </div>
              )}
              {/* removed explanatory note for self-use */}
            </Card>
          </TabPane>

          {/* 交易配置 */}
          <TabPane 
            tab={
              <Space>
                <SettingOutlined />
                交易配置
              </Space>
            } 
            key="config"
          >
            <Card title="自动交易配置">
              <Form 
                form={configForm} 
                onFinish={saveConfig} 
                layout="vertical"
                initialValues={config || {}}
              >
                <Row gutter={[16, 16]}>
                  <Col xs={24} sm={12}>
                    <Form.Item name="auto_sell_enabled" valuePropName="checked">
                      <Space>
                        <Switch />
                        启用自动上架
                      </Space>
                    </Form.Item>
                  </Col>
                  <Col xs={24} sm={12}>
                    <Form.Item name="take_profile_enabled" valuePropName="checked">
                      <Space>
                        <Switch />
                        启用止盈功能
                      </Space>
                    </Form.Item>
                  </Col>
                </Row>

                <Form.Item
                  name="sell_item_names"
                  label="上架物品名称（每行一个）"
                >
                  <TextArea rows={4} placeholder="AK-47&#10;AWP&#10;Glock-18" />
                </Form.Item>

                <Form.Item
                  name="blacklist_words"
                  label="黑名单关键词（每行一个）"
                >
                  <TextArea rows={3} placeholder="Battle-Scarred&#10;Well-Worn" />
                </Form.Item>

                <Row gutter={[16, 16]}>
                  <Col xs={24} sm={12}>
                    <Form.Item
                      name="max_sale_price"
                      label="最高上架价格"
                    >
                      <InputNumber 
                        style={{ width: '100%' }}
                        placeholder="1000"
                        min={0}
                        step={0.01}
                        addonAfter="元"
                      />
                    </Form.Item>
                  </Col>
                  <Col xs={24} sm={12}>
                    <Form.Item
                      name="take_profile_ratio"
                      label="止盈率"
                    >
                      <InputNumber 
                        style={{ width: '100%' }}
                        placeholder="15"
                        min={0}
                        max={100}
                        step={0.1}
                        addonAfter="%"
                      />
                    </Form.Item>
                  </Col>
                </Row>

                <Row gutter={[16, 16]}>
                  <Col xs={24} sm={12}>
                    <Form.Item name="use_price_adjustment" valuePropName="checked">
                      <Space>
                        <Switch />
                        启用价格调整
                      </Space>
                    </Form.Item>
                  </Col>
                  <Col xs={24} sm={12}>
                    <Form.Item
                      name="price_adjustment_threshold"
                      label="价格调整阈值"
                    >
                      <InputNumber 
                        style={{ width: '100%' }}
                        placeholder="1.0"
                        min={0}
                        step={0.01}
                        addonAfter="元"
                      />
                    </Form.Item>
                  </Col>
                </Row>

                <Row gutter={[16, 16]}>
                  <Col xs={24} sm={12}>
                    <Form.Item
                      name="run_time"
                      label="定时运行时间"
                    >
                      <Input type="time" />
                    </Form.Item>
                  </Col>
                  <Col xs={24} sm={12}>
                    <Form.Item
                      name="interval"
                      label="改价间隔"
                    >
                      <InputNumber 
                        style={{ width: '100%' }}
                        placeholder="60"
                        min={5}
                        addonAfter="分钟"
                      />
                    </Form.Item>
                  </Col>
                </Row>

                <Form.Item>
                  <Space>
                    <Button type="primary" htmlType="submit" loading={loading}>
                      保存配置
                    </Button>
                    <Button onClick={loadConfig}>
                      重置
                    </Button>
                  </Space>
                </Form.Item>
              </Form>
            </Card>
          </TabPane>

          {/* 自动交易 */}
          <TabPane 
            tab={
              <Space>
                <PlayCircleOutlined />
                自动交易
              </Space>
            } 
            key="trading"
          >
            <Row gutter={[24, 24]}>
              <Col xs={24} sm={8}>
                <Card>
                  <Statistic
                    title="交易状态"
                    value="未启动"
                    prefix={<Badge status="default" />}
                  />
                </Card>
              </Col>
              <Col xs={24} sm={8}>
                <Card>
                  <Statistic
                    title="今日交易"
                    value={0}
                    suffix="笔"
                  />
                </Card>
              </Col>
              <Col xs={24} sm={8}>
                <Card>
                  <Statistic
                    title="总收益"
                    value={0}
                    precision={2}
                    prefix="¥"
                  />
                </Card>
              </Col>
            </Row>

            <Card title="交易控制" style={{ marginTop: '24px' }}>
              <Space wrap>
                <Button 
                  type="primary" 
                  loading={loading}
                  onClick={() => startFunction('auto-sell', '自动上架')}
                >
                  启动自动上架
                </Button>
                <Button 
                  type="primary" 
                  loading={loading}
                  onClick={() => startFunction('auto-change-price', '自动改价')}
                >
                  启动自动改价
                </Button>
                <Button 
                  type="primary" 
                  loading={loading}
                  onClick={() => startFunction('auto-accept-offer', '自动收货')}
                >
                  启动自动收货
                </Button>
              </Space>
            </Card>
          </TabPane>

          {/* 库存管理 */}
          <TabPane 
            tab={
              <Space>
                <AppstoreOutlined />
                库存管理
              </Space>
            } 
            key="inventory"
          >
            <Card
              title="库存管理"
              extra={
                <Space>
                  {selectedItems.length > 0 && (
                    <Button
                      type="default"
                      onClick={() => {
                        Modal.confirm({
                          title: '批量上架确认',
                          content: (
                            <div>
                              <p>您选择了 {selectedItems.length} 件物品</p>
                              <Form layout="vertical">
                                <Form.Item label="上架价格" name="price">
                                  <InputNumber
                                    style={{width: '100%'}}
                                    placeholder="请输入上架价格"
                                    min={0.01}
                                    step={0.01}
                                    addonAfter="元"
                                  />
                                </Form.Item>
                                <Form.Item label="备注" name="remark">
                                  <Input placeholder="可选备注" />
                                </Form.Item>
                              </Form>
                            </div>
                          ),
                          onOk: () => {
                            const price = 10; // 这里应该从表单获取，简化处理
                            sellSelectedItems(price);
                          }
                        });
                      }}
                    >
                      <DollarOutlined />
                      批量上架 ({selectedItems.length})
                    </Button>
                  )}
                  <Button type="primary" loading={loading} onClick={loadInventory}>
                    <ReloadOutlined />
                    刷新库存
                  </Button>
                </Space>
              }
            >
              {inventory.length > 0 ? (
                <div>
                  <div style={{ marginBottom: 16, padding: '12px', backgroundColor: '#f5f5f5', borderRadius: 6 }}>
                    <Space>
                      <Button
                        type="link"
                                               onClick={() => handleSelectAll(selectedItems.length !== inventory.length)}
                      >
                        {selectedItems.length === inventory.length ? '取消全选' : '全选'}
                      </Button>
                      <span style={{ color: '#666' }}>
                        已选择 {selectedItems.length} / {inventory.length} 件物品
                      </span>
                    </Space>
                  </div>
                  <List
                    dataSource={inventory}
                    renderItem={(item: any, index: number) => (
                      <List.Item
                        actions={[
                          <Button
                            type="primary"
                                                       icon={<DollarOutlined />}
                            disabled={!item.Tradable}
                            onClick={() => openSellModal(item)}
                          >
                            上架
                          </Button>
                        ]}
                      >
                        <div style={{ display: 'flex', alignItems: 'flex-start', width: '100%' }}>
                          <div style={{ marginRight: 12, paddingTop: 8 }}>
                            <input
                              type="checkbox"
                              checked={selectedItems.includes(index)}
                              onChange={(e) => handleSelectItem(index, e.target.checked)}
                              disabled={!item.Tradable}
                            />
                          </div>
                          <List.Item.Meta
                            avatar={item.TemplateInfo?.IconUrl ?
                              <img
                                src={item.TemplateInfo.IconUrl}
                                alt={item.ShotName}
                                style={{width: 48, height: 48, borderRadius: 4}}
                              /> : null
                            }
                            title={
                              <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                                <span>{item.ShotName || item.TemplateInfo?.CommodityName || '未知物品'}</span>
                                {!item.Tradable && (
                                  <Tag color="red">不可交易</Tag>
                                )}
                                <Tag color="blue">#{index}</Tag>
                              </div>
                            }
                            description={
                              <div>
                                <div>市场价格: ¥{item.TemplateInfo?.MarkPrice || 0}</div>
                                <div>账户: {item.account_nickname || 'N/A'}</div>
                                <div>可交易: {item.Tradable ? '是' : '否'}</div>
                                {item.Stickers && item.Stickers.length > 0 && (
                                  <div style={{marginTop: 4}}>
                                    <span style={{color: '#1890ff'}}>
                                      贴纸: {item.Stickers.slice(0, 2).map((s: any) => s.Name).join(', ')}
                                      {item.Stickers.length > 2 && ` 等${item.Stickers.length}个`}
                                    </span>
                                  </div>
                                )}
                              </div>
                            }
                          />
                        </div>
                      </List.Item>
                    )}
                    pagination={{
                      pageSize: 10,
                      showSizeChanger: true,
                      showQuickJumper: true,
                      showTotal: (total) => `共 ${total} 件物品`
                    }}
                  />
                </div>
              ) : (
                <Alert
                  message="暂无库存数据"
                  description="点击刷新库存按钮加载数据"
                  type="info"
                  style={{ marginBottom: '16px' }}
                />
              )}
            </Card>
          </TabPane>

          {/* 订单查询 */}
          <TabPane 
            tab={
              <Space>
                <UnorderedListOutlined />
                订单查询
              </Space>
            } 
            key="orders"
          >
            <Card 
              title="交易订单"
              extra={
                <Button type="primary" loading={loading} onClick={loadOrders}>
                  <ReloadOutlined />
                  刷新订单
                </Button>
              }
            >
              {orders.length > 0 ? (
                <List
                  dataSource={orders}
                  renderItem={(order: any) => (
                    <List.Item>
                      <List.Item.Meta
                        title={`订单 #${order.id || '未知'}`}
                        description={
                          <Space>
                            <span>物品: {order.item_name || '未知物品'}</span>
                            <span>金额: ¥{order.amount || '0.00'}</span>
                            <span>状态: {order.status || '未知'}</span>
                            <span>时间: {order.created_at ? new Date(order.created_at).toLocaleString() : '未知'}</span>
                          </Space>
                        }
                      />
                    </List.Item>
                  )}
                  pagination={{
                    pageSize: 10,
                    showSizeChanger: true,
                    showQuickJumper: true,
                    showTotal: (total) => `共 ${total} 条订单`
                  }}
                />
              ) : (
                <Alert
                  message="暂无订单数据"
                  description="点击刷新订单按钮加载数据"
                  type="info"
                  style={{ marginBottom: '16px' }}
                />
              )}
            </Card>
          </TabPane>

          {/* 在售列表标签页 */}
          <TabPane tab="在售列表" key="sellList">
            <Card
              title="在售物品列表"
              extra={
                <Button
                  type="primary"
                  icon={<ReloadOutlined />}
                  onClick={loadSellList}
                  loading={loading}
                >
                  刷新在售列表
                </Button>
              }
            >
              {sellList.length > 0 ? (
                <Table
                  dataSource={sellList}
                  rowKey={(record) => record.id || record.steamAssetId}
                  pagination={{
                    pageSize: 10,
                    showSizeChanger: true,
                    showQuickJumper: true,
                    showTotal: (total) => `共 ${total} 件在售物品`
                  }}
                  columns={[
                    {
                      title: '物品图片',
                      dataIndex: 'imgUrl',
                      key: 'image',
                      width: 80,
                      render: (imgUrl: string) => (
                        <img
                          src={imgUrl || '/placeholder.png'}
                          alt="物品图片"
                          style={{ width: 60, height: 60, objectFit: 'cover' }}
                        />
                      ),
                    },
                    {
                      title: '物品名称',
                      dataIndex: 'name',
                      key: 'name',
                      ellipsis: true,
                      render: (text: string, record: any) => (
                        <div>
                          <div style={{ fontWeight: 'bold' }}>{text}</div>
                          <div style={{ color: '#666', fontSize: '12px' }}>
                            {record.commodityHashName}
                          </div>
                        </div>
                      ),
                    },
                    {
                      title: '在售价格',
                      dataIndex: 'sellAmountDesc',
                      key: 'price',
                      width: 100,
                      render: (sellAmountDesc: string) => (
                        <span style={{ color: '#f56a00', fontWeight: 'bold' }}>
                          {sellAmountDesc || '¥0.00'}
                        </span>
                      ),
                    },
                    {
                      title: '参考价格',
                      dataIndex: 'referencePrice',
                      key: 'marketPrice',
                      width: 100,
                      render: (referencePrice: string) => (
                        <span style={{ color: '#52c41a' }}>
                          {referencePrice || '¥0.00'}
                        </span>
                      ),
                    },
                    {
                      title: '状态',
                      dataIndex: 'status',
                      key: 'status',
                      width: 100,
                      render: (status: number) => {
                        const statusMap: Record<number, { text: string; color: string }> = {
                          20: { text: '在售', color: 'green' },
                          10: { text: '待审核', color: 'orange' },
                          0: { text: '下架', color: 'red' },
                        };
                        const statusInfo = statusMap[status] || { text: '未知', color: 'gray' };
                        return <Tag color={statusInfo.color}>{statusInfo.text}</Tag>;
                      },
                    },
                    {
                      title: '磨损',
                      dataIndex: 'exteriorName',
                      key: 'exterior',
                      width: 80,
                      render: (exteriorName: string) => (
                        <Tag color="blue">{exteriorName || '未知'}</Tag>
                      ),
                    },
                    {
                      title: '操作',
                      key: 'action',
                      width: 120,
                      render: (_: any, record: any) => (
                        <Space>
                          <Button
                            type="primary"
                            size="small"
                            onClick={() => {
                              setPriceTargetItem(record);
                              setNewPrice(undefined as any);
                              setPriceModalVisible(true);
                            }}
                          >
                            改价
                          </Button>
                          <Button
                            danger
                            size="small"
                            onClick={() => {
                              Modal.confirm({
                                title: '确认下架该商品？',
                                content: `${record.name || record.commodityName || ''}`,
                                onOk: () => offSale(record)
                              });
                            }}
                          >
                            下架
                          </Button>
                        </Space>
                      ),
                    },
                  ]}
                />
              ) : (
                <Alert
                  message="暂无在售物品"
                  description="点击刷新在售列表按钮加载数据"
                  type="info"
                  style={{ marginBottom: '16px' }}
                />
              )}
            </Card>
          </TabPane>
        </Tabs>
      </Card>

      {/* 上架弹窗 */}
      <Modal
        title="上架物品"
        open={sellModalVisible}
        onCancel={() => {
          setSellModalVisible(false);
          setCurrentSellItem(null);
        }}
        footer={null}
        width={500}
      >
        {currentSellItem && (
          <div>
            <div style={{ marginBottom: 16, padding: 16, backgroundColor: '#f5f5f5', borderRadius: 6 }}>
              <div style={{ display: 'flex', alignItems: 'center', gap: 12 }}>
                {currentSellItem.TemplateInfo?.IconUrl && (
                  <img
                    src={currentSellItem.TemplateInfo.IconUrl}
                    alt={currentSellItem.ShotName}
                    style={{ width: 64, height: 64, borderRadius: 4 }}
                  />
                )}
                <div>
                  <h4 style={{ margin: 0, marginBottom: 4 }}>
                    {currentSellItem.ShotName || currentSellItem.TemplateInfo?.CommodityName || '未知物品'}
                  </h4>
                  <p style={{ margin: 0, color: '#666' }}>
                    市场价格: ¥{currentSellItem.TemplateInfo?.MarkPrice || 0}
                  </p>
                  <p style={{ margin: 0, color: '#666' }}>
                    账户: {currentSellItem.account_nickname || 'N/A'}
                  </p>
                </div>
              </div>
            </div>

            <Form
              layout="vertical"
              onFinish={(values) => {
                sellItem(currentSellItem, values.price, values.remark || '');
                setSellModalVisible(false);
                setCurrentSellItem(null);
              }}
              initialValues={{
                price: currentSellItem.TemplateInfo?.MarkPrice || 1
              }}
            >
              <Form.Item
                name="price"
                label="上架价格"
                rules={[
                  { required: true, message: '请输入上架价格' },
                  { type: 'number', min: 0.01, message: '价格不能小于0.01元' }
                ]}
              >
                <InputNumber
                  style={{ width: '100%' }}
                  placeholder="请输入上架价格"
                  min={0.01}
                  step={0.01}
                  addonAfter="元"
                />
              </Form.Item>

              <Form.Item
                name="remark"
                label="备注（可选）"
              >
                <Input placeholder="可选备注信息" />
              </Form.Item>

              <Form.Item>
                <Space>
                  <Button type="primary" htmlType="submit" loading={loading}>
                    <CheckOutlined />
                    确认上架
                  </Button>
                  <Button
                    onClick={() => {
                      setSellModalVisible(false);
                      setCurrentSellItem(null);
                    }}
                  >
                    取消
                  </Button>
                </Space>
              </Form.Item>
            </Form>
          </div>
        )}
      </Modal>
      {/* 改价弹窗 */}
      <Modal
        title="修改在售价格"
        open={priceModalVisible}
        onCancel={() => {
          setPriceModalVisible(false);
          setPriceTargetItem(null);
          setNewPrice(null);
        }}
        onOk={() => {
          if (!priceTargetItem || newPrice == null) {
            message.warning('请输入价格');
            return;
          }
          changePrice(priceTargetItem, newPrice);
          setPriceModalVisible(false);
        }}
        okText="确认改价"
        cancelText="取消"
      >
        <div>
          <div style={{ marginBottom: 12 }}>
            物品：{priceTargetItem?.name || priceTargetItem?.commodityName || '未知'}
          </div>
          <div>
            新价格：
            <InputNumber
              min={0}
              precision={2}
              value={newPrice as number | null}
              onChange={(v) => setNewPrice((v as number) ?? null)}
            />
          </div>
        </div>
      </Modal>
    </div>
  );
};

export default YouPin;
