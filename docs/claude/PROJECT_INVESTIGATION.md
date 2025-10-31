# LiveKit プロジェクト解説

## プロジェクト概要

LiveKitは、WebRTCベースのリアルタイムビデオ、音声、データ通信を提供するオープンソースのメディアサーバーです。スケーラブルでマルチユーザーの会議システムを構築するために必要な全ての機能を備えています。

### 主な特徴

- **スケーラブルなWebRTC SFU（Selective Forwarding Unit）**: 分散型アーキテクチャに対応
- **本格的な本番環境対応**: JWT認証、UDP/TCP/TURNサポート
- **簡単なデプロイメント**: 単一バイナリ、Docker、Kubernetesに対応
- **高度な機能**:
  - スピーカー検出
  - Simulcast（複数品質のストリーム配信）
  - エンドツーエンド最適化
  - 選択的サブスクリプション
  - モデレーションAPI
  - エンドツーエンド暗号化
  - SVC コーデック（VP9、AV1）
  - Webhook
  - 分散・マルチリージョン対応

### 技術スタック

- **言語**: Go 1.24+
- **WebRTC実装**: [Pion WebRTC](https://github.com/pion/webrtc)
- **ライセンス**: Apache License v2.0

## プロジェクト利用方法

### インストール

#### macOS

```bash
brew install livekit
```

#### Linux

```bash
curl -sSL https://get.livekit.io | bash
```

#### Windows

[最新リリース](https://github.com/livekit/livekit/releases/latest)からダウンロード

### 起動方法

開発モードでLiveKitを起動:

```bash
livekit-server --dev
```

開発モードでは以下のプレースホルダーキーが使用されます:

- **API Key**: devkey
- **API Secret**: secret

### アクセストークンの作成

ユーザーがルームに接続するには、アクセストークン（JWT）が必要です:

```bash
lk token create \
    --api-key devkey --api-secret secret \
    --join --room my-first-room --identity user1 \
    --valid-for 24h
```

### ソースからのビルド

前提条件:

- Go 1.23+がインストールされていること
- GOPATH/binがPATHに含まれていること

ビルド手順:

```bash
git clone https://github.com/livekit/livekit
cd livekit
./bootstrap.sh
mage
```

## ディレクトリ構造と主要ファイル

### ルートディレクトリ

- **README.md**: プロジェクトの概要とドキュメント
- **go.mod**: Goモジュール定義ファイル（モジュール名: `github.com/livekit/livekit-server`）
- **config-sample.yaml**: サーバー設定のサンプルファイル
- **bootstrap.sh**: 開発環境のセットアップスクリプト
- **magefile.go**: ビルドツール（Mage）の設定ファイル
- **Dockerfile**: Dockerイメージのビルド定義

### cmd/ - エントリーポイント

#### cmd/server/main.go

サーバーのメインエントリーポイント（cmd/server/main.go:122-192）

主な機能:

- CLIフラグの定義と処理
- サーバー起動処理
- 設定ファイルの読み込み
- シグナルハンドリング（SIGINT、SIGTERM、SIGQUIT）
- プロファイリング機能（CPU、メモリ）

実装されているサブコマンド:

- `generate-keys`: API キーとシークレットペアの生成
- `ports`: サーバーが使用するポートの表示
- `list-nodes`: 全ノードのリスト表示
- `help-verbose`: 詳細なヘルプ表示

### pkg/ - コアパッケージ

#### pkg/service/ - サービス層

**pkg/service/server.go**: LiveKitServerの実装（pkg/service/server.go:48-96）

LivekitServerは以下のコンポーネントで構成されています:

- `rtcService`: WebRTC通信のハンドリング
- `whipService`: WHIP（WebRTC-HTTP Ingestion Protocol）のサポート
- `agentService`: エージェントサービスの管理
- `signalServer`: シグナリングサーバー
- `turnServer`: TURNサーバー（NAT越え通信）
- `roomManager`: ルーム管理
- `router`: ルーティング処理

#### pkg/rtc/ - リアルタイム通信層

**pkg/rtc/room.go**: Roomの実装（pkg/rtc/room.go:91-100）

Roomは以下を管理します:

- 参加者（パーティシパント）の管理
- メディアトラックの管理
- データチャネル通信
- ルームのライフサイクル（参加、退出時刻の記録）

主要なファイル:

- **participant.go**: 参加者の管理
- **mediatrack.go**: メディアトラック（音声・映像）の管理
- **transport.go**: WebRTCトランスポートの管理
- **mediaengine.go**: コーデックやメディアエンジンの設定

#### pkg/sfu/ - SFU（Selective Forwarding Unit）実装

メディアパケットの効率的な転送を実現する中核コンポーネント:

- **buffer/**: RTPバッファ管理、フレーム整合性チェック
- **bwe/**: 帯域幅推定（Bandwidth Estimation）
  - **remotebwe/**: リモート側BWE実装
  - **sendsidebwe/**: 送信側BWE実装
- **forwarder.go**: パケット転送ロジック
- **downtrack.go**: ダウンストリームトラック管理
- **receiver.go**: RTPレシーバー実装

#### pkg/routing/ - ルーティング層

分散環境でのノード間通信とルーム管理:

- **localrouter.go**: ローカルルーター
- **redisrouter.go**: Redis経由の分散ルーター
- **roommanager.go**: ルーム管理
- **selector/**: ノード選択アルゴリズム
  - CPUロードベース
  - システムロードベース
  - リージョンアウェア

#### pkg/config/ - 設定管理

**config.go**: サーバー設定の読み込みと管理

設定項目:

- ポート設定
- Redis接続設定
- WebRTC設定（UDPポート範囲、STUN/TURNサーバー）
- ロギング設定
- APIキー管理

#### pkg/telemetry/ - テレメトリ・メトリクス

- **events.go**: イベント収集
- **stats.go**: 統計情報管理
- **prometheus/**: Prometheus メトリクスエクスポート
  - ノード情報
  - パケット統計
  - 品質メトリクス
  - ルーム情報

#### pkg/agent/ - エージェント機能

プログラマブルな参加者（ボット）を実装するための機能:

- **client.go**: エージェントクライアント
- **worker.go**: エージェントワーカー
- **config.go**: エージェント設定

### test/ - テストコード

統合テスト:

- **singlenode_test.go**: 単一ノードでのテスト
- **multinode_test.go**: 複数ノードでの分散テスト
- **agent_test.go**: エージェント機能のテスト
- **webhook_test.go**: Webhook機能のテスト

### deploy/ - デプロイメント設定

- **grafana/**: Grafanaダッシュボード定義

### version/ - バージョン情報

- **version.go**: バージョン情報の定義

## 設定ファイル（config-sample.yaml）

主要な設定項目（config-sample.yaml:15-100）:

### 基本設定

- **port**: メインTCPポート（デフォルト: 7880）
  - RoomServiceとRTCエンドポイント用

### Redis設定

分散モードを有効にするための設定:

- アドレス、DB、認証情報
- Sentinelモードのサポート
- クラスターモードのサポート
- TLS接続のサポート

### WebRTC設定（rtc）

- **port_range_start/end**: UDP ポート範囲（50000-60000）
- **tcp_port**: TCP ポート（デフォルト: 7881）
- **use_external_ip**: STUNを使用した外部IPの検出
- **stun_servers**: STUNサーバーの設定
- **turn_servers**: TURNサーバーの設定
- **congestion_control**: 輻輳制御の設定
- **allow_tcp_fallback**: TCP フォールバックの許可

## エコシステム

LiveKitは以下の関連サービスと連携:

- **Agents**: AIアプリケーション構築用のプログラマブル参加者
- **Egress**: ルームの録画やストリームのエクスポート
- **Ingress**: RTMP、WHIP、HLS、OBSからのストリーム取り込み

## SDK

### クライアントSDK

- JavaScript/TypeScript
- Swift (iOS/macOS)
- Kotlin (Android)
- Flutter
- React Native
- Unity
- Rust

### サーバーSDK

- Go
- JavaScript/TypeScript
- Ruby
- Java/Kotlin
- Python
- PHP（コミュニティ）

## まとめ

LiveKitは、WebRTCベースのリアルタイム通信を簡単に構築できる包括的なプラットフォームです。単一バイナリとして動作し、必要に応じてRedisを使用した分散環境にスケールアウト可能です。本番環境での利用を想定した堅牢な設計と、豊富なクライアント/サーバーSDKにより、様々なプラットフォームで活用できます。
