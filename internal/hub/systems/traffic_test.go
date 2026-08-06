//go:build testing

package systems

import (
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTrafficCycleShanghaiClamp(t *testing.T) {
	tests := []struct {
		name      string
		at        string
		day       int
		wantStart string
		wantEnd   string
	}{
		{"before billing day", "2026-03-15T12:00:00+08:00", 20, "2026-02-20T00:00:00+08:00", "2026-03-20T00:00:00+08:00"},
		{"after billing day", "2026-03-25T12:00:00+08:00", 20, "2026-03-20T00:00:00+08:00", "2026-04-20T00:00:00+08:00"},
		{"clamps short months", "2026-02-28T12:00:00+08:00", 31, "2026-02-28T00:00:00+08:00", "2026-03-31T00:00:00+08:00"},
		{"previous clamped month", "2026-03-01T12:00:00+08:00", 31, "2026-02-28T00:00:00+08:00", "2026-03-31T00:00:00+08:00"},
		{"next clamped month", "2026-01-31T12:00:00+08:00", 31, "2026-01-31T00:00:00+08:00", "2026-02-28T00:00:00+08:00"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			at, err := time.Parse(time.RFC3339, test.at)
			require.NoError(t, err)
			start, end := trafficCycle(at, test.day)
			assert.Equal(t, test.wantStart, start.Format(time.RFC3339))
			assert.Equal(t, test.wantEnd, end.Format(time.RFC3339))
		})
	}
}

func TestTrafficDeltaModesAndCounterChanges(t *testing.T) {
	previous := map[string][2]string{
		"eth0": {"18446744073709551600", "100"},
		"gone": {"50", "50"},
	}
	current := map[string][2]string{
		"eth0": {"18446744073709551610", "20"},
		"new":  {"999", "999"},
	}

	sent, recv, resets, interfaceChanges := trafficDelta(previous, current)
	assert.Equal(t, "10", sent.String(), "preserves uint64-range precision")
	assert.Equal(t, "20", recv.String(), "counts bytes since a counter reset")
	assert.Equal(t, 1, resets)
	assert.Equal(t, 2, interfaceChanges)

	sent, recv, resets, interfaceChanges = trafficDelta(
		map[string][2]string{"eth0": {"100", "200"}},
		map[string][2]string{"eth0": {"130", "280"}},
	)
	assert.Equal(t, "30", sent.String())
	assert.Equal(t, "80", recv.String())
	assert.Zero(t, resets)
	assert.Zero(t, interfaceChanges)
}

func TestSplitTrafficDeltaConservesIntegers(t *testing.T) {
	from := time.Date(2026, 8, 5, 23, 59, 0, 0, shanghaiLocation)
	boundary := time.Date(2026, 8, 6, 0, 0, 0, 0, shanghaiLocation)
	to := time.Date(2026, 8, 6, 0, 2, 0, 0, shanghaiLocation)
	total := big.NewInt(10)
	current := splitTrafficDelta(total, from, to, boundary)
	previous := new(big.Int).Sub(total, current)

	assert.Equal(t, "7", current.String())
	assert.Equal(t, "3", previous.String())
	assert.Equal(t, total, new(big.Int).Add(previous, current))
}
