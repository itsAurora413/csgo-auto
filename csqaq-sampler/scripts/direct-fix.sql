-- 直接修复脚本 - 最可靠的方式
-- 不需要Go工具，直接执行SQL

-- ============================================================================
-- 显示当前外键信息
-- ============================================================================

SELECT '=== 当前外键信息 ===' as info;

SELECT CONSTRAINT_NAME, TABLE_NAME, COLUMN_NAME, REFERENCED_TABLE_NAME, REFERENCED_COLUMN_NAME
FROM INFORMATION_SCHEMA.KEY_COLUMN_USAGE
WHERE TABLE_NAME = 'csqaq_good_snapshots'
AND COLUMN_NAME = 'good_id';

-- ============================================================================
-- 第一步: 删除冲突的外键
-- ============================================================================

SELECT '=== 删除外键 ===' as step;

-- 方式1: 删除指定的外键
ALTER TABLE csqaq_good_snapshots DROP FOREIGN KEY csqaq_good_snapshots_FK_0_0;

SELECT '✓ 外键已删除' as info;

-- ============================================================================
-- 第二步: 修改列类型使其匹配
-- ============================================================================

SELECT '=== 修改列类型 ===' as step;

-- 确保两个列都是 BIGINT NOT NULL
ALTER TABLE csqaq_goods MODIFY COLUMN good_id BIGINT NOT NULL;
SELECT '✓ csqaq_goods.good_id 已修改为 BIGINT NOT NULL' as info;

ALTER TABLE csqaq_good_snapshots MODIFY COLUMN good_id BIGINT NOT NULL;
SELECT '✓ csqaq_good_snapshots.good_id 已修改为 BIGINT NOT NULL' as info;

-- ============================================================================
-- 第三步: 验证列类型一致
-- ============================================================================

SELECT '=== 验证列类型 ===' as step;

SELECT TABLE_NAME, COLUMN_NAME, COLUMN_TYPE, IS_NULLABLE, COLUMN_KEY
FROM INFORMATION_SCHEMA.COLUMNS
WHERE TABLE_NAME IN ('csqaq_goods', 'csqaq_good_snapshots')
AND COLUMN_NAME = 'good_id'
ORDER BY TABLE_NAME;

-- ============================================================================
-- 第四步: 添加 yyyp_template_id 列（如果不存在）
-- ============================================================================

SELECT '=== 添加 yyyp_template_id 列 ===' as step;

ALTER TABLE csqaq_goods
ADD COLUMN IF NOT EXISTS yyyp_template_id BIGINT NULL COMMENT '悠悠有品模板ID';

-- 创建索引
CREATE INDEX IF NOT EXISTS idx_goods_yyyp_template_id
ON csqaq_goods(yyyp_template_id);

SELECT '✓ yyyp_template_id 列已添加' as info;

-- ============================================================================
-- 第五步: 重新创建外键约束
-- ============================================================================

SELECT '=== 重新创建外键 ===' as step;

-- 先删除可能存在的其他外键
ALTER TABLE csqaq_good_snapshots DROP FOREIGN KEY IF EXISTS csqaq_good_snapshots_FK_good_id;

-- 创建新的外键
ALTER TABLE csqaq_good_snapshots
ADD CONSTRAINT csqaq_good_snapshots_FK_good_id
FOREIGN KEY (good_id)
REFERENCES csqaq_goods(good_id)
ON DELETE CASCADE
ON UPDATE CASCADE;

SELECT '✓ 外键已重新创建' as info;

-- ============================================================================
-- 第六步: 数据迁移 - 从 snapshots 获取 yyyp_template_id
-- ============================================================================

SELECT '=== 迁移数据 ===' as step;

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

SELECT CONCAT('✓ 已迁移 ', ROW_COUNT(), ' 条记录') as info;

-- ============================================================================
-- 最终验证
-- ============================================================================

SELECT '=== 最终验证 ===' as step;

-- 验证外键
SELECT 'FK验证结果:' as check_type;
SELECT CONSTRAINT_NAME, TABLE_NAME, COLUMN_NAME, REFERENCED_TABLE_NAME, REFERENCED_COLUMN_NAME
FROM INFORMATION_SCHEMA.KEY_COLUMN_USAGE
WHERE TABLE_NAME = 'csqaq_good_snapshots'
AND COLUMN_NAME = 'good_id';

-- 验证列
SELECT 'Column验证结果:' as check_type;
SELECT TABLE_NAME, COLUMN_NAME, COLUMN_TYPE, IS_NULLABLE, COLUMN_KEY
FROM INFORMATION_SCHEMA.COLUMNS
WHERE TABLE_NAME IN ('csqaq_goods', 'csqaq_good_snapshots')
AND COLUMN_NAME IN ('good_id', 'yyyp_template_id')
ORDER BY TABLE_NAME, COLUMN_NAME;

-- 统计数据迁移情况
SELECT 'Data统计结果:' as check_type;
SELECT
    COUNT(*) as total_goods,
    SUM(CASE WHEN yyyp_template_id IS NOT NULL THEN 1 ELSE 0 END) as with_template_id,
    SUM(CASE WHEN yyyp_template_id IS NULL THEN 1 ELSE 0 END) as without_template_id,
    ROUND(SUM(CASE WHEN yyyp_template_id IS NOT NULL THEN 1 ELSE 0 END) / COUNT(*) * 100, 2) as fill_rate_percent
FROM csqaq_goods;

SELECT '✅ 迁移完成!' as final_status;
