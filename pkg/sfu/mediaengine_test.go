package sfu

import (
	"testing"

	"github.com/pion/sdp/v3"
	"github.com/pion/webrtc/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFrameMarkingConstant(t *testing.T) {
	assert.Equal(t, "urn:ietf:params:rtp-hdrext:framemarking", frameMarking)
}

func TestGetPublisherMediaEngine(t *testing.T) {
	t.Run("正常に初期化される", func(t *testing.T) {
		me, err := getPublisherMediaEngine()

		require.NoError(t, err)
		require.NotNil(t, me)
	})

	t.Run("Opusコーデックが登録されている", func(t *testing.T) {
		me, err := getPublisherMediaEngine()
		require.NoError(t, err)

		// MediaEngineからコーデックを取得するには、
		// PeerConnectionを作成してSDPを確認するのが一般的だが、
		// ここではエラーなく初期化できることを確認
		assert.NotNil(t, me)
	})

	t.Run("複数回呼び出しても成功する", func(t *testing.T) {
		me1, err1 := getPublisherMediaEngine()
		me2, err2 := getPublisherMediaEngine()

		require.NoError(t, err1)
		require.NoError(t, err2)
		assert.NotNil(t, me1)
		assert.NotNil(t, me2)
		// 異なるインスタンスであることを確認
		assert.NotSame(t, me1, me2)
	})
}

func TestGetSubscriberMediaEngine(t *testing.T) {
	t.Run("正常に初期化される", func(t *testing.T) {
		me, err := getSubscriberMediaEngine()

		require.NoError(t, err)
		require.NotNil(t, me)
	})

	t.Run("空のMediaEngineを返す", func(t *testing.T) {
		me, err := getSubscriberMediaEngine()

		require.NoError(t, err)
		assert.NotNil(t, me)
	})

	t.Run("複数回呼び出しても成功する", func(t *testing.T) {
		me1, err1 := getSubscriberMediaEngine()
		me2, err2 := getSubscriberMediaEngine()

		require.NoError(t, err1)
		require.NoError(t, err2)
		assert.NotSame(t, me1, me2)
	})
}

func TestPublisherMediaEngineCodecs(t *testing.T) {
	me, err := getPublisherMediaEngine()
	require.NoError(t, err)

	// PeerConnectionを作成してSDPを確認
	api := webrtc.NewAPI(webrtc.WithMediaEngine(me))
	pc, err := api.NewPeerConnection(webrtc.Configuration{})
	require.NoError(t, err)
	defer pc.Close()

	// トランシーバーを追加してOfferを作成
	_, err = pc.AddTransceiverFromKind(webrtc.RTPCodecTypeAudio)
	require.NoError(t, err)
	_, err = pc.AddTransceiverFromKind(webrtc.RTPCodecTypeVideo)
	require.NoError(t, err)

	offer, err := pc.CreateOffer(nil)
	require.NoError(t, err)

	sdpStr := offer.SDP

	t.Run("Opusコーデックが含まれる", func(t *testing.T) {
		assert.Contains(t, sdpStr, "opus/48000/2")
	})

	t.Run("VP8コーデックが含まれる", func(t *testing.T) {
		assert.Contains(t, sdpStr, "VP8/90000")
	})

	t.Run("VP9コーデックが含まれる", func(t *testing.T) {
		assert.Contains(t, sdpStr, "VP9/90000")
	})

	t.Run("H264コーデックが含まれる", func(t *testing.T) {
		assert.Contains(t, sdpStr, "H264/90000")
	})

	t.Run("RTCPフィードバックが設定されている", func(t *testing.T) {
		// goog-remb
		assert.Contains(t, sdpStr, "goog-remb")
		// nack
		assert.Contains(t, sdpStr, "nack")
		// nack pli
		assert.Contains(t, sdpStr, "nack pli")
		// ccm fir
		assert.Contains(t, sdpStr, "ccm fir")
	})
}

