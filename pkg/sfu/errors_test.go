package sfu

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestErrorDefinitions(t *testing.T) {
	t.Run("エラーがnilでない", func(t *testing.T) {
		assert.NotNil(t, errPeerConnectionInitFailed)
		assert.NotNil(t, errCreatingDataChannel)
		assert.NotNil(t, errNoReceiverFound)
		assert.NotNil(t, errShortPacket)
		assert.NotNil(t, errNilPacket)
		assert.NotNil(t, ErrSpatialNotSupported)
		assert.NotNil(t, ErrSpatialLayerBusy)
	})

	t.Run("エラーメッセージが空でない", func(t *testing.T) {
		assert.NotEmpty(t, errPeerConnectionInitFailed.Error())
		assert.NotEmpty(t, errCreatingDataChannel.Error())
		assert.NotEmpty(t, errNoReceiverFound.Error())
		assert.NotEmpty(t, errShortPacket.Error())
		assert.NotEmpty(t, errNilPacket.Error())
		assert.NotEmpty(t, ErrSpatialNotSupported.Error())
		assert.NotEmpty(t, ErrSpatialLayerBusy.Error())
	})

	t.Run("エラーメッセージの内容確認", func(t *testing.T) {
		assert.Equal(t, "pc init failed", errPeerConnectionInitFailed.Error())
		assert.Equal(t, "failed to create data channel", errCreatingDataChannel.Error())
		assert.Equal(t, "no receiver found", errNoReceiverFound.Error())
		assert.Equal(t, "packet is not large enough", errShortPacket.Error())
		assert.Equal(t, "invalid nil packet", errNilPacket.Error())
		assert.Equal(t, "current track does not support simulcast/SVC", ErrSpatialNotSupported.Error())
		assert.Equal(t, "a spatial layer change is in progress, try latter", ErrSpatialLayerBusy.Error())
	})

	t.Run("errors.Isでの比較", func(t *testing.T) {
		// 同じエラーインスタンスとの比較
		assert.True(t, errors.Is(errPeerConnectionInitFailed, errPeerConnectionInitFailed))
		assert.True(t, errors.Is(ErrSpatialNotSupported, ErrSpatialNotSupported))

		// 異なるエラーとの比較
		assert.False(t, errors.Is(errPeerConnectionInitFailed, errCreatingDataChannel))
		assert.False(t, errors.Is(ErrSpatialNotSupported, ErrSpatialLayerBusy))
	})
}

func TestExportedErrors(t *testing.T) {
	// エクスポートされたエラー（大文字始まり）のテスト
	t.Run("ErrSpatialNotSupportedはエクスポートされている", func(t *testing.T) {
		var err error = ErrSpatialNotSupported
		assert.Error(t, err)
	})

	t.Run("ErrSpatialLayerBusyはエクスポートされている", func(t *testing.T) {
		var err error = ErrSpatialLayerBusy
		assert.Error(t, err)
	})
}
