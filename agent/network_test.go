//go:build testing

package agent

import (
	"testing"
	"time"

	"github.com/henrygd/beszel/agent/deltatracker"
	"github.com/henrygd/beszel/internal/entities/system"
	psutilNet "github.com/shirou/gopsutil/v4/net"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsValidNic(t *testing.T) {
	tests := []struct {
		name          string
		nicName       string
		config        *NicConfig
		expectedValid bool
	}{
		{
			name:    "Whitelist - NIC in list",
			nicName: "eth0",
			config: &NicConfig{
				nics:        map[string]struct{}{"eth0": {}},
				isBlacklist: false,
			},
			expectedValid: true,
		},
		{
			name:    "Whitelist - NIC not in list",
			nicName: "wlan0",
			config: &NicConfig{
				nics:        map[string]struct{}{"eth0": {}},
				isBlacklist: false,
			},
			expectedValid: false,
		},
		{
			name:    "Blacklist - NIC in list",
			nicName: "eth0",
			config: &NicConfig{
				nics:        map[string]struct{}{"eth0": {}},
				isBlacklist: true,
			},
			expectedValid: false,
		},
		{
			name:    "Blacklist - NIC not in list",
			nicName: "wlan0",
			config: &NicConfig{
				nics:        map[string]struct{}{"eth0": {}},
				isBlacklist: true,
			},
			expectedValid: true,
		},
		{
			name:    "Whitelist with wildcard - matching pattern",
			nicName: "eth1",
			config: &NicConfig{
				nics:         map[string]struct{}{"eth*": {}},
				isBlacklist:  false,
				hasWildcards: true,
			},
			expectedValid: true,
		},
		{
			name:    "Whitelist with wildcard - non-matching pattern",
			nicName: "wlan0",
			config: &NicConfig{
				nics:         map[string]struct{}{"eth*": {}},
				isBlacklist:  false,
				hasWildcards: true,
			},
			expectedValid: false,
		},
		{
			name:    "Blacklist with wildcard - matching pattern",
			nicName: "eth1",
			config: &NicConfig{
				nics:         map[string]struct{}{"eth*": {}},
				isBlacklist:  true,
				hasWildcards: true,
			},
			expectedValid: false,
		},
		{
			name:    "Blacklist with wildcard - non-matching pattern",
			nicName: "wlan0",
			config: &NicConfig{
				nics:         map[string]struct{}{"eth*": {}},
				isBlacklist:  true,
				hasWildcards: true,
			},
			expectedValid: true,
		},
		{
			name:    "Empty whitelist config - no NICs allowed",
			nicName: "eth0",
			config: &NicConfig{
				nics:        map[string]struct{}{},
				isBlacklist: false,
			},
			expectedValid: false,
		},
		{
			name:    "Empty blacklist config - all NICs allowed",
			nicName: "eth0",
			config: &NicConfig{
				nics:        map[string]struct{}{},
				isBlacklist: true,
			},
			expectedValid: true,
		},
		{
			name:    "Multiple patterns - exact match",
			nicName: "eth0",
			config: &NicConfig{
				nics:        map[string]struct{}{"eth0": {}, "wlan*": {}},
				isBlacklist: false,
			},
			expectedValid: true,
		},
		{
			name:    "Multiple patterns - wildcard match",
			nicName: "wlan1",
			config: &NicConfig{
				nics:         map[string]struct{}{"eth0": {}, "wlan*": {}},
				isBlacklist:  false,
				hasWildcards: true,
			},
			expectedValid: true,
		},
		{
			name:    "Multiple patterns - no match",
			nicName: "bond0",
			config: &NicConfig{
				nics:         map[string]struct{}{"eth0": {}, "wlan*": {}},
				isBlacklist:  false,
				hasWildcards: true,
			},
			expectedValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidNic(tt.nicName, tt.config)
			assert.Equal(t, tt.expectedValid, result)
		})
	}
}

