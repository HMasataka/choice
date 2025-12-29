package buffer

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVP8_Unmarshal(t *testing.T) {
	t.Run("nilペイロード", func(t *testing.T) {
		vp8 := &VP8{}
		err := vp8.Unmarshal(nil)
		assert.Equal(t, errNilPacket, err)
	})

	t.Run("空のペイロード", func(t *testing.T) {
		vp8 := &VP8{}
		err := vp8.Unmarshal([]byte{})
		assert.Equal(t, errShortPacket, err)
	})

	t.Run("拡張なしキーフレーム", func(t *testing.T) {
		// X=0, S=1 (開始パーティション)
		// 2バイト目: キーフレーム (bit0=0)
		payload := []byte{0x10, 0x00}
		vp8 := &VP8{}

		err := vp8.Unmarshal(payload)

		require.NoError(t, err)
		assert.True(t, vp8.IsKeyFrame)
	})

	t.Run("拡張なし非キーフレーム", func(t *testing.T) {
		// X=0, S=1 (開始パーティション)
		// 2バイト目: 非キーフレーム (bit0=1)
		payload := []byte{0x10, 0x01}
		vp8 := &VP8{}

		err := vp8.Unmarshal(payload)

		require.NoError(t, err)
		assert.False(t, vp8.IsKeyFrame)
	})

	t.Run("拡張なしS=0は非キーフレーム", func(t *testing.T) {
		// X=0, S=0 (継続パーティション)
		// 2バイト目: キーフレームビットは0だがS=0なので非キーフレーム
		payload := []byte{0x00, 0x00}
		vp8 := &VP8{}

		err := vp8.Unmarshal(payload)

		require.NoError(t, err)
		assert.False(t, vp8.IsKeyFrame)
	})

	t.Run("拡張ありPictureID（7ビット）", func(t *testing.T) {
		// X=1, S=1
		// 拡張バイト: I=1 (PictureID present)
		// PictureID: 0x55 (M=0, 7ビット)
		payload := []byte{0x90, 0x80, 0x55, 0x00}
		vp8 := &VP8{}

		err := vp8.Unmarshal(payload)

		require.NoError(t, err)
		assert.Equal(t, uint16(0x55), vp8.PictureID)
		assert.False(t, vp8.MBit)
		assert.True(t, vp8.IsKeyFrame)
	})

	t.Run("拡張ありPictureID（15ビット）", func(t *testing.T) {
		// X=1, S=1
		// 拡張バイト: I=1 (PictureID present)
		// PictureID: M=1, 15ビット (0x1234)
		payload := []byte{0x90, 0x80, 0x92, 0x34, 0x00}
		vp8 := &VP8{}

		err := vp8.Unmarshal(payload)

		require.NoError(t, err)
		assert.Equal(t, uint16(0x1234), vp8.PictureID)
		assert.True(t, vp8.MBit)
		assert.True(t, vp8.IsKeyFrame)
	})

	t.Run("拡張ありTL0PICIDX", func(t *testing.T) {
		// X=1, S=1
		// 拡張バイト: L=1 (TL0PICIDX present)
		// TL0PICIDX: 0xAB
		payload := []byte{0x90, 0x40, 0xAB, 0x00}
		vp8 := &VP8{}

		err := vp8.Unmarshal(payload)

		require.NoError(t, err)
		assert.Equal(t, uint8(0xAB), vp8.TL0PICIDX)
		assert.True(t, vp8.IsKeyFrame)
	})

	t.Run("拡張ありTID", func(t *testing.T) {
		// X=1, S=1
		// 拡張バイト: T=1 (TID present)
		// TID: 2 (上位2ビット = 0x80)
		payload := []byte{0x90, 0x20, 0x80, 0x00}
		vp8 := &VP8{}

		err := vp8.Unmarshal(payload)

		require.NoError(t, err)
		assert.Equal(t, uint8(2), vp8.TID)
		assert.True(t, vp8.TemporalSupported)
		assert.True(t, vp8.IsKeyFrame)
	})

	t.Run("拡張ありK=1でTID", func(t *testing.T) {
		// X=1, S=1
		// 拡張バイト: K=1 (TID/Y/KEYIDX present)
		// TID: 1 (上位2ビット = 0x40)
		payload := []byte{0x90, 0x10, 0x40, 0x00}
		vp8 := &VP8{}

		err := vp8.Unmarshal(payload)

		require.NoError(t, err)
		assert.Equal(t, uint8(1), vp8.TID)
		assert.True(t, vp8.IsKeyFrame)
	})

	t.Run("全拡張あり", func(t *testing.T) {
		// X=1, S=1
		// 拡張バイト: I=1, L=1, T=1
		// PictureID: 0x55 (7ビット)
		// TL0PICIDX: 0xAB
		// TID: 2
		payload := []byte{0x90, 0xE0, 0x55, 0xAB, 0x80, 0x00}
		vp8 := &VP8{}

		err := vp8.Unmarshal(payload)

		require.NoError(t, err)
		assert.Equal(t, uint16(0x55), vp8.PictureID)
		assert.Equal(t, uint8(0xAB), vp8.TL0PICIDX)
		assert.Equal(t, uint8(2), vp8.TID)
		assert.True(t, vp8.TemporalSupported)
		assert.True(t, vp8.IsKeyFrame)
	})

	t.Run("ペイロードが短すぎる（拡張あり）", func(t *testing.T) {
		// X=1だが拡張バイトがない
		payload := []byte{0x80}
		vp8 := &VP8{}

		err := vp8.Unmarshal(payload)

		assert.Equal(t, errShortPacket, err)
	})

	t.Run("ペイロードが短すぎる（PictureID）", func(t *testing.T) {
		// X=1, I=1だがPictureIDがない
		payload := []byte{0x80, 0x80}
		vp8 := &VP8{}

		err := vp8.Unmarshal(payload)

		assert.Equal(t, errShortPacket, err)
	})
}

