package view

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// fakeRepository 在内存中实现 Repository，记录调用并返回预设错误。
//
// 消费者单元测试只关心“提交 offset 的决策”是否正确，因此这里不实现真实的
// 事务逻辑（那些由 repository_gorm_integration_test.go 的集成测试覆盖）。
type fakeRepository struct {
	processed []ViewEvent // 历次 ProcessEvent 收到的事件
	err       error       // ProcessEvent 返回的错误
}

// ProcessEvent 记录事件并返回预设错误。
func (f *fakeRepository) ProcessEvent(_ context.Context, event ViewEvent) error {
	f.processed = append(f.processed, event)
	return f.err
}

// newTestConsumer 构造一个不依赖真实 Kafka 客户端的 Consumer，仅用于测试 handleEvent。
func newTestConsumer(repo Repository) *Consumer {
	return &Consumer{
		repo: repo,
		log:  zap.NewNop(), // 测试不关心日志输出，用 Nop 丢弃
	}
}

// validEventValue 生成一条合法事件的消息体（JSON 字节）。
func validEventValue(postID uint64) []byte {
	event := NewViewEvent(postID, time.Now())
	data, _ := event.Marshal()
	return data
}

// TestHandleEventMalformedJSON 验证无法反序列化的消息被当作永久非法事件跳过
// （返回 true 表示提交 offset），且不会触发数据库处理。
func TestHandleEventMalformedJSON(t *testing.T) {
	repo := &fakeRepository{}
	consumer := newTestConsumer(repo)

	commit := consumer.handleEvent(context.Background(), []byte("this is not json"))

	assert.True(t, commit, "损坏消息应提交 offset 跳过")
	assert.Empty(t, repo.processed, "损坏消息不应进入数据库处理")
}

// TestHandleEventInvalidEventType 验证 event_type 不认识的事件被跳过。
func TestHandleEventInvalidEventType(t *testing.T) {
	repo := &fakeRepository{}
	consumer := newTestConsumer(repo)

	// 构造一条 event_type 非法的事件。
	event := ViewEvent{
		EventID:    "e",
		EventType:  "post.liked.v1", // 未知类型
		PostID:     1,
		OccurredAt: time.Now(),
	}
	value, err := event.Marshal()
	require.NoError(t, err)

	commit := consumer.handleEvent(context.Background(), value)

	assert.True(t, commit, "非法事件应提交 offset 跳过")
	assert.Empty(t, repo.processed, "非法事件不应进入数据库处理")
}

// TestHandleEventSuccess 验证合法事件会进入数据库处理，并返回提交 offset。
func TestHandleEventSuccess(t *testing.T) {
	repo := &fakeRepository{}
	consumer := newTestConsumer(repo)

	commit := consumer.handleEvent(context.Background(), validEventValue(42))

	assert.True(t, commit, "处理成功应提交 offset")
	require.Len(t, repo.processed, 1)
	assert.Equal(t, uint64(42), repo.processed[0].PostID)
}

// TestHandleEventTransientError 验证数据库暂时失败时不提交 offset（返回 false），
// 让 Kafka 稍后重投；幂等表保证重投不会重复计数。
func TestHandleEventTransientError(t *testing.T) {
	repo := &fakeRepository{err: errors.New("mysql temporarily unavailable")}
	consumer := newTestConsumer(repo)

	commit := consumer.handleEvent(context.Background(), validEventValue(7))

	assert.False(t, commit, "暂时失败不应提交 offset")
	require.Len(t, repo.processed, 1)
}

// TestHandleEventInvalidEvent 验证 Repository 返回 ErrInvalidEvent（如文章不存在）
// 时被当作永久非法事件跳过并提交 offset。
func TestHandleEventInvalidEvent(t *testing.T) {
	repo := &fakeRepository{err: ErrInvalidEvent}
	consumer := newTestConsumer(repo)

	commit := consumer.handleEvent(context.Background(), validEventValue(9))

	assert.True(t, commit, "永久非法事件应提交 offset 跳过")
	require.Len(t, repo.processed, 1)
}
