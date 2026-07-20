package employee

import (
	"reflect"
	"testing"
)

// effectiveAllowedActions 是 P-enforce 的核心收敛逻辑(员工配置页 spec E3,交集,员工为上限)。
func TestEffectiveAllowedActions(t *testing.T) {
	tests := []struct {
		name             string
		requested        []string
		permissionPolicy map[string]any
		want             []string
	}{
		{
			name:             "员工白名单缺失 → 不约束,返回请求原值",
			requested:        []string{"code.write", "shell.exec"},
			permissionPolicy: map[string]any{},
			want:             []string{"code.write", "shell.exec"},
		},
		{
			name:             "员工白名单为空数组 → 不约束",
			requested:        []string{"code.write"},
			permissionPolicy: map[string]any{"allowed_actions": []any{}},
			want:             []string{"code.write"},
		},
		{
			name:             "员工白名单非空、请求为空 → 收敛到员工白名单",
			requested:        nil,
			permissionPolicy: map[string]any{"allowed_actions": []any{"code.read", "code.write"}},
			want:             []string{"code.read", "code.write"},
		},
		{
			name:             "两者非空 → 交集,保留请求顺序",
			requested:        []string{"shell.exec", "code.write", "network.egress"},
			permissionPolicy: map[string]any{"allowed_actions": []any{"code.write", "code.read", "shell.exec"}},
			want:             []string{"shell.exec", "code.write"},
		},
		{
			name:             "请求越界(不在员工上限内) → 交集为空,越权动作被剥离",
			requested:        []string{"network.egress"},
			permissionPolicy: map[string]any{"allowed_actions": []any{"code.read"}},
			want:             []string{},
		},
		{
			name:             "permission_policy 为 nil → 安全,返回请求原值",
			requested:        []string{"code.write"},
			permissionPolicy: nil,
			want:             []string{"code.write"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := effectiveAllowedActions(tt.requested, tt.permissionPolicy)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("effectiveAllowedActions(%v, %v) = %v, want %v", tt.requested, tt.permissionPolicy, got, tt.want)
			}
		})
	}
}
