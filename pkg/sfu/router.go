package sfu

// RouterはReceiverから受信したメディアを適切なDowntrackにルーティングするための抽象化された構造体です。
type Router struct {
}

func NewRouter() *Router {
	return &Router{}
}
