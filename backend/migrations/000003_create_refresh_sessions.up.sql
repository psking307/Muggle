-- 000003_create_refresh_sessions
-- 创建 Refresh Session 表。
--
-- 设计依据（Muggle-design.md 6.4 与 9.4）：
--   * 浏览器持有明文 Refresh Token（HttpOnly Cookie），数据库只保存它的 SHA-256。
--     即使数据库泄露，攻击者也拿不到可用的明文 Token。
--   * 每次 Refresh 都轮转：旧会话在同一个事务里被条件撤销，新会话随即创建。
--   * revoked_at 为 NULL 表示仍有效；写入时间即代表“已撤销”。

CREATE TABLE refresh_sessions (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    -- 关联管理员；外键保证会话一定属于真实存在的账号。
    admin_id BIGINT UNSIGNED NOT NULL,
    -- Refresh Token 的 SHA-256 十六进制摘要，固定 64 个字符。
    -- 唯一索引从数据库层保证同一个 Token 只会被保存一次。
    token_hash CHAR(64) NOT NULL,
    -- 会话过期时间；Refresh 轮转和登录检查都会读取它。
    expires_at DATETIME(3) NOT NULL,
    -- 撤销时间。NULL = 有效；非 NULL = 已撤销（退出或轮转后旧会话的状态）。
    revoked_at DATETIME(3) NULL,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),

    PRIMARY KEY (id),
    UNIQUE KEY uk_refresh_sessions_token_hash (token_hash),
    -- 按管理员查询会话列表时使用（例如将来做“踢出所有设备”）。
    KEY idx_refresh_sessions_admin (admin_id),
    -- 按过期时间清理历史会话时使用。
    KEY idx_refresh_sessions_expires (expires_at),

    CONSTRAINT fk_refresh_sessions_admin
        FOREIGN KEY (admin_id) REFERENCES admins (id)
        -- 管理员被删除时级联删除其全部会话，避免留下无主数据。
        ON DELETE CASCADE
) CHARACTER SET utf8mb4
  COLLATE utf8mb4_0900_ai_ci;
