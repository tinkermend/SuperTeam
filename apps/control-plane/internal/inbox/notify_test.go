package inbox

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestChangeNotifierWakesOnlyTheAffectedTenant(t *testing.T) {
	notifier := NewChangeNotifier(nil)
	tenantA, tenantB := uuid.New(), uuid.New()

	chA, cancelA := notifier.Subscribe(tenantA)
	defer cancelA()
	chB, cancelB := notifier.Subscribe(tenantB)
	defer cancelB()

	notifier.publish(tenantA)

	require.Len(t, chA, 1, "the changed tenant's stream must be woken")
	require.Empty(t, chB, "another tenant's stream must not be woken")
}

func TestChangeNotifierWakesEveryStreamOfTheTenant(t *testing.T) {
	notifier := NewChangeNotifier(nil)
	tenantID := uuid.New()

	first, cancelFirst := notifier.Subscribe(tenantID)
	defer cancelFirst()
	second, cancelSecond := notifier.Subscribe(tenantID)
	defer cancelSecond()

	notifier.publish(tenantID)

	require.Len(t, first, 1)
	require.Len(t, second, 1, "every open tab of the tenant must be woken")
}

// Bursts must collapse rather than queue: a subscriber holding an undrained
// token already knows it is stale, and a blocking send would let one slow SSE
// writer stall the single listener for every other stream.
func TestChangeNotifierCollapsesBurstsAndNeverBlocks(t *testing.T) {
	notifier := NewChangeNotifier(nil)
	tenantID := uuid.New()

	ch, cancel := notifier.Subscribe(tenantID)
	defer cancel()

	for i := 0; i < 100; i++ {
		notifier.publish(tenantID)
	}

	require.Len(t, ch, 1, "a burst must collapse into a single wake-up")
}

func TestChangeNotifierStopsWakingAfterUnsubscribe(t *testing.T) {
	notifier := NewChangeNotifier(nil)
	tenantID := uuid.New()

	ch, cancel := notifier.Subscribe(tenantID)
	cancel()

	notifier.publish(tenantID)

	require.Empty(t, ch, "a cancelled subscription must not be woken")
}

// The handler must survive without a notifier (tests, degraded start-up); the
// fallback poll is then the only wake-up source.
func TestChangeNotifierSubscribeIsSafeOnNilNotifier(t *testing.T) {
	var notifier *ChangeNotifier

	ch, cancel := notifier.Subscribe(uuid.New())

	require.Nil(t, ch)
	require.NotPanics(t, func() { cancel() })
}
