package sfu

import (
	"context"
	"sync"
	"testing"

	"github.com/pion/webrtc/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDatachannel(t *testing.T) {
	t.Run("初期化", func(t *testing.T) {
		dc := &Datachannel{
			Label: "test-channel",
		}

		assert.Equal(t, "test-channel", dc.Label)
		assert.Nil(t, dc.middlewares)
		assert.Nil(t, dc.onMessage)
	})

	t.Run("Use - ミドルウェア追加", func(t *testing.T) {
		dc := &Datachannel{Label: "test"}

		mw1 := func(next MessageProcessor) MessageProcessor { return next }
		mw2 := func(next MessageProcessor) MessageProcessor { return next }

		dc.Use(mw1)
		assert.Len(t, dc.middlewares, 1)

		dc.Use(mw2)
		assert.Len(t, dc.middlewares, 2)
	})

	t.Run("Use - 複数ミドルウェアを一度に追加", func(t *testing.T) {
		dc := &Datachannel{Label: "test"}

		mw1 := func(next MessageProcessor) MessageProcessor { return next }
		mw2 := func(next MessageProcessor) MessageProcessor { return next }
		mw3 := func(next MessageProcessor) MessageProcessor { return next }

		dc.Use(mw1, mw2, mw3)
		assert.Len(t, dc.middlewares, 3)
	})

	t.Run("OnMessage - コールバック設定", func(t *testing.T) {
		dc := &Datachannel{Label: "test"}

		called := false
		dc.OnMessage(func(ctx context.Context, args ProcessArgs) {
			called = true
		})

		require.NotNil(t, dc.onMessage)

		// コールバックを呼び出し
		dc.onMessage(context.Background(), ProcessArgs{})
		assert.True(t, called)
	})
}

func TestProcessArgs(t *testing.T) {
	t.Run("構造体の初期化", func(t *testing.T) {
		args := ProcessArgs{
			Message: webrtc.DataChannelMessage{
				IsString: true,
				Data:     []byte("hello"),
			},
		}

		assert.True(t, args.Message.IsString)
		assert.Equal(t, []byte("hello"), args.Message.Data)
		assert.Nil(t, args.Peer)
		assert.Nil(t, args.DataChannel)
	})
}

func TestProcessFunc(t *testing.T) {
	t.Run("Process呼び出し", func(t *testing.T) {
		var receivedCtx context.Context
		var receivedArgs ProcessArgs

		pf := ProcessFunc(func(ctx context.Context, args ProcessArgs) {
			receivedCtx = ctx
			receivedArgs = args
		})

		ctx := context.WithValue(context.Background(), "key", "value")
		args := ProcessArgs{
			Message: webrtc.DataChannelMessage{Data: []byte("test")},
		}

		pf.Process(ctx, args)

		assert.Equal(t, "value", receivedCtx.Value("key"))
		assert.Equal(t, []byte("test"), receivedArgs.Message.Data)
	})
}

func TestChainHandler(t *testing.T) {
	t.Run("Process呼び出し", func(t *testing.T) {
		called := false
		processor := ProcessFunc(func(ctx context.Context, args ProcessArgs) {
			called = true
		})

		ch := &chainHandler{
			middlewares: nil,
			Last:        processor,
			current:     processor,
		}

		ch.Process(context.Background(), ProcessArgs{})

		assert.True(t, called)
	})
}

