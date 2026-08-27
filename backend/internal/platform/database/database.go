// Package database 负责创建、验证和管理 MySQL 数据库连接。
package database

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"strconv"
	"time"

	drivermysql "github.com/go-sql-driver/mysql"
	"github.com/psking307/Muggle/backend/internal/config"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

const pingTimeout = 3 * time.Second

// Open 根据强类型配置创建 GORM 连接，设置底层连接池，并验证 MySQL 可用性。
//
// 返回两个对象是因为它们职责不同：
//   - *gorm.DB 交给 Repository 组织查询；
//   - *sql.DB 用于 Ping、连接池设置和程序退出时关闭连接。
func Open(cfg config.MySQLConfig) (*gorm.DB, *sql.DB, error) {
	// 使用驱动提供的 Config 生成 DSN（数据库连接字符串），避免手工拼接时
	// 因密码中的 @、: 等特殊字符破坏连接字符串。
	driverConfig := drivermysql.Config{
		User:      cfg.User,
		Passwd:    cfg.Password,
		Net:       "tcp",
		Addr:      net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port)),
		DBName:    cfg.Database,
		ParseTime: true,
		Loc:       time.UTC,
		Collation: "utf8mb4_0900_ai_ci",
		Timeout:   pingTimeout,
	}
	// FormatDSN 是指针方法，因此先保存配置对象，再调用生成方法。
	dsn := driverConfig.FormatDSN()

	db, err := gorm.Open(gormmysql.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, nil, fmt.Errorf("open GORM MySQL connection: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, nil, fmt.Errorf("get SQL database handle: %w", err)
	}

	// 连接池提前保留少量可复用连接，并限制总连接数，防止请求高峰时
	// API 无限制地占满 MySQL 的连接配额。
	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(cfg.ConnMaxLifetime)

	// gorm.Open 不保证已经与 MySQL 完成真实通信，因此启动时主动 Ping。
	// 短超时可以让错误密码、错误端口或 MySQL 未启动尽快暴露。
	ctx, cancel := context.WithTimeout(context.Background(), pingTimeout)
	defer cancel()

	if err := sqlDB.PingContext(ctx); err != nil {
		_ = sqlDB.Close()
		return nil, nil, fmt.Errorf("ping MySQL: %w", err)
	}

	return db, sqlDB, nil
}
