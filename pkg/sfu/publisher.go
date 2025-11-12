package sfu

// Publisherはclientがメディアを送信するための抽象化された構造体です。
// ClientとPublisherは1対1の関係にあり、ClientはPublisherを使用してメディアストリームをsfuに送信します。
type Publisher struct {
}

func NewPublisher() *Publisher {
	return &Publisher{}
}