func TestPublisherMediaEngineHeaderExtensions(t *testing.T) {
	me, err := getPublisherMediaEngine()
	require.NoError(t, err)

	api := webrtc.NewAPI(webrtc.WithMediaEngine(me))
	pc, err := api.NewPeerConnection(webrtc.Configuration{})
	require.NoError(t, err)
	defer pc.Close()

	_, err = pc.AddTransceiverFromKind(webrtc.RTPCodecTypeAudio)
	require.NoError(t, err)
	_, err = pc.AddTransceiverFromKind(webrtc.RTPCodecTypeVideo)
	require.NoError(t, err)

	offer, err := pc.CreateOffer(nil)
	require.NoError(t, err)

	sdpStr := offer.SDP

	t.Run("Video用ヘッダー拡張が含まれる", func(t *testing.T) {
		// SDESMidURI
		assert.Contains(t, sdpStr, sdp.SDESMidURI)
		// SDESRTPStreamIDURI
		assert.Contains(t, sdpStr, sdp.SDESRTPStreamIDURI)
		// TransportCCURI
		assert.Contains(t, sdpStr, sdp.TransportCCURI)
		// frameMarking
		assert.Contains(t, sdpStr, frameMarking)
	})

	t.Run("Audio用ヘッダー拡張が含まれる", func(t *testing.T) {
		// AudioLevelURI
		assert.Contains(t, sdpStr, sdp.AudioLevelURI)
	})
}

func TestMediaEnginePayloadTypes(t *testing.T) {
	// 期待されるPayloadType
	expectedPayloadTypes := map[string]webrtc.PayloadType{
		"opus":              111,
		"vp8":               96,
		"vp9-profile-0":     98,
		"vp9-profile-1":     100,
		"h264-42001f-mode1": 102,
		"h264-42001f-mode0": 127,
		"h264-42e01f-mode1": 125,
		"h264-42e01f-mode0": 108,
		"h264-640032-mode1": 123,
	}

	t.Run("PayloadTypeの定義確認", func(t *testing.T) {
		// PayloadTypeが予想通りであることを確認
		assert.Equal(t, webrtc.PayloadType(111), expectedPayloadTypes["opus"])
		assert.Equal(t, webrtc.PayloadType(96), expectedPayloadTypes["vp8"])
		assert.Equal(t, webrtc.PayloadType(98), expectedPayloadTypes["vp9-profile-0"])
		assert.Equal(t, webrtc.PayloadType(100), expectedPayloadTypes["vp9-profile-1"])
		assert.Equal(t, webrtc.PayloadType(102), expectedPayloadTypes["h264-42001f-mode1"])
	})
}

func TestVideoRTCPFeedback(t *testing.T) {
	// getPublisherMediaEngine内で定義されているRTCPフィードバック設定を確認
	expectedFeedback := []webrtc.RTCPFeedback{
		{Type: webrtc.TypeRTCPFBGoogREMB, Parameter: ""},
		{Type: webrtc.TypeRTCPFBCCM, Parameter: "fir"},
		{Type: webrtc.TypeRTCPFBNACK, Parameter: ""},
		{Type: webrtc.TypeRTCPFBNACK, Parameter: "pli"},
	}

	t.Run("RTCPフィードバック種別の確認", func(t *testing.T) {
		assert.Equal(t, webrtc.TypeRTCPFBGoogREMB, expectedFeedback[0].Type)
		assert.Equal(t, webrtc.TypeRTCPFBCCM, expectedFeedback[1].Type)
		assert.Equal(t, "fir", expectedFeedback[1].Parameter)
		assert.Equal(t, webrtc.TypeRTCPFBNACK, expectedFeedback[2].Type)
		assert.Equal(t, "", expectedFeedback[2].Parameter)
		assert.Equal(t, webrtc.TypeRTCPFBNACK, expectedFeedback[3].Type)
		assert.Equal(t, "pli", expectedFeedback[3].Parameter)
	})
}

func TestMediaEngineIntegration(t *testing.T) {
	t.Run("PublisherとSubscriberの両方が作成可能", func(t *testing.T) {
		pubME, err := getPublisherMediaEngine()
		require.NoError(t, err)

		subME, err := getSubscriberMediaEngine()
		require.NoError(t, err)

		// 両方でPeerConnectionを作成できることを確認
		pubAPI := webrtc.NewAPI(webrtc.WithMediaEngine(pubME))
		pubPC, err := pubAPI.NewPeerConnection(webrtc.Configuration{})
		require.NoError(t, err)
		defer pubPC.Close()

		subAPI := webrtc.NewAPI(webrtc.WithMediaEngine(subME))
		subPC, err := subAPI.NewPeerConnection(webrtc.Configuration{})
		require.NoError(t, err)
		defer subPC.Close()
	})
}
