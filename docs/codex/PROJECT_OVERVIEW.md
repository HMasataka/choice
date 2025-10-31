# LiveKit サーバー概要

LiveKit は、WebRTC を用いたスケーラブルな SFU（Selective Forwarding Unit）サーバーです。多人数の音声・映像・データを低遅延に中継し、アプリに会議・ライブ配信・インタラクティブ体験を組み込めます。本リポジトリのサーバー実装は Go 製（Pion WebRTC ベース）で、単体サーバーから Redis を用いた分散/マルチリージョン運用まで対応します。

- 言語/実装: Go（Pion WebRTC）
- 用途: 会議、ライブ配信、双方向データ通信（DataChannel）
- 配布: 単一バイナリ/Docker/Kubernetes、Prometheus メトリクス、Webhook、JWT 認証
- 分散: Redis によりノード間でルーム/シグナリングをルーティング

---

## 主な機能

- **SFU**: 選択配信、Simulcast/SVC、帯域制御（BWE/StreamAllocator）、スピーカ検出、選択購読、E2EE
- **接続性**: UDP/TCP/TURN、NAT 越え、TCP フォールバック
- **運用**: 単一バイナリ/Docker/K8s、Prometheus `/metrics`、Webhook、JWT
- **分散/マルチリージョン**: Redis 連携、ノードセレクタ（地域/負荷）

---

## クイックスタート

### インストール

- macOS: `brew install livekit`
- Linux: `curl -sSL https://get.livekit.io | bash`
- Windows: GitHub Releases から取得

### 起動（開発モード）

```bash
livekit-server --dev
```

- 開発用 API キー/シークレット: `devkey` / `secret`

### アクセストークン作成（LiveKit CLI 推奨）

```bash
lk token create \
  --api-key devkey --api-secret secret \
  --join --room my-first-room --identity user1 \
  --valid-for 24h
```

### 動作確認

- ブラウザ例: https://example.livekit.io にトークンを入力
- 疑似配信（デモ動画の発行）:

```bash
lk room join \
  --url ws://localhost:7880 \
  --api-key devkey --api-secret secret \
  --identity bot-user1 \
  --publish-demo \
  my-first-room
```

### 本番構成

1. `config-sample.yaml` をベースに `config.yaml` を用意（API キー、ポート、UDP レンジ、TCP、STUN/TURN、Redis、Webhook、Prometheus など）
2. サーバー起動:

```bash
livekit-server --config /path/to/config.yaml
```

3. Docker 例:

```bash
docker run \
  -p 7880:7880 -p 7881:7881 \
  -p 50000-60000:50000-60000/udp \
  -v $PWD/config.yaml:/config.yaml \
  livekit/livekit-server --config /config.yaml
```

---

## 運用/設定のポイント

- **分散/スケール**: `redis.address` を設定すると複数ノードでルーム共有可能。`region` とノードセレクタで地域最適化。
- **ネットワーク**:
  - HTTP/Twirp: `port`
  - WebRTC: `rtc.port_range_start/end`（UDP）、`rtc.tcp_port`（ICE/TCP）
  - 必要に応じて `rtc.use_external_ip` / `rtc.node_ip`
- **認証**: `keys:` または `key_file:` を必ず設定（本番は十分長い Secret）。
- **TURN**: 組込み/外部いずれも可（`turn.enabled`、`rtc.turn_servers`）。
- **監視**: `prometheus.port` を有効にして `/metrics` 収集。`/` のヘルスチェックはノード統計で 200/406 を返却。

---

## ディレクトリ/主要ファイルの役割

### エントリ/CLI

- `cmd/server/main.go`: エントリポイント。CLI フラグ/環境変数/設定読込、dev モード、プロファイリング、サブコマンド定義、サーバー起動。
- `cmd/server/commands.go`: サブコマンド実装（鍵生成、ポート出力、ノード一覧、開発用トークン生成など）。

### サービス層（HTTP/Twirp/Signal/TURNなど）

