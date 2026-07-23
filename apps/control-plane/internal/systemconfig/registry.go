package systemconfig

import (
	"fmt"
	"time"
)

// 领域标签:前端按此分 tab,新增 domain 前端零改动(未知 domain 落"其他")。
const (
	DomainArtifact     = "artifact"
	DomainExecution    = "execution"
	DomainSecurity     = "security"
	DomainOrganization = "organization"
)

// 各配置项 key。使用点经 Reader 取值时引用这些常量。
const (
	KeyArtifactMaxFileSizeBytes           = "artifact.max_file_size_bytes"
	KeyArtifactPresignUploadTTL           = "artifact.presign_upload_ttl_seconds"
	KeyArtifactContentGetTTL              = "artifact.content_get_ttl_seconds"
	KeyArtifactAttachmentMaxFileSizeBytes = "artifact.attachment_max_file_size_bytes"
	KeyArtifactAttachmentMaxCount         = "artifact.attachment_max_count"
	KeyArtifactAttachmentTotalMaxBytes    = "artifact.attachment_total_max_bytes"
	KeySkillUploadMaxBytes                = "skill.upload_max_bytes"
	KeySkillArchivePresignTTL             = "skill.archive_presign_ttl_seconds"
	KeySkillArchiveUnpackMaxBytes         = "skill.archive_unpack_max_bytes"
	KeySkillArchiveUnpackMaxFileCount     = "skill.archive_unpack_max_file_count"
	KeyRuntimeSessionTTLSeconds           = "runtime.session_ttl_seconds"
	KeyRuntimeHeartbeatTimeoutSeconds     = "runtime.heartbeat_timeout_seconds"
	KeyRuntimeWorkspaceBaseDir            = "runtime.workspace_base_dir"
	KeyAuthSessionTTLSeconds              = "auth.session_ttl_seconds"
	KeyTaskStuckRunningTimeoutSeconds     = "task.stuck_running_timeout_seconds"
	KeyEmployeeMaxPerTeam                 = "employee.max_per_team"
)

