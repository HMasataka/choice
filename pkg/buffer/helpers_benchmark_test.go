package buffer

import (
	"testing"
)

// VP8 ペイロードのテストデータ
var (
	// シンプルなVP8ペイロード（拡張なし）
	vp8PayloadSimple = []byte{0x10, 0x00, 0x01, 0x02, 0x03}

	// 拡張付きVP8ペイロード（PictureID, TL0PICIDX, TID）
	vp8PayloadExtended = []byte{
		0x90,       // X=1, S=1
		0xE0,       // I=1, L=1, T=1
		0x81, 0x23, // PictureID (M=1, 15-bit)
		0x45,       // TL0PICIDX
		0x80,       // TID
		0x00,       // キーフレームビット
	}

	// キーフレームVP8ペイロード
	vp8PayloadKeyframe = []byte{0x10, 0x00}

	// 非キーフレームVP8ペイロード
	vp8PayloadNonKeyframe = []byte{0x10, 0x01}
)

// H.264 ペイロードのテストデータ
var (
	// 単一NALUキーフレーム (IDR, type=5)
	h264SingleNALUKeyframe = []byte{0x05, 0x00, 0x01, 0x02}

	// 単一NALU非キーフレーム (non-IDR, type=1)
	h264SingleNALUNonKeyframe = []byte{0x01, 0x00, 0x01, 0x02}

	// STAP-A (type=24) キーフレーム含む
	h264STAPAKeyframe = []byte{
		0x18,       // STAP-A (24)
		0x00, 0x04, // length
		0x07, 0x00, 0x01, 0x02, // SPS (type=7)
		0x00, 0x03, // length
		0x08, 0x00, 0x01, // PPS (type=8)
	}

	// STAP-A (type=24) 非キーフレーム
	h264STAPANonKeyframe = []byte{
		0x18,       // STAP-A (24)
		0x00, 0x04, // length
		0x01, 0x00, 0x01, 0x02, // non-IDR (type=1)
	}

	// FU-A (type=28) キーフレーム開始
	h264FUAKeyframeStart = []byte{0x1C, 0x87, 0x00, 0x01}

	// FU-A (type=28) 非キーフレーム
	h264FUANonKeyframe = []byte{0x1C, 0x81, 0x00, 0x01}
)

// ベンチマーク: VP8.Unmarshal (シンプル)
func BenchmarkVP8UnmarshalSimple(b *testing.B) {
	var vp8 VP8
	b.ResetTimer()
	for b.Loop() {
		_ = vp8.Unmarshal(vp8PayloadSimple)
	}
}

// ベンチマーク: VP8.Unmarshal (拡張付き)
func BenchmarkVP8UnmarshalExtended(b *testing.B) {
	var vp8 VP8
	b.ResetTimer()
	for b.Loop() {
		_ = vp8.Unmarshal(vp8PayloadExtended)
	}
}

// ベンチマーク: VP8.Unmarshal (キーフレーム)
func BenchmarkVP8UnmarshalKeyframe(b *testing.B) {
	var vp8 VP8
	b.ResetTimer()
	for b.Loop() {
		_ = vp8.Unmarshal(vp8PayloadKeyframe)
	}
}

// ベンチマーク: VP8.Unmarshal (並列)
func BenchmarkVP8UnmarshalParallel(b *testing.B) {
	b.RunParallel(func(pb *testing.PB) {
		var vp8 VP8
		for pb.Next() {
			_ = vp8.Unmarshal(vp8PayloadExtended)
		}
	})
}

// ベンチマーク: isH264Keyframe (単一NALUキーフレーム)
func BenchmarkIsH264KeyframeSingleNALU(b *testing.B) {
	b.ResetTimer()
	for b.Loop() {
		_ = isH264Keyframe(h264SingleNALUKeyframe)
	}
}

// ベンチマーク: isH264Keyframe (単一NALU非キーフレーム)
func BenchmarkIsH264KeyframeSingleNALUNonKeyframe(b *testing.B) {
	b.ResetTimer()
	for b.Loop() {
		_ = isH264Keyframe(h264SingleNALUNonKeyframe)
	}
}

// ベンチマーク: isH264Keyframe (STAP-A キーフレーム)
func BenchmarkIsH264KeyframeSTAPA(b *testing.B) {
	b.ResetTimer()
	for b.Loop() {
		_ = isH264Keyframe(h264STAPAKeyframe)
	}
}

// ベンチマーク: isH264Keyframe (STAP-A 非キーフレーム)
func BenchmarkIsH264KeyframeSTAPANonKeyframe(b *testing.B) {
	b.ResetTimer()
	for b.Loop() {
		_ = isH264Keyframe(h264STAPANonKeyframe)
	}
}

// ベンチマーク: isH264Keyframe (FU-A キーフレーム開始)
func BenchmarkIsH264KeyframeFUA(b *testing.B) {
	b.ResetTimer()
	for b.Loop() {
		_ = isH264Keyframe(h264FUAKeyframeStart)
	}
}

// ベンチマーク: isH264Keyframe (FU-A 非キーフレーム)
func BenchmarkIsH264KeyframeFUANonKeyframe(b *testing.B) {
	b.ResetTimer()
	for b.Loop() {
		_ = isH264Keyframe(h264FUANonKeyframe)
	}
}

// ベンチマーク: isH264Keyframe (並列)
func BenchmarkIsH264KeyframeParallel(b *testing.B) {
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_ = isH264Keyframe(h264STAPAKeyframe)
		}
	})
}

// ベンチマーク: atomicBool set/get
func BenchmarkAtomicBoolSetGet(b *testing.B) {
	var ab atomicBool
	b.ResetTimer()
	for i := 0; b.Loop(); i++ {
		ab.set(i%2 == 0)
		_ = ab.get()
	}
}

// ベンチマーク: atomicBool set/get (並列)
func BenchmarkAtomicBoolSetGetParallel(b *testing.B) {
	var ab atomicBool
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			ab.set(i%2 == 0)
			_ = ab.get()
			i++
		}
	})
}
