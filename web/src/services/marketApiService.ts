import axios from 'axios';

// 使用本地Go后端代理，避免CORS问题
const MARKET_API_BASE_URL = '/api/v1/csqaq';

// 创建axios实例用于本地代理API
const marketApi = axios.create({
  baseURL: MARKET_API_BASE_URL,
  timeout: 10000,
  headers: {
    'Content-Type': 'application/json'
  }
});

// 市场数据接口定义
export interface MarketItemData {
  id: string;
  name: string;
  market_name: string;
  icon_url: string;
  type: string;
  rarity: string;
  exterior?: string;
  weapon?: string;
}

export interface PriceData {
  timestamp: string;
  price: number;
  volume: number;
  platform: string;
}

export interface MarketPriceChart {
  item_name: string;
  item_id: string;
  data: PriceData[];
  period: string;
}

export interface CurrentPrice {
  steam: number;
  buff: number;
  youpin: number;
  c5game: number;
  igxe: number;
}

export interface MarketSearchResult {
  items: MarketItemData[];
  total: number;
  page: number;
  limit: number;
}

export interface KlineData {
  timestamp: number;
  open: number;
  high: number;
  low: number;
  close: number;
  volume: number;
}

export interface KlineResponse {
  item_name: string;
  item_id: string;
  interval: string;
  kline: KlineData[];
}

// CSQAQ 指数K线返回结构
export interface IndexKlinePoint {
  t: string; // timestamp in ms string
  o: number;
  c: number;
  h: number;
  l: number;
  v: number;
}

export interface IndexKlineResponse {
  code: number;
  msg: string;
  data: IndexKlinePoint[];
}

// Apifox 周期类型
export type IndexKlineType = '1hour' | '4hour' | '1day' | '7day';

class MarketApiService {
  // 初始化饰品ID（按输入关键词）
  async initGoods(keyword: string, pageSize: number = 50): Promise<{count:number, items:any[]}> {
    const response = await marketApi.post('/init-goods', { keyword, page_size: pageSize });
    return response.data?.data || { count: 0, items: [] };
  }

  // 列出已保存的饰品ID
  async listGoods(params?: { search?: string; page?: number; page_size?: number }) {
    const response = await marketApi.get('/goods', { params });
    return response.data?.data || { items: [], page: 1, page_size: 20, total: 0 };
  }

  // 获取单个饰品详情（通过CSQAQ）
  async getGoodDetail(id: number | string) {
    const response = await marketApi.get('/good', { params: { id } });
    return response.data; // { code, msg, data }
  }

  // 立即采样某个饰品（后端创建一条快照）
  async sampleGoodNow(id: number | string) {
    const response = await marketApi.post('/good/snapshot', { id: Number(id) });
    return response.data;
  }

  // 全量初始化（ID扫描）
  async startFullInit(params?: { start_id?: number; end_id?: number; throttle_ms?: number }) {
    const response = await marketApi.post('/init-goods-full/start', params || {});
    return response.data;
  }
  async getFullInitStatus() {
    const response = await marketApi.get('/init-goods-full/status');
    return response.data?.status || {};
  }
  async stopFullInit() {
    const response = await marketApi.post('/init-goods-full/stop', {});
    return response.data;
  }
  // 搜索市场物品
  async searchItems(query: string, page: number = 1, limit: number = 20): Promise<MarketSearchResult> {
    try {
      const response = await marketApi.get('/items', {
        params: {
          search: query,
          page,
          limit
        }
      });
      
      // 检查响应格式并转换
      if (response.data && response.data.code === 200) {
        return {
          items: response.data.data.items || [],
          total: response.data.data.total || 0,
          page: response.data.data.page || page,
          limit: response.data.data.limit || limit
        };
      }
      
      return response.data;
    } catch (error) {
      console.error('搜索物品失败:', error);
      // 返回模拟数据作为fallback
      return this.getMockSearchResult(query);
    }
  }

  // 获取物品价格历史
  async getPriceHistory(itemId: string, days: number = 7): Promise<MarketPriceChart> {
    try {
      const response = await marketApi.get(`/items/${itemId}/history`, {
        params: {
          days,
          platforms: 'steam,buff,youpin,c5game,igxe'
        }
      });
      
      // 检查响应格式并转换
      if (response.data && response.data.code === 200) {
        return response.data.data;
      }
      
      return response.data;
    } catch (error) {
      console.error('获取价格历史失败:', error);
      // 返回模拟数据作为fallback
      return this.getMockPriceHistory(itemId, days);
    }
  }

