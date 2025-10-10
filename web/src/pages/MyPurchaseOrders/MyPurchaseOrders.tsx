import React, { useState, useEffect } from 'react';
import {
  Card,
  Table,
  Button,
  Modal,
  Form,
  InputNumber,
  message,
  Space,
  Typography,
  Tag,
  Popconfirm,
  Image,
  Checkbox,
  Tooltip
} from 'antd';
import {
  DeleteOutlined,
  EditOutlined,
  ReloadOutlined,
  DollarOutlined
} from '@ant-design/icons';
import axios from 'axios';

const { Title, Text } = Typography;

interface MyPurchaseOrder {
  isNew: number;
  orderNo: string;
  templateId: number;
  commodityName: string;
  iconUrl: string;
  unitPrice: string;
  styleSpecial: any;
  abradeText: any;
  fadeText: any;
  buyQuantity: number;
  quantity: number;
  maxPurchasePrice: string;
  autoReceived: number;
  rank: string;
  status: number;
  statusText: string;
  statusTextColor: string;
  createTime: string;
  countDownTime: any;
  lastPriceUpdateTime: any;
  checkPriceMessage: any;
}

const MyPurchaseOrders: React.FC = () => {
  const [orders, setOrders] = useState<MyPurchaseOrder[]>([]);
  const [loading, setLoading] = useState(false);
  const [editModalVisible, setEditModalVisible] = useState(false);
  const [editingOrder, setEditingOrder] = useState<MyPurchaseOrder | null>(null);
  const [form] = Form.useForm();

  // 加载求购订单列表
  const loadOrders = async () => {
    setLoading(true);
    try {
      const response = await axios.post('/youpin/purchase/my-list', {
        pageIndex: 1,
        pageSize: 100,
        status: 20 // 20表示求购中
      });

      if (response.data.success) {
        setOrders(response.data.data || []);
      } else {
        message.error('加载求购订单失败');
      }
    } catch (error: any) {
      message.error('加载求购订单失败: ' + (error.response?.data?.error || error.message));
    } finally {
      setLoading(false);
    }
  };

  // 删除求购订单
  const handleDelete = async (orderNo: string) => {
    try {
      const response = await axios.post('/youpin/purchase/delete', {
        order_no_list: [orderNo]
      });

      if (response.data.success) {
        message.success('删除成功');
        loadOrders(); // 重新加载列表
      } else {
        message.error('删除失败');
      }
    } catch (error: any) {
      message.error('删除失败: ' + (error.response?.data?.error || error.message));
    }
  };

  // 打开编辑模态框
  const handleEdit = (order: MyPurchaseOrder) => {
    setEditingOrder(order);
    form.setFieldsValue({
      purchasePrice: parseFloat(order.unitPrice),
      purchaseNum: order.quantity,
      autoReceived: order.autoReceived === 1
    });
    setEditModalVisible(true);
  };

  // 提交修改
  const handleUpdate = async (values: any) => {
    if (!editingOrder) return;

    try {
      // 需要从原订单获取完整的物品信息
      // 这里我们需要先获取物品的完整信息（templateHashName等）
      const infoResponse = await axios.post('/youpin/purchase/info', {
        template_id: editingOrder.templateId.toString()
      });

      if (!infoResponse.data || !infoResponse.data.templateInfo) {
        message.error('获取物品信息失败');
        return;
      }

      const templateInfo = infoResponse.data.templateInfo;

      const response = await axios.post('/youpin/purchase/update', {
        order_no: editingOrder.orderNo,
        template_id: editingOrder.templateId,
        template_hash_name: templateInfo.templateHashName,
        commodity_name: editingOrder.commodityName,
        purchase_price: values.purchasePrice,
        purchase_num: values.purchaseNum,
        reference_price: templateInfo.referencePrice,
        min_sell_price: templateInfo.minSellPrice,
        max_purchase_price: templateInfo.maxPurchasePrice,
        auto_received: values.autoReceived || false
      });

      if (response.data.success) {
        message.success('修改成功');
        setEditModalVisible(false);
        setEditingOrder(null);
        form.resetFields();
        loadOrders(); // 重新加载列表
      } else {
        message.error('修改失败');
      }
    } catch (error: any) {
      message.error('修改失败: ' + (error.response?.data?.error || error.message));
    }
  };

  useEffect(() => {
    loadOrders();
  }, []);

  const columns = [
    {
      title: '饰品',
      dataIndex: 'commodityName',
      key: 'commodityName',
      render: (text: string, record: MyPurchaseOrder) => (
        <Space>
          <Image
            width={50}
            src={record.iconUrl}
            preview={false}
            fallback="data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="
          />
          <div>
            <div>{text}</div>
            {record.rank && (
              <Tag color="blue">排名: {record.rank}</Tag>
            )}
          </div>
        </Space>
      )
    },
    {
      title: '单价',
      dataIndex: 'unitPrice',
      key: 'unitPrice',
      render: (text: string) => <Text strong>¥{text}</Text>
    },
    {
      title: '数量',
      key: 'quantity',
      render: (record: MyPurchaseOrder) => (
        <div>
          <div>求购: {record.quantity}</div>
          <div>
            <Text type="secondary">已购: {record.buyQuantity}</Text>
          </div>
        </div>
      )
    },
    {
      title: '总价',
      key: 'totalPrice',
      render: (record: MyPurchaseOrder) => {
        const total = parseFloat(record.unitPrice) * record.quantity;
        return <Text strong>¥{total.toFixed(2)}</Text>;
      }
    },
    {
      title: '自动收货',
      dataIndex: 'autoReceived',
      key: 'autoReceived',
      render: (autoReceived: number) => (
        <Tag color={autoReceived === 1 ? 'green' : 'default'}>
          {autoReceived === 1 ? '已开启' : '未开启'}
        </Tag>
      )
    },
    {
      title: '状态',
      dataIndex: 'statusText',
      key: 'statusText',
      render: (text: string, record: MyPurchaseOrder) => (
        <Tag color={record.statusTextColor}>{text}</Tag>
      )
    },
    {
      title: '创建时间',
      dataIndex: 'createTime',
      key: 'createTime'
    },
    {
      title: '操作',
      key: 'action',
      render: (record: MyPurchaseOrder) => (
        <Space>
          <Tooltip title="修改求购价格和数量">
            <Button
              type="link"
              icon={<EditOutlined />}
              onClick={() => handleEdit(record)}
            >
              修改
            </Button>
          </Tooltip>
          <Popconfirm
            title="确定要删除这个求购订单吗？"
            onConfirm={() => handleDelete(record.orderNo)}
            okText="确定"
            cancelText="取消"
          >
            <Button type="link" danger icon={<DeleteOutlined />}>
              删除
            </Button>
          </Popconfirm>
        </Space>
      )
    }
  ];

  return (
    <div style={{ padding: '24px', maxWidth: '1400px', margin: '0 auto' }}>
      <Title level={2}>
        <DollarOutlined /> 我的求购订单
      </Title>

      <Card>
        <Space style={{ marginBottom: 16 }}>
          <Button
            type="primary"
            icon={<ReloadOutlined />}
            onClick={loadOrders}
            loading={loading}
          >
            刷新
          </Button>
          <Text type="secondary">
            共 {orders.length} 条求购订单
          </Text>
        </Space>

        <Table
          columns={columns}
          dataSource={orders}
          rowKey="orderNo"
          loading={loading}
          pagination={{
            pageSize: 20,
            showSizeChanger: true,
            showTotal: (total) => `共 ${total} 条`
          }}
        />
      </Card>

      {/* 编辑求购订单模态框 */}
      <Modal
        title="修改求购订单"
        open={editModalVisible}
        onCancel={() => {
          setEditModalVisible(false);
          setEditingOrder(null);
          form.resetFields();
        }}
        onOk={() => form.submit()}
        okText="确认修改"
        cancelText="取消"
      >
        {editingOrder && (
          <div>
            <div style={{ marginBottom: 16 }}>
              <Space>
                <Image
                  width={60}
                  src={editingOrder.iconUrl}
                  preview={false}
                />
                <div>
                  <Text strong>{editingOrder.commodityName}</Text>
                  <div>
                    <Text type="secondary">当前价格: ¥{editingOrder.unitPrice}</Text>
                  </div>
                  <div>
                    <Text type="secondary">最高求购价: ¥{editingOrder.maxPurchasePrice}</Text>
                  </div>
                </div>
              </Space>
            </div>

            <Form
              form={form}
              onFinish={handleUpdate}
              layout="vertical"
            >
              <Form.Item
                label="求购单价 (元)"
                name="purchasePrice"
                rules={[
                  { required: true, message: '请输入求购单价' },
                  { type: 'number', min: 0.01, message: '价格必须大于0' }
                ]}
              >
                <InputNumber
                  style={{ width: '100%' }}
                  precision={2}
                  min={0.01}
                  placeholder="请输入求购单价"
                />
              </Form.Item>

              <Form.Item
                label="求购数量"
                name="purchaseNum"
                rules={[
                  { required: true, message: '请输入求购数量' },
                  { type: 'number', min: 1, message: '数量必须大于0' }
                ]}
              >
                <InputNumber
                  style={{ width: '100%' }}
                  min={1}
                  placeholder="请输入求购数量"
                />
              </Form.Item>

              <Form.Item
                name="autoReceived"
                valuePropName="checked"
              >
                <Checkbox>
                  开启自动收货（推荐，收到饰品后自动确认收货）
                </Checkbox>
              </Form.Item>
            </Form>
          </div>
        )}
      </Modal>
    </div>
  );
};

export default MyPurchaseOrders;