func TestMiddlewares(t *testing.T) {
	t.Run("空のミドルウェア", func(t *testing.T) {
		mws := Middlewares{}

		called := false
		final := ProcessFunc(func(ctx context.Context, args ProcessArgs) {
			called = true
		})

		processor := mws.Process(final)
		processor.Process(context.Background(), ProcessArgs{})

		assert.True(t, called)
	})

	t.Run("単一のミドルウェア", func(t *testing.T) {
		order := []string{}

		mw := func(next MessageProcessor) MessageProcessor {
			return ProcessFunc(func(ctx context.Context, args ProcessArgs) {
				order = append(order, "middleware")
				next.Process(ctx, args)
			})
		}

		mws := Middlewares{mw}

		final := ProcessFunc(func(ctx context.Context, args ProcessArgs) {
			order = append(order, "final")
		})

		processor := mws.Process(final)
		processor.Process(context.Background(), ProcessArgs{})

		assert.Equal(t, []string{"middleware", "final"}, order)
	})

	t.Run("複数のミドルウェア（実行順序）", func(t *testing.T) {
		order := []string{}

		mw1 := func(next MessageProcessor) MessageProcessor {
			return ProcessFunc(func(ctx context.Context, args ProcessArgs) {
				order = append(order, "mw1-before")
				next.Process(ctx, args)
				order = append(order, "mw1-after")
			})
		}

		mw2 := func(next MessageProcessor) MessageProcessor {
			return ProcessFunc(func(ctx context.Context, args ProcessArgs) {
				order = append(order, "mw2-before")
				next.Process(ctx, args)
				order = append(order, "mw2-after")
			})
		}

		mw3 := func(next MessageProcessor) MessageProcessor {
			return ProcessFunc(func(ctx context.Context, args ProcessArgs) {
				order = append(order, "mw3-before")
				next.Process(ctx, args)
				order = append(order, "mw3-after")
			})
		}

		mws := Middlewares{mw1, mw2, mw3}

		final := ProcessFunc(func(ctx context.Context, args ProcessArgs) {
			order = append(order, "final")
		})

		processor := mws.Process(final)
		processor.Process(context.Background(), ProcessArgs{})

		// ミドルウェアは先頭から順に実行される
		expected := []string{
			"mw1-before",
			"mw2-before",
			"mw3-before",
			"final",
			"mw3-after",
			"mw2-after",
			"mw1-after",
		}
		assert.Equal(t, expected, order)
	})
}

func TestNewDataChannelChain(t *testing.T) {
	t.Run("ミドルウェアスライスをMiddlewaresに変換", func(t *testing.T) {
		mw := func(next MessageProcessor) MessageProcessor { return next }
		slice := []func(MessageProcessor) MessageProcessor{mw, mw}

		mws := newDataChannelChain(slice)

		assert.Len(t, mws, 2)
	})

	t.Run("空のスライス", func(t *testing.T) {
		mws := newDataChannelChain(nil)

		assert.Len(t, mws, 0)
	})
}

func TestChain(t *testing.T) {
	t.Run("空のミドルウェア", func(t *testing.T) {
		called := false
		last := ProcessFunc(func(ctx context.Context, args ProcessArgs) {
			called = true
		})

		result := chain(nil, last)
		result.Process(context.Background(), ProcessArgs{})

		assert.True(t, called)
	})

	t.Run("ミドルウェアチェーン", func(t *testing.T) {
		order := []int{}

		mw1 := func(next MessageProcessor) MessageProcessor {
			return ProcessFunc(func(ctx context.Context, args ProcessArgs) {
				order = append(order, 1)
				next.Process(ctx, args)
			})
		}

		mw2 := func(next MessageProcessor) MessageProcessor {
			return ProcessFunc(func(ctx context.Context, args ProcessArgs) {
				order = append(order, 2)
				next.Process(ctx, args)
			})
		}

		last := ProcessFunc(func(ctx context.Context, args ProcessArgs) {
			order = append(order, 3)
		})

		middlewares := []func(MessageProcessor) MessageProcessor{mw1, mw2}
		result := chain(middlewares, last)
		result.Process(context.Background(), ProcessArgs{})

		assert.Equal(t, []int{1, 2, 3}, order)
	})

	t.Run("ミドルウェアがnextを呼ばない場合", func(t *testing.T) {
		order := []int{}

		mw1 := func(next MessageProcessor) MessageProcessor {
			return ProcessFunc(func(ctx context.Context, args ProcessArgs) {
				order = append(order, 1)
				// nextを呼ばない（チェーンを中断）
			})
		}

		mw2 := func(next MessageProcessor) MessageProcessor {
			return ProcessFunc(func(ctx context.Context, args ProcessArgs) {
				order = append(order, 2)
				next.Process(ctx, args)
			})
		}

		last := ProcessFunc(func(ctx context.Context, args ProcessArgs) {
			order = append(order, 3)
		})

		middlewares := []func(MessageProcessor) MessageProcessor{mw1, mw2}
		result := chain(middlewares, last)
		result.Process(context.Background(), ProcessArgs{})

		// mw1でチェーンが中断されるため、1のみ
		assert.Equal(t, []int{1}, order)
	})
}