func TestIsH264Keyframe(t *testing.T) {
	t.Run("空のペイロード", func(t *testing.T) {
		result := isH264Keyframe([]byte{})
		assert.False(t, result)
	})

	t.Run("NALU type 0は非キーフレーム", func(t *testing.T) {
		payload := []byte{0x00}
		result := isH264Keyframe(payload)
		assert.False(t, result)
	})

	t.Run("NALU type 5（IDR）はキーフレーム", func(t *testing.T) {
		payload := []byte{0x05}
		result := isH264Keyframe(payload)
		assert.True(t, result)
	})

	t.Run("NALU type 1（非IDR）は非キーフレーム", func(t *testing.T) {
		payload := []byte{0x01}
		result := isH264Keyframe(payload)
		assert.False(t, result)
	})

	t.Run("NALU type 7（SPS）は非キーフレーム", func(t *testing.T) {
		// 単独のSPSは非キーフレーム（STAP-A内のSPSは別）
		payload := []byte{0x07}
		result := isH264Keyframe(payload)
		assert.False(t, result)
	})

	t.Run("STAP-A内にSPS（type 7）があればキーフレーム", func(t *testing.T) {
		// NALU type 24 (STAP-A)
		// NALUサイズ(2バイト) + NALU(type 7)
		payload := []byte{
			0x18,       // STAP-A (type 24)
			0x00, 0x02, // 長さ 2
			0x07, 0x00, // NALU type 7 (SPS)
		}
		result := isH264Keyframe(payload)
		assert.True(t, result)
	})

	t.Run("STAP-A内にSPSがなければ非キーフレーム", func(t *testing.T) {
		// NALU type 24 (STAP-A)
		// NALUサイズ(2バイト) + NALU(type 1)
		payload := []byte{
			0x18,       // STAP-A (type 24)
			0x00, 0x02, // 長さ 2
			0x01, 0x00, // NALU type 1 (非IDR)
		}
		result := isH264Keyframe(payload)
		assert.False(t, result)
	})

	t.Run("STAP-A複数NALU、2番目にSPS", func(t *testing.T) {
		payload := []byte{
			0x18,       // STAP-A (type 24)
			0x00, 0x02, // 長さ 2
			0x01, 0x00, // NALU type 1
			0x00, 0x02, // 長さ 2
			0x07, 0x00, // NALU type 7 (SPS)
		}
		result := isH264Keyframe(payload)
		assert.True(t, result)
	})

	t.Run("FU-A開始フラグメント、type 7はキーフレーム", func(t *testing.T) {
		// NALU type 28 (FU-A)
		// FU header: S=1 (開始), type=7
		payload := []byte{
			0x1C,        // FU-A (type 28)
			0x80 | 0x07, // S=1, type=7
		}
		result := isH264Keyframe(payload)
		assert.True(t, result)
	})

	t.Run("FU-A開始フラグメント、type 5は非キーフレーム", func(t *testing.T) {
		// FU-Aでtype 7のみがキーフレーム
		payload := []byte{
			0x1C,        // FU-A (type 28)
			0x80 | 0x05, // S=1, type=5
		}
		result := isH264Keyframe(payload)
		assert.False(t, result)
	})

	t.Run("FU-A継続フラグメントは非キーフレーム", func(t *testing.T) {
		// S=0 (継続フラグメント)
		payload := []byte{
			0x1C, // FU-A (type 28)
			0x07, // S=0, type=7
		}
		result := isH264Keyframe(payload)
		assert.False(t, result)
	})

	t.Run("FU-B開始フラグメント、type 7はキーフレーム", func(t *testing.T) {
		// NALU type 29 (FU-B)
		payload := []byte{
			0x1D,        // FU-B (type 29)
			0x80 | 0x07, // S=1, type=7
		}
		result := isH264Keyframe(payload)
		assert.True(t, result)
	})

	t.Run("不正なNALU type（30以上）は非キーフレーム", func(t *testing.T) {
		payload := []byte{0x1E} // type 30
		result := isH264Keyframe(payload)
		assert.False(t, result)
	})

	t.Run("STAP-Bの処理", func(t *testing.T) {
		// NALU type 25 (STAP-B) - DON(2バイト)をスキップ
		payload := []byte{
			0x19,       // STAP-B (type 25)
			0x00, 0x00, // DON
			0x00, 0x02, // 長さ 2
			0x07, 0x00, // NALU type 7 (SPS)
		}
		result := isH264Keyframe(payload)
		assert.True(t, result)
	})

	t.Run("MTAP16の処理", func(t *testing.T) {
		// NALU type 26 (MTAP16) - DON(2バイト)をスキップ、各NALUにオフセット(3バイト)
		payload := []byte{
			0x1A,       // MTAP16 (type 26)
			0x00, 0x00, // DON
			0x00, 0x05, // 長さ 5 (offset 3 + NALU 2)
			0x00, 0x00, 0x00, // TS offset
			0x07, 0x00, // NALU type 7 (SPS)
		}
		result := isH264Keyframe(payload)
		assert.True(t, result)
	})

	t.Run("MTAP24の処理", func(t *testing.T) {
		// NALU type 27 (MTAP24) - DON(2バイト)をスキップ、各NALUにオフセット(4バイト)
		payload := []byte{
			0x1B,       // MTAP24 (type 27)
			0x00, 0x00, // DON
			0x00, 0x06, // 長さ 6 (offset 4 + NALU 2)
			0x00, 0x00, 0x00, 0x00, // TS offset
			0x07, 0x00, // NALU type 7 (SPS)
		}
		result := isH264Keyframe(payload)
		assert.True(t, result)
	})

	t.Run("STAP-A長さが不正", func(t *testing.T) {
		payload := []byte{
			0x18, // STAP-A (type 24)
			0x00, // 長さ不完全
		}
		result := isH264Keyframe(payload)
		assert.False(t, result)
	})

	t.Run("STAP-A NALUがバッファを超える", func(t *testing.T) {
		payload := []byte{
			0x18,       // STAP-A (type 24)
			0x00, 0xFF, // 長さ 255 (バッファを超える)
			0x07,
		}
		result := isH264Keyframe(payload)
		assert.False(t, result)
	})

	t.Run("FU-Aペイロードが短すぎる", func(t *testing.T) {
		payload := []byte{0x1C} // FU-Aだが2バイト目がない
		result := isH264Keyframe(payload)
		assert.False(t, result)
	})
}

