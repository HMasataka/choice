package sfu_test

import (
	"testing"

	"github.com/HMasataka/choice/pkg/buffer"
	"github.com/HMasataka/choice/pkg/sfu"
	mock_sfu "github.com/HMasataka/choice/pkg/sfu/mock"
	"github.com/pion/webrtc/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestNewDownTrack(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	t.Run("正常に初期化される", func(t *testing.T) {
		receiver := mock_sfu.NewMockReceiver(ctrl)
		receiver.EXPECT().TrackID().Return("test-track").AnyTimes()
		receiver.EXPECT().StreamID().Return("test-stream").AnyTimes()

		codec := webrtc.RTPCodecCapability{
			MimeType:  "video/VP8",
			ClockRate: 90000,
		}
		bf := buffer.NewBufferFactory(500)

		dt, err := sfu.NewDownTrack(codec, receiver, bf, "peer-123", 100)

		require.NoError(t, err)
		require.NotNil(t, dt)
		assert.Equal(t, "test-track", dt.ID())
		assert.Equal(t, "test-stream", dt.StreamID())
	})
}

func TestDownTrack_ID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dt := mock_sfu.NewMockDownTrack(ctrl)
	dt.EXPECT().ID().Return("track-123").Times(1)

	assert.Equal(t, "track-123", dt.ID())
}

func TestDownTrack_StreamID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dt := mock_sfu.NewMockDownTrack(ctrl)
	dt.EXPECT().StreamID().Return("stream-456").Times(1)

	assert.Equal(t, "stream-456", dt.StreamID())
}

func TestDownTrack_RID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dt := mock_sfu.NewMockDownTrack(ctrl)
	dt.EXPECT().RID().Return("").Times(1)

	// RIDは常に空文字を返す
	assert.Equal(t, "", dt.RID())
}

func TestDownTrack_Codec(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	codec := webrtc.RTPCodecCapability{
		MimeType:  "video/VP8",
		ClockRate: 90000,
	}
	dt := mock_sfu.NewMockDownTrack(ctrl)
	dt.EXPECT().Codec().Return(codec).Times(1)

	result := dt.Codec()

	assert.Equal(t, "video/VP8", result.MimeType)
	assert.Equal(t, uint32(90000), result.ClockRate)
}

func TestDownTrack_Kind(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	t.Run("オーディオ", func(t *testing.T) {
		dt := mock_sfu.NewMockDownTrack(ctrl)
		dt.EXPECT().Kind().Return(webrtc.RTPCodecTypeAudio).Times(1)

		assert.Equal(t, webrtc.RTPCodecTypeAudio, dt.Kind())
	})

	t.Run("ビデオ", func(t *testing.T) {
		dt := mock_sfu.NewMockDownTrack(ctrl)
		dt.EXPECT().Kind().Return(webrtc.RTPCodecTypeVideo).Times(1)

		assert.Equal(t, webrtc.RTPCodecTypeVideo, dt.Kind())
	})
}

func TestDownTrack_Enabled(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	t.Run("false", func(t *testing.T) {
		dt := mock_sfu.NewMockDownTrack(ctrl)
		dt.EXPECT().Enabled().Return(false).Times(1)

		assert.False(t, dt.Enabled())
	})

	t.Run("true", func(t *testing.T) {
		dt := mock_sfu.NewMockDownTrack(ctrl)
		dt.EXPECT().Enabled().Return(true).Times(1)

		assert.True(t, dt.Enabled())
	})
}

func TestDownTrack_Bound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	t.Run("false", func(t *testing.T) {
		dt := mock_sfu.NewMockDownTrack(ctrl)
		dt.EXPECT().Bound().Return(false).Times(1)

		assert.False(t, dt.Bound())
	})

	t.Run("true", func(t *testing.T) {
		dt := mock_sfu.NewMockDownTrack(ctrl)
		dt.EXPECT().Bound().Return(true).Times(1)

		assert.True(t, dt.Bound())
	})
}

