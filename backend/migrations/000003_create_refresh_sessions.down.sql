-- 回退 000003：删除 refresh_sessions 表。
-- 先删除引用表（本表），golang-migrate 才能继续回退 000002 的 admins 表。
DROP TABLE IF EXISTS refresh_sessions;