  // 获取当前价格
  async getCurrentPrices(itemId: string): Promise<CurrentPrice> {
    try {
      const response = await marketApi.get(`/items/${itemId}/prices`);
      
      // 检查响应格式并转换
      if (response.data && response.data.code === 200) {
        return response.data.data.prices;
      }
      
      return response.data.prices || response.data;
    } catch (error) {
      console.error('获取当前价格失败:', error);
      // 返回模拟数据作为fallback
      return this.getMockCurrentPrices();
    }
  }

  // 获取热门物品
  async getPopularItems(limit: number = 50): Promise<MarketItemData[]> {
    try {
      const response = await marketApi.get('/popular', {
        params: { limit }
      });
      
      // 检查响应格式并转换
      if (response.data && response.data.code === 200) {
        return response.data.data.items || [];
      }
      
      return response.data.items || response.data || [];
    } catch (error) {
      console.error('获取热门物品失败:', error);
      // 返回模拟数据作为fallback
      return this.getMockPopularItems();
    }
  }

  // 获取价格变动排行
  async getPriceMovers(type: 'gainers' | 'losers' = 'gainers', limit: number = 20): Promise<any[]> {
    try {
      const response = await marketApi.get(`/market/movers/${type}`, {
        params: { limit }
      });
      return response.data.items;
    } catch (error) {
      console.error('获取价格变动失败:', error);
      return [];
    }
  }

  // 获取K线图数据（改为CSQAQ指数K线 /api/v1/sub/kline）
  async getKlineDataByIndex(indexId: string, type: IndexKlineType = '1day'): Promise<IndexKlineResponse> {
    // 避免与全局 axios.defaults.baseURL ('/api/v1') 叠加，url 不要再带 '/api/v1'
    const response = await axios.get(`/sub/kline`, {
      baseURL: '/api/v1',
      params: { id: indexId, type }
    });
    return response.data;
  }

  // 模拟数据方法 - 当API不可用时使用
  private getMockSearchResult(query: string): MarketSearchResult {
    const mockItems: MarketItemData[] = [
      {
        id: '1',
        name: 'AK-47 | Redline (Field-Tested)',
        market_name: 'AK-47 | Redline (Field-Tested)',
        icon_url: 'https://community.cloudflare.steamstatic.com/economy/image/-9a81dlWLwJ2UUGcVs_nsVtzdOEdtWwKGZZLQHTxDZ7I56KU0Zwwo4NUX4oFJZEHLbXH5ApeO4YmlhxYQknCRvCo04DEVlxkKgpot7HxfDhjxszJemkV09-5lpKKqPrxN7LEmyVQ7MEpiLuSrYmnjQO3-UdsZGHyd4_Bd1RvNQ7T_FDrw-_ng5Pu75iY1zI97bhJhJJl/360fx360f',
        type: 'Rifle',
        rarity: 'Classified',
        exterior: 'Field-Tested',
        weapon: 'AK-47'
      },
      {
        id: '2',
        name: 'AWP | Dragon Lore (Factory New)',
        market_name: 'AWP | Dragon Lore (Factory New)',
        icon_url: 'https://community.cloudflare.steamstatic.com/economy/image/-9a81dlWLwJ2UUGcVs_nsVtzdOEdtWwKGZZLQHTxDZ7I56KU0Zwwo4NUX4oFJZEHLbXH5ApeO4YmlhxYQknCRvCo04DEVlxkKgpot621FAR17PLfYQJD_9W7m5a0mvLwOq7c2D8G68Nz3-qWpI2t2wDi_0Y4YmGhJY6UdQE2aVyF-gK9kuvxxcjrjJGdwXFhvCUj7HfVgVXp1kpMPOJxxavJVUyLUPISXPLPUg/360fx360f',
        type: 'Sniper Rifle',
        rarity: 'Covert',
        exterior: 'Factory New',
        weapon: 'AWP'
      }
    ];

    return {
      items: mockItems.filter(item => 
        item.name.toLowerCase().includes(query.toLowerCase())
      ),
      total: mockItems.length,
      page: 1,
      limit: 20
    };
  }

