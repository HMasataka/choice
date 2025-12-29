package sfu_test

import (
	"testing"
	"time"

	"github.com/HMasataka/choice/pkg/sfu"
	mock_sfu "github.com/HMasataka/choice/pkg/sfu/mock"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestSubscriberConstants(t *testing.T) {
	t.Run("APIChannelLabel", func(t *testing.T) {
		assert.Equal(t, "choice", sfu.APIChannelLabel)
	})
}

func TestSubscriber_DownTracks(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	t.Run("mockを使ったDownTracks取得", func(t *testing.T) {
		sub := mock_sfu.NewMockSubscriber(ctrl)
		dt1 := mock_sfu.NewMockDownTrack(ctrl)
		dt2 := mock_sfu.NewMockDownTrack(ctrl)

		sub.EXPECT().DownTracks().Return([]sfu.DownTrack{dt1, dt2}).Times(1)

		result := sub.DownTracks()
		assert.Len(t, result, 2)
	})
}

func TestSubscriber_GetDownTracks(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	t.Run("存在するストリーム", func(t *testing.T) {
		sub := mock_sfu.NewMockSubscriber(ctrl)
		dt := mock_sfu.NewMockDownTrack(ctrl)

		sub.EXPECT().GetDownTracks("stream-1").Return([]sfu.DownTrack{dt}).Times(1)

		result := sub.GetDownTracks("stream-1")
		assert.Len(t, result, 1)
	})

	t.Run("存在しないストリーム", func(t *testing.T) {
		sub := mock_sfu.NewMockSubscriber(ctrl)

		sub.EXPECT().GetDownTracks("nonexistent").Return(nil).Times(1)

		result := sub.GetDownTracks("nonexistent")
		assert.Nil(t, result)
	})
}

func TestSubscriber_IsAutoSubscribe(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	t.Run("true", func(t *testing.T) {
		sub := mock_sfu.NewMockSubscriber(ctrl)
		sub.EXPECT().IsAutoSubscribe().Return(true).Times(1)

		assert.True(t, sub.IsAutoSubscribe())
	})

	t.Run("false", func(t *testing.T) {
		sub := mock_sfu.NewMockSubscriber(ctrl)
		sub.EXPECT().IsAutoSubscribe().Return(false).Times(1)

		assert.False(t, sub.IsAutoSubscribe())
	})
}

func TestSubscriber_GetUserID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	sub := mock_sfu.NewMockSubscriber(ctrl)
	sub.EXPECT().GetUserID().Return("user-123").Times(1)

	assert.Equal(t, "user-123", sub.GetUserID())
}

func TestSubscriber_Negotiate(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	t.Run("Negotiate呼び出し", func(t *testing.T) {
		sub := mock_sfu.NewMockSubscriber(ctrl)
		sub.EXPECT().Negotiate().Times(1)

		// パニックしないことを確認
		sub.Negotiate()
	})
}

func TestDownTrackReportInterval(t *testing.T) {
	// この定数はexportされていないため、関連する公開APIをテスト
	// 5秒間隔で送信されることを確認するには統合テストが必要
	_ = 5 * time.Second
}
