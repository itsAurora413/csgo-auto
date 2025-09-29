import React, { useEffect, useState } from 'react';
import { Button, Card, Input, Space, Table, message, Typography, Pagination, Tag } from 'antd';
import { ReloadOutlined, SearchOutlined, PlusSquareOutlined } from '@ant-design/icons';
import { marketApiService } from '../../services/marketApiService';
import { useNavigate } from 'react-router-dom';

const { Text } = Typography;

interface CSQAQGoodRow {
  id: number;
  good_id: number;
  name: string;
  market_hash_name: string;
  snapshot_count?: number;
  last_sampled_at?: string;
  created_at?: string;
  updated_at?: string;
}

const CSQAQGoods: React.FC = () => {
  const navigate = useNavigate();
  const [search, setSearch] = useState('');
  const [keyword, setKeyword] = useState('');
  const [loading, setLoading] = useState(false);
  const [rows, setRows] = useState<CSQAQGoodRow[]>([]);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const [total, setTotal] = useState(0);
  const [jobStatus, setJobStatus] = useState<any>({ running: false });

  const pollJob = async () => {
    try { const st = await marketApiService.getFullInitStatus(); setJobStatus(st || { running:false }); } catch {}
  };
  useEffect(() => { pollJob(); const t = setInterval(pollJob, 5000); return () => clearInterval(t); }, []);

  const load = async (p = page, ps = pageSize) => {
    setLoading(true);
    try {
      const data = await marketApiService.listGoods({ search, page: p, page_size: ps });
      setRows(data.items || []);
      setTotal(data.total || 0);
      setPage(data.page || p);
      setPageSize(data.page_size || ps);
    } catch (e: any) {
      message.error(e?.message || '加载失败');
    } finally { setLoading(false); }
  };

  useEffect(() => { load(1, pageSize); /* eslint-disable-next-line */ }, []);

  const initGoods = async () => {
    setLoading(true);
    try {
      if (!keyword.trim()) { message.warning('请输入关键词'); return; }
      const res = await marketApiService.initGoods(keyword.trim());
      message.success(`执行完成，共获取 ${res.count || 0} 条`);
      load(1, pageSize);
    } catch (e: any) {
      message.error(e?.response?.data?.error || e?.message || '初始化失败');
    } finally { setLoading(false); }
  };

  return (
    <div>
      <Card
        title={
          <Space size={12}>
            <Text strong>CSQAQ 饰品ID 列表</Text>
          </Space>
        }
        extra={
          <Space>
            {/* 全量初始化控制 */}
            <Button onClick={async ()=>{ try { await marketApiService.startFullInit(); message.success('已启动全量初始化'); pollJob(); } catch(e:any){ message.error(e?.response?.data?.error || e?.message || '启动失败'); } }}>全量初始化(1~101466)</Button>
            {jobStatus?.running ? (
              <>
                <Tag color="blue">进行中 {jobStatus?.current}/{jobStatus?.end_id} 成功{jobStatus?.success} 失败{jobStatus?.failed}</Tag>
                <Button onClick={async ()=>{ try { await marketApiService.stopFullInit(); message.success('已请求停止'); pollJob(); } catch(e:any){ message.error('停止失败'); } }}>停止</Button>
              </>
            ) : (
              <Tag>未运行</Tag>
            )}
            <Input
              allowClear
              placeholder="输入关键词用于初始化（POST get_good_id）"
              value={keyword}
              onChange={(e) => setKeyword(e.target.value)}
              onPressEnter={initGoods}
              style={{ width: 260 }}
            />
            <Button type="primary" icon={<PlusSquareOutlined />} onClick={initGoods}>执行</Button>
            <Input
              allowClear
              prefix={<SearchOutlined />}
              placeholder="搜索已保存的映射（ID/中英文名）"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              onPressEnter={() => load(1, pageSize)}
              style={{ width: 280 }}
            />
            <Button icon={<ReloadOutlined />} onClick={() => load(1, pageSize)}>搜索</Button>
          </Space>
        }
      >
        <Table
          rowKey={(r) => String(r.good_id)}
          loading={loading}
          dataSource={rows}
          pagination={false}
          columns={[
            { title: 'Good ID', dataIndex: 'good_id', width: 120, sorter:(a:any,b:any)=> (a.good_id||0)-(b.good_id||0), defaultSortOrder:'ascend' as any },
            { title: '中文名称', dataIndex: 'name' },
            { title: '英文名称', dataIndex: 'market_hash_name' },
            { title: '采样次数', dataIndex: 'snapshot_count', width: 100, render:(v)=> v ?? 0 },
            { title: '最近采样时间', dataIndex: 'last_sampled_at', width: 180, render:(v)=> v ? new Date(v).toLocaleString() : '-' },
            {
              title: '操作',
              width: 120,
              render: (_, r) => (
                <Space size={8}>
                  <Button size="small" onClick={() => navigate(`/csqaq/goods/${r.good_id}`)}>详情</Button>
                  <Button size="small" onClick={async () => {
                    try {
                      await marketApiService.sampleGoodNow(r.good_id);
                      message.success('已采样');
                    } catch (e: any) {
                      message.error(e?.response?.data?.error || e?.message || '采样失败');
                    }
                  }}>立即采样</Button>
                </Space>
              )
            }
          ]}
        />
        <div style={{ marginTop: 12, display: 'flex', justifyContent: 'flex-end' }}>
          <Pagination
            current={page}
            pageSize={pageSize}
            total={total}
            showSizeChanger
            onChange={(p, ps) => { setPage(p); setPageSize(ps); load(p, ps); }}
          />
        </div>
      </Card>
    </div>
  );
};

export default CSQAQGoods;
