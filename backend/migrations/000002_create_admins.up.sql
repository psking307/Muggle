-- 000002_create_admins
-- 创建管理员账号表。
--
-- 设计依据（Muggle-design.md 6.3）：
--   * v1.0 不提供网页注册，初始管理员只能通过 cmd/admin 离线命令创建。
--   * 密码只保存 bcrypt 哈希，绝不保存明文。
--   * status 只允许 active / disabled 两种状态，由 CHECK 约束在数据库层兜底。

CREATE TABLE admins (
    -- 自增主键；文章等业务表也使用 BIGINT UNSIGNED，保持风格一致。
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    -- 登录名；唯一索引保证不会出现两个同名管理员。
    username VARCHAR(64) NOT NULL,
    -- bcrypt 哈希固定 60 字节左右，VARCHAR(255) 为未来算法升级保留余量。
    password_hash VARCHAR(255) NOT NULL,
    -- 禁用账号必须拒绝新的登录和 Refresh，状态由 Service 在每次请求时重新检查。
    status VARCHAR(16) NOT NULL,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),

    PRIMARY KEY (id),
    UNIQUE KEY uk_admins_username (username),

    -- 数据库层兜底：即使代码出现 bug，也不可能写入未知状态。
    CONSTRAINT chk_admins_status
        CHECK (status IN ('active', 'disabled'))
) CHARACTER SET utf8mb4
  COLLATE utf8mb4_0900_ai_ci;
