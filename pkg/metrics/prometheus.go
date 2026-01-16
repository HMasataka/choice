// Package metrics provides Prometheus metrics for the SFU server.
// Per tasks.md Phase 4.1: Implements observability metrics for rooms, connections,
// tracks, and media statistics.
package metrics

import (
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const (
	namespace = "sfu"
)

// Metrics holds all Prometheus metrics for the SFU server.
type Metrics struct {
	// Room metrics
	RoomsTotal       prometheus.Gauge
	ConnectionsTotal prometheus.Gauge

	// Per-room metrics
	ConnectionsPerRoom *prometheus.HistogramVec
	TrackCount         *prometheus.GaugeVec
	SubscriptionCount  *prometheus.GaugeVec

	// Traffic metrics
	BytesReceivedTotal prometheus.Counter
	BytesSentTotal     prometheus.Counter
	PacketsReceived    prometheus.Counter
	PacketsSent        prometheus.Counter
	PacketsLost        prometheus.Counter

	// Quality metrics
	RTTSeconds    *prometheus.HistogramVec
	JitterSeconds *prometheus.HistogramVec
	BitrateBps    *prometheus.GaugeVec
	SimulcastLayer *prometheus.GaugeVec

	// registerer is the prometheus registerer
	registerer prometheus.Registerer
}

var (
	instance *Metrics
	once     sync.Once
)

// New creates a new Metrics instance with default prometheus registerer.
func New() *Metrics {
	return NewWithRegisterer(prometheus.DefaultRegisterer)
}

// NewWithRegisterer creates a new Metrics instance with a custom registerer.
func NewWithRegisterer(reg prometheus.Registerer) *Metrics {
	factory := promauto.With(reg)

	return &Metrics{
		registerer: reg,

		// sfu_rooms_total (Gauge)
		// Per tasks.md: Total number of active rooms
		RoomsTotal: factory.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "rooms_total",
			Help:      "Total number of active rooms",
		}),

		// sfu_connections_total (Gauge)
		// Per tasks.md: Total number of active connections
		ConnectionsTotal: factory.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "connections_total",
			Help:      "Total number of active WebRTC connections",
		}),

		// sfu_connections_per_room (Histogram)
		// Per tasks.md: Distribution of connections per room
		ConnectionsPerRoom: factory.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "connections_per_room",
			Help:      "Distribution of connections per room",
			Buckets:   []float64{1, 5, 10, 20, 50, 100},
		}, []string{"room_id"}),

		// sfu_track_count (Gauge)
		// Per tasks.md: Number of tracks by room_id and kind
		TrackCount: factory.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "track_count",
			Help:      "Number of active tracks",
		}, []string{"room_id", "kind"}),

		// sfu_subscription_count (Gauge)
		// Per tasks.md: Number of subscriptions by room_id
		SubscriptionCount: factory.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "subscription_count",
			Help:      "Number of active subscriptions",
		}, []string{"room_id"}),

		// sfu_bytes_received_total (Counter)
		// Per tasks.md: Total bytes received
		BytesReceivedTotal: factory.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "bytes_received_total",
			Help:      "Total bytes received from publishers",
		}),

		// sfu_bytes_sent_total (Counter)
		// Per tasks.md: Total bytes sent
		BytesSentTotal: factory.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "bytes_sent_total",
			Help:      "Total bytes sent to subscribers",
		}),

		// sfu_packets_received_total (Counter)
		// Per tasks.md: Total packets received
		PacketsReceived: factory.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "packets_received_total",
			Help:      "Total RTP packets received from publishers",
		}),

		// sfu_packets_sent_total (Counter)
		// Per tasks.md: Total packets sent
		PacketsSent: factory.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "packets_sent_total",
			Help:      "Total RTP packets sent to subscribers",
		}),

		// sfu_packets_lost_total (Counter)
		// Per tasks.md: Total packets lost
		PacketsLost: factory.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "packets_lost_total",
			Help:      "Total RTP packets lost",
		}),

		// sfu_rtt_seconds (Histogram)
		// Per tasks.md: RTT distribution
		RTTSeconds: factory.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "rtt_seconds",
			Help:      "Round-trip time in seconds",
			Buckets:   []float64{0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0},
		}, []string{"room_id", "participant_id"}),

		// sfu_jitter_seconds (Histogram)
		// Per tasks.md: Jitter distribution
		JitterSeconds: factory.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "jitter_seconds",
			Help:      "Jitter in seconds",
			Buckets:   []float64{0.005, 0.01, 0.02, 0.05, 0.1},
		}, []string{"room_id", "participant_id"}),

		// sfu_bitrate_bps (Gauge)
		// Per tasks.md: Current bitrate
		BitrateBps: factory.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "bitrate_bps",
			Help:      "Current bitrate in bits per second",
		}, []string{"room_id", "participant_id", "direction"}),

		// sfu_simulcast_layer (Gauge)
		// Per tasks.md: Current simulcast layer (0=l, 1=m, 2=h)
		SimulcastLayer: factory.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "simulcast_layer",
			Help:      "Current simulcast layer (0=low, 1=medium, 2=high)",
		}, []string{"room_id", "participant_id", "track_id"}),
	}
}

