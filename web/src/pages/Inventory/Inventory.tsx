import React, { useState, useEffect } from 'react';
import {
  Card,
  Table,
  Button,
  Form,
  message,
  Modal,
  Tag,
  Space,
  Checkbox,
  InputNumber,
  Row,
  Col,
  Spin,
  Alert,
} from 'antd';
import {
  ReloadOutlined,
  ShopOutlined,
  DollarOutlined,
  CheckOutlined,
} from '@ant-design/icons';
import axios from 'axios';

interface InventoryItem {
  item_asset_id: string;
  template_id: number;
  template_name: string;
  template_hash_name: string;
  item_img_url: string;
  exterior_name: string;
  tradable: number;
  marketable: number;
  asset_status: number;
  market_price: string;
  market_min_price: string;
  commodity_status: string;
  commodity_price: string;
}

interface PriceInfo {
  template_id: number;
  min_sell_price: string;
  suggested_price: string;
}

const Inventory: React.FC = () => {
  const [inventory, setInventory] = useState<InventoryItem[]>([]);
  const [selectedRows, setSelectedRows] = useState<string[]>([]);
  const [loading, setLoading] = useState(false);
  const [steamID] = useState<string>('76561199078507841');
  // 76561198150114577
  const [shelfModalVisible, setShelfModalVisible] = useState(false);
  const [currentItem, setCurrentItem] = useState<InventoryItem | null>(null);
  const [shelfPrice, setShelfPrice] = useState<string>('');
  const [priceInfo, setPriceInfo] = useState<PriceInfo | null>(null);
  const [batchShelfModalVisible, setBatchShelfModalVisible] = useState(false);
  const [batchUnifiedPrice, setBatchUnifiedPrice] = useState<string>('');
  const [priceLoading, setPriceLoading] = useState(false);

  // 加载库存数据
  const loadInventory = async () => {
    try {
      setLoading(true);
      const response = await axios.get(
        `/youpin/inventory/steam-data?steam_id=${steamID}`
      );
      if (response.data.success) {
        setInventory(response.data.data || []);
        message.success(`成功加载 ${response.data.count} 件物品`);
        setSelectedRows([]);
      } else {
        message.error(response.data.error || '加载库存失败');
      }
    } catch (error: any) {
      message.error('加载库存失败: ' + (error.response?.data?.error || error.message));
    } finally {
      setLoading(false);
    }
  };

  // 页面加载时直接调用库存数据
  useEffect(() => {
    loadInventory();
  }, [steamID]);

  // 获取商品价格信息
  const fetchPriceInfo = async (templateId: number): Promise<PriceInfo | null> => {
    try {
      setPriceLoading(true);
      const response = await axios.post(
        `/youpin/inventory/commodity-price-info`,
        { template_ids: [templateId] }
      );
      if (response.data.success && response.data.data.length > 0) {
        const priceData = response.data.data[0];
        return {
          template_id: priceData.template_id,
          min_sell_price: priceData.min_sell_price,
          suggested_price: priceData.suggested_price,
        };
      }
      return null;
    } catch (error: any) {
      message.error('获取价格信息失败: ' + error.message);
      return null;
    } finally {
      setPriceLoading(false);
    }
  };

  // 打开单个上架弹窗
  const openShelfModal = async (item: InventoryItem) => {
    if (item.tradable === 0) {
      message.error('该物品不可交易');
      return;
    }

    setCurrentItem(item);
    setPriceInfo(null);
    setShelfPrice('');

    // 获取价格信息和建议价格
    const price = await fetchPriceInfo(item.template_id);
    if (price) {
      setPriceInfo(price);
      setShelfPrice(price.suggested_price);
    } else {
      setShelfPrice('1.00');
    }

    setShelfModalVisible(true);
  };

  // 执行单个上架
  const handleShelfItem = async () => {
    if (!currentItem || !shelfPrice) {
      message.error('请输入上架价格');
      return;
    }

    try {
      setLoading(true);
      const response = await axios.post(
        `/youpin/inventory/on-shelf-single`,
        {
          steam_id: steamID,
          item_asset_id: currentItem.item_asset_id,
          template_id: currentItem.template_id,
          price: shelfPrice,
        }
      );

      if (response.data.success) {
        message.success('物品上架成功！');
        setShelfModalVisible(false);
        setCurrentItem(null);
      } else {
        message.error(response.data.error || '上架失败');
      }
    } catch (error: any) {
      message.error('上架失败: ' + (error.response?.data?.error || error.message));
    } finally {
      setLoading(false);
    }
  };

  // 打开批量上架弹窗
  const openBatchShelfModal = () => {
    if (selectedRows.length === 0) {
      message.error('请先选择要上架的物品');
      return;
    }
    setBatchUnifiedPrice('');
    setBatchShelfModalVisible(true);
  };

  // 执行批量上架
  const handleBatchShelfItems = async () => {
    if (selectedRows.length === 0) {
      message.error('请先选择要上架的物品');
      return;
    }

    try {
      setLoading(true);
      const selectedItems = inventory.filter(
        (item) => selectedRows.includes(item.item_asset_id)
      );

      const items = selectedItems.map((item) => ({
        item_asset_id: item.item_asset_id,
        template_id: item.template_id,
      }));

      const response = await axios.post(
        `/youpin/inventory/on-shelf-batch`,
        {
          steam_id: steamID,
          items,
          unified_price: batchUnifiedPrice || null,
        }
      );

      if (response.data.success) {
        message.success(
          `批量上架完成：成功${response.data.success_count}件，失败${response.data.fail_count}件`
        );
        setBatchShelfModalVisible(false);
        setSelectedRows([]);
      } else {
        message.error(response.data.error || '上架失败');
      }
    } catch (error: any) {
      message.error('批量上架失败: ' + (error.response?.data?.error || error.message));
    } finally {
      setLoading(false);
    }
  };

  // 表格列配置
  const columns: any[] = [
    {
      title: '物品名称',
      dataIndex: 'template_name',
      key: 'template_name',
      width: 250,
      ellipsis: true,
      render: (text: string, record: InventoryItem) => (
        <div>
          <div style={{ fontWeight: 'bold' }}>{text}</div>
          <div style={{ color: '#666', fontSize: '12px' }}>
            ID: {record.template_id}
          </div>
        </div>
      ),
    },
    {
      title: '磨损度',
      dataIndex: 'exterior_name',
      key: 'exterior_name',
      width: 120,
      render: (text: string) => <Tag>{text}</Tag>,
    },
    {
      title: '市场最低价',
      dataIndex: 'market_min_price',
      key: 'market_min_price',
      width: 120,
      render: (text: string) => <span style={{ color: '#f56a00' }}>¥{text}</span>,
    },
    {
      title: '参考价格',
      dataIndex: 'market_price',
      key: 'market_price',
      width: 120,
      render: (text: string) => <span style={{ color: '#52c41a' }}>¥{text}</span>,
    },
    {
      title: '状态',
      key: 'status',
      width: 100,
      render: (_: any, record: InventoryItem) => (
        <Space direction="vertical" size={0}>
          <Tag color={record.tradable === 1 ? 'green' : 'red'}>
            {record.tradable === 1 ? '可交易' : '不可交易'}
          </Tag>
          <Tag color={record.marketable === 1 ? 'blue' : 'orange'}>
            {record.marketable === 1 ? '可上架' : '不可上架'}
          </Tag>
        </Space>
      ),
    },
    {
      title: '操作',
      key: 'action',
      width: 120,
      render: (_: any, record: InventoryItem) => (
        <Button
          type="primary"
          size="small"
          icon={<DollarOutlined />}
          disabled={record.tradable === 0 || record.marketable === 0}
          onClick={() => openShelfModal(record)}
        >
          上架
        </Button>
      ),
    },
  ];

  return (
    <div style={{ padding: '24px' }}>
      <Card
        title={
          <Space>
            <ShopOutlined />
            库存管理
          </Space>
        }
        style={{ marginBottom: '24px' }}
      >
        <Row gutter={16} style={{ marginBottom: '16px' }}>
          <Col xs={24}>
            <Space>
              <Button
                type="primary"
                loading={loading}
                onClick={loadInventory}
                icon={<ReloadOutlined />}
              >
                刷新库存
              </Button>
              {selectedRows.length > 0 && (
                <Button
                  type="dashed"
                  onClick={openBatchShelfModal}
                  icon={<CheckOutlined />}
                >
                  批量上架 ({selectedRows.length})
                </Button>
              )}
            </Space>
          </Col>
        </Row>

        {inventory.length > 0 && (
          <div style={{ marginBottom: '16px', padding: '12px', backgroundColor: '#f5f5f5', borderRadius: '6px' }}>
            <Checkbox
              indeterminate={
                selectedRows.length > 0 && selectedRows.length < inventory.length
              }
              checked={selectedRows.length === inventory.length && inventory.length > 0}
              onChange={(e) => {
                if (e.target.checked) {
                  setSelectedRows(inventory.map((item) => item.item_asset_id));
                } else {
                  setSelectedRows([]);
                }
              }}
            >
              全选
            </Checkbox>
            <span style={{ marginLeft: '16px', color: '#666' }}>
              已选择 {selectedRows.length} / {inventory.length} 件物品
            </span>
          </div>
        )}

        <Spin spinning={loading}>
          <Table
            columns={columns}
            dataSource={inventory}
            rowKey="item_asset_id"
            pagination={{ pageSize: 10, showSizeChanger: true }}
            locale={{ emptyText: inventory.length === 0 ? '暂无库存数据' : undefined }}
            rowSelection={{
              selectedRowKeys: selectedRows,
              onChange: (keys) => setSelectedRows(keys as string[]),
              getCheckboxProps: (record) => ({
                disabled: record.tradable === 0 || record.marketable === 0,
              }),
            }}
          />
        </Spin>
      </Card>

      {/* 单个上架弹窗 */}
      <Modal
        title="上架物品"
        open={shelfModalVisible}
        onOk={handleShelfItem}
        onCancel={() => setShelfModalVisible(false)}
        okText="确认上架"
        cancelText="取消"
        confirmLoading={loading}
      >
        <Spin spinning={priceLoading}>
          {currentItem && (
            <div>
              <div style={{ marginBottom: '16px', padding: '12px', backgroundColor: '#f5f5f5', borderRadius: '6px' }}>
                <p>
                  <strong>物品：</strong> {currentItem.template_name}
                </p>
                <p>
                  <strong>模板ID：</strong> {currentItem.template_id}
                </p>
                <p>
                  <strong>磨损度：</strong> {currentItem.exterior_name}
                </p>
              </div>

              {priceInfo && (
                <Alert
                  message="价格信息"
                  description={
                    <div>
                      <p style={{ marginBottom: '8px' }}>
                        当前市场最低价：
                        <span style={{ color: '#f56a00', fontWeight: 'bold', marginLeft: '8px' }}>
                          ¥{priceInfo.min_sell_price}
                        </span>
                      </p>
                      <p>
                        建议上架价格（最低价-0.1）：
                        <span style={{ color: '#52c41a', fontWeight: 'bold', marginLeft: '8px' }}>
                          ¥{priceInfo.suggested_price}
                        </span>
                      </p>
                    </div>
                  }
                  type="info"
                  style={{ marginBottom: '16px' }}
                />
              )}

              <Form layout="vertical">
                <Form.Item label="上架价格（元）">
                  <InputNumber
                    style={{ width: '100%' }}
                    value={shelfPrice ? parseFloat(shelfPrice) : undefined}
                    onChange={(v) => setShelfPrice(v ? v.toString() : '')}
                    min={0.01}
                    step={0.1}
                    precision={2}
                    placeholder="输入上架价格"
                  />
                </Form.Item>
              </Form>
            </div>
          )}
        </Spin>
      </Modal>

      {/* 批量上架弹窗 */}
      <Modal
        title={`批量上架 (${selectedRows.length}件物品)`}
        open={batchShelfModalVisible}
        onOk={handleBatchShelfItems}
        onCancel={() => setBatchShelfModalVisible(false)}
        okText="确认上架"
        cancelText="取消"
        confirmLoading={loading}
      >
        <Form layout="vertical">
          <Form.Item
            label="统一上架价格（可选，留空则自动按市场最低价-0.1定价）"
            tooltip="如果填写该字段，所有选中的物品都将按此价格上架"
          >
            <InputNumber
              style={{ width: '100%' }}
              value={batchUnifiedPrice ? parseFloat(batchUnifiedPrice) : undefined}
              onChange={(v) => setBatchUnifiedPrice(v ? v.toString() : '')}
              min={0.01}
              step={0.1}
              precision={2}
              placeholder="不填则按各物品市场最低价自动定价"
            />
          </Form.Item>
          <Alert
            message="提示"
            description={
              batchUnifiedPrice
                ? `将以 ¥${batchUnifiedPrice} 的价格上架所有 ${selectedRows.length} 件物品`
                : `将根据每件物品的市场最低价自动定价（最低价-0.1）`
            }
            type="info"
          />
        </Form>
      </Modal>
    </div>
  );
};

export default Inventory;
