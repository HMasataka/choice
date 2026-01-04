# choice

Go で実装された WebRTC SFU (Selective Forwarding Unit)

## 概要

choice は多人数ビデオ会議を実現する WebRTC SFU サーバーです。パブリッシャーからメディアストリームを受信し、サブスクライバーに選択的に転送することで、クライアント間のメッシュ接続を必要とせず、効率的なリアルタイム通信を可能にします。

### 主な機能

- Simulcast 対応（low/mid/high の3レイヤー）
- レイヤー選択によるクライアントへの適応的配信
- キーフレームベースのレイヤー切り替え
- JSON-RPC 2.0 シグナリング

## アーキテクチャ

```text
┌─────────────────────────────────────────────────────────────────────────┐
│                              SFU                                        │
│  ┌───────────────────────────────────────────────────────────────────┐  │
│  │                          Session                                  │  │
│  │                                                                   │  │
│  │  ┌─────────────────────┐         ┌─────────────────────┐          │  │
│  │  │       Peer A        │         │       Peer B        │          │  │
│  │  │  ┌───────────────┐  │         │  ┌───────────────┐  │          │  │
│  │  │  │   Publisher   │  │         │  │   Publisher   │  │          │  │
│  │  │  │ ┌───────────┐ │  │         │  │ ┌───────────┐ │  │          │  │
│  │  │  │ │  Router   │ │  │         │  │ │  Router   │ │  │          │  │
│  │  │  │ │┌─────────┐│ │  │         │  │ │┌─────────┐│ │  │          │  │
│  │  │  │ ││Forwarder││ │  │         │  │ ││Forwarder││ │  │          │  │
│  │  │  │ │└────┬────┘│ │  │         │  │ │└────┬────┘│ │  │          │  │
│  │  │  │ └─────┼─────┘ │  │         │  │ └─────┼─────┘ │  │          │  │
│  │  │  └───────┼───────┘  │         │  └───────┼───────┘  │          │  │
│  │  │          │          │         │          │          │          │  │
│  │  │          ▼          │         │          ▼          │          │  │
│  │  │  ┌───────────────┐  │         │  ┌───────────────┐  │          │  │
│  │  │  │  Subscriber   │◄─┼─────────┼──│  Subscriber   │  │          │  │
│  │  │  │ ┌───────────┐ │  │         │  │ ┌───────────┐ │  │          │  │
│  │  │  │ │ DownTrack │ │  │         │  │ │ DownTrack │ │  │          │  │
│  │  │  │ └───────────┘ │  │         │  │ └───────────┘ │  │          │  │
│  │  │  └───────────────┘  │         │  └───────────────┘  │          │  │
│  │  └─────────────────────┘         └─────────────────────┘          │  │
│  └───────────────────────────────────────────────────────────────────┘  │
└─────────────────────────────────────────────────────────────────────────┘
```

## パケットフロー

```text
Publisher (Simulcast: low/mid/high)
    │
    ▼
TrackReceiver ─── 複数レイヤーを管理
    │
    ├─ Layer (high) ─┐
    ├─ Layer (mid)  ─┼─ LayerReceiver (RTP受信)
    └─ Layer (low)  ─┘
    │
    ▼
Forwarder ─── 全レイヤーのパケットを DownTrack に転送
    │
    ▼
DownTrack ─── currentLayer と一致するパケットのみ送信
    │
    ▼
Subscriber ─── 1つのレイヤーのみ受信
```

## パッケージ構成

```text
pkg/sfu/
├── sfu.go          # SFU コア - セッション管理と WebRTC API
├── session.go      # セッション（ルーム）管理
├── peer.go         # クライアントの抽象化（Publisher + Subscriber）
├── publisher.go    # アップストリーム接続（クライアント → SFU）
├── subscriber.go   # ダウンストリーム接続（SFU → クライアント）
├── router.go       # パブリッシャーからサブスクライバーへのメディアルーティング
├── forwarder.go    # RTP パケットを複数の DownTrack に転送
├── track.go        # TrackReceiver - 複数レイヤーの管理
├── layer.go        # Layer - 品質レイヤー（low/mid/high）
├── receiver.go     # LayerReceiver - RTP パケットの受信
├── downtrack.go    # DownTrack - レイヤー選択と RTP 送信
├── rtp.go          # RTP ユーティリティ（キーフレーム検出など）
├── signaling.go    # JSON-RPC シグナリングハンドラー
└── transport.go    # WebSocket 接続ラッパー（スレッドセーフ）
```

