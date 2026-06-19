package employee

type DigitalEmployeeOperationalStatus string

const (
	DigitalEmployeeOperationalStatusWorking            DigitalEmployeeOperationalStatus = "working"
	DigitalEmployeeOperationalStatusIdle               DigitalEmployeeOperationalStatus = "idle"
	DigitalEmployeeOperationalStatusQueued             DigitalEmployeeOperationalStatus = "queued"
	DigitalEmployeeOperationalStatusWaitingHuman       DigitalEmployeeOperationalStatus = "waiting_human"
	DigitalEmployeeOperationalStatusError              DigitalEmployeeOperationalStatus = "error"
	DigitalEmployeeOperationalStatusUnavailable        DigitalEmployeeOperationalStatus = "unavailable"
	DigitalEmployeeOperationalStatusNeedsConfiguration DigitalEmployeeOperationalStatus = "needs_configuration"
)

type DigitalEmployeeOperationalReason struct {
	Code    string
	Message string
}

type DigitalEmployeeOperationalState struct {
	Status      DigitalEmployeeOperationalStatus
	Reasons     []DigitalEmployeeOperationalReason
	CanDispatch bool
}

type DigitalEmployeeOperationalInput struct {
	DispatchReady                 bool
	ConfigurationMissing          bool
	RuntimeUnavailable            bool
	HasProviderFailure            bool
	HasTaskFailure                bool
	HasActiveWork                 bool
	HasWorkingRun                 bool
	HasQueuedWork                 bool
	HasEmployeeScopedHumanBlocker bool
	// HasProjectAcceptanceBlocker is carried only as an explicit guard fact; it must not affect employee-level waiting_human.
	HasProjectAcceptanceBlocker bool
}

func ResolveDigitalEmployeeOperationalState(input DigitalEmployeeOperationalInput) DigitalEmployeeOperationalState {
	if input.HasEmployeeScopedHumanBlocker {
		return newDigitalEmployeeOperationalState(input, DigitalEmployeeOperationalStatusWaitingHuman, []DigitalEmployeeOperationalReason{
			{Code: "approval_blocked", Message: "等待人工确认后继续执行"},
		})
	}

	if reasons := resolveDigitalEmployeeOperationalErrorReasons(input); len(reasons) > 0 {
		return newDigitalEmployeeOperationalState(input, DigitalEmployeeOperationalStatusError, reasons)
	}

	if input.HasWorkingRun {
		return newDigitalEmployeeOperationalState(input, DigitalEmployeeOperationalStatusWorking, nil)
	}

	if input.HasQueuedWork {
		return newDigitalEmployeeOperationalState(input, DigitalEmployeeOperationalStatusQueued, nil)
	}

	if input.ConfigurationMissing {
		return newDigitalEmployeeOperationalState(input, DigitalEmployeeOperationalStatusNeedsConfiguration, []DigitalEmployeeOperationalReason{
			{Code: "configuration_missing", Message: "缺少执行所需配置"},
		})
	}

	if input.RuntimeUnavailable {
		return newDigitalEmployeeOperationalState(input, DigitalEmployeeOperationalStatusUnavailable, []DigitalEmployeeOperationalReason{
			{Code: "runtime_offline", Message: "Runtime 当前不可用"},
		})
	}

	if !input.DispatchReady {
		return newDigitalEmployeeOperationalState(input, DigitalEmployeeOperationalStatusUnavailable, []DigitalEmployeeOperationalReason{
			{Code: "not_dispatchable", Message: "当前不可调度"},
		})
	}

	return newDigitalEmployeeOperationalState(input, DigitalEmployeeOperationalStatusIdle, nil)
}

func resolveDigitalEmployeeOperationalErrorReasons(input DigitalEmployeeOperationalInput) []DigitalEmployeeOperationalReason {
	reasons := make([]DigitalEmployeeOperationalReason, 0, 3)
	if input.RuntimeUnavailable && (input.HasActiveWork || input.HasWorkingRun || input.HasQueuedWork) {
		reasons = append(reasons, DigitalEmployeeOperationalReason{
			Code:    "runtime_offline_with_work",
			Message: "Runtime 离线，已有任务受影响",
		})
	}
	if input.HasProviderFailure {
		reasons = append(reasons, DigitalEmployeeOperationalReason{
			Code:    "provider_failure",
			Message: "Provider 执行失败或不可用",
		})
	}
	if input.HasTaskFailure {
		reasons = append(reasons, DigitalEmployeeOperationalReason{
			Code:    "task_failed",
			Message: "任务失败，等待恢复策略或后续处理",
		})
	}
	return reasons
}

func newDigitalEmployeeOperationalState(input DigitalEmployeeOperationalInput, status DigitalEmployeeOperationalStatus, reasons []DigitalEmployeeOperationalReason) DigitalEmployeeOperationalState {
	if reasons == nil {
		reasons = []DigitalEmployeeOperationalReason{}
	}
	return DigitalEmployeeOperationalState{
		Status:      status,
		Reasons:     reasons,
		CanDispatch: canDispatchDigitalEmployeeOperationalState(input, status),
	}
}

func canDispatchDigitalEmployeeOperationalState(input DigitalEmployeeOperationalInput, status DigitalEmployeeOperationalStatus) bool {
	if !input.DispatchReady {
		return false
	}
	switch status {
	case DigitalEmployeeOperationalStatusIdle, DigitalEmployeeOperationalStatusQueued, DigitalEmployeeOperationalStatusWorking:
		return true
	default:
		return false
	}
}
