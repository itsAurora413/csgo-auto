import React, { useState } from 'react';
import { Routes, Route, Navigate, useNavigate, useLocation } from 'react-router-dom';
import { Layout, Menu } from 'antd';
import {
  DashboardOutlined,
  WalletOutlined,
  SettingOutlined,
  LineChartOutlined,
  ShoppingCartOutlined,
  ShopOutlined,
  DollarOutlined
} from '@ant-design/icons';
import { QuestionCircleOutlined } from '@ant-design/icons';
import Dashboard from './pages/Dashboard/Dashboard';
import Market from './pages/Market/Market';
import Trading from './pages/Trading/Trading';
import Inventory from './pages/Inventory/Inventory';
import Strategies from './pages/Strategies/Strategies';
import YouPin from './pages/YouPin/YouPin';
import Purchase from './pages/Purchase/Purchase';
import Help from './pages/Help/Help';
import CSQAQGoods from './pages/CSQAQGoods/CSQAQGoods';
import GoodDetail from './pages/CSQAQGoods/GoodDetail';
import './App.css';

const { Header, Sider, Content } = Layout;

function App() {
  const [collapsed, setCollapsed] = useState(false);
  const navigate = useNavigate();
  const location = useLocation();

  const menuItems = [
    {
      key: '/dashboard',
      icon: <DashboardOutlined />,
      label: '仪表盘'
    },
    {
      key: '/csqaq/goods',
      icon: <LineChartOutlined />,
      label: 'CSQAQ饰品'
    },
    {
      key: '/market',
      icon: <LineChartOutlined />,
      label: '市场分析'
    },
    {
      key: '/youpin',
      icon: <ShopOutlined />,
      label: 'YouPin管理'
    },
    {
      key: '/trading',
      icon: <ShoppingCartOutlined />,
      label: '交易中心'
    },
    {
      key: '/strategies',
      icon: <SettingOutlined />,
      label: '交易策略'
    },
    {
      key: '/inventory',
      icon: <WalletOutlined />,
      label: '库存管理'
    },
    {
      key: '/purchase',
      icon: <DollarOutlined />,
      label: '购买中心'
    },
    {
      key: '/help',
      icon: <QuestionCircleOutlined />,
      label: '帮助中心'
    }
  ];

  return (
    <Layout style={{ minHeight: '100vh' }}>
      <Header style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
        <div className="header-title">CSGO2 自动交易平台</div>
      </Header>

      <Layout>
        <Sider
          collapsible
          collapsed={collapsed}
          onCollapse={setCollapsed}
          theme="light"
          width={200}
        >
          <Menu
            mode="inline"
            selectedKeys={[location.pathname]}
            items={menuItems}
            onClick={({ key }) => {
              navigate(key);
            }}
          />
        </Sider>

        <Layout>
          <Content>
            <Routes>
              <Route path="/" element={<Navigate to="/dashboard" replace />} />
              <Route path="/dashboard" element={<Dashboard />} />
              <Route path="/market" element={<Market />} />
              <Route path="/youpin" element={<YouPin />} />
              <Route path="/trading" element={<Trading />} />
              <Route path="/strategies" element={<Strategies />} />
              <Route path="/inventory" element={<Inventory />} />
              <Route path="/purchase" element={<Purchase />} />
              <Route path="/csqaq/goods" element={<CSQAQGoods />} />
              <Route path="/csqaq/goods/:id" element={<GoodDetail />} />
              <Route path="/help" element={<Help />} />
            </Routes>
          </Content>
        </Layout>
      </Layout>
    </Layout>
  );
}

export default App;
