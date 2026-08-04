package employee

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type stubVocabularyValidator struct {
	unknown []string
	err     error
	seen    []string
}

func (s *stubVocabularyValidator) ValidateCapabilityKeys(_ context.Context, _ uuid.UUID, keys []string) ([]string, error) {
	s.seen = append(s.seen, keys...)
	return s.unknown, s.err
}

// 能力词表是模板 required_capabilities 与员工声明的**共用**词表。此前只有模板
// 侧校验、员工侧随便写，于是"词表里注册的键 0 人声明 / 员工在用的键根本没注册"
// 两头落空。单边校验等于没有词表。
func TestValidateDeclaredCapabilities(t *testing.T) {
	ctx := context.Background()
	tenantID := uuid.New()
	bindings := func(keys ...string) map[string]any {
		values := make([]any, 0, len(keys))
		for _, key := range keys {
			values = append(values, key)
		}
		return map[string]any{"external_capabilities": values}
	}

	t.Run("未注入校验器时放行（与包内其他可选端口同约定）", func(t *testing.T) {
		service := &Service{}
		require.NoError(t, service.validateDeclaredCapabilities(ctx, tenantID, bindings("whatever")))
	})

	t.Run("全部已注册：放行", func(t *testing.T) {
		stub := &stubVocabularyValidator{}
		service := &Service{vocabulary: stub}
		require.NoError(t, service.validateDeclaredCapabilities(ctx, tenantID, bindings("code_review", "test_execution")))
		require.Equal(t, []string{"code_review", "test_execution"}, stub.seen)
	})

	t.Run("存在未注册键：拒绝并点名", func(t *testing.T) {
		service := &Service{vocabulary: &stubVocabularyValidator{unknown: []string{"made_up"}}}
		err := service.validateDeclaredCapabilities(ctx, tenantID, bindings("code_review", "made_up"))
		require.ErrorIs(t, err, ErrInvalidInput)
		require.Contains(t, err.Error(), "made_up", "必须点名是哪个键，否则用户不知道改什么")
	})

	t.Run("空声明不触发查询", func(t *testing.T) {
		stub := &stubVocabularyValidator{}
		service := &Service{vocabulary: stub}
		require.NoError(t, service.validateDeclaredCapabilities(ctx, tenantID, map[string]any{}))
		require.Empty(t, stub.seen)
	})

	t.Run("词表查询失败必须冒泡，不得静默放行", func(t *testing.T) {
		sentinel := errors.New("vocabulary unavailable")
		service := &Service{vocabulary: &stubVocabularyValidator{err: sentinel}}
		require.ErrorIs(t, service.validateDeclaredCapabilities(ctx, tenantID, bindings("code_review")), sentinel)
	})
}
