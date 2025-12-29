package sfu

// export_test.go はテスト用のエクスポートヘルパーを提供します
// このファイルは _test.go で終わるため、テスト時のみビルドされます

// ErrNoReceiverFoundForTest はテスト用にerrNoReceiverFoundを公開します
var ErrNoReceiverFoundForTest = errNoReceiverFound

// エクスポートされた定数
const (
	SimpleDownTrackForTest    = SimpleDownTrack
	SimulcastDownTrackForTest = SimulcastDownTrack
)
