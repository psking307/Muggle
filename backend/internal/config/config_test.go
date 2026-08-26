package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// configEnvironmentNames 列出所有可能影响配置加载结果的环境变量。
// 每个测试开始前都会暂时清除它们，避免开发者电脑或 CI 环境中的变量污染测试结果。
var configEnvironmentNames = []string{
	"APP_ENV",
	"APP_NAME",
	"LOG_LEVEL",
	"HTTP_ADDR",
	"HTTP_READ_HEADER_TIMEOUT",
	"HTTP_READ_TIMEOUT",
	"HTTP_WRITE_TIMEOUT",
	"HTTP_IDLE_TIMEOUT",
	"HTTP_SHUTDOWN_TIMEOUT",
}

// validTestConfig 是多项测试共用的一份完整且合法的 YAML 配置。
// 使用原始字符串（反引号）可以保留换行与缩进，使内容和真实 YAML 文件一致。
const validTestConfig = `app:
  env: development
  name: Test Muggle
log:
  level: debug
http:
  addr: ":8080"
  read_header_timeout: 5s
  read_timeout: 10s
  write_timeout: 15s
  idle_timeout: 60s
  shutdown_timeout: 10s
`

// TestLoadFile 验证 loadFile 能读取 YAML，并把字符串形式的时长转换成 time.Duration。
func TestLoadFile(t *testing.T) {
	// 准备：清除外部环境变量，再把合法配置写入只属于本测试的临时文件。
	clearConfigEnvironment(t)
	configFile := writeTestConfig(t, validTestConfig)

	// 执行：调用真正的配置加载函数。
	cfg, err := loadFile(configFile)

	// 验证：require 失败时会立刻停止当前测试，防止继续使用无效的 cfg；
	// assert 失败时仍会继续检查后续字段，一次运行可以看到更多不一致之处。
	require.NoError(t, err)
	assert.Equal(t, "development", cfg.App.Env)
	assert.Equal(t, "Test Muggle", cfg.App.Name)
	assert.Equal(t, ":8080", cfg.HTTP.Addr)
	assert.Equal(t, 5*time.Second, cfg.HTTP.ReadHeaderTimeout)
	assert.Equal(t, 10*time.Second, cfg.HTTP.ReadTimeout)
	assert.Equal(t, 15*time.Second, cfg.HTTP.WriteTimeout)
	assert.Equal(t, 60*time.Second, cfg.HTTP.IdleTimeout)
	assert.Equal(t, 10*time.Second, cfg.HTTP.ShutdownTimeout)
}

// TestLoadFileUsesEnvironmentOverrides 验证环境变量的优先级高于 YAML 文件。
func TestLoadFileUsesEnvironmentOverrides(t *testing.T) {
	// 先清空环境，再只设置本测试关心的两个覆盖值。
	clearConfigEnvironment(t)
	t.Setenv("APP_ENV", "production")
	t.Setenv("HTTP_ADDR", ":9090")
	configFile := writeTestConfig(t, validTestConfig)

	cfg, err := loadFile(configFile)

	require.NoError(t, err)
	// YAML 中原本是 development 和 :8080；这里必须得到环境变量提供的新值。
	assert.Equal(t, "production", cfg.App.Env)
	assert.Equal(t, ":9090", cfg.HTTP.Addr)
}

// TestLoadFileRejectsInvalidAddress 验证非法监听地址不会悄悄通过配置检查。
func TestLoadFileRejectsInvalidAddress(t *testing.T) {
	clearConfigEnvironment(t)
	// 这份配置只有 addr: invalid 是故意写错的，其余必填项保持合法，
	// 从而保证测试失败原因明确来自地址校验。
	configFile := writeTestConfig(t, `app:
  env: development
  name: Test Muggle
log:
  level: debug
http:
  addr: invalid
  read_header_timeout: 5s
  read_timeout: 10s
  write_timeout: 15s
  idle_timeout: 60s
  shutdown_timeout: 10s
`)

	// 使用下划线忽略失败情况下没有意义的配置对象，只检查返回的错误。
	_, err := loadFile(configFile)

	require.Error(t, err)
	// 同时检查错误上下文，确认报错确实发生在 HTTP 地址校验阶段。
	assert.Contains(t, err.Error(), "validate HTTP address")
}

// writeTestConfig 把给定内容写入一个临时 YAML 文件，并返回文件路径。
func writeTestConfig(t *testing.T, contents string) string {
	// Helper 告诉 testing：若本辅助函数内断言失败，错误位置应指向调用它的测试代码。
	t.Helper()

	// t.TempDir 创建的目录会在测试结束后自动删除，不会在项目中留下测试文件。
	configFile := filepath.Join(t.TempDir(), "default.yaml")
	// 0o600 表示只有当前用户可以读写该文件，其他用户没有权限。
	require.NoError(t, os.WriteFile(configFile, []byte(contents), 0o600))
	return configFile
}

// clearConfigEnvironment 暂时清除配置相关环境变量，并在测试结束后恢复原状。
func clearConfigEnvironment(t *testing.T) {
	t.Helper()

	for _, name := range configEnvironmentNames {
		// 先保存变量原来的值和“是否存在”，再从当前测试环境中删除它。
		previousValue, existed := os.LookupEnv(name)
		require.NoError(t, os.Unsetenv(name))

		// 为清理闭包保存本轮循环的数据，确保每个清理函数恢复的是对应变量。
		environmentName := name
		value := previousValue
		wasSet := existed
		t.Cleanup(func() {
			if wasSet {
				// 测试前存在的变量恢复原值。
				_ = os.Setenv(environmentName, value)
				return
			}
			// 测试前不存在的变量保持不存在。
			_ = os.Unsetenv(environmentName)
		})
	}
}