func TestReadSTAPLength(t *testing.T) {
	t.Run("正常な長さ読み取り", func(t *testing.T) {
		payload := []byte{0x01, 0x02, 0x03, 0x04}
		length, ok := readSTAPLength(payload, 1)

		assert.True(t, ok)
		assert.Equal(t, uint16(0x0203), length)
	})

	t.Run("バッファ不足", func(t *testing.T) {
		payload := []byte{0x01, 0x02}
		length, ok := readSTAPLength(payload, 1)

		assert.False(t, ok)
		assert.Equal(t, uint16(0), length)
	})
}

func TestGetSTAPOffset(t *testing.T) {
	tests := []struct {
		nalu     byte
		expected int
	}{
		{24, 0}, // STAP-A
		{25, 0}, // STAP-B (DONは別途スキップ)
		{26, 3}, // MTAP16
		{27, 4}, // MTAP24
		{28, 0}, // FU-A (default)
	}

	for _, tt := range tests {
		result := getSTAPOffset(tt.nalu)
		assert.Equal(t, tt.expected, result, "NALU type %d", tt.nalu)
	}
}

func TestIsKeyframeNALU(t *testing.T) {
	t.Run("offsetがlengthを超える", func(t *testing.T) {
		// MTAP24でoffset=4だが、length=2
		payload := []byte{0x00, 0x00, 0x07}
		result := isKeyframeNALU(payload, 0, 27, 2)
		assert.False(t, result)
	})

	t.Run("NALU type 7はキーフレーム", func(t *testing.T) {
		payload := []byte{0x07, 0x00, 0x00, 0x00, 0x00}
		result := isKeyframeNALU(payload, 0, 24, 5)
		assert.True(t, result)
	})

	t.Run("NALU type 1は非キーフレーム", func(t *testing.T) {
		payload := []byte{0x01, 0x00, 0x00, 0x00, 0x00}
		result := isKeyframeNALU(payload, 0, 24, 5)
		assert.False(t, result)
	})
}

