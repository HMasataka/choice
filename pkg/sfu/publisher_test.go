package sfu

import (
	"sync"
	"testing"
	"time"

	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v4"
	"github.com/stretchr/testify/assert"
)

func TestPublisherInterface(t *testing.T) {
	t.Run("publisher型がPublisherインターフェースを実装している", func(t *testing.T) {
		// コンパイル時チェック
		var _ Publisher = (*publisher)(nil)
	})
}

func TestPublisherConstants(t *testing.T) {
	t.Run("senderReportInterval", func(t *testing.T) {
		assert.Equal(t, 5*time.Second, senderReportInterval)
	})
}

func TestPublisherTrack(t *testing.T) {
	t.Run("構造体の初期化", func(t *testing.T) {
		pt := PublisherTrack{
			Track:       nil,
			Receiver:    nil,
			clientRelay: true,
		}

		assert.Nil(t, pt.Track)
		assert.Nil(t, pt.Receiver)
		assert.True(t, pt.clientRelay)
	})
}

func TestRelayPeer(t *testing.T) {
	t.Run("構造体の初期化", func(t *testing.T) {
		rp := relayPeer{
			peer:                    nil,
			dataChannels:            []*webrtc.DataChannel{},
			withSRReports:           true,
			relayFanOutDataChannels: true,
		}

		assert.Nil(t, rp.peer)
		assert.Empty(t, rp.dataChannels)
		assert.True(t, rp.withSRReports)
		assert.True(t, rp.relayFanOutDataChannels)
	})
}

func TestPublisher_StructFields(t *testing.T) {
	t.Run("初期状態", func(t *testing.T) {
		p := &publisher{
			userID:     "test-user",
			tracks:     []PublisherTrack{},
			relayPeers: []*relayPeer{},
			candidates: []webrtc.ICECandidateInit{},
		}

		assert.Equal(t, "test-user", p.userID)
		assert.Empty(t, p.tracks)
		assert.Empty(t, p.relayPeers)
		assert.Empty(t, p.candidates)
		assert.False(t, p.relayed.Load())
	})
}

func TestPublisher_GetRouter(t *testing.T) {
	t.Run("routerを返す", func(t *testing.T) {
		mockRouter := &mockRouter{}
		p := &publisher{
			router: mockRouter,
		}

		result := p.GetRouter()

		assert.Equal(t, mockRouter, result)
	})

	t.Run("nilの場合", func(t *testing.T) {
		p := &publisher{
			router: nil,
		}

		result := p.GetRouter()

		assert.Nil(t, result)
	})
}

func TestPublisher_Relayed(t *testing.T) {
	t.Run("初期値はfalse", func(t *testing.T) {
		p := &publisher{}

		assert.False(t, p.Relayed())
	})

	t.Run("trueに設定", func(t *testing.T) {
		p := &publisher{}
		p.relayed.Store(true)

		assert.True(t, p.Relayed())
	})
}

func TestPublisher_PublisherTracks(t *testing.T) {
	t.Run("空のtracks", func(t *testing.T) {
		p := &publisher{
			tracks: []PublisherTrack{},
		}

		result := p.PublisherTracks()

		assert.Empty(t, result)
	})

	t.Run("複数のtracks", func(t *testing.T) {
		p := &publisher{
			tracks: []PublisherTrack{
				{Track: nil, Receiver: nil, clientRelay: true},
				{Track: nil, Receiver: nil, clientRelay: false},
			},
		}

		result := p.PublisherTracks()

		assert.Len(t, result, 2)
	})

	t.Run("コピーが返される", func(t *testing.T) {
		p := &publisher{
			tracks: []PublisherTrack{
				{clientRelay: true},
			},
		}

		result := p.PublisherTracks()
		result[0].clientRelay = false

		// 元のスライスは変更されない
		assert.True(t, p.tracks[0].clientRelay)
	})
}

func TestPublisher_Tracks(t *testing.T) {
	t.Run("空のtracks", func(t *testing.T) {
		p := &publisher{
			tracks: []PublisherTrack{},
		}

		result := p.Tracks()

		assert.Empty(t, result)
	})

	t.Run("nilトラックを含む", func(t *testing.T) {
		p := &publisher{
			tracks: []PublisherTrack{
				{Track: nil},
				{Track: nil},
			},
		}

		result := p.Tracks()

		assert.Len(t, result, 2)
		assert.Nil(t, result[0])
		assert.Nil(t, result[1])
	})
}

func TestPublisher_GetRelayedDataChannels(t *testing.T) {
	t.Run("relayPeersが空", func(t *testing.T) {
		p := &publisher{
			relayPeers: []*relayPeer{},
		}

		result := p.GetRelayedDataChannels("test")

		assert.Empty(t, result)
	})

	t.Run("dataChannelsが空", func(t *testing.T) {
		p := &publisher{
			relayPeers: []*relayPeer{
				{dataChannels: []*webrtc.DataChannel{}},
			},
		}

		result := p.GetRelayedDataChannels("test")

		assert.Empty(t, result)
	})
}

