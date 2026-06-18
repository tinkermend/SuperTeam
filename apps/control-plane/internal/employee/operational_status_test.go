package employee

import (
	"reflect"
	"testing"
)

func TestResolveDigitalEmployeeOperationalStatePriority(t *testing.T) {
	tests := []struct {
		name            string
		input           DigitalEmployeeOperationalInput
		wantStatus      DigitalEmployeeOperationalStatus
		wantCanDispatch bool
		wantReasons     []DigitalEmployeeOperationalReason
	}{
		{
			name: "employee scoped human decision wins over provider error",
			input: DigitalEmployeeOperationalInput{
				HasEmployeeScopedHumanBlocker: true,
				HasProviderFailure:            true,
				DispatchReady:                 true,
			},
			wantStatus:      DigitalEmployeeOperationalStatusWaitingHuman,
			wantCanDispatch: false,
			wantReasons: []DigitalEmployeeOperationalReason{
				{Code: "approval_blocked", Message: "等待人工确认后继续执行"},
			},
		},
		{
			name: "provider failure wins over queued work",
			input: DigitalEmployeeOperationalInput{
				HasProviderFailure: true,
				HasQueuedWork:      true,
				DispatchReady:      true,
			},
			wantStatus:      DigitalEmployeeOperationalStatusError,
			wantCanDispatch: false,
			wantReasons: []DigitalEmployeeOperationalReason{
				{Code: "provider_failure", Message: "Provider 执行失败或不可用"},
			},
		},
		{
			name: "runtime offline with active work is error",
			input: DigitalEmployeeOperationalInput{
				RuntimeUnavailable: true,
				HasActiveWork:      true,
				DispatchReady:      true,
			},
			wantStatus:      DigitalEmployeeOperationalStatusError,
			wantCanDispatch: false,
			wantReasons: []DigitalEmployeeOperationalReason{
				{Code: "runtime_offline_with_work", Message: "Runtime 离线，已有执行或排队任务受影响"},
			},
		},
		{
			name: "task failure is error",
			input: DigitalEmployeeOperationalInput{
				HasTaskFailure: true,
				DispatchReady:  true,
			},
			wantStatus:      DigitalEmployeeOperationalStatusError,
			wantCanDispatch: false,
			wantReasons: []DigitalEmployeeOperationalReason{
				{Code: "task_failed", Message: "任务失败，等待恢复策略或后续处理"},
			},
		},
		{
			name: "multiple error facts accumulate reasons in deterministic order",
			input: DigitalEmployeeOperationalInput{
				RuntimeUnavailable: true,
				HasActiveWork:      true,
				HasProviderFailure: true,
				HasTaskFailure:     true,
				DispatchReady:      true,
			},
			wantStatus:      DigitalEmployeeOperationalStatusError,
			wantCanDispatch: false,
			wantReasons: []DigitalEmployeeOperationalReason{
				{Code: "runtime_offline_with_work", Message: "Runtime 离线，已有执行或排队任务受影响"},
				{Code: "provider_failure", Message: "Provider 执行失败或不可用"},
				{Code: "task_failed", Message: "任务失败，等待恢复策略或后续处理"},
			},
		},
		{
			name: "runtime offline without active work is unavailable",
			input: DigitalEmployeeOperationalInput{
				RuntimeUnavailable: true,
				DispatchReady:      false,
			},
			wantStatus:      DigitalEmployeeOperationalStatusUnavailable,
			wantCanDispatch: false,
			wantReasons: []DigitalEmployeeOperationalReason{
				{Code: "runtime_offline", Message: "Runtime 当前不可用"},
			},
		},
		{
			name: "running or cancelling run is working",
			input: DigitalEmployeeOperationalInput{
				HasWorkingRun: true,
				DispatchReady: true,
			},
			wantStatus:      DigitalEmployeeOperationalStatusWorking,
			wantCanDispatch: true,
			wantReasons:     []DigitalEmployeeOperationalReason{},
		},
		{
			name: "assigned queued work is queued",
			input: DigitalEmployeeOperationalInput{
				HasQueuedWork: true,
				DispatchReady: true,
			},
			wantStatus:      DigitalEmployeeOperationalStatusQueued,
			wantCanDispatch: true,
			wantReasons:     []DigitalEmployeeOperationalReason{},
		},
		{
			name: "missing provider binding is needs configuration",
			input: DigitalEmployeeOperationalInput{
				ConfigurationMissing: true,
				DispatchReady:        false,
			},
			wantStatus:      DigitalEmployeeOperationalStatusNeedsConfiguration,
			wantCanDispatch: false,
			wantReasons: []DigitalEmployeeOperationalReason{
				{Code: "configuration_missing", Message: "缺少执行所需配置"},
			},
		},
		{
			name: "missing configuration wins over offline runtime",
			input: DigitalEmployeeOperationalInput{
				ConfigurationMissing: true,
				RuntimeUnavailable:   true,
				DispatchReady:        false,
			},
			wantStatus:      DigitalEmployeeOperationalStatusNeedsConfiguration,
			wantCanDispatch: false,
			wantReasons: []DigitalEmployeeOperationalReason{
				{Code: "configuration_missing", Message: "缺少执行所需配置"},
			},
		},
		{
			name: "dispatchable employee without active facts is idle",
			input: DigitalEmployeeOperationalInput{
				DispatchReady: true,
			},
			wantStatus:      DigitalEmployeeOperationalStatusIdle,
			wantCanDispatch: true,
			wantReasons:     []DigitalEmployeeOperationalReason{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveDigitalEmployeeOperationalState(tt.input)
			assertDigitalEmployeeOperationalState(t, got, tt.wantStatus, tt.wantCanDispatch, tt.wantReasons)
		})
	}
}

func TestResolveDigitalEmployeeOperationalStateDoesNotProjectProjectAcceptance(t *testing.T) {
	got := ResolveDigitalEmployeeOperationalState(DigitalEmployeeOperationalInput{
		HasProjectAcceptanceBlocker: true,
		DispatchReady:               true,
	})

	assertDigitalEmployeeOperationalState(t, got, DigitalEmployeeOperationalStatusIdle, true, []DigitalEmployeeOperationalReason{})
}

func assertDigitalEmployeeOperationalState(t *testing.T, got DigitalEmployeeOperationalState, wantStatus DigitalEmployeeOperationalStatus, wantCanDispatch bool, wantReasons []DigitalEmployeeOperationalReason) {
	t.Helper()

	if got.Status != wantStatus {
		t.Fatalf("status = %q, want %q; state = %+v", got.Status, wantStatus, got)
	}
	if got.CanDispatch != wantCanDispatch {
		t.Fatalf("can dispatch = %t, want %t; state = %+v", got.CanDispatch, wantCanDispatch, got)
	}
	if got.Reasons == nil {
		t.Fatalf("reasons is nil, want non-nil %v; state = %+v", wantReasons, got)
	}
	if !reflect.DeepEqual(got.Reasons, wantReasons) {
		t.Fatalf("reasons = %+v, want %+v; state = %+v", got.Reasons, wantReasons, got)
	}
}
