package metrics

import (
	"testing"

	io_prometheus_client "github.com/prometheus/client_model/go"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	// Use a custom registry to avoid conflicts with other tests
	reg := prometheus.NewRegistry()
	m := NewWithRegisterer(reg)

	assert.NotNil(t, m)
	assert.NotNil(t, m.RoomsTotal)
	assert.NotNil(t, m.ConnectionsTotal)
	assert.NotNil(t, m.ConnectionsPerRoom)
	assert.NotNil(t, m.TrackCount)
	assert.NotNil(t, m.SubscriptionCount)
	assert.NotNil(t, m.BytesReceivedTotal)
	assert.NotNil(t, m.BytesSentTotal)
	assert.NotNil(t, m.PacketsReceived)
	assert.NotNil(t, m.PacketsSent)
	assert.NotNil(t, m.PacketsLost)
	assert.NotNil(t, m.RTTSeconds)
	assert.NotNil(t, m.JitterSeconds)
	assert.NotNil(t, m.BitrateBps)
	assert.NotNil(t, m.SimulcastLayer)
}

func TestGetInstance(t *testing.T) {
	// Reset the singleton first
	ResetInstance()

	m1 := GetInstance()
	m2 := GetInstance()

	assert.Same(t, m1, m2)

	// Clean up
	ResetInstance()
}

func TestRoomMetrics(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewWithRegisterer(reg)

	// Initially zero
	assertGaugeValue(t, m.RoomsTotal, 0)

	// Create rooms
	m.RoomCreated()
	assertGaugeValue(t, m.RoomsTotal, 1)

	m.RoomCreated()
	assertGaugeValue(t, m.RoomsTotal, 2)

	// Close rooms
	m.RoomClosed()
	assertGaugeValue(t, m.RoomsTotal, 1)

	m.RoomClosed()
	assertGaugeValue(t, m.RoomsTotal, 0)
}

func TestConnectionMetrics(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewWithRegisterer(reg)

	// Initially zero
	assertGaugeValue(t, m.ConnectionsTotal, 0)

	// Open connections
	m.ConnectionOpened()
	assertGaugeValue(t, m.ConnectionsTotal, 1)

	m.ConnectionOpened()
	m.ConnectionOpened()
	assertGaugeValue(t, m.ConnectionsTotal, 3)

	// Close connections
	m.ConnectionClosed()
	assertGaugeValue(t, m.ConnectionsTotal, 2)
}

func TestConnectionsPerRoom(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewWithRegisterer(reg)

	// Observe connections for different rooms
	m.ObserveConnectionsPerRoom("room-1", 5)
	m.ObserveConnectionsPerRoom("room-1", 10)
	m.ObserveConnectionsPerRoom("room-2", 3)

	// Verify histogram was recorded (we can only check count)
	gathered, err := reg.Gather()
	require.NoError(t, err)

	for _, mf := range gathered {
		if *mf.Name == "sfu_connections_per_room" {
			assert.NotEmpty(t, mf.Metric)
			return
		}
	}
	t.Error("sfu_connections_per_room metric not found")
}

func TestTrackMetrics(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewWithRegisterer(reg)

	// Publish tracks
	m.TrackPublished("room-1", "video")
	m.TrackPublished("room-1", "audio")
	m.TrackPublished("room-1", "video")

	// Check values
	assertGaugeVecValue(t, m.TrackCount, prometheus.Labels{"room_id": "room-1", "kind": "video"}, 2)
	assertGaugeVecValue(t, m.TrackCount, prometheus.Labels{"room_id": "room-1", "kind": "audio"}, 1)

	// Unpublish tracks
	m.TrackUnpublished("room-1", "video")
	assertGaugeVecValue(t, m.TrackCount, prometheus.Labels{"room_id": "room-1", "kind": "video"}, 1)
}

func TestSubscriptionMetrics(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewWithRegisterer(reg)

	// Create subscriptions
	m.SubscriptionCreated("room-1")
	m.SubscriptionCreated("room-1")
	m.SubscriptionCreated("room-2")

	assertGaugeVecValue(t, m.SubscriptionCount, prometheus.Labels{"room_id": "room-1"}, 2)
	assertGaugeVecValue(t, m.SubscriptionCount, prometheus.Labels{"room_id": "room-2"}, 1)

	// Remove subscription
	m.SubscriptionRemoved("room-1")
	assertGaugeVecValue(t, m.SubscriptionCount, prometheus.Labels{"room_id": "room-1"}, 1)
}

