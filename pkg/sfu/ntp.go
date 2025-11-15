package sfu

import "time"

// ntpEpoch はNTPエポックの開始点を定義します。
// NTPタイムスタンプは1900年1月1日 00:00:00 UTCから計数を開始します。
// これはUnixエポック（1970年1月1日 00:00:00 UTC）とは異なります。
// すべてのNTP時刻変換では、この70年の差を考慮する必要があります。
var ntpEpoch = time.Date(1900, 1, 1, 0, 0, 0, 0, time.UTC)

// ntpTime はNTP（Network Time Protocol）タイムスタンプ形式を表現します。
// NTPタイムスタンプは、WebRTCのRTCP Sender Reportで時刻同期に使用される64ビット値です。
//
// 形式:
//   - 上位32ビット: NTPエポック（1900年1月1日 00:00:00 UTC）からの経過秒数
//   - 下位32ビット: 秒の小数部分（1/2^32 秒単位）
//
// この形式により、マイクロ秒以下の分解能で正確な時刻表現が可能になり、
// メディアストリーミングの同期やネットワーク遅延計算に不可欠です。
type ntpTime uint64

// Duration はNTPタイムスタンプをNTPエポックからのtime.Durationに変換します。
// このメソッドは64ビットNTP形式を以下の手順で処理します:
//  1. 上位32ビットから秒を抽出し、ナノ秒に変換
//  2. 下位32ビットから小数部分を抽出
//  3. 小数部分を適切に四捨五入してナノ秒に変換
func (t ntpTime) Duration() time.Duration {
	sec := (t >> 32) * 1e9          // 上位32ビット（秒）をナノ秒に変換
	frac := (t & 0xffffffff) * 1e9  // 下位32ビット（小数部）を取得
	nsec := frac >> 32              // 小数部をナノ秒に変換
	if uint32(frac) >= 0x80000000 { // 0.5以上なら四捨五入
		nsec++
	}
	return time.Duration(sec + nsec)
}

// Time はNTPタイムスタンプをGoの標準time.Timeに変換します。
// NTPエポックは1900-01-01から開始し、Goの時刻エポックは1970-01-01から開始するため、
// このメソッドは計算された時間間隔をNTPエポックに加算します。
func (t ntpTime) Time() time.Time {
	return ntpEpoch.Add(t.Duration())
}

// toNtpTime はGoのtime.TimeをNTPタイムスタンプ形式に変換します。
// この関数は以下の処理を実行します:
//  1. NTPエポック（1900-01-01）からのナノ秒を計算
//  2. 秒部分と小数部分に分離
//  3. 64ビット形式にパック: 上位32ビット（秒）+ 下位32ビット（小数部）
//  4. 小数部に適切な四捨五入を適用
func toNtpTime(t time.Time) ntpTime {
	nsec := uint64(t.Sub(ntpEpoch)) // NTPエポックからの経過ナノ秒
	sec := nsec / 1e9               // 秒部分を抽出
	nsec = (nsec - sec*1e9) << 32   // 残りの小数部を上位ビットにシフト
	frac := nsec / 1e9              // 小数部を32ビット形式に変換
	if nsec%1e9 >= 1e9/2 {          // 0.5ナノ秒以上なら四捨五入
		frac++
	}

	return ntpTime(sec<<32 | frac) // 上位32ビット（秒）+ 下位32ビット（小数部）
}
