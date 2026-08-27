-- 000005_create_processed_events
-- 创建“已处理浏览事件”幂等表。
--
-- 设计依据（Muggle-design.md 6.6）：
--   * Worker 消费 Kafka 事件后，先把 event_id 写入本表，再累加浏览量。
--   * 因为本表主键是 event_id，同一个事件被重复消费（例如 Worker 处理完
--     但 offset 提交失败导致重投）时，第二次 INSERT 会因主键冲突失败，
--     Worker 据此识别“已处理过”，从而跳过重复计数。
--   * 这保证“至少一次投递 + 幂等消费”：Kafka 允许重复投递，但浏览量不会重复累计。
--
-- 字段说明：
--   * event_id 固定 36 字符（UUID 字符串，含连字符），对应 Kafka 事件里的 event_id。
--   * event_type 记录事件类型（当前只有 post.viewed.v1），便于将来扩展其它事件。
--   * processed_at 记录首次成功处理的时间。

CREATE TABLE processed_events (
    -- 事件唯一标识（UUID 字符串）。主键即幂等防线。
    event_id CHAR(36) NOT NULL,
    -- 事件类型，带版本号（如 post.viewed.v1），不原地修改旧契约。
    event_type VARCHAR(80) NOT NULL,
    -- 首次成功处理的时间。
    processed_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),

    PRIMARY KEY (event_id)
) CHARACTER SET utf8mb4
  COLLATE utf8mb4_0900_ai_ci;
