package sfu

// DownTrackはSubscriberにメディアを送信するための抽象化された構造体です。
// DownTrackはReceiverから受信したメディアをSubscriberに配信します。
// SubscriberとDownTrackは1対多の関係です。
type DownTrack struct {
}

func NewDownTrack() *DownTrack {
	return &DownTrack{}
}
