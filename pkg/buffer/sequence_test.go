package buffer

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsSequenceNumberLater(t *testing.T) {
	tests := []struct {
		name     string
		sn1      uint16
		sn2      uint16
		expected bool
	}{
		{
			name:     "sn1がsn2より大きい（通常）",
			sn1:      100,
			sn2:      50,
			expected: true,
		},
		{
			name:     "sn1がsn2より小さい（通常）",
			sn1:      50,
			sn2:      100,
			expected: false,
		},
		{
			name:     "sn1とsn2が同じ",
			sn1:      100,
			sn2:      100,
			expected: true, // 差が0なので0x8000ビットは立たない
		},
		{
			name:     "ラップアラウンド: sn1が0付近、sn2が65535付近",
			sn1:      10,
			sn2:      65530,
			expected: true, // 10は65530より後（ラップアラウンド考慮）
		},
		{
			name:     "ラップアラウンド: sn1が65535付近、sn2が0付近",
			sn1:      65530,
			sn2:      10,
			expected: false, // 65530は10より前（ラップアラウンド考慮）
		},
		{
			name:     "境界: 差がちょうど32768",
			sn1:      32768,
			sn2:      0,
			expected: false, // 差が0x8000でビットが立つ
		},
		{
			name:     "境界: 差が32767",
			sn1:      32767,
			sn2:      0,
			expected: true, // 差が0x7FFFでビットは立たない
		},
		{
			name:     "境界: 差が32769",
			sn1:      32769,
			sn2:      0,
			expected: false,
		},
		{
			name:     "0と0",
			sn1:      0,
			sn2:      0,
			expected: true,
		},
		{
			name:     "最大値同士",
			sn1:      65535,
			sn2:      65535,
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isSequenceNumberLater(tt.sn1, tt.sn2)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsSequenceNumberEarlier(t *testing.T) {
	tests := []struct {
		name     string
		sn1      uint16
		sn2      uint16
		expected bool
	}{
		{
			name:     "sn1がsn2より小さい（通常）",
			sn1:      50,
			sn2:      100,
			expected: true,
		},
		{
			name:     "sn1がsn2より大きい（通常）",
			sn1:      100,
			sn2:      50,
			expected: false,
		},
		{
			name:     "sn1とsn2が同じ",
			sn1:      100,
			sn2:      100,
			expected: false,
		},
		{
			name:     "ラップアラウンド: sn1が65535付近、sn2が0付近",
			sn1:      65530,
			sn2:      10,
			expected: true, // 65530は10より前
		},
		{
			name:     "ラップアラウンド: sn1が0付近、sn2が65535付近",
			sn1:      10,
			sn2:      65530,
			expected: false, // 10は65530より後
		},
		{
			name:     "境界: 差がちょうど32768",
			sn1:      0,
			sn2:      32768,
			expected: true,
		},
		{
			name:     "境界: 差が32767",
			sn1:      0,
			sn2:      32767,
			expected: true, // 0 - 32767 = 32769 (uint16) → 0x8000ビットあり
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isSequenceNumberEarlier(tt.sn1, tt.sn2)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsCrossingWrapAroundBoundary(t *testing.T) {
	tests := []struct {
		name     string
		sn1      uint16
		sn2      uint16
		expected bool
	}{
		{
			name:     "境界をまたぐ: sn1が大きく上位ビットあり、sn2が小さく上位ビットなし",
			sn1:      0x8001, // 32769
			sn2:      0x7FFF, // 32767
			expected: true,
		},
		{
			name:     "境界をまたぐ: sn1=65535、sn2=0",
			sn1:      65535,
			sn2:      0,
			expected: true,
		},
		{
			name:     "境界をまたがない: 両方上位ビットあり",
			sn1:      0x9000,
			sn2:      0x8000,
			expected: false,
		},
		{
			name:     "境界をまたがない: 両方上位ビットなし",
			sn1:      0x7000,
			sn2:      0x6000,
			expected: false,
		},
		{
			name:     "境界をまたがない: sn1 < sn2",
			sn1:      100,
			sn2:      200,
			expected: false,
		},
		{
			name:     "境界をまたがない: sn1=sn2",
			sn1:      0x8000,
			sn2:      0x8000,
			expected: false,
		},
		{
			name:     "境界をまたぐ: sn1=0x8000、sn2=0x7FFF",
			sn1:      0x8000, // 32768
			sn2:      0x7FFF, // 32767
			expected: true,
		},
		{
			name:     "境界をまたがない: sn1が上位ビットなし、sn2が上位ビットあり",
			sn1:      0x7FFF,
			sn2:      0x8000,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isCrossingWrapAroundBoundary(tt.sn1, tt.sn2)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSequenceNumberComparison_Symmetry(t *testing.T) {
	// isSequenceNumberLaterとisSequenceNumberEarlierは対称的であるべき
	// ただし、同じ値の場合は両方falseにはならない

	testCases := []struct {
		sn1 uint16
		sn2 uint16
	}{
		{100, 200},
		{200, 100},
		{0, 65535},
		{65535, 0},
		{32767, 32768},
		{32768, 32767},
	}

	for _, tc := range testCases {
		later := isSequenceNumberLater(tc.sn1, tc.sn2)
		earlier := isSequenceNumberEarlier(tc.sn1, tc.sn2)

		// sn1 != sn2の場合、どちらか一方のみtrue
		if tc.sn1 != tc.sn2 {
			assert.NotEqual(t, later, earlier,
				"sn1=%d, sn2=%d: laterとearlierは排他的であるべき", tc.sn1, tc.sn2)
		}
	}
}

func TestSequenceNumberComparison_RFC3550(t *testing.T) {
	// RFC 3550 Appendix A.1の仕様に基づくテスト
	// シーケンス番号の比較は符号なし算術で行い、
	// 差の最上位ビットで前後を判定する

	t.Run("連続したシーケンス番号", func(t *testing.T) {
		for i := uint16(0); i < 1000; i++ {
			assert.True(t, isSequenceNumberLater(i+1, i),
				"i+1はiより後であるべき: i=%d", i)
			assert.True(t, isSequenceNumberEarlier(i, i+1),
				"iはi+1より前であるべき: i=%d", i)
		}
	})

	t.Run("ラップアラウンド付近", func(t *testing.T) {
		// 65534, 65535, 0, 1, 2 の順序
		sequence := []uint16{65534, 65535, 0, 1, 2}
		for i := 0; i < len(sequence)-1; i++ {
			assert.True(t, isSequenceNumberLater(sequence[i+1], sequence[i]),
				"%dは%dより後であるべき", sequence[i+1], sequence[i])
		}
	})
}
