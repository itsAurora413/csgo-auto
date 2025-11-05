-- 修复外键约束问题脚本
-- 问题: Error 3780 - Referencing column 'good_id' and referenced column 'good_id' in foreign key constraint are incompatible
-- 原因: csqaq_goods.good_id 和 csqaq_good_snapshots.good_id 的类型不匹配

-- ============================================================================
-- 检查当前约束
-- ============================================================================

SELECT 'Current Foreign Keys' as info;
SHOW CREATE TABLE csqaq_good_snapshots\G

-- ============================================================================
-- 第一步: 删除外键约束
-- ============================================================================

SELECT 'Step 1: Dropping Foreign Key Constraints' as step;

-- 查找外键名称
SELECT CONSTRAINT_NAME
FROM INFORMATION_SCHEMA.KEY_COLUMN_USAGE
WHERE TABLE_NAME = 'csqaq_good_snapshots'
AND COLUMN_NAME = 'good_id'
AND REFERENCED_TABLE_NAME = 'csqaq_goods';

-- 删除外键约束
ALTER TABLE csqaq_good_snapshots DROP FOREIGN KEY csqaq_good_snapshots_FK_0_0;

SELECT '✅ Foreign key constraint dropped' as info;

-- ============================================================================
-- 第二步: 检查并对齐列类型
-- ============================================================================

SELECT 'Step 2: Checking Column Types' as step;

-- 检查 csqaq_goods.good_id 的类型
SELECT
    TABLE_NAME,
    COLUMN_NAME,
    COLUMN_TYPE,
    IS_NULLABLE,
    COLUMN_KEY
FROM INFORMATION_SCHEMA.COLUMNS
WHERE TABLE_NAME IN ('csqaq_goods', 'csqaq_good_snapshots')
AND COLUMN_NAME = 'good_id';

-- ============================================================================
-- 第三步: 修改列类型以匹配
-- ============================================================================

SELECT 'Step 3: Modifying Column Types to Match' as step;

-- 先检查 csqaq_goods.good_id 是否已经是 BIGINT NOT NULL
-- 如果不是，先修改它
ALTER TABLE csqaq_goods
MODIFY COLUMN good_id BIGINT NOT NULL;

-- 确保 csqaq_good_snapshots.good_id 也是 BIGINT NOT NULL
ALTER TABLE csqaq_good_snapshots
MODIFY COLUMN good_id BIGINT NOT NULL;

SELECT '✅ Column types aligned' as info;

-- ============================================================================
-- 第四步: 重新创建外键约束
-- ============================================================================

SELECT 'Step 4: Recreating Foreign Key Constraint' as step;

ALTER TABLE csqaq_good_snapshots
ADD CONSTRAINT csqaq_good_snapshots_FK_good_id
FOREIGN KEY (good_id)
REFERENCES csqaq_goods(good_id)
ON DELETE CASCADE
ON UPDATE CASCADE;

SELECT '✅ Foreign key constraint recreated' as info;

-- ============================================================================
-- 第五步: 验证
-- ============================================================================

SELECT 'Step 5: Verification' as step;

-- 验证外键已创建
SELECT
    CONSTRAINT_NAME,
    TABLE_NAME,
    COLUMN_NAME,
    REFERENCED_TABLE_NAME,
    REFERENCED_COLUMN_NAME
FROM INFORMATION_SCHEMA.KEY_COLUMN_USAGE
WHERE TABLE_NAME = 'csqaq_good_snapshots'
AND COLUMN_NAME = 'good_id';

-- 验证列类型
SELECT
    TABLE_NAME,
    COLUMN_NAME,
    COLUMN_TYPE,
    IS_NULLABLE,
    COLUMN_KEY
FROM INFORMATION_SCHEMA.COLUMNS
WHERE TABLE_NAME IN ('csqaq_goods', 'csqaq_good_snapshots')
AND COLUMN_NAME = 'good_id'
ORDER BY TABLE_NAME;

SELECT '✅ All constraints and column types verified' as info;

-- ============================================================================
-- 完成
-- ============================================================================

SELECT CONCAT('✅ 修复完成 - ', NOW()) as completion;
