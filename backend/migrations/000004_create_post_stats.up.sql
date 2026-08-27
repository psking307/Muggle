-- 000004_create_post_stats
-- 创建文章浏览统计表。
--
-- 设计依据（Muggle-design.md 6.5）：
--   * 浏览量是“最终一致”的：文章详情 API 只负责把浏览事件投递到 Kafka，
--     由 Worker 异步消费后累加到这里，因此这里不追求与实时访问严格同步。
--   * post_id 既是主键也是外键：每篇文章最多一行统计记录，直接用主键去重。
--   * view_count 用 BIGINT UNSIGNED，避免大访问量下普通 INT 溢出。
--   * 阶段 6 之前创建的文章没有统计行，详情读取时用 LEFT JOIN + COALESCE
--     返回 0，因此不需要为旧文章预先补行。
--
-- 注意：表名/列名统一 snake_case，字符集 utf8mb4，时间存 UTC（DATETIME(3) 毫秒精度）。

CREATE TABLE post_stats (
    -- 文章主键，同时是统计表主键，保证一篇文章只有一行统计。
    post_id BIGINT UNSIGNED NOT NULL,
    -- 累计浏览量。由 Worker 在幂等事务里做 view_count + 1 原子累加。
    view_count BIGINT UNSIGNED NOT NULL DEFAULT 0,
    -- 最近一次更新计数的时间。
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),

    PRIMARY KEY (post_id),

    -- 外键：统计行必须对应一篇真实存在的文章。
    -- v1.0 不提供删除文章接口，但 ON DELETE CASCADE 仍作为防御性约束：
    -- 一旦将来出现物理删除，统计行会随之清理，不留无主数据。
    CONSTRAINT fk_post_stats_post
        FOREIGN KEY (post_id) REFERENCES posts (id)
        ON DELETE CASCADE
) CHARACTER SET utf8mb4
  COLLATE utf8mb4_0900_ai_ci;
