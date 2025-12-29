package sfu_test

import (
	"testing"

	"github.com/HMasataka/choice/pkg/sfu"
	"github.com/stretchr/testify/assert"
)

func TestPublisherTrack(t *testing.T) {
	t.Run("構造体の初期化", func(t *testing.T) {
		pt := sfu.PublisherTrack{
			Track:    nil,
			Receiver: nil,
		}

		assert.Nil(t, pt.Track)
		assert.Nil(t, pt.Receiver)
	})
}
