package view

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestViewEventMarshalRoundTrip 验证事件序列化后再反序列化能还原所有字段。
// 这保证 API 生产者写出的 JSON 与 Worker 消费者读回的 JSON 结构一致。
func TestViewEventMarshalRoundTrip(t *testing.T) {
	occurred := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	original := ViewEvent{
		EventID:    "4d6314d3-f834-4e26-84a1-afe7f940b5dd",
		EventType:  EventTypePostViewed,
		PostID:     12,
		OccurredAt: occurred,
	}

	data, err := original.Marshal()
	require.NoError(t, err)

	restored, err := ParseViewEvent(data)
	require.NoError(t, err)
	assert.Equal(t, original, restored)
}

// TestViewEventJSONFieldNames 验证 JSON 字段名与设计文档 7.2 完全一致。
// 字段名一旦写进 Kafka 就属于契约的一部分，改动会导致旧消息无法解析。
func TestViewEventJSONFieldNames(t *testing.T) {
	event := ViewEvent{
		EventID:    "id",
		EventType:  EventTypePostViewed,
		PostID:     3,
		OccurredAt: time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC),
	}

	data, err := json.Marshal(event)
	require.NoError(t, err)

	var raw map[string]any
	require.NoError(t, json.Unmarshal(data, &raw))

	assert.Contains(t, raw, "event_id")
	assert.Contains(t, raw, "event_type")
	assert.Contains(t, raw, "post_id")
	assert.Contains(t, raw, "occurred_at")
}

// TestNewViewEventGeneratesUniqueIDs 验证每次构造的事件都带唯一且非空的 event_id。
// event_id 是幂等去重的关键：若两次浏览生成相同 ID，就会少算一次浏览量。
func TestNewViewEventGeneratesUniqueIDs(t *testing.T) {
	now := time.Now()

	first := NewViewEvent(1, now)
	second := NewViewEvent(1, now)

	assert.NotEmpty(t, first.EventID)
	assert.NotEmpty(t, second.EventID)
	assert.NotEqual(t, first.EventID, second.EventID)
	assert.Equal(t, EventTypePostViewed, first.EventType)
}

// TestViewEventValidate 验证业务字段校验覆盖各种非法输入。
func TestViewEventValidate(t *testing.T) {
	valid := ViewEvent{
		EventID:    "e",
		EventType:  EventTypePostViewed,
		PostID:     1,
		OccurredAt: time.Now(),
	}

	testCases := []struct {
		name    string
		mutate  func(*ViewEvent)
		wantErr bool
	}{
		{name: "合法事件", mutate: func(*ViewEvent) {}, wantErr: false},
		{name: "缺少事件类型", mutate: func(e *ViewEvent) { e.EventType = "" }, wantErr: true},
		{name: "未知事件类型", mutate: func(e *ViewEvent) { e.EventType = "post.viewed.v2" }, wantErr: true},
		{name: "post_id 为零", mutate: func(e *ViewEvent) { e.PostID = 0 }, wantErr: true},
		{name: "event_id 为空", mutate: func(e *ViewEvent) { e.EventID = "" }, wantErr: true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			event := valid
			tc.mutate(&event)

			err := event.Validate()
			if tc.wantErr {
				assert.ErrorIs(t, err, ErrInvalidEvent)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
