package buffer

// isSequenceNumberLater はsn1がsn2より後かを判定する（RFC 3550 Appendix A.1）
func isSequenceNumberLater(sn1, sn2 uint16) bool {
	return (sn1-sn2)&0x8000 == 0
}

// isSequenceNumberEarlier はsn1がsn2より前かを判定する
func isSequenceNumberEarlier(sn1, sn2 uint16) bool {
	return (sn1-sn2)&0x8000 != 0
}

// isCrossingWrapAroundBoundary はラップアラウンド境界をまたいでいるかを判定する
func isCrossingWrapAroundBoundary(sn1, sn2 uint16) bool {
	return sn1 > sn2 && sn1&0x8000 > 0 && sn2&0x8000 == 0
}
