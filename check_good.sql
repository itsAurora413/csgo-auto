-- 检查 good_id = 12577 的饰品信息
SELECT 
  g.id,
  g.good_id,
  g.name,
  g.cate_id,
  COUNT(*) as snapshot_count,
  MAX(s.created_at) as latest_snapshot,
  MAX(s.yyyp_buy_price) as latest_buy_price,
  MAX(s.yyyp_sell_price) as latest_sell_price,
  MAX(s.yyyp_buy_count) as latest_buy_count,
  MAX(s.yyyp_sell_count) as latest_sell_count
FROM csqaq_goods g
LEFT JOIN csqaq_good_snapshots s ON g.good_id = s.good_id
WHERE g.good_id = 12577
GROUP BY g.id, g.good_id, g.name, g.cate_id;

-- 查看最近7天的历史快照数据
SELECT 
  s.good_id,
  s.created_at,
  s.yyyp_buy_price,
  s.yyyp_sell_price,
  s.yyyp_buy_count,
  s.yyyp_sell_count
FROM csqaq_good_snapshots s
WHERE s.good_id = 12577
  AND s.created_at >= DATE_SUB(NOW(), INTERVAL 7 DAY)
ORDER BY s.created_at DESC;

-- 检查套利机会表中的数据
SELECT 
  id,
  good_id,
  good_name,
  current_buy_price,
  current_sell_price,
  profit_rate,
  estimated_profit,
  price_trend,
  risk_level,
  days_of_data,
  buy_order_count,
  sell_order_count,
  recommended_quantity,
  analysis_time
FROM arbitrage_opportunities
WHERE good_id = 12577
ORDER BY analysis_time DESC
LIMIT 5;
