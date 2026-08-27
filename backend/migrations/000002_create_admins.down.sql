-- 回退 000002：删除 admins 表。
-- 注意：refresh_sessions 通过外键引用本表，golang-migrate 按版本逆序执行回退时
-- 会先回退 000003 再回退本文件，因此这里直接 DROP 即可。
DROP TABLE IF EXISTS admins;
