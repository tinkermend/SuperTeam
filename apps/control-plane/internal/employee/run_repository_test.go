package employee

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDigitalEmployeeRunStatusTerminal(t *testing.T) {
	require.True(t, DigitalEmployeeRunStatusCompleted.IsTerminal())
	require.True(t, DigitalEmployeeRunStatusFailed.IsTerminal())
	require.True(t, DigitalEmployeeRunStatusCancelled.IsTerminal())
	require.True(t, DigitalEmployeeRunStatusTimedOut.IsTerminal())
	require.False(t, DigitalEmployeeRunStatusRunning.IsTerminal())
	require.False(t, DigitalEmployeeRunStatusCancelling.IsTerminal())
}

func TestRuntimeWritebackEventRedactsSensitivePayload(t *testing.T) {
	event := RuntimeCommandEventWriteback{
		EventType:      "text_delta",
		SequenceNumber: 1,
		Payload: map[string]any{
			"text":          "ok",
			"authorization": "Bearer secret",
			"nested": map[string]any{
				"token": "secret",
			},
			"events": []any{
				map[string]any{"token": "array item stays intact"},
			},
		},
	}

	redacted := redactRuntimeEventPayload(event.Payload)

	require.Equal(t, "[redacted]", redacted["authorization"])
	require.Equal(t, "[redacted]", redacted["nested"].(map[string]any)["token"])
	require.Equal(t, event.Payload["events"], redacted["events"])
}