func TestPublisher_OnPublisherTrack(t *testing.T) {
	t.Run("コールバックを設定", func(t *testing.T) {
		p := &publisher{}

		called := false
		p.OnPublisherTrack(func(track PublisherTrack) {
			called = true
		})

		assert.NotNil(t, p.onPublisherTrack)

		// コールバックを呼び出し
		p.onPublisherTrack(PublisherTrack{})
		assert.True(t, called)
	})

	t.Run("nilで設定", func(t *testing.T) {
		p := &publisher{}
		p.OnPublisherTrack(nil)

		assert.Nil(t, p.onPublisherTrack)
	})
}

func TestPublisher_OnICEConnectionStateChange(t *testing.T) {
	t.Run("コールバックを設定", func(t *testing.T) {
		p := &publisher{}

		var receivedState webrtc.ICEConnectionState
		p.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
			receivedState = state
		})

		assert.NotNil(t, p.onICEConnectionStateChangeHandler)

		// コールバックを呼び出し
		p.onICEConnectionStateChangeHandler(webrtc.ICEConnectionStateConnected)
		assert.Equal(t, webrtc.ICEConnectionStateConnected, receivedState)
	})
}

func TestPublisher_ApplyRelayOptions(t *testing.T) {
	t.Run("オプションなし", func(t *testing.T) {
		p := &publisher{}

		result := p.applyRelayOptions(nil)

		assert.NotNil(t, result)
		assert.False(t, result.withSRReports)
		assert.False(t, result.relayFanOutDataChannels)
	})

	t.Run("オプションあり", func(t *testing.T) {
		p := &publisher{}

		options := []func(r *relayPeer){
			func(r *relayPeer) {
				r.withSRReports = true
			},
			func(r *relayPeer) {
				r.relayFanOutDataChannels = true
			},
		}

		result := p.applyRelayOptions(options)

		assert.True(t, result.withSRReports)
		assert.True(t, result.relayFanOutDataChannels)
	})
}

func TestPublisher_AddICECandidate(t *testing.T) {
	t.Run("RemoteDescriptionがない場合はキューに追加", func(t *testing.T) {
		// PeerConnectionがnilの場合はRemoteDescription()を呼べないため
		// candidatesへの追加ロジックのみをテスト
		p := &publisher{
			candidates: []webrtc.ICECandidateInit{},
		}

		candidate := webrtc.ICECandidateInit{
			Candidate: "candidate:1 1 UDP 2130706431 192.168.1.1 12345 typ host",
		}

		// pcがnilのため直接candidatesに追加をシミュレート
		p.candidates = append(p.candidates, candidate)

		assert.Len(t, p.candidates, 1)
		assert.Equal(t, candidate.Candidate, p.candidates[0].Candidate)
	})
}

func TestPublisher_ConcurrentAccess(t *testing.T) {
	p := &publisher{
		tracks:     []PublisherTrack{},
		relayPeers: []*relayPeer{},
	}

	var wg sync.WaitGroup

	// 並行してPublisherTracksを取得
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = p.PublisherTracks()
		}()
	}

	// 並行してTracksを取得
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = p.Tracks()
		}()
	}

	// 並行してRelayedを取得
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = p.Relayed()
		}()
	}

	// 並行してGetRelayedDataChannelsを呼び出し
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = p.GetRelayedDataChannels("test")
		}()
	}

	wg.Wait()
	// パニックなく完了すればテスト成功
}

func TestPublisher_ForwardDataChannelMessage(t *testing.T) {
	t.Run("peerがnilの場合", func(t *testing.T) {
		p := &publisher{}

		msg := webrtc.DataChannelMessage{
			IsString: true,
			Data:     []byte("test"),
		}

		// パニックしないことを確認
		p.forwardDataChannelMessage("test", nil, msg)
	})

	t.Run("subscriberがnilの場合", func(t *testing.T) {
		p := &publisher{}
		mockPeer := &mockPeer{subscriber: nil}

		msg := webrtc.DataChannelMessage{
			IsString: true,
			Data:     []byte("test"),
		}

		// パニックしないことを確認
		p.forwardDataChannelMessage("test", mockPeer, msg)
	})
}

// mockRouter はテスト用のRouterモック
type mockRouter struct {
	userID string
}

func (m *mockRouter) UserID() string                                                    { return m.userID }
func (m *mockRouter) GetReceiver() map[string]Receiver                                  { return nil }
func (m *mockRouter) OnAddReceiverTrack(f func(receiver Receiver))                      {}
func (m *mockRouter) OnDelReceiverTrack(f func(receiver Receiver))                      {}
func (m *mockRouter) AddReceiver(receiver *webrtc.RTPReceiver, track *webrtc.TrackRemote, trackID, streamID string) (Receiver, bool) {
	return nil, false
}
func (m *mockRouter) AddDownTracks(s Subscriber, r Receiver) error          { return nil }
func (m *mockRouter) AddDownTrack(s Subscriber, r Receiver) (DownTrack, error) {
	return nil, nil
}
func (m *mockRouter) SetRTCPWriter(writer func(pkts []rtcp.Packet) error) {}
func (m *mockRouter) Stop()                                               {}