// registry 是配置项注册表:非封闭枚举,新增配置项 = 此处加一条 + 使用点接入,
// 无迁移、无契约变更(列表接口按注册表动态返回)。默认值是原使用点常量的单一定义处。
var registry = []Definition{
	{
		Key:    KeyArtifactMaxFileSizeBytes,
		Domain: DomainArtifact,
		Label:  "单个工件大小上限",
		Description: "runtime 回传单个工件(含报告文件与声明式交付物)的最大字节数,超限的 presign 请求被拒绝。" +
			"生效值经心跳下发到 runtime(P2);租户内存在不支持限额下发的在线旧 runtime 时,presign 按 10MiB 收紧(版本偏斜护栏),避免静默丢文件。",
		ValueType:    ValueTypeBytes,
		DefaultValue: 10 * 1024 * 1024,
		MinValue:     1 * 1024 * 1024,
		MaxValue:     100 * 1024 * 1024,
	},
	{
		Key:          KeyArtifactPresignUploadTTL,
		Domain:       DomainArtifact,
		Label:        "工件上传链接有效期",
		Description:  "为 runtime 签发的工件直传 URL 的有效秒数。",
		ValueType:    ValueTypeDurationSeconds,
		DefaultValue: 15 * 60,
		MinValue:     60,
		MaxValue:     3600,
	},
	{
		Key:          KeyArtifactContentGetTTL,
		Domain:       DomainArtifact,
		Label:        "工件下载链接有效期",
		Description:  "控制台取回工件内容的 presign GET URL 的有效秒数。",
		ValueType:    ValueTypeDurationSeconds,
		DefaultValue: 5 * 60,
		MinValue:     60,
		MaxValue:     3600,
	},
	{
		Key:    KeyArtifactAttachmentMaxFileSizeBytes,
		Domain: DomainArtifact,
		Label:  "单个附件大小上限",
		Description: "runtime 兜底采集的输出附件单文件最大字节数,超限文件跳过并留 execution_output_skipped 痕。" +
			"经心跳下发到 runtime,收敛粒度为下一次任务。",
		ValueType:    ValueTypeBytes,
		DefaultValue: 5 * 1024 * 1024,
		MinValue:     1 * 1024 * 1024,
		MaxValue:     10 * 1024 * 1024,
	},
	{
		Key:          KeyArtifactAttachmentMaxCount,
		Domain:       DomainArtifact,
		Label:        "附件数量上限",
		Description:  "单次执行兜底采集的输出附件最大个数,超出部分跳过并留痕。经心跳下发到 runtime。",
		ValueType:    ValueTypeInt,
		DefaultValue: 20,
		MinValue:     1,
		MaxValue:     100,
	},
	{
		Key:          KeyArtifactAttachmentTotalMaxBytes,
		Domain:       DomainArtifact,
		Label:        "附件总大小上限",
		Description:  "单次执行兜底采集的输出附件合计最大字节数,超出部分跳过并留痕。经心跳下发到 runtime。",
		ValueType:    ValueTypeBytes,
		DefaultValue: 50 * 1024 * 1024,
		MinValue:     10 * 1024 * 1024,
		MaxValue:     200 * 1024 * 1024,
	},
	{
		Key:          KeySkillUploadMaxBytes,
		Domain:       DomainArtifact,
		Label:        "技能包上传大小上限",
		Description:  "技能包 zip 上传的最大字节数。上限 200MiB 与 runtime 解包限额对齐。",
		ValueType:    ValueTypeBytes,
		DefaultValue: 50 * 1024 * 1024,
		MinValue:     1 * 1024 * 1024,
		MaxValue:     200 * 1024 * 1024,
	},
	{
		Key:          KeySkillArchivePresignTTL,
		Domain:       DomainArtifact,
		Label:        "技能归档下载链接有效期",
		Description:  "为 runtime 物化技能签发的归档下载 URL 的有效秒数。",
		ValueType:    ValueTypeDurationSeconds,
		DefaultValue: 15 * 60,
		MinValue:     60,
		MaxValue:     3600,
	},
	{
		Key:          KeySkillArchiveUnpackMaxBytes,
		Domain:       DomainArtifact,
		Label:        "技能包解包大小上限",
		Description:  "runtime 物化技能时允许的归档最大字节数,超限拒绝解包。经心跳下发到 runtime。",
		ValueType:    ValueTypeBytes,
		DefaultValue: 200 * 1024 * 1024,
		MinValue:     50 * 1024 * 1024,
		MaxValue:     500 * 1024 * 1024,
	},
	{
		Key:          KeySkillArchiveUnpackMaxFileCount,
		Domain:       DomainArtifact,
		Label:        "技能包解包文件数上限",
		Description:  "runtime 物化技能时允许的归档最大文件条目数,超限拒绝解包。经心跳下发到 runtime。",
		ValueType:    ValueTypeInt,
		DefaultValue: 10000,
		MinValue:     100,
		MaxValue:     50000,
	},
	{
		Key:    KeyRuntimeHeartbeatTimeoutSeconds,
		Domain: DomainExecution,
		Label:  "Runtime 心跳超时",
		Description: "节点最近一次心跳距今超过该秒数即判离线,影响\"节点在线\"判定、调度候选与 runtime scope 授权的活性窗口。" +
			"调大意味着离线节点更晚被判离线;上限 600 秒防止把僵尸节点长期当在线。",
		ValueType:    ValueTypeDurationSeconds,
		DefaultValue: 60,
		MinValue:     10,
		MaxValue:     600,
	},
	{
		Key:    KeyRuntimeWorkspaceBaseDir,
		Domain: DomainExecution,
		Label:  "系统工作区根目录",
		Description: "Runtime 派生项目目录与员工能力缓存的工作区根路径约定。" +
			"高危、不宜改动；改后不自动迁存量数据，已有目录会相对旧根 orphan。" +
			"节点本地 config.yaml / RUNTIME_AGENT_WORKSPACE_DIR 可覆盖平台下发值；生效优先级：节点本地 > 平台下发 > 二进制内置默认。" +
			"经心跳 platform_limits 下发到 runtime。",
		ValueType:          ValueTypeString,
		DefaultStringValue: "/var/superteam/workspaces",
		MaxStringLength:    512,
	},
	{
		Key:          KeyRuntimeSessionTTLSeconds,
		Domain:       DomainSecurity,
		Label:        "Runtime 会话有效期",
		Description:  "runtime 节点会话令牌的有效秒数;只影响新签发/续期的会话,存量会话按签发时值到期。",
		ValueType:    ValueTypeDurationSeconds,
		DefaultValue: 12 * 3600,
		MinValue:     3600,
		MaxValue:     7 * 24 * 3600,
	},
	{
		Key:          KeyAuthSessionTTLSeconds,
		Domain:       DomainSecurity,
		Label:        "登录会话有效期",
		Description:  "控制台登录会话的有效秒数;只影响新登录,存量会话按签发时值到期,不做存量回收。",
		ValueType:    ValueTypeDurationSeconds,
		DefaultValue: 12 * 3600,
		MinValue:     3600,
		MaxValue:     7 * 24 * 3600,
	},
	{
		Key:    KeyTaskStuckRunningTimeoutSeconds,
		Domain: DomainExecution,
		Label:  "僵尸任务收敛超时",
		Description: "项目任务停留在\"执行中\"但无任何活跃执行尝试(无 attempt/无 run)超过该秒数,系统看门狗判定为卡死并置为失败,触发失败恢复决策卡转人工。" +
			"用于兜底 runtime 整个失联、协调线程死亡或异常数据导致的任务永久卡 running;仅在控制平面内部判定,不下发 runtime。" +
			"下界防止误判正在派发的正常任务,上界防止卡死任务长期占用员工\"工作中\"状态。",
		ValueType:    ValueTypeDurationSeconds,
		DefaultValue: 15 * 60,
		MinValue:     2 * 60,
		MaxValue:     6 * 3600,
	},
	{
		Key:    KeyEmployeeMaxPerTeam,
		Domain: DomainOrganization,
		Label:  "单团队数字员工上限",
		Description: "一个团队内在册数字员工的最大数量;创建选项按此做容量预检,超限的入队创建与建团初始成员列表被拒绝。" +
			"历史超限团队不受影响(只拦新增),调大即可继续向该团队补员。",
		ValueType:    ValueTypeInt,
		DefaultValue: 10,
		MinValue:     1,
		MaxValue:     500,
	},
}