func TestNewNicConfig(t *testing.T) {
	tests := []struct {
		name        string
		nicsEnvVal  string
		expectedCfg *NicConfig
	}{
		{
			name:       "Empty string",
			nicsEnvVal: "",
			expectedCfg: &NicConfig{
				nics:         map[string]struct{}{},
				isBlacklist:  false,
				hasWildcards: false,
			},
		},
		{
			name:       "Single NIC whitelist",
			nicsEnvVal: "eth0",
			expectedCfg: &NicConfig{
				nics:         map[string]struct{}{"eth0": {}},
				isBlacklist:  false,
				hasWildcards: false,
			},
		},
		{
			name:       "Multiple NICs whitelist",
			nicsEnvVal: "eth0,wlan0",
			expectedCfg: &NicConfig{
				nics:         map[string]struct{}{"eth0": {}, "wlan0": {}},
				isBlacklist:  false,
				hasWildcards: false,
			},
		},
		{
			name:       "Blacklist mode",
			nicsEnvVal: "-eth0,wlan0",
			expectedCfg: &NicConfig{
				nics:         map[string]struct{}{"eth0": {}, "wlan0": {}},
				isBlacklist:  true,
				hasWildcards: false,
			},
		},
		{
			name:       "With wildcards",
			nicsEnvVal: "eth*,wlan0",
			expectedCfg: &NicConfig{
				nics:         map[string]struct{}{"eth*": {}, "wlan0": {}},
				isBlacklist:  false,
				hasWildcards: true,
			},
		},
		{
			name:       "Blacklist with wildcards",
			nicsEnvVal: "-eth*,wlan0",
			expectedCfg: &NicConfig{
				nics:         map[string]struct{}{"eth*": {}, "wlan0": {}},
				isBlacklist:  true,
				hasWildcards: true,
			},
		},
		{
			name:       "With whitespace",
			nicsEnvVal: "eth0, wlan0 , eth1",
			expectedCfg: &NicConfig{
				nics:         map[string]struct{}{"eth0": {}, "wlan0": {}, "eth1": {}},
				isBlacklist:  false,
				hasWildcards: false,
			},
		},
		{
			name:       "Only wildcards",
			nicsEnvVal: "eth*,wlan*",
			expectedCfg: &NicConfig{
				nics:         map[string]struct{}{"eth*": {}, "wlan*": {}},
				isBlacklist:  false,
				hasWildcards: true,
			},
		},
		{
			name:       "Leading dash only",
			nicsEnvVal: "-",
			expectedCfg: &NicConfig{
				nics:         map[string]struct{}{},
				isBlacklist:  true,
				hasWildcards: false,
			},
		},
		{
			name:       "Mixed exact and wildcard",
			nicsEnvVal: "eth0,br-*",
			expectedCfg: &NicConfig{
				nics:         map[string]struct{}{"eth0": {}, "br-*": {}},
				isBlacklist:  false,
				hasWildcards: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := newNicConfig(tt.nicsEnvVal)
			require.NotNil(t, cfg)
			assert.Equal(t, tt.expectedCfg.isBlacklist, cfg.isBlacklist)
			assert.Equal(t, tt.expectedCfg.hasWildcards, cfg.hasWildcards)
			assert.Equal(t, tt.expectedCfg.nics, cfg.nics)
		})
	}
}
func TestSkipNetworkInterface(t *testing.T) {
	tests := []struct {
		name       string
		nic        psutilNet.IOCountersStat
		nicCfg     *NicConfig
		expectSkip bool
	}{
		{"loopback lo", psutilNet.IOCountersStat{Name: "lo", BytesSent: 100, BytesRecv: 100}, nil, true},
		{"loopback lo0", psutilNet.IOCountersStat{Name: "lo0", BytesSent: 100, BytesRecv: 100}, nil, true},
		{"docker prefix", psutilNet.IOCountersStat{Name: "docker0", BytesSent: 100, BytesRecv: 100}, nil, true},
		{"br- prefix", psutilNet.IOCountersStat{Name: "br-lan", BytesSent: 100, BytesRecv: 100}, nil, true},
		{"veth prefix", psutilNet.IOCountersStat{Name: "veth0abc", BytesSent: 100, BytesRecv: 100}, nil, true},
		{"bond prefix", psutilNet.IOCountersStat{Name: "bond0", BytesSent: 100, BytesRecv: 100}, nil, true},
		{"cali prefix", psutilNet.IOCountersStat{Name: "cali1234", BytesSent: 100, BytesRecv: 100}, nil, true},
		{"zero BytesRecv", psutilNet.IOCountersStat{Name: "eth0", BytesSent: 100, BytesRecv: 0}, nil, false},
		{"zero BytesSent", psutilNet.IOCountersStat{Name: "eth0", BytesSent: 0, BytesRecv: 100}, nil, false},
		{"both zero", psutilNet.IOCountersStat{Name: "eth0", BytesSent: 0, BytesRecv: 0}, nil, true},
		{"normal eth0", psutilNet.IOCountersStat{Name: "eth0", BytesSent: 100, BytesRecv: 200}, nil, false},
		{"normal wlan0", psutilNet.IOCountersStat{Name: "wlan0", BytesSent: 1, BytesRecv: 1}, nil, false},
		{"whitelist overrides skip (docker)", psutilNet.IOCountersStat{Name: "docker0", BytesSent: 100, BytesRecv: 100}, newNicConfig("docker0"), false},
		{"whitelist overrides skip (lo)", psutilNet.IOCountersStat{Name: "lo", BytesSent: 100, BytesRecv: 100}, newNicConfig("lo"), false},
		{"whitelist exclusion", psutilNet.IOCountersStat{Name: "eth1", BytesSent: 100, BytesRecv: 100}, newNicConfig("eth0"), true},
		{"blacklist skip lo", psutilNet.IOCountersStat{Name: "lo", BytesSent: 100, BytesRecv: 100}, newNicConfig("-eth0"), true},
		{"blacklist explicit eth0", psutilNet.IOCountersStat{Name: "eth0", BytesSent: 100, BytesRecv: 100}, newNicConfig("-eth0"), true},
		{"blacklist allow eth1", psutilNet.IOCountersStat{Name: "eth1", BytesSent: 100, BytesRecv: 100}, newNicConfig("-eth0"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expectSkip, skipNetworkInterface(tt.nic, tt.nicCfg))
		})
	}
}

