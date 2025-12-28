package retry

import (
	"io"
	"math/rand"
	"time"
)

// Action はリトライループで取るべきアクションを表す
type Action int

const (
	Abort    Action = iota // 処理を中止
	Wait                   // 待機して再試行
	Execute                // 処理を実行
)

// Config はリトライの設定を保持する
type Config struct {
	Attempts    int
	BaseInterval time.Duration
	MaxBackoff  time.Duration
}

// DefaultConfig はデフォルトのリトライ設定を返す
func DefaultConfig() Config {
	return Config{
		Attempts:     6,
		BaseInterval: 20 * time.Millisecond,
		MaxBackoff:   500 * time.Millisecond,
	}
}

// Backoff は指数バックオフ + ジッターを計算する
func Backoff(attempt int, baseInterval, maxBackoff time.Duration) time.Duration {
	d := baseInterval << attempt
	if d > maxBackoff {
		d = maxBackoff
	}
	// +/-10% jitter
	jitter := time.Duration(int64(d) * int64(9+rand.Intn(3)) / 10)
	return jitter
}

// ShouldRetry はエラーに基づいてリトライすべきか判定する
func ShouldRetry(err error) bool {
	if err == nil {
		return false
	}
	if err == io.EOF || err == io.ErrClosedPipe {
		return false
	}
	return true
}

// Executor はリトライ可能な処理を実行するインターフェース
type Executor interface {
	// DetermineAction は次に取るべきアクションを決定する
	DetermineAction() Action
	// Execute は処理を実行し、成功またはリトライ不可の場合trueを返す
	Execute(attempt int) bool
}

// Run はExecutorを使用してリトライループを実行する
func Run(cfg Config, executor Executor) {
	for i := 0; i < cfg.Attempts; i++ {
		switch executor.DetermineAction() {
		case Abort:
			return
		case Wait:
			time.Sleep(Backoff(i, cfg.BaseInterval, cfg.MaxBackoff))
			continue
		case Execute:
			if executor.Execute(i) {
				return
			}
		}
	}
}
