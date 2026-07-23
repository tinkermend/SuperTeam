// Package systemconfig 是平台运行态参数的配置中心(spec 2026-07-19)。
// 配置项定义(key/类型/默认值/边界/文案)在服务端注册表(registry.go)随代码演进;
// 数据库只存管理员显式修改的覆盖值,"恢复默认" = 删除覆盖行。
// 部署态基础设施配置(env/config.yaml)与资源级业务配置不属于本模块。
package systemconfig

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

var (
	// ErrUnknownKey 配置 key 不在注册表中。
	ErrUnknownKey = errors.New("unknown system config key")
	// ErrInvalidValue 值类型不匹配或越界。
	ErrInvalidValue = errors.New("invalid system config value")
)

// 值类型:数值型以 int64 承载;string 以 UTF-8 文本承载。
// int 是纯计数型(P2 新增),前端编辑弹窗对其免单位换算。
const (
	ValueTypeBytes           = "bytes"
	ValueTypeDurationSeconds = "duration_seconds"
	ValueTypeInt             = "int"
	ValueTypeString          = "string"

	// DefaultMaxStringLength 是 string 型配置未显式声明 MaxStringLength 时的上限。
	DefaultMaxStringLength = 1024
)

// Definition 是一个配置项的注册表定义。
// 数值型用 DefaultValue/MinValue/MaxValue;string 型用 DefaultStringValue/MaxStringLength。
type Definition struct {
	Key          string
	Domain       string
	Label        string
	Description  string
	ValueType    string
	DefaultValue int64
	// MinValue/MaxValue 是服务端防呆边界(含端点),防止一次误操作弄瘫平台。
	MinValue int64
	MaxValue int64
	// DefaultStringValue / MaxStringLength 仅用于 ValueTypeString。
	// MaxStringLength 为 0 时使用 DefaultMaxStringLength。
	DefaultStringValue string
	MaxStringLength    int
}

// Override 是数据库中的一条覆盖记录。
// 数值型填 Value;string 型填 StringValue(二者按定义类型互斥使用)。
type Override struct {
	ConfigKey     string
	Value         int64
	StringValue   string
	UpdatedBy     *uuid.UUID
	UpdatedByName string
	UpdatedAt     time.Time
}

// EffectiveConfig 是列表接口返回的投影:定义 + 生效值 + 覆盖态。
type EffectiveConfig struct {
	Definition
	EffectiveValue       int64
	EffectiveStringValue string
	IsOverridden         bool
	UpdatedAt            *time.Time
	UpdatedByName        string
}

// IsStringType 报告定义是否为 string 值类型。
func (d Definition) IsStringType() bool {
	return d.ValueType == ValueTypeString
}

// EffectiveMaxStringLength 返回 string 型配置的生效长度上限。
func (d Definition) EffectiveMaxStringLength() int {
	if d.MaxStringLength > 0 {
		return d.MaxStringLength
	}
	return DefaultMaxStringLength
}
