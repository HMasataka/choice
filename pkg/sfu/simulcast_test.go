package sfu

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestSimulcastResolutionConstants(t *testing.T) {
	// 解像度定数が正しく定義されていることを確認
	assert.Equal(t, "q", quarterResolution)
	assert.Equal(t, "h", halfResolution)
	assert.Equal(t, "f", fullResolution)
}

func TestSimulcastConfig(t *testing.T) {
	t.Run("デフォルト値", func(t *testing.T) {
		config := SimulcastConfig{}

		assert.False(t, config.BestQualityFirst)
		assert.False(t, config.EnableTemporalLayer)
	})

	t.Run("値を設定", func(t *testing.T) {
		config := SimulcastConfig{
			BestQualityFirst:    true,
			EnableTemporalLayer: true,
		}

		assert.True(t, config.BestQualityFirst)
		assert.True(t, config.EnableTemporalLayer)
	})
}

func TestSimulcastTrackHelpers(t *testing.T) {
	t.Run("デフォルト値", func(t *testing.T) {
		helpers := simulcastTrackHelpers{}

		assert.True(t, helpers.switchDelay.IsZero())
		assert.False(t, helpers.temporalSupported)
		assert.False(t, helpers.temporalEnabled)
		assert.Equal(t, int64(0), helpers.lTSCalc)
		assert.Equal(t, uint16(0), helpers.pRefPicID)
		assert.Equal(t, uint16(0), helpers.refPicID)
		assert.Equal(t, uint16(0), helpers.lPicID)
		assert.Equal(t, uint8(0), helpers.pRefTlZIdx)
		assert.Equal(t, uint8(0), helpers.refTlZIdx)
		assert.Equal(t, uint8(0), helpers.lTlZIdx)
		assert.Equal(t, uint16(0), helpers.refSN)
	})

	t.Run("値を設定", func(t *testing.T) {
		now := time.Now()
		helpers := simulcastTrackHelpers{
			switchDelay:       now,
			temporalSupported: true,
			temporalEnabled:   true,
			lTSCalc:           12345,
			pRefPicID:         100,
			refPicID:          101,
			lPicID:            102,
			pRefTlZIdx:        1,
			refTlZIdx:         2,
			lTlZIdx:           3,
			refSN:             1000,
		}

		assert.Equal(t, now, helpers.switchDelay)
		assert.True(t, helpers.temporalSupported)
		assert.True(t, helpers.temporalEnabled)
		assert.Equal(t, int64(12345), helpers.lTSCalc)
		assert.Equal(t, uint16(100), helpers.pRefPicID)
		assert.Equal(t, uint16(101), helpers.refPicID)
		assert.Equal(t, uint16(102), helpers.lPicID)
		assert.Equal(t, uint8(1), helpers.pRefTlZIdx)
		assert.Equal(t, uint8(2), helpers.refTlZIdx)
		assert.Equal(t, uint8(3), helpers.lTlZIdx)
		assert.Equal(t, uint16(1000), helpers.refSN)
	})
}