func TestDatachannelIntegration(t *testing.T) {
	t.Run("完全なデータチャネル処理フロー", func(t *testing.T) {
		dc := &Datachannel{Label: "chat"}

		// 処理ログを記録
		log := []string{}

		// 認証ミドルウェア
		authMiddleware := func(next MessageProcessor) MessageProcessor {
			return ProcessFunc(func(ctx context.Context, args ProcessArgs) {
				log = append(log, "auth:start")
				// 認証処理をシミュレート
				ctx = context.WithValue(ctx, "authenticated", true)
				next.Process(ctx, args)
				log = append(log, "auth:end")
			})
		}

		// ログミドルウェア
		logMiddleware := func(next MessageProcessor) MessageProcessor {
			return ProcessFunc(func(ctx context.Context, args ProcessArgs) {
				log = append(log, "log:message_received")
				next.Process(ctx, args)
			})
		}

		dc.Use(authMiddleware, logMiddleware)

		dc.OnMessage(func(ctx context.Context, args ProcessArgs) {
			if ctx.Value("authenticated") == true {
				log = append(log, "handler:authenticated_message")
			}
		})

		// チェーンを構築して実行
		mws := newDataChannelChain(dc.middlewares)
		final := ProcessFunc(dc.onMessage)
		processor := mws.Process(final)

		args := ProcessArgs{
			Message: webrtc.DataChannelMessage{
				IsString: true,
				Data:     []byte("Hello, World!"),
			},
		}

		processor.Process(context.Background(), args)

		expected := []string{
			"auth:start",
			"log:message_received",
			"handler:authenticated_message",
			"auth:end",
		}
		assert.Equal(t, expected, log)
	})

	t.Run("メッセージデータの変換", func(t *testing.T) {
		var finalMessage []byte

		// メッセージを変換するミドルウェア
		transformMiddleware := func(next MessageProcessor) MessageProcessor {
			return ProcessFunc(func(ctx context.Context, args ProcessArgs) {
				// メッセージを大文字に変換（シミュレート）
				args.Message.Data = append([]byte("prefix:"), args.Message.Data...)
				next.Process(ctx, args)
			})
		}

		mws := Middlewares{transformMiddleware}
		final := ProcessFunc(func(ctx context.Context, args ProcessArgs) {
			finalMessage = args.Message.Data
		})

		processor := mws.Process(final)
		processor.Process(context.Background(), ProcessArgs{
			Message: webrtc.DataChannelMessage{Data: []byte("hello")},
		})

		assert.Equal(t, []byte("prefix:hello"), finalMessage)
	})
}

func TestDatachannelConcurrent(t *testing.T) {
	t.Run("並行メッセージ処理", func(t *testing.T) {
		var mu sync.Mutex
		count := 0

		mw := func(next MessageProcessor) MessageProcessor {
			return ProcessFunc(func(ctx context.Context, args ProcessArgs) {
				next.Process(ctx, args)
			})
		}

		mws := Middlewares{mw}
		final := ProcessFunc(func(ctx context.Context, args ProcessArgs) {
			mu.Lock()
			count++
			mu.Unlock()
		})

		processor := mws.Process(final)

		var wg sync.WaitGroup
		for i := 0; i < 100; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				processor.Process(context.Background(), ProcessArgs{})
			}()
		}

		wg.Wait()
		assert.Equal(t, 100, count)
	})
}
