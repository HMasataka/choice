package sfu

import (
	"testing"

	"github.com/pion/webrtc/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModifyVP8TemporalPayload(t *testing.T) {
	t.Run("MBitなし（8ビットPictureID）", func(t *testing.T) {
		// VP8ペイロード: [desc, picID, tlz0Idx, ...]
		payload := make([]byte, 10)
		payload[1] = 0x00 // 元のpicID
		payload[2] = 0x00 // 元のtlz0Idx

		modifyVP8TemporalPayload(payload, 1, 2, 0x1234, 0x56, false)

		// MBitなしの場合、picIDは上位バイトのみ（0x80なし）
		assert.Equal(t, byte(0x12), payload[1])
		assert.Equal(t, byte(0x56), payload[2])
	})

	t.Run("MBitあり（16ビットPictureID）", func(t *testing.T) {
		payload := make([]byte, 10)
		payload[1] = 0x00
		payload[2] = 0x00
		payload[3] = 0x00

		modifyVP8TemporalPayload(payload, 1, 3, 0x1234, 0x56, true)

		// MBitありの場合、picIDは16ビット（0x80 OR）
		assert.Equal(t, byte(0x12|0x80), payload[1])
		assert.Equal(t, byte(0x34), payload[2])
		assert.Equal(t, byte(0x56), payload[3])
	})

	t.Run("PictureID境界値", func(t *testing.T) {
		payload := make([]byte, 10)

		// 最大値
		modifyVP8TemporalPayload(payload, 1, 3, 0xFFFF, 0xFF, true)
		assert.Equal(t, byte(0xFF), payload[1])
		assert.Equal(t, byte(0xFF), payload[2])
		assert.Equal(t, byte(0xFF), payload[3])

		// 最小値
		modifyVP8TemporalPayload(payload, 1, 3, 0x0000, 0x00, true)
		assert.Equal(t, byte(0x80), payload[1]) // 0x00 | 0x80
		assert.Equal(t, byte(0x00), payload[2])
		assert.Equal(t, byte(0x00), payload[3])
	})
}

func TestCodecParametersFuzzySearch(t *testing.T) {
	haystack := []webrtc.RTPCodecParameters{
		{
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType:    "video/VP8",
				SDPFmtpLine: "",
			},
			PayloadType: 96,
		},
		{
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType:    "video/H264",
				SDPFmtpLine: "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42001f",
			},
			PayloadType: 97,
		},
		{
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType:    "video/H264",
				SDPFmtpLine: "level-asymmetry-allowed=1;packetization-mode=0;profile-level-id=42001f",
			},
			PayloadType: 98,
		},
		{
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType:    "audio/opus",
				SDPFmtpLine: "minptime=10;useinbandfec=1",
			},
			PayloadType: 111,
		},
	}

	t.Run("MimeTypeとSDPFmtpLineで完全一致", func(t *testing.T) {
		needle := webrtc.RTPCodecParameters{
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType:    "video/H264",
				SDPFmtpLine: "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42001f",
			},
		}

		result, err := codecParametersFuzzySearch(needle, haystack)

		require.NoError(t, err)
		assert.Equal(t, webrtc.PayloadType(97), result.PayloadType)
	})

	t.Run("MimeTypeのみで一致（大文字小文字無視）", func(t *testing.T) {
		needle := webrtc.RTPCodecParameters{
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType:    "VIDEO/VP8", // 大文字
				SDPFmtpLine: "different-fmtp",
			},
		}

		result, err := codecParametersFuzzySearch(needle, haystack)

		require.NoError(t, err)
		assert.Equal(t, webrtc.PayloadType(96), result.PayloadType)
	})

	t.Run("MimeType小文字で検索", func(t *testing.T) {
		needle := webrtc.RTPCodecParameters{
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType:    "audio/opus",
				SDPFmtpLine: "minptime=10;useinbandfec=1",
			},
		}

		result, err := codecParametersFuzzySearch(needle, haystack)

		require.NoError(t, err)
		assert.Equal(t, webrtc.PayloadType(111), result.PayloadType)
	})

	t.Run("存在しないコーデック", func(t *testing.T) {
		needle := webrtc.RTPCodecParameters{
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType: "video/AV1",
			},
		}

		_, err := codecParametersFuzzySearch(needle, haystack)

		assert.Equal(t, webrtc.ErrCodecNotFound, err)
	})

	t.Run("空のhaystack", func(t *testing.T) {
		needle := webrtc.RTPCodecParameters{
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType: "video/VP8",
			},
		}

		_, err := codecParametersFuzzySearch(needle, []webrtc.RTPCodecParameters{})

		assert.Equal(t, webrtc.ErrCodecNotFound, err)
	})

	t.Run("SDPFmtpLine優先でマッチ", func(t *testing.T) {
		// 同じMimeTypeで異なるSDPFmtpLineがある場合、
		// SDPFmtpLineが一致するものを優先
		needle := webrtc.RTPCodecParameters{
			RTPCodecCapability: webrtc.RTPCodecCapability{
				MimeType:    "video/H264",
				SDPFmtpLine: "level-asymmetry-allowed=1;packetization-mode=0;profile-level-id=42001f",
			},
		}

		result, err := codecParametersFuzzySearch(needle, haystack)

		require.NoError(t, err)
		assert.Equal(t, webrtc.PayloadType(98), result.PayloadType) // packetization-mode=0の方
	})
}
