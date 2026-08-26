// Package logger 提供项目统一的 Zap 日志初始化逻辑。
package logger

import (
	"fmt"

	"github.com/psking307/Muggle/backend/internal/config"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// New 根据应用配置创建 Logger。
//
// 开发环境输出适合人阅读的 Console 格式，生产环境输出适合日志系统采集的 JSON。
// 返回的 Logger 已预先携带 service 和 environment 字段。
func New(cfg *config.Config) (*zap.Logger, error) {
	// AtomicLevel 是 Zap 可使用的日志级别对象。
	// UnmarshalText 把 "debug"、"info" 等配置字符串转换成该对象。
	level := zap.NewAtomicLevel()
	if err := level.UnmarshalText([]byte(cfg.Log.Level)); err != nil {
		return nil, fmt.Errorf("parse log level: %w", err)
	}

	var zapConfig zap.Config

	// 开发日志强调可读性；生产日志强调结构固定、便于机器检索。
	if cfg.App.Env == "development" {
		zapConfig = zap.NewDevelopmentConfig()
		zapConfig.Encoding = "console"
	} else {
		zapConfig = zap.NewProductionConfig()
		zapConfig.Encoding = "json"
	}

	// 使用统一字段名和 ISO8601 时间，避免不同环境的日志结构产生不必要差异。
	zapConfig.Level = level
	zapConfig.EncoderConfig.TimeKey = "timestamp"
	zapConfig.EncoderConfig.MessageKey = "message"
	zapConfig.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	log, err := zapConfig.Build()
	if err != nil {
		return nil, fmt.Errorf("build logger: %w", err)
	}

	// With 返回带固定字段的新 Logger，之后每条日志都会自动包含这两个字段。
	return log.With(
		zap.String("service", cfg.App.Name),
		zap.String("environment", cfg.App.Env),
	), nil
}
