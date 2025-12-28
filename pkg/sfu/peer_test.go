package sfu

import (
	"context"
	"testing"

	"github.com/HMasataka/choice/pkg/relay"
	"github.com/pion/webrtc/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChannelAPIMessage(t *testing.T) {
	t.Run("構造体の初期化", func(t *testing.T) {
		msg := ChannelAPIMessage{
			Method: "audioLevels",
			Params: map[string]interface{}{"level": 50},
		}

		assert.Equal(t, "audioLevels", msg.Method)
		assert.NotNil(t, msg.Params)
	})

	t.Run("Paramsなし", func(t *testing.T) {
		msg := ChannelAPIMessage{
			Method: "ping",
		}

		assert.Equal(t, "ping", msg.Method)
		assert.Nil(t, msg.Params)
	})
}

func TestConnectionType(t *testing.T) {
	t.Run("定数の値", func(t *testing.T) {
		assert.Equal(t, ConnectionType("publisher"), ConnectionTypePublisher)
		assert.Equal(t, ConnectionType("subscriber"), ConnectionTypeSubscriber)
	})

	t.Run("文字列比較", func(t *testing.T) {
		ct := ConnectionTypePublisher
		assert.Equal(t, "publisher", string(ct))

		ct = ConnectionTypeSubscriber
		assert.Equal(t, "subscriber", string(ct))
	})
}

func TestPeerErrors(t *testing.T) {
	t.Run("ErrTransportExists", func(t *testing.T) {
		assert.NotNil(t, ErrTransportExists)
		assert.Equal(t, "rtc transport already exists for this connection", ErrTransportExists.Error())
	})

	t.Run("ErrNoTransportEstablished", func(t *testing.T) {
		assert.NotNil(t, ErrNoTransportEstablished)
		assert.Equal(t, "no rtc transport exists for this Peer", ErrNoTransportEstablished.Error())
	})

	t.Run("ErrOfferIgnored", func(t *testing.T) {
		assert.NotNil(t, ErrOfferIgnored)
		assert.Equal(t, "offered ignored", ErrOfferIgnored.Error())
	})
}

func TestJoinConfig(t *testing.T) {
	t.Run("デフォルト値", func(t *testing.T) {
		cfg := JoinConfig{}

		assert.False(t, cfg.NoPublish)
		assert.False(t, cfg.NoSubscribe)
		assert.False(t, cfg.AutoSubscribe)
	})

	t.Run("値を設定", func(t *testing.T) {
		cfg := JoinConfig{
			NoPublish:     true,
			NoSubscribe:   true,
			AutoSubscribe: true,
		}

		assert.True(t, cfg.NoPublish)
		assert.True(t, cfg.NoSubscribe)
		assert.True(t, cfg.AutoSubscribe)
	})

	t.Run("Publish専用設定", func(t *testing.T) {
		cfg := JoinConfig{
			NoPublish:   false,
			NoSubscribe: true,
		}

		assert.False(t, cfg.NoPublish)
		assert.True(t, cfg.NoSubscribe)
	})

	t.Run("Subscribe専用設定", func(t *testing.T) {
		cfg := JoinConfig{
			NoPublish:     true,
			NoSubscribe:   false,
			AutoSubscribe: true,
		}

		assert.True(t, cfg.NoPublish)
		assert.False(t, cfg.NoSubscribe)
		assert.True(t, cfg.AutoSubscribe)
	})
}

func TestNewPeer(t *testing.T) {
	t.Run("正常に初期化される", func(t *testing.T) {
		provider := &mockSessionProvider{}

		peer := NewPeer(provider)

		require.NotNil(t, peer)
	})

	t.Run("peerLocal型を返す", func(t *testing.T) {
		provider := &mockSessionProvider{}

		peer := NewPeer(provider)

		_, ok := peer.(*peerLocal)
		assert.True(t, ok)
	})
}

func TestPeerLocal_UserID(t *testing.T) {
	provider := &mockSessionProvider{}
	peer := NewPeer(provider).(*peerLocal)

	t.Run("初期値は空", func(t *testing.T) {
		assert.Equal(t, "", peer.UserID())
	})

	t.Run("設定後の値", func(t *testing.T) {
		peer.userID = "test-user-123"
		assert.Equal(t, "test-user-123", peer.UserID())
	})
}

func TestPeerLocal_Publisher(t *testing.T) {
	provider := &mockSessionProvider{}
	peer := NewPeer(provider).(*peerLocal)

	t.Run("初期値はnil", func(t *testing.T) {
		assert.Nil(t, peer.Publisher())
	})
}