### コンポーネント説明

| コンポーネント    | 説明                                                                |
| ----------------- | ------------------------------------------------------------------- |
| **SFU**           | メインエントリーポイント。セッションを管理し、WebRTC 接続を作成     |
| **Session**       | ピアが参加してメディアを共有できるルーム                            |
| **Peer**          | クライアントを表す。Publisher と Subscriber を保持                  |
| **Publisher**     | クライアントからの受信メディアを処理。Router を保持                 |
| **Subscriber**    | クライアントへの送信メディアを処理。DownTrack を管理                |
| **Router**        | TrackReceiver と Forwarder を管理し、サブスクライバーへルーティング |
| **Forwarder**     | 複数の DownTrack に RTP パケットを転送                              |
| **TrackReceiver** | 1つのトラックの複数レイヤー（low/mid/high）を管理                   |
| **Layer**         | 品質レイヤーを表す。LayerReceiver を保持                            |
| **LayerReceiver** | リモートトラックから RTP パケットを受信                             |
| **DownTrack**     | サブスクライバーに RTP パケットを送信。レイヤー選択を担当           |
| **LayerSelector** | 現在のレイヤーと目標レイヤーを管理。キーフレームでレイヤー切り替え  |

## Simulcast とレイヤー選択

choice は Simulcast に対応しており、パブリッシャーから複数の品質レイヤー（low/mid/high）を受信します。

### レイヤー切り替えの仕組み

1. DownTrack は `currentLayer`（デフォルト: mid）を持つ
2. 全レイヤーのパケットが Forwarder 経由で DownTrack に届く
3. DownTrack は `currentLayer` と一致するパケットのみをクライアントに転送
4. レイヤー切り替えは**キーフレーム到着時**に行われる（映像の乱れを防ぐため）

### レイヤー優先度

| レイヤー | 優先度 | 用途           |
| -------- | ------ | -------------- |
| high     | 3      | 高品質（フル） |
| mid      | 2      | 中品質         |
| low      | 1      | 低品質         |

## シグナリングプロトコル

WebSocket 上で JSON-RPC 2.0 を使用。

### メソッド

| メソッド    | 説明                                        |
| ----------- | ------------------------------------------- |
| `join`      | SDP オファーを使用してセッションに参加      |
| `leave`     | 現在のセッションから退出                    |
| `subscribe` | 他のピアのメディアを購読                    |
| `candidate` | ICE 候補を交換                              |
| `answer`    | サブスクライバー接続用の SDP アンサーを送信 |

### 通知（サーバー → クライアント）

| メソッド     | 説明                                  |
| ------------ | ------------------------------------- |
| `offer`      | サブスクライバー接続用の SDP オファー |
| `candidate`  | サーバーからの ICE 候補               |
| `trackAdded` | ピアから新しいトラックが利用可能      |

### 例: join

リクエスト:

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "join",
  "params": {
    "sessionId": "room1",
    "peerId": "peer-abc123",
    "offer": { "type": "offer", "sdp": "..." }
  }
}
```

レスポンス:

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "answer": { "type": "answer", "sdp": "..." }
  }
}
```

## はじめに

### 前提条件

- Go 1.21 以降

### インストール

```bash
git clone https://github.com/HMasataka/choice.git
cd choice
go mod tidy
```

### サーバーの起動

```bash
go run cmd/server/main.go
```

サーバーは `http://localhost:8080` で起動します。

### Web クライアントの使用方法

ブラウザで `http://localhost:8080` を開きます。

1. Session ID（ルーム名）を入力
2. Peer ID（自分の名前）を入力
3. 「Join」をクリックして接続
4. ローカルのカメラ映像が表示される
5. 他の参加者の映像が自動的に表示される

### ビルド

```bash
go build -o choice ./cmd/server
./choice
```

### テスト

```bash
go test ./...
```

## 設定

SFU は ICE サーバーで設定できます:

```go
sfu := sfu.NewSFU(sfu.Config{
    ICEServers: []webrtc.ICEServer{
        {URLs: []string{"stun:stun.l.google.com:19302"}},
    },
})
```

## ライセンス

このプロジェクトは MIT ライセンスの下でライセンスされています。
