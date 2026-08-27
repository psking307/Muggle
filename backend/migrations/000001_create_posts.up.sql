CREATE TABLE posts (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    slug VARCHAR(160) NOT NULL,
    title VARCHAR(200) NOT NULL,
    summary VARCHAR(500) NOT NULL DEFAULT '',
    content_md LONGTEXT NOT NULL,
    status VARCHAR(20) NOT NULL,
    published_at DATETIME(3) NULL,
    version BIGINT UNSIGNED NOT NULL DEFAULT 1,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),

    PRIMARY KEY (id),
    UNIQUE KEY uk_posts_slug (slug),
    KEY idx_posts_public_list (status, published_at, id),
    KEY idx_posts_admin_list (status, updated_at, id),

    CONSTRAINT chk_posts_status
        CHECK (status IN ('draft', 'published'))
) CHARACTER SET utf8mb4
  COLLATE utf8mb4_0900_ai_ci;