func TestPeerLocal_Subscriber(t *testing.T) {
	provider := &mockSessionProvider{}
	peer := NewPeer(provider).(*peerLocal)

	t.Run("初期値はnil", func(t *testing.T) {
		assert.Nil(t, peer.Subscriber())
	})
}

func TestPeerLocal_SetOnOffer(t *testing.T) {
	provider := &mockSessionProvider{}
	peer := NewPeer(provider).(*peerLocal)

	t.Run("コールバックを設定", func(t *testing.T) {
		called := false
		peer.SetOnOffer(func(sdp *webrtc.SessionDescription) {
			called = true
		})

		require.NotNil(t, peer.OnOffer)

		// コールバックを呼び出し
		peer.OnOffer(nil)
		assert.True(t, called)
	})

	t.Run("nilで設定", func(t *testing.T) {
		peer.SetOnOffer(nil)
		assert.Nil(t, peer.OnOffer)
	})
}

func TestPeerLocal_SetOnIceCandidate(t *testing.T) {
	provider := &mockSessionProvider{}
	peer := NewPeer(provider).(*peerLocal)

	t.Run("コールバックを設定", func(t *testing.T) {
		var receivedType ConnectionType
		peer.SetOnIceCandidate(func(candidate *webrtc.ICECandidateInit, ct ConnectionType) {
			receivedType = ct
		})

		require.NotNil(t, peer.OnIceCandidate)

		// コールバックを呼び出し
		peer.OnIceCandidate(nil, ConnectionTypePublisher)
		assert.Equal(t, ConnectionTypePublisher, receivedType)
	})
}

func TestPeerLocal_SetOnIceConnectionStateChange(t *testing.T) {
	provider := &mockSessionProvider{}
	peer := NewPeer(provider).(*peerLocal)

	t.Run("コールバックを設定", func(t *testing.T) {
		var receivedState webrtc.ICEConnectionState
		peer.SetOnIceConnectionStateChange(func(state webrtc.ICEConnectionState) {
			receivedState = state
		})

		require.NotNil(t, peer.OnICEConnectionStateChange)

		// コールバックを呼び出し
		peer.OnICEConnectionStateChange(webrtc.ICEConnectionStateConnected)
		assert.Equal(t, webrtc.ICEConnectionStateConnected, receivedState)
	})
}

func TestPeerLocal_Trickle(t *testing.T) {
	provider := &mockSessionProvider{}
	peer := NewPeer(provider).(*peerLocal)

	t.Run("トランスポート未確立でエラー", func(t *testing.T) {
		candidate := webrtc.ICECandidateInit{
			Candidate: "candidate:1 1 UDP 2130706431 192.168.1.1 12345 typ host",
		}

		err := peer.Trickle(candidate, ConnectionTypePublisher)

		assert.ErrorIs(t, err, ErrNoTransportEstablished)
	})

	t.Run("subscriberのみnilでエラー", func(t *testing.T) {
		peer.publisher = &mockPublisher{}
		peer.subscriber = nil

		candidate := webrtc.ICECandidateInit{}
		err := peer.Trickle(candidate, ConnectionTypePublisher)

		assert.ErrorIs(t, err, ErrNoTransportEstablished)
	})

	t.Run("publisherのみnilでエラー", func(t *testing.T) {
		peer.publisher = nil
		peer.subscriber = &mockSubscriber{}

		candidate := webrtc.ICECandidateInit{}
		err := peer.Trickle(candidate, ConnectionTypeSubscriber)

		assert.ErrorIs(t, err, ErrNoTransportEstablished)
	})
}

func TestPeerLocal_SetRemoteDescription(t *testing.T) {
	provider := &mockSessionProvider{}
	peer := NewPeer(provider).(*peerLocal)

	t.Run("subscriberなしでエラー", func(t *testing.T) {
		sdp := webrtc.SessionDescription{
			Type: webrtc.SDPTypeAnswer,
			SDP:  "v=0\r\n",
		}

		err := peer.SetRemoteDescription(sdp)

		assert.ErrorIs(t, err, ErrNoTransportEstablished)
	})
}

func TestPeerInterface(t *testing.T) {
	t.Run("peerLocal型がPeerインターフェースを実装している", func(t *testing.T) {
		provider := &mockSessionProvider{}
		var _ Peer = NewPeer(provider)
	})
}

func TestPeerLocal_ClosedFlag(t *testing.T) {
	provider := &mockSessionProvider{}
	peer := NewPeer(provider).(*peerLocal)

	t.Run("初期値はfalse", func(t *testing.T) {
		assert.False(t, peer.closed.Load())
	})

	t.Run("閉じた状態を設定", func(t *testing.T) {
		peer.closed.Store(true)
		assert.True(t, peer.closed.Load())
	})
}

