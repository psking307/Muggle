// Package main 提供离线创建初始管理员的命令行工具。
//
// 设计文档 6.3 规定：v1.0 不提供网页创建管理员，
// 初始管理员只能通过本命令创建。用法（在 backend 目录）：
//
//	go run ./cmd/admin
//
// 命令会交互式询问用户名和密码；密码输入不回显、不写入任何日志。
package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/psking307/Muggle/backend/internal/admin"
	"github.com/psking307/Muggle/backend/internal/config"
	"github.com/psking307/Muggle/backend/internal/platform/database"
	"golang.org/x/term"
)

// main 只处理退出码，真正的流程放在 run 里，
// 这样 run 可以用 defer 保证数据库连接在退出前关闭。
func main() {
	if err := run(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// run 按“配置 -> 数据库 -> 输入凭据 -> 创建账号”的顺序执行。
func run() error {
	// 与 API 进程共用同一份配置，保证连的是同一个开发数据库。
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	db, sqlDB, err := database.Open(cfg.MySQL)
	if err != nil {
		return fmt.Errorf("connect MySQL: %w", err)
	}
	defer func() {
		_ = sqlDB.Close()
	}()

	// 组装与 API 完全相同的业务栈：离线命令也走 Service，
	// 密码哈希、格式校验等规则不会出现两份实现。
	repository := admin.NewGORMRepository(db)
	service := admin.NewService(
		repository,
		cfg.Auth.JWTSecret,
		cfg.Auth.AccessTokenTTL,
		cfg.Auth.RefreshSessionTTL,
	)

	fmt.Println("== Muggle 离线创建管理员 ==")

	// 所有行读取共用一个 bufio.Reader：
	// Reader 会提前把输入缓冲进内部缓存，如果每次读取都新建实例，
	// 管道模式下后续内容会被上一个实例“吞掉”而导致 EOF。
	input := bufio.NewReader(os.Stdin)

	username, err := promptLine(input, "用户名")
	if err != nil {
		return fmt.Errorf("read username: %w", err)
	}

	password, err := promptSecret(input, "密码（输入不回显）")
	if err != nil {
		return fmt.Errorf("read password: %w", err)
	}
	confirmation, err := promptSecret(input, "再次输入密码确认")
	if err != nil {
		return fmt.Errorf("read password confirmation: %w", err)
	}
	if password != confirmation {
		return errors.New("两次输入的密码不一致，已取消创建")
	}

	// 创建失败（例如用户名已存在）时直接返回错误，不输出任何密码或哈希。
	ctx := context.Background()
	if err := service.CreateInitialAdmin(ctx, username, password); err != nil {
		if errors.Is(err, admin.ErrUsernameTaken) {
			return fmt.Errorf("用户名 %q 已存在，请换一个用户名", username)
		}
		return fmt.Errorf("create admin: %w", err)
	}

	fmt.Printf("管理员 %q 创建成功，现在可以登录后台。\n", username)
	return nil
}

// promptLine 打印提示并读取一行非空文本。
func promptLine(input *bufio.Reader, prompt string) (string, error) {
	fmt.Printf("%s: ", prompt)

	line, err := input.ReadString('\n')
	if err != nil {
		return "", err
	}

	value := strings.TrimSpace(line)
	if value == "" {
		return "", errors.New("输入不能为空")
	}
	return value, nil
}

// promptSecret 读取密码。终端支持时关闭回显（x/term.ReadPassword）；
// 不支持时（例如脚本管道输入）退回普通行读取，保证命令在 CI 中也能使用。
// input 只在非终端回退路径使用，与调用方共用以避免缓冲吞掉后续输入。
func promptSecret(input *bufio.Reader, prompt string) (string, error) {
	fmt.Printf("%s: ", prompt)

	// os.Stdin.Fd() 返回文件描述符；term.IsTerminal 判断它是否真的是交互终端。
	if term.IsTerminal(int(os.Stdin.Fd())) {
		raw, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Println() // ReadPassword 不会回显换行，这里手动补一个。
		if err != nil {
			return "", err
		}
		value := strings.TrimSpace(string(raw))
		if value == "" {
			return "", errors.New("输入不能为空")
		}
		return value, nil
	}

	return promptLine(input, prompt)
}
