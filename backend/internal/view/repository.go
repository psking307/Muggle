package view

import "context"

// Repository 描述 Worker 在消费一条浏览事件后，向数据层提出的请求。
//
// 只有 ProcessEvent 一个方法：它把「幂等去重 + 计数累加」封装成一次原子操作，
// 由 GORM 实现保证要么全部成功、要么全部回滚。测试时可以用内存 fake 替换。
type Repository interface {
	// ProcessEvent 处理一条浏览事件，返回 error 表示处理失败。
	//
	// 约定（由实现保证）：
	//   * 同一 event_id 重复调用不会重复累加（幂等）；
	//   * 事件指向的文章不存在时返回 ErrInvalidEvent（永久非法事件）；
	//   * 其它错误（如数据库暂时不可用）会返回普通错误，由调用方决定是否重试。
	ProcessEvent(ctx context.Context, event ViewEvent) error
}