func TestPeerLocal_NegotiationState(t *testing.T) {
	provider := &mockSessionProvider{}
	peer := NewPeer(provider).(*peerLocal)

	t.Run("初期状態", func(t *testing.T) {
		assert.False(t, peer.remoteAnswerPending)
		assert.False(t, peer.negotiationPending)
	})

	t.Run("状態を設定", func(t *testing.T) {
		peer.remoteAnswerPending = true
		peer.negotiationPending = true

		assert.True(t, peer.remoteAnswerPending)
		assert.True(t, peer.negotiationPending)
	})
}

// mockSessionProvider はテスト用のSessionProviderモック
type mockSessionProvider struct {
	session Session
	config  WebRTCTransportConfig
}

func (m *mockSessionProvider) GetTransportConfig() WebRTCTransportConfig {
	return m.config
}

func (m *mockSessionProvider) GetSession(id string) Session {
	if m.session != nil {
		return m.session
	}
	return newMockSession()
}

// mockPublisher はテスト用のPublisherモック
type mockPublisher struct{}

func (m *mockPublisher) Answer(offer webrtc.SessionDescription) (webrtc.SessionDescription, error) {
	return webrtc.SessionDescription{}, nil
}
func (m *mockPublisher) GetRouter() Router                                            { return nil }
func (m *mockPublisher) Close()                                                       {}
func (m *mockPublisher) OnPublisherTrack(f func(track PublisherTrack))                {}
func (m *mockPublisher) OnICECandidate(f func(*webrtc.ICECandidate))                  {}
func (m *mockPublisher) OnICEConnectionStateChange(f func(webrtc.ICEConnectionState)) {}
func (m *mockPublisher) SignalingState() webrtc.SignalingState                        { return webrtc.SignalingStateStable }
func (m *mockPublisher) PeerConnection() *webrtc.PeerConnection                       { return nil }
func (m *mockPublisher) Relay(signalFn func(meta relay.PeerMeta, signal []byte) ([]byte, error), options ...func(r *relayPeer)) (*relay.Peer, error) {
	return nil, nil
}
func (m *mockPublisher) PublisherTracks() []PublisherTrack                         { return nil }
func (m *mockPublisher) AddRelayFanOutDataChannel(label string)                    {}
func (m *mockPublisher) GetRelayedDataChannels(label string) []*webrtc.DataChannel { return nil }
func (m *mockPublisher) Relayed() bool                                             { return false }
func (m *mockPublisher) Tracks() []*webrtc.TrackRemote                             { return nil }
func (m *mockPublisher) AddICECandidate(candidate webrtc.ICECandidateInit) error   { return nil }

// mockSubscriber はテスト用のSubscriberモック
type mockSubscriber struct {
	autoSubscribe bool
}

func (m *mockSubscriber) GetUserID() string                         { return "mock-user" }
func (m *mockSubscriber) GetPeerConnection() *webrtc.PeerConnection { return nil }
func (m *mockSubscriber) AddDatachannel(ctx context.Context, peer Peer, dc *Datachannel) error {
	return nil
}
func (m *mockSubscriber) DataChannel(label string) *webrtc.DataChannel { return nil }
func (m *mockSubscriber) OnNegotiationNeeded(f func())                 {}
func (m *mockSubscriber) CreateOffer() (webrtc.SessionDescription, error) {
	return webrtc.SessionDescription{}, nil
}
func (m *mockSubscriber) OnICECandidate(f func(*webrtc.ICECandidate))              {}
func (m *mockSubscriber) AddICECandidate(candidate webrtc.ICECandidateInit) error  { return nil }
func (m *mockSubscriber) AddDownTrack(streamID string, dt DownTrack)               {}
func (m *mockSubscriber) RemoveDownTrack(streamID string, dt DownTrack)            {}
func (m *mockSubscriber) AddDataChannel(label string) (*webrtc.DataChannel, error) { return nil, nil }
func (m *mockSubscriber) SetRemoteDescription(sdp webrtc.SessionDescription) error { return nil }
func (m *mockSubscriber) RegisterDatachannel(label string, dc *webrtc.DataChannel) {}
func (m *mockSubscriber) GetDatachannel(label string) *webrtc.DataChannel          { return nil }
func (m *mockSubscriber) DownTracks() []DownTrack                                  { return nil }
func (m *mockSubscriber) GetDownTracks(streamID string) []DownTrack                { return nil }
func (m *mockSubscriber) Negotiate()                                               {}
func (m *mockSubscriber) Close() error                                             { return nil }
func (m *mockSubscriber) IsAutoSubscribe() bool                                    { return m.autoSubscribe }
func (m *mockSubscriber) GetMediaEngine() *webrtc.MediaEngine                      { return nil }
func (m *mockSubscriber) SendStreamDownTracksReports(streamID string)              {}
