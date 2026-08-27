-- down migration 只用于明确回退一个版本；执行后 posts 表及其中数据都会被删除。
DROP TABLE IF EXISTS posts;