func TestReconcileNetworkInterfaces(t *testing.T) {
	a := &Agent{netInterfaces: map[string]struct{}{"eth0": {}}}

	a.reconcileNetworkInterfaces([]psutilNet.IOCountersStat{
		{Name: "eth1", BytesSent: 100},
		{Name: "docker0", BytesSent: 100, BytesRecv: 100},
	})

	assert.Equal(t, map[string]struct{}{"eth1": {}}, a.netInterfaces)
}

func TestReconcileNetworkInterfacesHonorsExplicitNics(t *testing.T) {
	a := &Agent{
		netInterfaces: map[string]struct{}{},
		nicConfig:     newNicConfig("docker0,eth0"),
	}

	a.reconcileNetworkInterfaces([]psutilNet.IOCountersStat{
		{Name: "docker0"},
		{Name: "eth0"},
		{Name: "eth1", BytesSent: 100, BytesRecv: 100},
	})

	assert.Equal(t, map[string]struct{}{"docker0": {}, "eth0": {}}, a.netInterfaces)
}

func TestEnsureNetworkInterfacesMap(t *testing.T) {
	var a Agent
	var stats system.Stats

	// Initially nil
	assert.Nil(t, stats.NetworkInterfaces)
	// Ensure map is created
	a.ensureNetworkInterfacesMap(&stats)
	assert.NotNil(t, stats.NetworkInterfaces)
	// Idempotent
	a.ensureNetworkInterfacesMap(&stats)
	assert.NotNil(t, stats.NetworkInterfaces)
}

func TestLoadAndTickNetBaseline(t *testing.T) {
	a := &Agent{netIoStats: make(map[uint16]system.NetIoStats)}

	// First call initializes time and returns 0 elapsed
	ni, elapsed := a.loadAndTickNetBaseline(100)
	assert.Equal(t, uint64(0), elapsed)
	assert.False(t, ni.Time.IsZero())

	// Store back what loadAndTick returns to mimic updateNetworkStats behavior
	a.netIoStats[100] = ni

	time.Sleep(2 * time.Millisecond)

	// Next call should produce >= 0 elapsed and update time
	ni2, elapsed2 := a.loadAndTickNetBaseline(100)
	assert.True(t, elapsed2 > 0)
	assert.False(t, ni2.Time.IsZero())
}

func TestSumAndTrackPerNicDeltas(t *testing.T) {
	a := &Agent{
		netInterfaces:             map[string]struct{}{"eth0": {}, "wlan0": {}},
		netInterfaceDeltaTrackers: make(map[uint16]*deltatracker.DeltaTracker[string, uint64]),
	}

	// Two samples for same cache interval to verify delta behavior
	cache := uint16(42)
	net1 := []psutilNet.IOCountersStat{{Name: "eth0", BytesSent: 1000, BytesRecv: 2000}}
	stats1 := &system.Stats{}
	a.ensureNetworkInterfacesMap(stats1)
	tx1, rx1 := a.sumAndTrackPerNicDeltas(cache, 0, net1, stats1)
	assert.Zero(t, tx1)
	assert.Zero(t, rx1)

	// Second cycle with elapsed, larger counters -> deltas computed inside
	net2 := []psutilNet.IOCountersStat{{Name: "eth0", BytesSent: 4000, BytesRecv: 9000}}
	stats := &system.Stats{}
	a.ensureNetworkInterfacesMap(stats)
	tx2, rx2 := a.sumAndTrackPerNicDeltas(cache, 1000, net2, stats)
	assert.Equal(t, uint64(3000), tx2)
	assert.Equal(t, uint64(7000), rx2)
	// Up/Down deltas per second should be (4000-1000)/1s = 3000 and (9000-2000)/1s = 7000
	ni, ok := stats.NetworkInterfaces["eth0"]
	assert.True(t, ok)
	assert.Equal(t, uint64(3000), ni[0])
	assert.Equal(t, uint64(7000), ni[1])
	assert.Equal(t, uint64(4000), ni[2])
	assert.Equal(t, uint64(9000), ni[3])
}

