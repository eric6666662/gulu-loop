-- +goose Up
-- 激活的服务商（ADR-0015）：全局恰好 0 或 1 个
ALTER TABLE x_service_provider ADD COLUMN is_active INTEGER NOT NULL DEFAULT 0;

-- 存量回填：激活 created_at 最早的那一条，等价于升级前的 firstProvider() 行为，
-- 保证升级后不会突然换服务商。第二排序键让 created_at 相同时结果确定。
UPDATE x_service_provider
   SET is_active = 1
 WHERE id = (SELECT id FROM x_service_provider ORDER BY created_at, service_provider LIMIT 1);

-- 至多一个激活：部分唯一索引在 DB 层兜底（0 值不参与唯一性，所以可以有多行 0）
CREATE UNIQUE INDEX uk_x_service_provider_active ON x_service_provider (is_active) WHERE is_active = 1;

-- +goose Down
DROP INDEX IF EXISTS uk_x_service_provider_active;
ALTER TABLE x_service_provider DROP COLUMN is_active;
