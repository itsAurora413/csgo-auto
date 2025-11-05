-- 迁移脚本: 添加 yyyp_template_id 字段并从 csqaq_good_snapshots 填充
-- 用途: 将悠悠有品的模板ID从快照表迁移到商品表

-- ============================================================================
-- 第一步: 检查列是否存在并创建
-- ============================================================================

-- 添加列（如果不存在）
ALTER TABLE csqaq_goods
ADD COLUMN yyyp_template_id BIGINT NULL COMMENT '悠悠有品模板ID'
AFTER name;

-- 为新列创建索引（提高查询性能）
CREATE INDEX IF NOT EXISTS idx_goods_yyyp_template_id
ON csqaq_goods(yyyp_template_id);

-- ============================================================================
-- 第二步: 数据迁移
-- ============================================================================

-- 显示迁移前的统计
SELECT
    '迁移前统计' as stage,
    COUNT(*) as total_goods,
    SUM(CASE WHEN yyyp_template_id IS NOT NULL THEN 1 ELSE 0 END) as with_template_id,
    SUM(CASE WHEN yyyp_template_id IS NULL THEN 1 ELSE 0 END) as without_template_id
FROM csqaq_goods;

-- 迁移逻辑: 从 csqaq_good_snapshots 获取最新的 yyyp_template_id
-- 使用子查询获取每个 good_id 对应的最新的、非NULL的 yyyp_template_id
UPDATE csqaq_goods g
SET yyyp_template_id = (
    SELECT DISTINCT yyyp_template_id
    FROM csqaq_good_snapshots s
    WHERE s.good_id = g.good_id
    AND s.yyyp_template_id IS NOT NULL
    ORDER BY s.created_at DESC
    LIMIT 1
)
WHERE yyyp_template_id IS NULL
AND EXISTS (
    SELECT 1 FROM csqaq_good_snapshots s
    WHERE s.good_id = g.good_id
    AND s.yyyp_template_id IS NOT NULL
);

-- 显示迁移后的统计
SELECT
    '迁移后统计' as stage,
    COUNT(*) as total_goods,
    SUM(CASE WHEN yyyp_template_id IS NOT NULL THEN 1 ELSE 0 END) as with_template_id,
    SUM(CASE WHEN yyyp_template_id IS NULL THEN 1 ELSE 0 END) as without_template_id
FROM csqaq_goods;

-- ============================================================================
-- 第三步: 数据质量检查
-- ============================================================================

-- 检查是否有重复的 yyyp_template_id
SELECT
    '重复值检查' as check_name,
    yyyp_template_id,
    COUNT(*) as count
FROM csqaq_goods
WHERE yyyp_template_id IS NOT NULL
GROUP BY yyyp_template_id
HAVING COUNT(*) > 1
ORDER BY count DESC;

-- 检查没有 yyyp_template_id 的商品
SELECT
    '缺失yyyp_template_id的商品' as check_name,
    g.good_id,
    g.name,
    COUNT(s.id) as snapshot_count,
    SUM(CASE WHEN s.yyyp_template_id IS NOT NULL THEN 1 ELSE 0 END) as snapshots_with_template_id
FROM csqaq_goods g
LEFT JOIN csqaq_good_snapshots s ON g.good_id = s.good_id
WHERE g.yyyp_template_id IS NULL
GROUP BY g.good_id, g.name
HAVING snapshot_count > 0
LIMIT 20;

-- ============================================================================
-- 第四步: 数据验证（可选）
-- ============================================================================

-- 验证迁移的数据一致性（抽样检查）
SELECT
    '数据一致性验证' as check_name,
    g.good_id,
    g.yyyp_template_id as goods_template_id,
    MAX(s.yyyp_template_id) as latest_snapshot_template_id,
    CASE
        WHEN g.yyyp_template_id = MAX(s.yyyp_template_id) THEN '✓ 一致'
        WHEN g.yyyp_template_id IS NOT NULL AND MAX(s.yyyp_template_id) IS NULL THEN '⚠ 只在goods中'
        WHEN g.yyyp_template_id IS NULL AND MAX(s.yyyp_template_id) IS NOT NULL THEN '⚠ 未迁移'
        ELSE '❌ 不一致'
    END as status
FROM csqaq_goods g
LEFT JOIN csqaq_good_snapshots s ON g.good_id = s.good_id AND s.yyyp_template_id IS NOT NULL
GROUP BY g.good_id, g.yyyp_template_id
WHERE g.yyyp_template_id IS NOT NULL OR MAX(s.yyyp_template_id) IS NOT NULL
LIMIT 50;

-- ============================================================================
-- 使用说明
-- ============================================================================
/*
执行步骤:
1. 备份数据库 (重要!)
   mysqldump -u root -p csgo_trader > backup_before_migration.sql

2. 连接到数据库
   mysql -u root -p csgo_trader

3. 执行迁移脚本
   source scripts/migrate-yyyp-template-id.sql;

4. 检查迁移结果
   SELECT COUNT(*) as total,
          SUM(IF(yyyp_template_id IS NOT NULL, 1, 0)) as with_template_id,
          SUM(IF(yyyp_template_id IS NULL, 1, 0)) as without_template_id
   FROM csqaq_goods;

5. 如果需要回滚
   ALTER TABLE csqaq_goods DROP COLUMN yyyp_template_id;
   - 或者从备份恢复: mysql -u root -p csgo_trader < backup_before_migration.sql

常见问题:
- Q: 为什么有些商品仍然没有 yyyp_template_id?
  A: 因为这些商品在 csqaq_good_snapshots 中没有非NULL的 yyyp_template_id 记录

- Q: 迁移需要多长时间?
  A: 取决于数据量，通常几秒到几分钟

- Q: 可以并行执行其他操作吗?
  A: 不建议，最好独占数据库进行迁移
*/
