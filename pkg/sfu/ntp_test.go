package sfu

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNtpEpoch(t *testing.T) {
	// NTPエポックは1900年1月1日 00:00:00 UTC
	expected := time.Date(1900, 1, 1, 0, 0, 0, 0, time.UTC)
	assert.Equal(t, expected, ntpEpoch)
}

func TestNtpTime_Duration(t *testing.T) {
	tests := []struct {
		name     string
		ntp      ntpTime
		expected time.Duration
	}{
		{
			name:     "ゼロ",
			ntp:      0,
			expected: 0,
		},
		{
			name:     "1秒",
			ntp:      1 << 32, // 上位32ビットに1
			expected: time.Second,
		},
		{
			name:     "2秒",
			ntp:      2 << 32,
			expected: 2 * time.Second,
		},
		{
			name:     "0.5秒",
			ntp:      0x80000000, // 下位32ビットの半分
			expected: 500 * time.Millisecond,
		},
		{
			name:     "1.5秒",
			ntp:      (1 << 32) | 0x80000000,
			expected: 1500 * time.Millisecond,
		},
		{
			name:     "0.25秒",
			ntp:      0x40000000, // 1/4
			expected: 250 * time.Millisecond,
		},
		{
			name:     "0.75秒",
			ntp:      0xC0000000, // 3/4
			expected: 750 * time.Millisecond,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.ntp.Duration()
			// 小数部の変換で若干の誤差が生じる可能性があるため、許容範囲を設定
			assert.InDelta(t, float64(tt.expected), float64(result), float64(time.Microsecond))
		})
	}
}

func TestNtpTime_Time(t *testing.T) {
	tests := []struct {
		name     string
		ntp      ntpTime
		expected time.Time
	}{
		{
			name:     "NTPエポック",
			ntp:      0,
			expected: ntpEpoch,
		},
		{
			name:     "NTPエポック + 1秒",
			ntp:      1 << 32,
			expected: ntpEpoch.Add(time.Second),
		},
		{
			name:     "NTPエポック + 1日",
			ntp:      ntpTime(86400) << 32,
			expected: ntpEpoch.Add(24 * time.Hour),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.ntp.Time()
			assert.Equal(t, tt.expected.Unix(), result.Unix())
		})
	}
}

func TestToNtpTime(t *testing.T) {
	tests := []struct {
		name string
		time time.Time
	}{
		{
			name: "NTPエポック",
			time: ntpEpoch,
		},
		{
			name: "Unixエポック（1970-01-01）",
			time: time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			name: "2000年1月1日",
			time: time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			name: "2024年1月1日",
			time: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			name: "現在時刻",
			time: time.Now().UTC().Truncate(time.Millisecond),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ntp := toNtpTime(tt.time)
			result := ntp.Time()

			// マイクロ秒レベルの精度で比較
			diff := tt.time.Sub(result)
			if diff < 0 {
				diff = -diff
			}
			assert.Less(t, diff, time.Millisecond, "変換の誤差が大きすぎます")
		})
	}
}

func TestNtpTimeRoundTrip(t *testing.T) {
	// time.Time -> ntpTime -> time.Time の往復変換テスト
	testTimes := []time.Time{
		ntpEpoch,
		ntpEpoch.Add(time.Hour),
		ntpEpoch.Add(24 * time.Hour),
		time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2000, 1, 1, 12, 30, 45, 0, time.UTC),
		time.Date(2024, 6, 15, 10, 20, 30, 0, time.UTC),
	}

	for _, original := range testTimes {
		t.Run(original.Format(time.RFC3339), func(t *testing.T) {
			ntp := toNtpTime(original)
			converted := ntp.Time()

			// 秒単位で一致することを確認
			assert.Equal(t, original.Unix(), converted.Unix())
		})
	}
}

func TestNtpTimeFractionalSeconds(t *testing.T) {
	// 小数秒の精度テスト
	t.Run("500ミリ秒", func(t *testing.T) {
		original := ntpEpoch.Add(500 * time.Millisecond)
		ntp := toNtpTime(original)
		converted := ntp.Time()

		diff := original.Sub(converted)
		if diff < 0 {
			diff = -diff
		}
		assert.Less(t, diff, time.Millisecond)
	})

	t.Run("250ミリ秒", func(t *testing.T) {
		original := ntpEpoch.Add(250 * time.Millisecond)
		ntp := toNtpTime(original)
		converted := ntp.Time()

		diff := original.Sub(converted)
		if diff < 0 {
			diff = -diff
		}
		assert.Less(t, diff, time.Millisecond)
	})

	t.Run("123ミリ秒456マイクロ秒", func(t *testing.T) {
		original := ntpEpoch.Add(123*time.Millisecond + 456*time.Microsecond)
		ntp := toNtpTime(original)
		converted := ntp.Time()

		diff := original.Sub(converted)
		if diff < 0 {
			diff = -diff
		}
		assert.Less(t, diff, time.Millisecond)
	})
}

func TestNtpTimeDurationRounding(t *testing.T) {
	// 四捨五入のテスト（下位32ビットの0x80000000境界）
	t.Run("0.25秒は正確に変換される", func(t *testing.T) {
		// 0x40000000 は 0.25秒
		ntp := ntpTime(0x40000000)
		d := ntp.Duration()
		assert.InDelta(t, float64(250*time.Millisecond), float64(d), float64(time.Millisecond))
	})

	t.Run("0.5秒は正確に変換される", func(t *testing.T) {
		// 0x80000000 はちょうど0.5秒
		ntp := ntpTime(0x80000000)
		d := ntp.Duration()
		assert.InDelta(t, float64(500*time.Millisecond), float64(d), float64(time.Millisecond))
	})

	t.Run("0.75秒は正確に変換される", func(t *testing.T) {
		// 0xC0000000 は 0.75秒
		ntp := ntpTime(0xC0000000)
		d := ntp.Duration()
		assert.InDelta(t, float64(750*time.Millisecond), float64(d), float64(time.Millisecond))
	})
}
