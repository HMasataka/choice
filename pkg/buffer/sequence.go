package buffer

// isSequenceNumberLater はsn1がsn2より後のシーケンス番号かどうかを判定します。
// RFC 3550 Appendix A.1のシーケンス番号比較アルゴリズムに基づく。
//
// 16ビット符号なし整数の引き算では負の数を表現できないため、
// ラップアラウンド（65535→0）を考慮した比較が必要。
//
// アルゴリズム:
//
//	diff = sn1 - sn2 (uint16)
//	diff & 0x8000 == 0 → ビット15が0 → 正の差分 → sn1はsn2より後
//	diff & 0x8000 != 0 → ビット15が1 → 負の差分 → sn1はsn2より前
//
// 例:
//
//	sn1=100, sn2=50  → diff=50    (0x0032, ビット15=0) → true (後)
//	sn1=50,  sn2=100 → diff=65486 (0xFFCE, ビット15=1) → false (前)
//	sn1=10,  sn2=65530 → diff=16  (0x0010, ビット15=0) → true (ラップアラウンド後)
func isSequenceNumberLater(sn1, sn2 uint16) bool {
	return (sn1-sn2)&0x8000 == 0
}

// isSequenceNumberEarlier はsn1がsn2より前のシーケンス番号かどうかを判定します。
// RFC 3550 Appendix A.1のシーケンス番号比較アルゴリズムに基づく。
//
// isSequenceNumberLaterの逆の判定を行います。
// ビット15が1の場合、sn1はsn2より前（順序が乱れている）。
func isSequenceNumberEarlier(sn1, sn2 uint16) bool {
	return (sn1-sn2)&0x8000 != 0
}

// isCrossingWrapAroundBoundary は2つのシーケンス番号がラップアラウンド境界をまたいでいるかを判定します。
// RFC 3550のシーケンス番号ラップアラウンド処理に基づく。
//
// ラップアラウンド境界:
//
//	シーケンス番号は16ビット（0～65535）で、65535の次は0に戻る。
//	この境界をまたぐ場合、特別な処理が必要。
//
// 判定条件:
//
//   - sn1 > sn2: 単純な数値比較では大きい
//   - sn1 & 0x8000 > 0: sn1のビット15が1（32768以上の値）
//   - sn2 & 0x8000 == 0: sn2のビット15が0（32768未満の値）
//
// 例:
//
//	sn1=50000 (0xC350, ビット15=1), sn2=100 (0x0064, ビット15=0)
//	→ 境界をまたいでいる（sn1は実際にはsn2より前のサイクル）
//
//	sn1=100 (0x0064, ビット15=0), sn2=200 (0x00C8, ビット15=0)
//	→ 境界をまたいでいない（同じサイクル内）
func isCrossingWrapAroundBoundary(sn1, sn2 uint16) bool {
	return sn1 > sn2 && sn1&0x8000 > 0 && sn2&0x8000 == 0
}
