package sfu

// ReceiverはPublisherから着信したRTPストリームを管理するための抽象化された構造体です。
// 受信したメディアはDowntrackに分配され、Subscriberに送信されます。
// ReceiverとDownTrackは1対多の関係です。
type Receiver struct {
}

func NewReceiver() *Receiver {
	return &Receiver{}
}
