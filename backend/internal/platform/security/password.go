// Package security 提供管理员认证使用的密码哈希、Access JWT 和 Refresh Token 工具。
//
// 这些函数与 HTTP、GORM 都无关，是纯计算逻辑，便于单独单元测试；
// admin 包通过它们完成“哈希”“签发”“验证”，而不是各写一份实现。
package security

import (
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

// bcryptCost 是 bcrypt 的计算开销参数（4～31，越大越慢）。
// 12 是当前常见的平衡值：单次哈希约几十毫秒，能显著拖慢离线暴力破解，
// 又不至于让登录接口的延迟无法接受。
const bcryptCost = 12

// bcryptMaxPasswordBytes 是 bcrypt 算法的输入上限。
// 超过 72 字节的密码会被算法静默截断，因此我们在业务层提前拒绝，避免
// “输入了超长密码，但真正参与哈希的只有前 72 字节”的意外。
const bcryptMaxPasswordBytes = 72

// HashPassword 使用 bcrypt 把明文密码转成不可逆哈希。
//
// bcrypt 每次都会生成随机盐，同一密码两次调用得到的哈希也不同；
// 校验时必须使用 CheckPassword 而不是直接比较两个哈希字符串。
func HashPassword(password string) (string, error) {
	if len(password) > bcryptMaxPasswordBytes {
		return "", fmt.Errorf("hash password: length %d exceeds bcrypt limit %d bytes",
			len(password), bcryptMaxPasswordBytes)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}
	return string(hash), nil
}

// CheckPassword 判断明文密码是否与已保存的 bcrypt 哈希匹配。
//
// 返回 bool 而不是 error：密码不匹配是预期的业务结果（登录失败），
// 不需要把“哈希格式损坏”这类细节泄露给调用方。
func CheckPassword(passwordHash, password string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(password))
	return err == nil
}