var registryByKey = buildRegistryIndex()

func buildRegistryIndex() map[string]Definition {
	index := make(map[string]Definition, len(registry))
	for _, def := range registry {
		if def.Key == "" || def.Label == "" || def.Domain == "" {
			panic(fmt.Sprintf("systemconfig: definition %q missing key/label/domain", def.Key))
		}
		if _, dup := index[def.Key]; dup {
			panic(fmt.Sprintf("systemconfig: duplicate definition key %q", def.Key))
		}
		switch def.ValueType {
		case ValueTypeBytes, ValueTypeDurationSeconds, ValueTypeInt:
			if def.MinValue > def.MaxValue {
				panic(fmt.Sprintf("systemconfig: definition %q has min > max", def.Key))
			}
			if def.DefaultValue < def.MinValue || def.DefaultValue > def.MaxValue {
				panic(fmt.Sprintf("systemconfig: definition %q default out of bounds", def.Key))
			}
		case ValueTypeString:
			maxLen := def.EffectiveMaxStringLength()
			if len(def.DefaultStringValue) == 0 {
				panic(fmt.Sprintf("systemconfig: definition %q string default is empty", def.Key))
			}
			if len(def.DefaultStringValue) > maxLen {
				panic(fmt.Sprintf("systemconfig: definition %q string default exceeds max length %d", def.Key, maxLen))
			}
		default:
			panic(fmt.Sprintf("systemconfig: definition %q has unknown value type %q", def.Key, def.ValueType))
		}
		index[def.Key] = def
	}
	return index
}

// Definitions 返回注册表快照(顺序稳定)。
func Definitions() []Definition {
	out := make([]Definition, len(registry))
	copy(out, registry)
	return out
}

// LookupDefinition 按 key 查定义。
func LookupDefinition(key string) (Definition, bool) {
	def, ok := registryByKey[key]
	return def, ok
}

// DefaultFor 返回注册表数值型默认值,是各使用点 Reader 未注入时的统一兜底,
// 避免默认值在消费方重复定义产生漂移。未注册 key 或非数值型 panic(编程错误)。
func DefaultFor(key string) int64 {
	def, ok := registryByKey[key]
	if !ok {
		panic(fmt.Sprintf("systemconfig: DefaultFor unknown key %q", key))
	}
	if def.IsStringType() {
		panic(fmt.Sprintf("systemconfig: DefaultFor called on string key %q; use DefaultStringFor", key))
	}
	return def.DefaultValue
}

// DefaultStringFor 返回注册表 string 型默认值。未注册 key 或非 string 型 panic。
func DefaultStringFor(key string) string {
	def, ok := registryByKey[key]
	if !ok {
		panic(fmt.Sprintf("systemconfig: DefaultStringFor unknown key %q", key))
	}
	if !def.IsStringType() {
		panic(fmt.Sprintf("systemconfig: DefaultStringFor called on non-string key %q", key))
	}
	return def.DefaultStringValue
}

// DefaultDurationFor 是 DefaultFor 的 duration_seconds 便捷形态。
func DefaultDurationFor(key string) time.Duration {
	return time.Duration(DefaultFor(key)) * time.Second
}