  private getMockPriceHistory(itemId: string, days: number): MarketPriceChart {
    const data: PriceData[] = [];
    const platforms = ['steam', 'buff', 'youpin', 'c5game', 'igxe'];
    const basePrice = Math.random() * 100 + 50;

    for (let i = days; i >= 0; i--) {
      const date = new Date();
      date.setDate(date.getDate() - i);
      
      platforms.forEach(platform => {
        const variation = (Math.random() - 0.5) * 20;
        data.push({
          timestamp: date.toISOString(),
          price: Math.max(basePrice + variation, 10),
          volume: Math.floor(Math.random() * 100) + 10,
          platform
        });
      });
    }

    return {
      item_name: `Mock Item ${itemId}`,
      item_id: itemId,
      data,
      period: `${days}d`
    };
  }

  private getMockCurrentPrices(): CurrentPrice {
    return {
      steam: Math.random() * 100 + 50,
      buff: Math.random() * 100 + 45,
      youpin: Math.random() * 100 + 48,
      c5game: Math.random() * 100 + 52,
      igxe: Math.random() * 100 + 47
    };
  }

  private getMockPopularItems(): MarketItemData[] {
    return [
      {
        id: '1',
        name: 'AK-47 | Redline (Field-Tested)',
        market_name: 'AK-47 | Redline (Field-Tested)',
        icon_url: 'https://community.cloudflare.steamstatic.com/economy/image/-9a81dlWLwJ2UUGcVs_nsVtzdOEdtWwKGZZLQHTxDZ7I56KU0Zwwo4NUX4oFJZEHLbXH5ApeO4YmlhxYQknCRvCo04DEVlxkKgpot7HxfDhjxszJemkV09-5lpKKqPrxN7LEmyVQ7MEpiLuSrYmnjQO3-UdsZGHyd4_Bd1RvNQ7T_FDrw-_ng5Pu75iY1zI97bhJhJJl/360fx360f',
        type: 'Rifle',
        rarity: 'Classified'
      },
      {
        id: '2',
        name: 'M4A4 | Howl (Minimal Wear)',
        market_name: 'M4A4 | Howl (Minimal Wear)',
        icon_url: 'https://community.cloudflare.steamstatic.com/economy/image/-9a81dlWLwJ2UUGcVs_nsVtzdOEdtWwKGZZLQHTxDZ7I56KU0Zwwo4NUX4oFJZEHLbXH5ApeO4YmlhxYQknCRvCo04DEVlxkKgpou-6kejhz2v_Nfz5H_uO1gb-Gw_alDLPIhm5u5Mx2gv2P8d2t2wDsqEo_ZmGmLYGRdlQ3aVnU-lLqxOjxxcjrjJGdwXFhvCUj7HfVgVXp1kpMPOJxxavJVUyLUPISXPLPUg/360fx360f',
        type: 'Rifle',
        rarity: 'Contraband'
      }
    ];
  }

  private getMockKlineData(itemId: string, interval: string, limit: number): KlineResponse {
    const data: KlineData[] = [];
    const basePrice = 150;

    // Parse interval to determine time step in milliseconds
    let timeStep: number;
    switch (interval) {
      case '1m':
        timeStep = 60 * 1000;
        break;
      case '5m':
        timeStep = 5 * 60 * 1000;
        break;
      case '15m':
        timeStep = 15 * 60 * 1000;
        break;
      case '30m':
        timeStep = 30 * 60 * 1000;
        break;
      case '1h':
        timeStep = 60 * 60 * 1000;
        break;
      case '4h':
        timeStep = 4 * 60 * 60 * 1000;
        break;
      case '1d':
        timeStep = 24 * 60 * 60 * 1000;
        break;
      default:
        timeStep = 60 * 60 * 1000; // 1 hour
    }

    const now = Date.now();
    for (let i = limit - 1; i >= 0; i--) {
      const timestamp = Math.floor((now - i * timeStep) / 1000);

      // Generate realistic OHLCV data
      const seed = timestamp;
      const open = basePrice + ((seed % 200) - 100) / 10;
      const variation = ((seed % 100) - 50) / 20;

      const high = open + (seed % 50) / 10 + 2;
      const low = open - (seed % 40) / 10 - 1;
      const close = open + variation;

      // Ensure price relationships are correct
      const adjustedHigh = Math.max(high, open, close) + 0.5;
      const adjustedLow = Math.min(low, open, close) - 0.5;

      const volume = 100 + (seed % 500);

      data.push({
        timestamp,
        open: Math.round(open * 100) / 100,
        high: Math.round(adjustedHigh * 100) / 100,
        low: Math.round(adjustedLow * 100) / 100,
        close: Math.round(close * 100) / 100,
        volume
      });
    }

    return {
      item_name: `Mock Item ${itemId}`,
      item_id: itemId,
      interval,
      kline: data
    };
  }
}

export const marketApiService = new MarketApiService();