func TestSumAndTrackPerNicDeltasHandlesCounterReset(t *testing.T) {
	a := &Agent{
		netInterfaces:             map[string]struct{}{"eth0": {}},
		netInterfaceDeltaTrackers: make(map[uint16]*deltatracker.DeltaTracker[string, uint64]),
	}

	cache := uint16(77)

	// First interval establishes baseline values
	initial := []psutilNet.IOCountersStat{{Name: "eth0", BytesSent: 4_000, BytesRecv: 6_000}}
	statsInitial := &system.Stats{}
	a.ensureNetworkInterfacesMap(statsInitial)
	_, _ = a.sumAndTrackPerNicDeltas(cache, 0, initial, statsInitial)

	// Second interval increments counters normally so previous snapshot gets populated
	increment := []psutilNet.IOCountersStat{{Name: "eth0", BytesSent: 9_000, BytesRecv: 11_000}}
	statsIncrement := &system.Stats{}
	a.ensureNetworkInterfacesMap(statsIncrement)
	_, _ = a.sumAndTrackPerNicDeltas(cache, 1_000, increment, statsIncrement)

	niIncrement, ok := statsIncrement.NetworkInterfaces["eth0"]
	require.True(t, ok)
	assert.Equal(t, uint64(5_000), niIncrement[0])
	assert.Equal(t, uint64(5_000), niIncrement[1])

	// Third interval simulates counter reset (values drop below previous totals)
	reset := []psutilNet.IOCountersStat{{Name: "eth0", BytesSent: 1_200, BytesRecv: 1_500}}
	statsReset := &system.Stats{}
	a.ensureNetworkInterfacesMap(statsReset)
	_, _ = a.sumAndTrackPerNicDeltas(cache, 1_000, reset, statsReset)

	niReset, ok := statsReset.NetworkInterfaces["eth0"]
	require.True(t, ok)
	assert.Equal(t, uint64(1_200), niReset[0], "upload delta should match new counter value after reset")
	assert.Equal(t, uint64(1_500), niReset[1], "download delta should match new counter value after reset")
	assert.Equal(t, uint64(1_200), niReset[2], "tuple index 2 should remain the raw sent counter")
	assert.Equal(t, uint64(1_500), niReset[3], "tuple index 3 should remain the raw received counter")
}

func TestAggregateBandwidthHandlesNicResetAndRemoval(t *testing.T) {
	a := &Agent{
		netInterfaces:             map[string]struct{}{"eth0": {}, "eth1": {}},
		netInterfaceDeltaTrackers: make(map[uint16]*deltatracker.DeltaTracker[string, uint64]),
	}
	cache := uint16(88)
	initial := []psutilNet.IOCountersStat{
		{Name: "eth0", BytesSent: 10_000, BytesRecv: 20_000},
		{Name: "eth1", BytesSent: 30_000, BytesRecv: 40_000},
	}
	a.sumAndTrackPerNicDeltas(cache, 0, initial, &system.Stats{NetworkInterfaces: make(map[string][4]uint64)})

	// eth0 resets while eth1 increments; aggregate rates must not underflow.
	reset := []psutilNet.IOCountersStat{
		{Name: "eth0", BytesSent: 500, BytesRecv: 700},
		{Name: "eth1", BytesSent: 31_000, BytesRecv: 42_000},
	}
	up, down := a.sumAndTrackPerNicDeltas(cache, 1_000, reset, &system.Stats{NetworkInterfaces: make(map[string][4]uint64)})
	assert.Equal(t, uint64(1_500), up)
	assert.Equal(t, uint64(2_700), down)

	// A disappeared NIC contributes nothing and does not corrupt the remaining NIC delta.
	a.netInterfaces = map[string]struct{}{"eth1": {}}
	remaining := []psutilNet.IOCountersStat{{Name: "eth1", BytesSent: 32_000, BytesRecv: 43_500}}
	up, down = a.sumAndTrackPerNicDeltas(cache, 1_000, remaining, &system.Stats{NetworkInterfaces: make(map[string][4]uint64)})
	assert.Equal(t, uint64(1_000), up)
	assert.Equal(t, uint64(1_500), down)
}