// GetInstance returns the singleton Metrics instance.
func GetInstance() *Metrics {
	once.Do(func() {
		instance = New()
	})
	return instance
}

// ResetInstance resets the singleton instance (for testing).
func ResetInstance() {
	once = sync.Once{}
	instance = nil
}

// Room metrics operations

// RoomCreated increments the room count when a room is created.
func (m *Metrics) RoomCreated() {
	m.RoomsTotal.Inc()
}

// RoomClosed decrements the room count when a room is closed.
func (m *Metrics) RoomClosed() {
	m.RoomsTotal.Dec()
}

// Connection metrics operations

// ConnectionOpened increments the connection count.
func (m *Metrics) ConnectionOpened() {
	m.ConnectionsTotal.Inc()
}

// ConnectionClosed decrements the connection count.
func (m *Metrics) ConnectionClosed() {
	m.ConnectionsTotal.Dec()
}

// ObserveConnectionsPerRoom records the number of connections in a room.
func (m *Metrics) ObserveConnectionsPerRoom(roomID string, count float64) {
	m.ConnectionsPerRoom.WithLabelValues(roomID).Observe(count)
}

// Track metrics operations

// TrackPublished increments the track count.
func (m *Metrics) TrackPublished(roomID, kind string) {
	m.TrackCount.WithLabelValues(roomID, kind).Inc()
}

// TrackUnpublished decrements the track count.
func (m *Metrics) TrackUnpublished(roomID, kind string) {
	m.TrackCount.WithLabelValues(roomID, kind).Dec()
}

// Subscription metrics operations

// SubscriptionCreated increments the subscription count.
func (m *Metrics) SubscriptionCreated(roomID string) {
	m.SubscriptionCount.WithLabelValues(roomID).Inc()
}

// SubscriptionRemoved decrements the subscription count.
func (m *Metrics) SubscriptionRemoved(roomID string) {
	m.SubscriptionCount.WithLabelValues(roomID).Dec()
}

// Traffic metrics operations

// AddBytesReceived adds to the total bytes received.
func (m *Metrics) AddBytesReceived(bytes float64) {
	m.BytesReceivedTotal.Add(bytes)
}

// AddBytesSent adds to the total bytes sent.
func (m *Metrics) AddBytesSent(bytes float64) {
	m.BytesSentTotal.Add(bytes)
}

// AddPacketsReceived adds to the total packets received.
func (m *Metrics) AddPacketsReceived(packets float64) {
	m.PacketsReceived.Add(packets)
}

// AddPacketsSent adds to the total packets sent.
func (m *Metrics) AddPacketsSent(packets float64) {
	m.PacketsSent.Add(packets)
}

// AddPacketsLost adds to the total packets lost.
func (m *Metrics) AddPacketsLost(packets float64) {
	m.PacketsLost.Add(packets)
}

// Quality metrics operations

// ObserveRTT records an RTT observation.
func (m *Metrics) ObserveRTT(roomID, participantID string, rttSeconds float64) {
	m.RTTSeconds.WithLabelValues(roomID, participantID).Observe(rttSeconds)
}

// ObserveJitter records a jitter observation.
func (m *Metrics) ObserveJitter(roomID, participantID string, jitterSeconds float64) {
	m.JitterSeconds.WithLabelValues(roomID, participantID).Observe(jitterSeconds)
}

// SetBitrate sets the current bitrate for a participant.
func (m *Metrics) SetBitrate(roomID, participantID, direction string, bitrateBps float64) {
	m.BitrateBps.WithLabelValues(roomID, participantID, direction).Set(bitrateBps)
}

// SetSimulcastLayer sets the current simulcast layer for a track.
func (m *Metrics) SetSimulcastLayer(roomID, participantID, trackID string, layer int) {
	m.SimulcastLayer.WithLabelValues(roomID, participantID, trackID).Set(float64(layer))
}

// Cleanup removes metrics for a participant when they leave.
func (m *Metrics) CleanupParticipant(roomID, participantID string) {
	m.RTTSeconds.DeleteLabelValues(roomID, participantID)
	m.JitterSeconds.DeleteLabelValues(roomID, participantID)
	m.BitrateBps.DeleteLabelValues(roomID, participantID, "inbound")
	m.BitrateBps.DeleteLabelValues(roomID, participantID, "outbound")
}

// CleanupTrack removes metrics for a track.
func (m *Metrics) CleanupTrack(roomID, participantID, trackID string) {
	m.SimulcastLayer.DeleteLabelValues(roomID, participantID, trackID)
}

// CleanupRoom removes all metrics for a room.
func (m *Metrics) CleanupRoom(roomID string) {
	m.TrackCount.DeleteLabelValues(roomID, "video")
	m.TrackCount.DeleteLabelValues(roomID, "audio")
	m.SubscriptionCount.DeleteLabelValues(roomID)
}
