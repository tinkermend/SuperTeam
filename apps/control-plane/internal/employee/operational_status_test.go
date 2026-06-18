package employee

import "testing"

func TestResolveDigitalEmployeeOperationalStatePriority(t *testing.T) {
	tests := []struct {
		name  string
		input DigitalEmployeeOperationalInput
		want  DigitalEmployeeOperationalStatus
	}{
		{
			name: "employee scoped human decision wins over provider error",
			input: DigitalEmployeeOperationalInput{
				HasEmployeeScopedHumanBlocker: true,
				HasProviderFailure:            true,
				DispatchReady:                 true,
			},
			want: DigitalEmployeeOperationalStatusWaitingHuman,
		},
		{
			name: "provider failure wins over queued work",
			input: DigitalEmployeeOperationalInput{
				HasProviderFailure: true,
				HasQueuedWork:      true,
				DispatchReady:      true,
			},
			want: DigitalEmployeeOperationalStatusError,
		},
		{
			name: "runtime offline with active work is error",
			input: DigitalEmployeeOperationalInput{
				RuntimeUnavailable: true,
				HasActiveWork:      true,
				DispatchReady:      true,
			},
			want: DigitalEmployeeOperationalStatusError,
		},
		{
			name: "runtime offline without active work is unavailable",
			input: DigitalEmployeeOperationalInput{
				RuntimeUnavailable: true,
				DispatchReady:      false,
			},
			want: DigitalEmployeeOperationalStatusUnavailable,
		},
		{
			name: "running or cancelling run is working",
			input: DigitalEmployeeOperationalInput{
				HasWorkingRun: true,
				DispatchReady: true,
			},
			want: DigitalEmployeeOperationalStatusWorking,
		},
		{
			name: "assigned queued work is queued",
			input: DigitalEmployeeOperationalInput{
				HasQueuedWork: true,
				DispatchReady: true,
			},
			want: DigitalEmployeeOperationalStatusQueued,
		},
		{
			name: "missing provider binding is needs configuration",
			input: DigitalEmployeeOperationalInput{
				ConfigurationMissing: true,
				DispatchReady:        false,
			},
			want: DigitalEmployeeOperationalStatusNeedsConfiguration,
		},
		{
			name: "missing configuration wins over offline runtime",
			input: DigitalEmployeeOperationalInput{
				ConfigurationMissing: true,
				RuntimeUnavailable:   true,
				DispatchReady:        false,
			},
			want: DigitalEmployeeOperationalStatusNeedsConfiguration,
		},
		{
			name: "dispatchable employee without active facts is idle",
			input: DigitalEmployeeOperationalInput{
				DispatchReady: true,
			},
			want: DigitalEmployeeOperationalStatusIdle,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveDigitalEmployeeOperationalState(tt.input)
			if got.Status != tt.want {
				t.Fatalf("status = %q, want %q; state = %+v", got.Status, tt.want, got)
			}
		})
	}
}

func TestResolveDigitalEmployeeOperationalStateDoesNotProjectProjectAcceptance(t *testing.T) {
	got := ResolveDigitalEmployeeOperationalState(DigitalEmployeeOperationalInput{
		HasProjectAcceptanceBlocker: true,
		DispatchReady:               true,
	})

	if got.Status != DigitalEmployeeOperationalStatusIdle {
		t.Fatalf("status = %q, want idle; state = %+v", got.Status, got)
	}
}