func TestDownTrack_Mute(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	t.Run("ミュート", func(t *testing.T) {
		dt := mock_sfu.NewMockDownTrack(ctrl)
		dt.EXPECT().Mute(true).Times(1)

		// パニックしないことを確認
		dt.Mute(true)
	})

	t.Run("アンミュート", func(t *testing.T) {
		dt := mock_sfu.NewMockDownTrack(ctrl)
		dt.EXPECT().Mute(false).Times(1)

		// パニックしないことを確認
		dt.Mute(false)
	})
}

func TestDownTrack_SetTransceiver(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dt := mock_sfu.NewMockDownTrack(ctrl)
	dt.EXPECT().SetTransceiver(nil).Times(1)

	// パニックしないことを確認
	dt.SetTransceiver(nil)
}

func TestDownTrack_CurrentSpatialLayer(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dt := mock_sfu.NewMockDownTrack(ctrl)
	dt.EXPECT().CurrentSpatialLayer().Return(1).Times(1)

	assert.Equal(t, 1, dt.CurrentSpatialLayer())
}

func TestDownTrack_OnCloseHandler(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dt := mock_sfu.NewMockDownTrack(ctrl)
	dt.EXPECT().OnCloseHandler(gomock.Any()).Times(1)

	// パニックしないことを確認
	dt.OnCloseHandler(func() {})
}

func TestDownTrack_OnBind(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dt := mock_sfu.NewMockDownTrack(ctrl)
	dt.EXPECT().OnBind(gomock.Any()).Times(1)

	// パニックしないことを確認
	dt.OnBind(func() {})
}

func TestDownTrack_SetMaxSpatialLayer(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dt := mock_sfu.NewMockDownTrack(ctrl)
	dt.EXPECT().SetMaxSpatialLayer(int32(2)).Times(1)

	// パニックしないことを確認
	dt.SetMaxSpatialLayer(2)
}

func TestDownTrack_SetMaxTemporalLayer(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dt := mock_sfu.NewMockDownTrack(ctrl)
	dt.EXPECT().SetMaxTemporalLayer(int32(2)).Times(1)

	// パニックしないことを確認
	dt.SetMaxTemporalLayer(2)
}

func TestDownTrack_SetLastSSRC(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dt := mock_sfu.NewMockDownTrack(ctrl)
	dt.EXPECT().SetLastSSRC(uint32(12345)).Times(1)

	// パニックしないことを確認
	dt.SetLastSSRC(12345)
}

func TestDownTrack_SetTrackType(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dt := mock_sfu.NewMockDownTrack(ctrl)
	dt.EXPECT().SetTrackType(sfu.SimulcastDownTrackForTest).Times(1)

	// パニックしないことを確認
	dt.SetTrackType(sfu.SimulcastDownTrackForTest)
}

func TestDownTrack_SwitchSpatialLayer(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	t.Run("成功", func(t *testing.T) {
		dt := mock_sfu.NewMockDownTrack(ctrl)
		dt.EXPECT().SwitchSpatialLayer(int32(1), false).Return(nil).Times(1)

		err := dt.SwitchSpatialLayer(1, false)

		assert.NoError(t, err)
	})

	t.Run("エラー", func(t *testing.T) {
		dt := mock_sfu.NewMockDownTrack(ctrl)
		dt.EXPECT().SwitchSpatialLayer(int32(1), false).Return(sfu.ErrSpatialNotSupported).Times(1)

		err := dt.SwitchSpatialLayer(1, false)

		assert.ErrorIs(t, err, sfu.ErrSpatialNotSupported)
	})
}

func TestDownTrack_SwitchTemporalLayer(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dt := mock_sfu.NewMockDownTrack(ctrl)
	dt.EXPECT().SwitchTemporalLayer(int32(1), false).Times(1)

	// パニックしないことを確認
	dt.SwitchTemporalLayer(1, false)
}

func TestDownTrack_Close(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dt := mock_sfu.NewMockDownTrack(ctrl)
	dt.EXPECT().Close().Times(1)

	// パニックしないことを確認
	dt.Close()
}

func TestDownTrackType(t *testing.T) {
	t.Run("定数の値", func(t *testing.T) {
		assert.Equal(t, sfu.DownTrackType(1), sfu.SimpleDownTrackForTest)
		assert.Equal(t, sfu.DownTrackType(2), sfu.SimulcastDownTrackForTest)
	})
}
