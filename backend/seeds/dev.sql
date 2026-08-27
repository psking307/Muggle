-- 这份 seed 只提供本地开发和学习数据，不应在生产环境自动执行。
-- slug 有唯一索引，INSERT IGNORE 让本命令可以重复运行而不产生重复文章。
INSERT IGNORE INTO posts (
    slug,
    title,
    summary,
    content_md,
    status,
    published_at,
    version,
    created_at,
    updated_at
)
VALUES
(
    'hello-muggle',
    '欢迎来到 Muggle',
    '这是阶段二的第一篇公开测试文章。',
    '# 欢迎来到 Muggle\n\n这篇文章来自 MySQL，而不是写死在 React 里。',
    'published',
    UTC_TIMESTAMP(3) - INTERVAL 2 DAY,
    1,
    UTC_TIMESTAMP(3) - INTERVAL 2 DAY,
    UTC_TIMESTAMP(3) - INTERVAL 2 DAY
),
(
    'gin-request-flow',
    '一次 Gin 请求经历了什么',
    '从 Router 到 Handler、Service 和 Repository。',
    '# Gin 请求流程\n\nRouter → Handler → Service → Repository → MySQL。',
    'published',
    UTC_TIMESTAMP(3) - INTERVAL 1 DAY,
    1,
    UTC_TIMESTAMP(3) - INTERVAL 1 DAY,
    UTC_TIMESTAMP(3) - INTERVAL 1 DAY
),
(
    'secret-draft',
    '不能公开的草稿',
    '公开接口绝对不应该返回这篇文章。',
    '# 草稿\n\n如果公开页面看到这篇文章，说明公开过滤规则有问题。',
    'draft',
    NULL,
    1,
    UTC_TIMESTAMP(3),
    UTC_TIMESTAMP(3)
);
