package sfu_test

import (
	"testing"

	"github.com/HMasataka/choice/pkg/sfu"
	mock_sfu "github.com/HMasataka/choice/pkg/sfu/mock"
	"github.com/pion/webrtc/v4"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestReceiver_TrackID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	receiver := mock_sfu.NewMockReceiver(ctrl)
	receiver.EXPECT().TrackID().Return("track-123").Times(1)

	assert.Equal(t, "track-123", receiver.TrackID())
}

func TestReceiver_StreamID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	receiver := mock_sfu.NewMockReceiver(ctrl)
	receiver.EXPECT().StreamID().Return("stream-456").Times(1)

	assert.Equal(t, "stream-456", receiver.StreamID())
}

func TestReceiver_Kind(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	t.Run("Video", func(t *testing.T) {
		receiver := mock_sfu.NewMockReceiver(ctrl)
		receiver.EXPECT().Kind().Return(webrtc.RTPCodecTypeVideo).Times(1)

		assert.Equal(t, webrtc.RTPCodecTypeVideo, receiver.Kind())
	})

	t.Run("Audio", func(t *testing.T) {
		receiver := mock_sfu.NewMockReceiver(ctrl)
		receiver.EXPECT().Kind().Return(webrtc.RTPCodecTypeAudio).Times(1)

		assert.Equal(t, webrtc.RTPCodecTypeAudio, receiver.Kind())
	})
}

func TestReceiver_Codec(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	t.Run("コーデック情報を返す", func(t *testing.T) {
		codec := webrtc.RTPCodecParameters{
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType:  "video/VP8",
				ClockRate: 90000,
			},
			PayloadType: 96,
		}
		receiver := mock_sfu.NewMockReceiver(ctrl)
		receiver.EXPECT().Codec().Return(codec).Times(1)

		result := receiver.Codec()

		assert.Equal(t, "video/VP8", result.MimeType)
		assert.Equal(t, uint32(90000), result.ClockRate)
		assert.Equal(t, webrtc.PayloadType(96), result.PayloadType)
	})
}

func TestReceiver_SSRC(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	t.Run("SSRCを返す", func(t *testing.T) {
		receiver := mock_sfu.NewMockReceiver(ctrl)
		receiver.EXPECT().SSRC(0).Return(uint32(12345)).Times(1)

		ssrc := receiver.SSRC(0)

		assert.Equal(t, uint32(12345), ssrc)
	})
}

func TestReceiver_GetBitrate(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	t.Run("ビットレートを返す", func(t *testing.T) {
		receiver := mock_sfu.NewMockReceiver(ctrl)
		receiver.EXPECT().GetBitrate().Return([3]uint64{1000, 2000, 3000}).Times(1)

		br := receiver.GetBitrate()

		assert.Equal(t, [3]uint64{1000, 2000, 3000}, br)
	})
}

func TestReceiver_GetMaxTemporalLayer(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	t.Run("最大テンポラルレイヤーを返す", func(t *testing.T) {
		receiver := mock_sfu.NewMockReceiver(ctrl)
		receiver.EXPECT().GetMaxTemporalLayer().Return([3]int32{0, 1, 2}).Times(1)

		tls := receiver.GetMaxTemporalLayer()

		assert.Equal(t, [3]int32{0, 1, 2}, tls)
	})
}

func TestReceiver_OnCloseHandler(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	t.Run("ハンドラを設定", func(t *testing.T) {
		receiver := mock_sfu.NewMockReceiver(ctrl)
		receiver.EXPECT().OnCloseHandler(gomock.Any()).Times(1)

		// パニックしないことを確認
		receiver.OnCloseHandler(func() {})
	})
}

func TestReceiver_AddDownTrack(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	t.Run("DownTrackを追加", func(t *testing.T) {
		receiver := mock_sfu.NewMockReceiver(ctrl)
		dt := mock_sfu.NewMockDownTrack(ctrl)
		receiver.EXPECT().AddDownTrack(dt, true).Times(1)

		// パニックしないことを確認
		receiver.AddDownTrack(dt, true)
	})
}

func TestReceiver_SwitchDownTrack(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	t.Run("レイヤー切り替え成功", func(t *testing.T) {
		receiver := mock_sfu.NewMockReceiver(ctrl)
		dt := mock_sfu.NewMockDownTrack(ctrl)
		receiver.EXPECT().SwitchDownTrack(dt, 1).Return(nil).Times(1)

		err := receiver.SwitchDownTrack(dt, 1)

		assert.NoError(t, err)
	})

	t.Run("レイヤー切り替えエラー", func(t *testing.T) {
		receiver := mock_sfu.NewMockReceiver(ctrl)
		dt := mock_sfu.NewMockDownTrack(ctrl)
		receiver.EXPECT().SwitchDownTrack(dt, 1).Return(sfu.ErrNoReceiverFoundForTest).Times(1)

		err := receiver.SwitchDownTrack(dt, 1)

		assert.Error(t, err)
	})
}

func TestReceiver_DeleteDownTrack(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	t.Run("DownTrackを削除", func(t *testing.T) {
		receiver := mock_sfu.NewMockReceiver(ctrl)
		receiver.EXPECT().DeleteDownTrack(0, "test-id").Times(1)

		// パニックしないことを確認
		receiver.DeleteDownTrack(0, "test-id")
	})
}
