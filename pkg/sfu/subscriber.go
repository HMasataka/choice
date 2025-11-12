package sfu

// SubscriberはDownTrackから受信したメディアをクライアントに送信するための抽象化された構造体です。
// Subscriberはクライアントと1対1の関係にあります。
type Subscriber struct {
}

func NewSubscriber() *Subscriber {
	return &Subscriber{}
}