func TestAggregateBandwidthSumsBeforeRounding(t *testing.T) {
	a := &Agent{
		netInterfaces:             map[string]struct{}{"eth0": {}, "eth1": {}},
		netInterfaceDeltaTrackers: make(map[uint16]*deltatracker.DeltaTracker[string, uint64]),
	}
	cache := uint16(89)
	initial := []psutilNet.IOCountersStat{
		{Name: "eth0", BytesSent: 100, BytesRecv: 100},
		{Name: "eth1", BytesSent: 100, BytesRecv: 100},
	}
	a.sumAndTrackPerNicDeltas(cache, 0, initial, &system.Stats{NetworkInterfaces: make(map[string][4]uint64)})

	next := []psutilNet.IOCountersStat{
		{Name: "eth0", BytesSent: 101, BytesRecv: 101},
		{Name: "eth1", BytesSent: 101, BytesRecv: 101},
	}
	up, down := a.sumAndTrackPerNicDeltas(cache, 1_500, next, &system.Stats{NetworkInterfaces: make(map[string][4]uint64)})
	assert.Equal(t, uint64(1), up)
	assert.Equal(t, uint64(1), down)
}

func TestApplyNetworkTotals(t *testing.T) {
	tests := []struct {
		name                  string
		bytesSentPerSecond    uint64
		bytesRecvPerSecond    uint64
		expectReset           bool
		expectedBandwidthSent uint64
		expectedBandwidthRecv uint64
	}{
		{
			name:                  "Valid network stats - normal values",
			bytesSentPerSecond:    1000000, // 1 MB/s
			bytesRecvPerSecond:    2000000, // 2 MB/s
			expectReset:           false,
			expectedBandwidthSent: 1000000,
			expectedBandwidthRecv: 2000000,
		},
		{
			name:               "Invalid network stats - sent exceeds threshold",
			bytesSentPerSecond: 11000000000, // ~10.5 GB/s > 10 GB/s threshold
			bytesRecvPerSecond: 1000000,     // 1 MB/s
			expectReset:        true,
		},
		{
			name:               "Invalid network stats - recv exceeds threshold",
			bytesSentPerSecond: 1000000,     // 1 MB/s
			bytesRecvPerSecond: 11000000000, // ~10.5 GB/s > 10 GB/s threshold
			expectReset:        true,
		},
		{
			name:               "Invalid network stats - both exceed threshold",
			bytesSentPerSecond: 12000000000, // ~11.4 GB/s
			bytesRecvPerSecond: 13000000000, // ~12.4 GB/s
			expectReset:        true,
		},
		{
			name:                  "Zero values",
			bytesSentPerSecond:    0,
			bytesRecvPerSecond:    0,
			expectReset:           false,
			expectedBandwidthSent: 0,
			expectedBandwidthRecv: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup agent with initialized maps
			a := &Agent{
				netInterfaces:             make(map[string]struct{}),
				netIoStats:                make(map[uint16]system.NetIoStats),
				netInterfaceDeltaTrackers: make(map[uint16]*deltatracker.DeltaTracker[string, uint64]),
			}

			cacheTimeMs := uint16(100)
			netIO := []psutilNet.IOCountersStat{
				{Name: "eth0", BytesSent: 1000, BytesRecv: 2000},
			}
			systemStats := &system.Stats{}
			nis := system.NetIoStats{}

			a.applyNetworkTotals(
				cacheTimeMs,
				netIO,
				systemStats,
				nis,
				tt.bytesSentPerSecond,
				tt.bytesRecvPerSecond,
			)

			if tt.expectReset {
				// Should have reset network tracking state - maps cleared and stats zeroed
				assert.NotContains(t, a.netIoStats, cacheTimeMs, "cache entry should be cleared after reset")
				assert.NotContains(t, a.netInterfaceDeltaTrackers, cacheTimeMs, "tracker should be cleared on reset")
				assert.Zero(t, systemStats.Bandwidth[0])
				assert.Zero(t, systemStats.Bandwidth[1])
			} else {
				// Should have applied stats
				assert.Equal(t, tt.expectedBandwidthSent, systemStats.Bandwidth[0])
				assert.Equal(t, tt.expectedBandwidthRecv, systemStats.Bandwidth[1])

				assert.Contains(t, a.netIoStats, cacheTimeMs)
			}
		})
	}
}