func TestTrafficMetrics(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewWithRegisterer(reg)

	// Add bytes
	m.AddBytesReceived(1000)
	m.AddBytesReceived(500)
	assertCounterValue(t, m.BytesReceivedTotal, 1500)

	m.AddBytesSent(2000)
	assertCounterValue(t, m.BytesSentTotal, 2000)

	// Add packets
	m.AddPacketsReceived(100)
	assertCounterValue(t, m.PacketsReceived, 100)

	m.AddPacketsSent(80)
	assertCounterValue(t, m.PacketsSent, 80)

	m.AddPacketsLost(5)
	assertCounterValue(t, m.PacketsLost, 5)
}

func TestQualityMetrics(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewWithRegisterer(reg)

	// Observe RTT
	m.ObserveRTT("room-1", "participant-1", 0.05)
	m.ObserveRTT("room-1", "participant-1", 0.08)

	// Observe Jitter
	m.ObserveJitter("room-1", "participant-1", 0.01)

	// Set Bitrate
	m.SetBitrate("room-1", "participant-1", "inbound", 1500000)
	m.SetBitrate("room-1", "participant-1", "outbound", 500000)

	assertGaugeVecValue(t, m.BitrateBps, prometheus.Labels{
		"room_id":        "room-1",
		"participant_id": "participant-1",
		"direction":      "inbound",
	}, 1500000)

	assertGaugeVecValue(t, m.BitrateBps, prometheus.Labels{
		"room_id":        "room-1",
		"participant_id": "participant-1",
		"direction":      "outbound",
	}, 500000)
}

func TestSimulcastLayerMetrics(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewWithRegisterer(reg)

	// Set simulcast layer (0=low, 1=medium, 2=high)
	m.SetSimulcastLayer("room-1", "participant-1", "track-1", 2)

	assertGaugeVecValue(t, m.SimulcastLayer, prometheus.Labels{
		"room_id":        "room-1",
		"participant_id": "participant-1",
		"track_id":       "track-1",
	}, 2)

	// Change to lower layer
	m.SetSimulcastLayer("room-1", "participant-1", "track-1", 0)

	assertGaugeVecValue(t, m.SimulcastLayer, prometheus.Labels{
		"room_id":        "room-1",
		"participant_id": "participant-1",
		"track_id":       "track-1",
	}, 0)
}

func TestCleanupParticipant(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewWithRegisterer(reg)

	// Add some metrics
	m.ObserveRTT("room-1", "participant-1", 0.05)
	m.ObserveJitter("room-1", "participant-1", 0.01)
	m.SetBitrate("room-1", "participant-1", "inbound", 1500000)
	m.SetBitrate("room-1", "participant-1", "outbound", 500000)

	// Cleanup should not panic
	m.CleanupParticipant("room-1", "participant-1")
}

func TestCleanupTrack(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewWithRegisterer(reg)

	// Add track metric
	m.SetSimulcastLayer("room-1", "participant-1", "track-1", 2)

	// Cleanup should not panic
	m.CleanupTrack("room-1", "participant-1", "track-1")
}

func TestCleanupRoom(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewWithRegisterer(reg)

	// Add room metrics
	m.TrackPublished("room-1", "video")
	m.TrackPublished("room-1", "audio")
	m.SubscriptionCreated("room-1")

	// Cleanup should not panic
	m.CleanupRoom("room-1")
}

// Helper functions

func assertGaugeValue(t *testing.T, gauge prometheus.Gauge, expected float64) {
	t.Helper()
	metric := &io_prometheus_client.Metric{}
	err := gauge.Write(metric)
	require.NoError(t, err)
	assert.Equal(t, expected, metric.GetGauge().GetValue())
}

func assertGaugeVecValue(t *testing.T, vec *prometheus.GaugeVec, labels prometheus.Labels, expected float64) {
	t.Helper()
	gauge := vec.With(labels)
	metric := &io_prometheus_client.Metric{}
	err := gauge.Write(metric)
	require.NoError(t, err)
	assert.Equal(t, expected, metric.GetGauge().GetValue())
}

func assertCounterValue(t *testing.T, counter prometheus.Counter, expected float64) {
	t.Helper()
	metric := &io_prometheus_client.Metric{}
	err := counter.Write(metric)
	require.NoError(t, err)
	assert.Equal(t, expected, metric.GetCounter().GetValue())
}