- `pkg/service/server.go`: HTTP/Twirp/Prometheus/TURN/Signal の起動と停止、CORS/ミドルウェア、ヘルスチェック。
- `pkg/service/wire.go`: 依存性の組み立て（Google Wire）。Redis/Store/Router/各 Service（Room/Ingress/Egress/SIP/Agent/RTC/WHIP）を結合。
- `pkg/service/roomservice.go`: Twirp Room API（作成/一覧/削除、メタデータ更新、データ送信、参加者操作）。
- `pkg/service/rtcservice.go`: RTC 関連 HTTP ルート（シグナリング補助など）。
- `pkg/service/signal.go`: psrpc ベース Signal サーバー（参加者セッション開始、ノード間リレー）。
- `pkg/service/turn.go`: 内蔵 TURN サーバー連携。
- `pkg/service/egress.go` / `ingress.go`: 録画/配信・取り込み（別サービス連携）。
- `pkg/service/agentservice.go` / `agent_dispatch_service.go`: Agents 連携（自動参加者/AI 等）。

### ルーティング/ノード管理

- `pkg/routing/interfaces.go`: ルータ/メッセージルーティング/参加者初期情報 の共通定義。
- `pkg/routing/localrouter.go`: 単一ノード用ルーティング。
- `pkg/routing/redisrouter.go`: Redis+psrpc による分散ルーティング（部屋→ノード割り当て、シグナリング配送）。
- `pkg/routing/node.go` `nodestats.go`: ノード情報/統計の管理。
- `pkg/routing/signal.go`: Signal クライアント（信頼性の高いノード間シグナリング）。

### RTC / SFU コア

- `pkg/rtc/room.go` `participant.go`: ルーム/参加者の中核（発行/購読、再交渉、ミュート、権限）。
- `pkg/rtc/mediatrack*.go` `uptrackmanager.go` `subscriptionmanager.go`: トラック管理、購読制御、ダイナキャスト、品質レイヤ選択。
- `pkg/rtc/transport*.go`: PeerConnection/トランスポート管理、ICE/TCP/UDP。
- `pkg/sfu/`: DownTrack/Receiver/Forwarder、帯域推定（`bwe/`）と配信調整（`streamallocator/`）、NACK/REMB/TCC、RED、プレイアウト遅延等。

### 設定/テレメトリ/その他

- `pkg/config/config.go`: 設定構造体、デフォルト、検証、CLI/環境変数バインド（`LIVEKIT_...`）。`ValidateKeys()` で鍵の検証。
- `config-sample.yaml`: 実運用のサンプル設定（ポート/UDP レンジ、TCP、STUN/TURN、Redis、Webhook、Prometheus、Room 既定など）。
- `pkg/telemetry/prometheus/`: ノード/ルーム/パケットのメトリクス。
- `pkg/clientconfiguration/`: クライアント特性に応じた設定マッチング。
- `pkg/agent/`: Agents 連携の設定とクライアント。
- `version/version.go`: バージョン定義。
- `Dockerfile`: コンテナビルド用。
- `magefile.go` `bootstrap.sh`: ビルド/コード生成（Wire）/テストのタスク。

---

## ソースからの開発/ビルド

- 前提: Go 1.23+、`$GOPATH/bin` が PATH に含まれている
- 初回セットアップ:

```bash
./bootstrap.sh
```

- ビルド:

```bash
mage
# bin/livekit-server が生成されます
```

- テスト:

```bash
mage test       # 単体（短縮）
mage testAll    # 統合含む
```

---

## 参考リンク

- ドキュメント: https://docs.livekit.io
- デモ: https://example.livekit.io
- React SDK: https://github.com/livekit/livekit-react
- Helm: https://github.com/livekit/livekit-helm
- CLI: https://github.com/livekit/livekit-cli

> 補足が必要であれば、特定のデプロイ形態（Docker Compose/Helm）や機能（Webhook、E2EE、SVC）の詳しい設定例も追加できます。