func TestVP8_parseTL0PICIDX_ShortPacket(t *testing.T) {
	// TL0PICIDXを読み取ろうとするが、バッファが足りない
	payload := []byte{0x90, 0x40} // X=1, L=1だがTL0PICIDXがない
	vp8 := &VP8{}

	err := vp8.Unmarshal(payload)

	assert.Equal(t, errShortPacket, err)
}

func TestVP8_parseTID_ShortPacket(t *testing.T) {
	// TIDを読み取ろうとするが、バッファが足りない
	payload := []byte{0x90, 0x20} // X=1, T=1だがTIDがない
	vp8 := &VP8{}

	err := vp8.Unmarshal(payload)

	assert.Equal(t, errShortPacket, err)
}

func TestVP8_parseSimple_ShortPacket(t *testing.T) {
	// 拡張なしだが2バイト目がない
	payload := []byte{0x10} // X=0, S=1だがペイロードバイトがない
	vp8 := &VP8{}

	err := vp8.Unmarshal(payload)

	assert.Equal(t, errShortPacket, err)
}

func TestVP8_parseExtended_ShortAtEnd(t *testing.T) {
	// 拡張ありで全部パースできるが、最後のペイロードバイトがない
	payload := []byte{0x90, 0x80, 0x55} // X=1, I=1, PictureID=0x55だが最後のバイトがない
	vp8 := &VP8{}

	err := vp8.Unmarshal(payload)

	assert.Equal(t, errShortPacket, err)
}

func TestVp8Reader(t *testing.T) {
	t.Run("advance正常", func(t *testing.T) {
		r := &vp8Reader{payload: []byte{0x01, 0x02, 0x03}, len: 3, idx: 0}

		err := r.advance()

		assert.NoError(t, err)
		assert.Equal(t, 1, r.idx)
	})

	t.Run("advanceでバッファ終端", func(t *testing.T) {
		r := &vp8Reader{payload: []byte{0x01, 0x02}, len: 2, idx: 1}

		err := r.advance()

		assert.Equal(t, errShortPacket, err)
	})

	t.Run("current", func(t *testing.T) {
		r := &vp8Reader{payload: []byte{0xAB, 0xCD}, len: 2, idx: 1}

		result := r.current()

		assert.Equal(t, byte(0xCD), result)
	})
}
