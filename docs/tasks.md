# WebRTC SFU 実装タスク一覧

本ドキュメントは、requirements.md（要件定義書）およびdesign.md（設計書）に基づき、
実装作業をcommit単位で分解したタスク一覧である。

## Phase 1: 基本機能

### 1.1 プロジェクト初期設定

#### Task 1.1.1: プロジェクト構造の初期化 ✅

- [x] Go modulesの初期化（`go mod init`）
- [x] 基本ディレクトリ構造の作成
- [x] .gitignore、.editorconfig等の設定ファイル作成
- [x] Makefile作成（ビルド、テスト、lint用）

**コミットメッセージ例**: `feat: initialize project structure with Go modules`

#### Task 1.1.2: 設定管理の実装 ✅

- [x] `pkg/config/config.go` - 設定構造体の定義
- [x] YAML設定ファイルの読み込み実装
- [x] 環境変数によるオーバーライド対応
- [x] `configs/config.yaml` - 設定ファイルサンプル作成

**コミットメッセージ例**: `feat(config): implement configuration management with YAML support`

#### Task 1.1.3: ログ基盤の実装 ✅

- [x] `pkg/logger/logger.go` - 構造化ログの設定
- [x] ログレベル制御（DEBUG/INFO/WARN/ERROR）
- [x] JSON形式出力対応
- [x] PII マスキング機能の実装
  - IPアドレス
  - JWTトークン
  - SDPコンテンツ（DEBUGレベル以外）
  - ICE candidate詳細
  - ユーザーIDハッシュオプション

**コミットメッセージ例**: `feat(logger): implement structured logging with PII masking`

### 1.2 HTTPサーバー基盤

#### Task 1.2.1: HTTPサーバーの実装 ✅

- [x] `cmd/sfu/main.go` - エントリーポイント作成
- [x] `internal/server/server.go` - HTTPサーバー実装
- [ ] TLS 1.3 対応（TLS 1.2フォールバック）※後続タスクで対応
- [x] Graceful shutdown対応
- [x] ヘルスチェックエンドポイント（`/health`）
- [x] レディネスチェックエンドポイント（`/ready`、Redis依存確認、失敗時503）

**コミットメッセージ例**: `feat(server): implement HTTP server with TLS and graceful shutdown`

#### Task 1.2.2: ミドルウェアの実装 ✅

- [x] `internal/server/middleware/logging.go` - リクエストログ
- [x] `internal/server/middleware/cors.go` - CORS設定
- [ ] `internal/server/middleware/auth.go` - 認証ミドルウェア（スタブ）※Task 1.3で実装

**コミットメッセージ例**: `feat(middleware): implement request logging and CORS`

#### Task 1.2.3: レート制限の実装 ✅

- [x] `internal/server/middleware/ratelimit.go` - レート制限基盤
- [x] REST API レート制限（100回/分/トークン）
- [x] ルーム作成レート制限（10回/分/ユーザー）
- [x] IPベースの接続数制限
- [x] グローバル接続数上限の実装
- [ ] Redisベースのレート制限カウンタ永続化 ※Redisストア実装タスクで対応

**コミットメッセージ例**: `feat(middleware): implement rate limiting for REST API`

### 1.3 認証機能

> **注記**: リフレッシュトークンの発行・管理は外部認証サービス（IdP）の責務とする。
> SFUサーバーはアクセストークン（JWT）の検証のみを行う。

#### Task 1.3.1: JWT検証の実装 ✅

- [x] `internal/auth/jwt.go` - JWT検証ロジック
- [x] RS256署名検証
- [x] クレーム検証（sub、iat、exp、iss、aud、room_id）
- [x] ユニットテスト作成

**コミットメッセージ例**: `feat(auth): implement JWT validation with RS256 signature`

#### Task 1.3.2: JWKS取得の実装 ✅

- [x] `internal/auth/jwks.go` - JWKSエンドポイントからの公開鍵取得
- [x] キーキャッシュ機能（TTL: 1時間）
- [x] キーID（kid）によるキー選択
- [x] キーローテーション対応（90日ごと、旧キー7日間猶予）
- [x] ユニットテスト作成

**コミットメッセージ例**: `feat(auth): implement JWKS fetching with caching and key rotation`

#### Task 1.3.3: JWT失効管理の実装 ✅

- [x] `internal/auth/blacklist.go` - トークンブラックリスト
- [x] Redisベースのブラックリスト実装
- [x] トークン失効登録・確認API
- [x] 期限切れエントリの自動クリーンアップ（10分間隔）
- [x] ユニットテスト作成

**コミットメッセージ例**: `feat(auth): implement JWT blacklist with Redis`

#### Task 1.3.4: 権限管理の実装 ✅

- [x] `internal/auth/permission.go` - ロールベース権限チェック
- [x] admin/moderator/publisher/subscriberロール対応
- [x] 操作ごとの権限マトリクス実装
- [x] ユニットテスト作成

**コミットメッセージ例**: `feat(auth): implement role-based permission management`

### 1.4 シグナリング基盤

#### Task 1.4.1: WebSocketハンドラの実装 ✅

- [x] `internal/signaling/handler.go` - WebSocket接続管理
- [x] `internal/signaling/connection.go` - 接続状態管理
- [x] TLS over WebSocket（wss://）対応
- [x] Ping/Pong による接続維持
- [x] 接続エラーハンドリング

**コミットメッセージ例**: `feat(signaling): implement WebSocket handler with TLS support`

#### Task 1.4.2: WebSocketレート制限の実装 ✅

- [x] WebSocket接続レート制限（10回/秒/IP、Token Bucketアルゴリズム）
- [x] シグナリングメッセージレート制限（100回/秒/接続）
- [x] 帯域幅上限（接続あたり、ルームあたり、設定可能）
- [x] 不正パケット検出・遮断（JSON検証、バイナリメッセージ拒否）
- [x] `internal/signaling/ratelimit.go` - レート制限実装
- [x] `internal/signaling/ratelimit_test.go` - テスト実装
- [x] handler.goへのレート制限統合
- [x] 並行アクセス時のデータ競合対策
- [x] OnDisconnect通知の適切な実装

**コミットメッセージ例**: `feat(signaling): implement WebSocket rate limiting and DoS protection`

#### Task 1.4.3: JSON-RPCプロトコルの実装 ✅

- [x] `internal/signaling/protocol/message.go` - JSON-RPCメッセージ型定義
- [x] `internal/signaling/protocol/request.go` - リクエスト型定義
- [x] `internal/signaling/protocol/response.go` - レスポンス型定義
- [x] `internal/signaling/protocol/notification.go` - 通知型定義
- [x] `internal/signaling/protocol/errors.go` - エラーコード定義（-32700〜1009）
- [x] `internal/signaling/protocol/validation.go` - UUIDv4/メソッド名バリデーション
- [x] `internal/signaling/protocol/protocol_test.go` - 包括的テスト

**コミットメッセージ例**: `feat(signaling): implement JSON-RPC 2.0 protocol types`

#### Task 1.4.4: メッセージディスパッチャの実装 ✅

- [x] `internal/signaling/dispatcher.go` - メソッドルーティング
- [x] メソッドハンドラ登録機構
- [x] エラーレスポンス生成
- [x] 同時リクエスト数制限（MaxConcurrentRequests）
- [x] DispatcherConnectionHandler（ConnectionHandlerインターフェース実装）
- [x] `internal/signaling/dispatcher_test.go` - ユニットテスト作成

**コミットメッセージ例**: `feat(signaling): implement message dispatcher with method routing`

#### Task 1.4.5: 基本メソッドハンドラの実装 ✅

- [x] `join` メソッドハンドラ（token、sessionId、metadata）
- [x] joinレスポンスにiceServersを組み立てて返却
- [x] `leave` メソッドハンドラ
- [x] `offer` メソッドハンドラ（SDP offer受信）
- [x] `answer` メソッドハンドラ（SDP answer受信）
- [x] `candidate` メソッドハンドラ（ICE candidate受信）

**コミットメッセージ例**: `feat(signaling): implement basic method handlers (join/leave/offer/answer/candidate)`

#### Task 1.4.6: メディアメソッドハンドラの実装 ✅

- [x] `publish` メソッドハンドラ（kind、simulcast、metadata、label）
- [x] `unpublish` メソッドハンドラ（trackId）
- [x] `subscribe` メソッドハンドラ（publisherId、trackId、preferredLayer）
- [x] `unsubscribe` メソッドハンドラ（subscriptionId）
- [x] `setPreferredLayer` メソッドハンドラ（trackId、layer）

**コミットメッセージ例**: `feat(signaling): implement media method handlers (publish/subscribe)`

#### Task 1.4.7: サーバー通知の実装 ✅

- [x] `joined` 通知（自身の参加完了確認）
- [x] `left` 通知（自身の退出完了確認）
- [x] `participantJoined` 通知
- [x] `participantLeft` 通知
- [x] `trackPublished` 通知
- [x] `trackUnpublished` 通知

**コミットメッセージ例**: `feat(signaling): implement server notifications for room events`

#### Task 1.4.8: 追加サーバー通知の実装 ✅

- [x] `offer` 通知（サーバー主導の再ネゴシエーション）
- [x] `candidate` 通知（サーバーからのICE candidate）
- [x] `layerChanged` 通知（requestedLayer、actualLayer、reason）
- [x] `error` 通知（code、message、fatal）
- [x] `reconnect` 通知（reason、retryAfterMs）

**コミットメッセージ例**: `feat(signaling): implement additional server notifications`

#### Task 1.4.9: メディアイベント通知の実装 ✅

- [x] `trackSubscribed` 通知（購読完了）
- [x] `trackSubscriptionFailed` 通知（購読失敗、trackId、エラー情報）
- [x] `connectionQualityChanged` 通知（参加者ごとの接続品質、サーバー側RTCP統計から算出して通知）
- [x] `serverStateChanged` 通知（ルームのサーバー側状態変化）

> **注記**: connectionQualityChangedはサーバーがRTCP統計から算出して通知する。SDKはこの通知を受け取りイベントとして発火する（SDK側で独自計算はしない）。

**コミットメッセージ例**: `feat(signaling): implement media event notifications`

### 1.5 WebRTC基盤

#### Task 1.5.1: PeerConnection管理の実装 ✅

- [x] `internal/webrtc/peer.go` - PeerConnection ラッパー
- [x] pion/webrtc の設定・初期化
- [x] ICE Lite モード設定
- [x] イベントハンドラ登録（OnTrack、OnICECandidate等）
- [x] sync.Once による idempotent Close
- [x] OnICECandidateError ハンドラ（pending candidate失敗時の通知）
- [x] nil MediaEngine バリデーション
- [x] 包括的なユニットテスト（並行処理テスト含む）

**コミットメッセージ例**: `feat(webrtc): implement PeerConnection management with pion/webrtc`

#### Task 1.5.2: SDP処理の実装

- [ ] `internal/webrtc/sdp.go` - SDP パース・生成
- [ ] Unified Plan 対応（Plan B非対応）
- [ ] BUNDLE、rtcp-mux 必須設定
- [ ] Safari互換性対応（SDP正規化）

**コミットメッセージ例**: `feat(webrtc): implement SDP processing with Unified Plan support`

#### Task 1.5.3: 必須RTCPフィードバック設定

- [ ] nack（パケット再送要求）設定
- [ ] nack pli（Picture Loss Indication）設定
- [ ] ccm fir（Full Intra Request）設定
- [ ] goog-remb（REMB帯域幅推定）設定
- [ ] transport-cc（TWCC輻輳制御）設定

**コミットメッセージ例**: `feat(webrtc): configure required RTCP feedback mechanisms`

#### Task 1.5.4: コーデック設定の実装

- [ ] `internal/webrtc/codec.go` - コーデック設定
- [ ] VP8（必須、全クライアント対応）
- [ ] H.264 High Profile Level 5.0（profile-level-id=640032）
- [ ] H.264 Constrained Baseline Level 3.1（profile-level-id=42e01f）
- [ ] H.264 fmtpパラメータ（packetization-mode=1、level-asymmetry-allowed=1）
- [ ] VP9（推奨、SVC対応）
- [ ] Opus（minptime=10、useinbandfec=1、stereo=1）
- [ ] コーデック優先順位設定
- [ ] オプション: AV1（将来対応）、G.711（レガシー互換用）

**コミットメッセージ例**: `feat(webrtc): implement codec configuration with H.264 profiles`

#### Task 1.5.5: RTPヘッダー拡張設定の実装

- [ ] mid 拡張（urn:ietf:params:rtp-hdrext:sdes:mid）
- [ ] rid 拡張（urn:ietf:params:rtp-hdrext:sdes:rtp-stream-id）
- [ ] transport-wide-cc 拡張
- [ ] abs-send-time 拡張

**コミットメッセージ例**: `feat(webrtc): implement RTP header extension configuration`

#### Task 1.5.6: ICE設定の実装

- [ ] `internal/webrtc/ice.go` - ICE設定
- [ ] ICE Lite モード（controlled role固定）
- [ ] host候補のみ通知（srflx/relay候補は生成しない）
- [ ] ICE候補優先順位（host > srflx > relay）
- [ ] NAT 1:1 マッピング対応
- [ ] UDPポート範囲設定（10000-20000）
- [ ] IPv4/IPv6デュアルスタック対応（IPv4優先）
- [ ] 接続確立タイムアウト（30秒）
- [ ] 複数ICEサーバーのフォールバック

**コミットメッセージ例**: `feat(webrtc): implement ICE Lite configuration`

#### Task 1.5.7: ICE再起動の実装

- [ ] ネットワーク切り替え検出
- [ ] ICE再起動トリガー
- [ ] 再起動時のSDP再ネゴシエーション

**コミットメッセージ例**: `feat(webrtc): implement ICE restart mechanism`

#### Task 1.5.8: TURN資格情報管理の実装

- [ ] `internal/webrtc/turn.go` - TURN資格情報サービス
- [ ] Long-term credentials (RFC 5389) 生成
- [ ] 資格情報有効期限管理（24時間）
- [ ] 資格情報自動ローテーション（12時間ごと）
- [ ] 外部STUN/TURNサーバー連携

**コミットメッセージ例**: `feat(webrtc): implement TURN credential management`

### 1.6 メディア基盤（基本）

#### Task 1.6.1: トラック管理基盤の実装

- [ ] `internal/media/track.go` - トラック基本構造
- [ ] トラックID生成（サーバー側）
- [ ] トラック種別（video/audio）管理
- [ ] トラックメタデータ管理

**コミットメッセージ例**: `feat(media): implement basic track management`

#### Task 1.6.2: 基本メディアルーターの実装

- [ ] `internal/media/router.go` - メディアルーティング基盤
- [ ] トラックレジストリ
- [ ] Publisher/Subscriber マッピング

**コミットメッセージ例**: `feat(media): implement basic media router`

#### Task 1.6.3: RTP処理基盤の実装

- [ ] `internal/media/rtp/processor.go` - RTPパケット処理
- [ ] SSRC管理（Publisher/Subscriber間マッピング）
- [ ] シーケンス番号書き換え
- [ ] タイムスタンプ正規化

**コミットメッセージ例**: `feat(media): implement RTP packet processing`

#### Task 1.6.4: RTP拡張処理の実装

- [ ] MID拡張ヘッダー処理（メディアID識別）
- [ ] RID拡張ヘッダー処理（Simulcastレイヤー識別）
- [ ] パケットペーシング（バースト送信平滑化）
- [ ] ジッタバッファ（50ms、適応的）

**コミットメッセージ例**: `feat(media): implement RTP extension header processing`

### 1.7 1:1接続の実装

#### Task 1.7.1: 基本的なOffer/Answer交換

- [ ] クライアントからのoffer受信
- [ ] answer生成・返却
- [ ] ICE candidate交換（Trickle ICE）
- [ ] 接続状態監視

**コミットメッセージ例**: `feat(webrtc): implement basic Offer/Answer exchange`

#### Task 1.7.2: メディアトラック受信

- [ ] Publisher からのトラック受信
- [ ] 映像・音声トラックの識別
- [ ] トラック制限チェック（映像3、音声2/参加者）

**コミットメッセージ例**: `feat(media): implement media track reception from publisher`

#### Task 1.7.3: メディアトラック転送

- [ ] Subscriber へのトラック転送
- [ ] RTPパケット転送
- [ ] 再ネゴシエーショントリガー（track_added/track_removed）

**コミットメッセージ例**: `feat(media): implement media track forwarding to subscriber`

## Phase 2: ルーム機能

### 2.1 ルーム管理

#### Task 2.1.1: ルームマネージャの実装

- [ ] `internal/room/manager.go` - ルームマネージャ
- [ ] ルーム作成・削除
- [ ] ルーム一覧取得
- [ ] ルーム検索（ID指定）

**コミットメッセージ例**: `feat(room): implement room manager with CRUD operations`

#### Task 2.1.2: ルームエンティティの実装

- [ ] `internal/room/room.go` - ルームエンティティ
- [ ] ルーム状態管理（created/active/locked/closing/closed）
- [ ] 最大参加者数制限（デフォルト: 100）
- [ ] 空ルームタイムアウト
- [ ] 総トラック数制限（500/ルーム）
- [ ] ルームメタデータ管理

**コミットメッセージ例**: `feat(room): implement room entity with state management`

#### Task 2.1.5: ルームlock/unlock機能の実装

- [ ] `internal/room/lock.go` - ルームロック制御
- [ ] lock操作（admin/moderatorのみ）
- [ ] unlock操作（admin/moderatorのみ）
- [ ] locked状態での新規参加拒否（エラーコード1010）
- [ ] ロック状態変更の通知（serverStateChanged）
- [ ] ユニットテスト作成

**コミットメッセージ例**: `feat(room): implement room lock/unlock functionality`

#### Task 2.1.6: ルームlock/unlock API/シグナリングの実装

- [ ] JSON-RPCメソッド `lock` / `unlock` の追加（シグナリング経由）
- [ ] REST API `POST /api/v1/rooms/{id}/lock` / `DELETE /api/v1/rooms/{id}/lock` の追加（オプション）
- [ ] `api/jsonrpc-schema.json` への lock/unlock メソッド追加
- [ ] 権限チェック（admin/moderatorのみ）

**コミットメッセージ例**: `feat(api): expose room lock/unlock via signaling and REST`

#### Task 2.1.3: 参加者管理の実装

- [ ] `internal/room/participant.go` - 参加者エンティティ
- [ ] 参加者状態管理（joining/joined/publishing/subscribing/leaving/left）
- [ ] 参加者メタデータ管理
- [ ] 参加・退出処理

**コミットメッセージ例**: `feat(room): implement participant management`

#### Task 2.1.4: ルームイベントの実装

- [ ] `internal/room/events.go` - ルームイベント定義
- [ ] participantJoined/participantLeft 通知
- [ ] trackPublished/trackUnpublished 通知
- [ ] イベントブロードキャスト

**コミットメッセージ例**: `feat(room): implement room events and notifications`

### 2.2 セッション管理

#### Task 2.2.1: セッションストアインターフェース

- [ ] `internal/store/interface.go` - ストアインターフェース定義
- [ ] Session型定義（PublishedTracks、Subscriptions含む）
- [ ] CRUD操作インターフェース

**コミットメッセージ例**: `feat(store): define session store interface`

#### Task 2.2.2: インメモリストアの実装

- [ ] `internal/store/memory.go` - インメモリ実装（開発用）
- [ ] セッション保存・取得
- [ ] TTL管理
- [ ] ユニットテスト作成

**コミットメッセージ例**: `feat(store): implement in-memory session store`

#### Task 2.2.3: Redisストアの実装

- [ ] `internal/store/redis.go` - Redis実装
- [ ] go-redis クライアント設定
- [ ] セッションシリアライズ/デシリアライズ
- [ ] TTL設定
- [ ] 統合テスト作成

**コミットメッセージ例**: `feat(store): implement Redis session store`

### 2.3 再接続機能

#### Task 2.3.1: セッションID管理

- [ ] セッションID生成（UUIDv4）
- [ ] セッション有効期限管理（30秒）
- [ ] セッションメタデータ保存（UserAgent、IPAddress等）

**コミットメッセージ例**: `feat(session): implement session ID management`

#### Task 2.3.2: 再接続処理の実装

- [ ] セッションIDによる再接続識別
- [ ] メディアストリーム状態の復元
- [ ] 購読状態の復元
- [ ] 再接続時のSDP再ネゴシエーション
- [ ] 指数バックオフ対応（初期1秒、最大30秒、係数2）

**コミットメッセージ例**: `feat(session): implement reconnection with state restoration`

### 2.4 REST API

#### Task 2.4.1: ルームAPI実装

- [ ] `internal/server/routes.go` - ルーティング定義
- [ ] POST `/api/v1/rooms` - ルーム作成
- [ ] GET `/api/v1/rooms/{id}` - ルーム情報取得
- [ ] DELETE `/api/v1/rooms/{id}` - ルーム削除
- [ ] GET `/api/v1/rooms/{id}/participants` - 参加者一覧
- [ ] POST `/api/v1/rooms/{id}/lock` - ルームロック（admin/moderatorのみ）
- [ ] DELETE `/api/v1/rooms/{id}/lock` - ルームアンロック（admin/moderatorのみ）

**コミットメッセージ例**: `feat(api): implement room REST API endpoints`

#### Task 2.4.2: トークンAPI実装

- [ ] POST `/api/v1/rooms/{id}/token` - 参加トークン発行
- [ ] `internal/auth/token.go` - トークン生成ロジック
- [ ] トークンクレーム設定（sub、iat、exp、iss、aud、room_id、role、permissions）

**コミットメッセージ例**: `feat(api): implement token generation API`

## Phase 3: 品質最適化

### 3.1 Simulcast対応

#### Task 3.1.1: Simulcastコントローラの実装

- [ ] `internal/media/simulcast/controller.go` - Simulcast制御
- [ ] レイヤー定義（h: 1280x720/2.5Mbps、m: 640x360/500Kbps、l: 320x180/150Kbps）
- [ ] RID解析

**コミットメッセージ例**: `feat(simulcast): implement simulcast controller`

#### Task 3.1.2: レイヤー選択の実装

- [ ] `internal/media/simulcast/layer.go` - レイヤー管理
- [ ] setPreferredLayer 対応（クライアント要求最優先）
- [ ] 自動レイヤー切り替え（クライアント要求範囲内）
- [ ] layerChanged 通知（requestedLayer、actualLayer、reason）
- [ ] 要求レイヤー不存在時の次善レイヤー選択

**コミットメッセージ例**: `feat(simulcast): implement layer selection and switching`

### 3.2 RTCP処理

#### Task 3.2.1: RTCPハンドラの実装

- [ ] `internal/media/rtcp/handler.go` - RTCPパケット処理
- [ ] Receiver Report (RR) 集約・転送
- [ ] パケット統計収集

**コミットメッセージ例**: `feat(rtcp): implement RTCP handler with receiver reports`

#### Task 3.2.2: TWCC処理の実装

- [ ] `internal/media/rtcp/twcc.go` - TWCC処理（優先）
- [ ] 帯域幅推定
- [ ] フィードバック生成
- [ ] 更新間隔: 100ms

**コミットメッセージ例**: `feat(rtcp): implement TWCC congestion control`

#### Task 3.2.3: REMB処理の実装

- [ ] `internal/media/rtcp/remb.go` - REMBフォールバック
- [ ] TWCC非対応クライアント向け帯域幅推定
- [ ] TWCC/REMB両方受信時はTWCC採用

**コミットメッセージ例**: `feat(rtcp): implement REMB fallback for legacy clients`

#### Task 3.2.4: NACK処理の実装

- [ ] `internal/media/rtcp/nack.go` - NACK処理
- [ ] パケットロス検出
- [ ] 再送要求（パケットロス検出から10ms以内）
- [ ] RTX対応（再送専用ストリーム）

**コミットメッセージ例**: `feat(rtcp): implement NACK handling with RTX support`

#### Task 3.2.5: PLI/FIR処理の実装

- [ ] PLI転送（Picture Loss Indication）
- [ ] FIR転送（Full Intra Request）
- [ ] キーフレーム要求

**コミットメッセージ例**: `feat(rtcp): implement PLI and FIR forwarding`

### 3.3 品質制御

#### Task 3.3.1: 帯域幅推定の実装

- [ ] TWCC/REMB ハイブリッド対応
- [ ] 帯域幅推定更新（100ms間隔）
- [ ] どちらも未受信時のデフォルト帯域幅使用

**コミットメッセージ例**: `feat(quality): implement bandwidth estimation with TWCC/REMB`

#### Task 3.3.2: 自動品質調整の実装

- [ ] パケットロス率監視（閾値: 5%で低レイヤー切り替え）
- [ ] RTT監視（閾値: 300msで低レイヤー切り替え）
- [ ] 帯域幅不足時の低レイヤー切り替え
- [ ] 品質回復時の上位レイヤー復帰（パケットロス率<1%かつ帯域幅余裕時）

**コミットメッセージ例**: `feat(quality): implement automatic quality adjustment`

#### Task 3.3.3: 接続品質算出・通知の実装

- [ ] `internal/media/quality/calculator.go` - 接続品質計算
- [ ] RTCP統計（パケットロス率、RTT、ジッタ）から接続品質を算出
- [ ] 品質レベル判定（excellent/good/fair/poor）
- [ ] 品質変化時のconnectionQualityChanged通知発火
- [ ] 参加者ごとの品質追跡

> **注記**: 接続品質はサーバー側で計算し、クライアントに通知する。Task 1.4.9のconnectionQualityChanged通知と連携。

**コミットメッセージ例**: `feat(quality): implement connection quality calculation and notification`

### 3.4 メディアルーター（拡張）

#### Task 3.4.1: メディアルーター拡張の実装

- [ ] 購読管理の拡張
- [ ] RTPパケット転送最適化
- [ ] マルチSubscriber対応

**コミットメッセージ例**: `feat(media): extend media router for multi-subscriber`

#### Task 3.4.2: パブリッシャーの実装

- [ ] `internal/media/publisher.go` - パブリッシャー管理
- [ ] トラック追加・削除
- [ ] メタデータ管理
- [ ] Simulcastレイヤー管理

**コミットメッセージ例**: `feat(media): implement publisher track management`

#### Task 3.4.3: サブスクライバーの実装

- [ ] `internal/media/subscriber.go` - サブスクライバー管理
- [ ] 購読作成・解除
- [ ] レイヤー選択
- [ ] subscriptionId管理

**コミットメッセージ例**: `feat(media): implement subscriber with layer selection`

## Phase 4: 運用機能

### 4.1 メトリクス

#### Task 4.1.1: Prometheusメトリクスの実装

- [ ] `pkg/metrics/prometheus.go` - メトリクス定義
- [ ] sfu_rooms_total（Gauge）
- [ ] sfu_connections_total（Gauge）
- [ ] sfu_connections_per_room（Histogram）
- [ ] sfu_track_count（Gauge、room_id/kind別）
- [ ] sfu_subscription_count（Gauge、room_id別）
- [ ] sfu_bytes_received_total / sfu_bytes_sent_total（Counter）
- [ ] sfu_packets_received_total / sfu_packets_sent_total / sfu_packets_lost_total（Counter）
- [ ] sfu_rtt_seconds / sfu_jitter_seconds（Histogram）
- [ ] sfu_bitrate_bps / sfu_simulcast_layer（Gauge）

**コミットメッセージ例**: `feat(metrics): implement Prometheus metrics`

#### Task 4.1.2: メトリクスエンドポイントの実装

- [ ] GET `/metrics` - Prometheusメトリクス公開
- [ ] メトリクス収集ポイントの追加
- [ ] ラベル設定（room_id、participant_id、kind等）

**コミットメッセージ例**: `feat(metrics): expose metrics endpoint`

#### Task 4.1.3: 監視・アラート設定の整備

- [ ] Prometheus ServiceMonitor設定（k8s/servicemonitor.yaml）
- [ ] Grafanaダッシュボード作成（grafana/dashboards/sfu.json）
- [ ] Cloud Monitoringアラートポリシー設定
  - CPU使用率 > 80%（Warning）、> 95%（Critical）
  - メモリ使用率 > 80%（Warning）
  - Pod再起動 > 3回/10分（Warning）
  - エラーレート > 1%（Warning）
  - 接続失敗率 > 5%（Critical）
- [ ] アラート通知設定（Slack、PagerDuty）

**コミットメッセージ例**: `feat(monitoring): configure alerting and dashboards`

### 4.2 録画機能（オプション）

#### Task 4.2.1: レコーダーの実装

- [ ] `internal/recording/recorder.go` - 録画制御
- [ ] 録画開始・停止
- [ ] WebM形式出力（VP8/VP9 + Opus）- 再エンコード不要
- [ ] MP4形式出力（H.264 + AAC）オプション - 音声トランスコード

**コミットメッセージ例**: `feat(recording): implement media recorder`

#### Task 4.2.2: 録画仕様の実装

- [ ] 音声/映像同期精度（±50ms）
- [ ] ファイル分割（1時間ごと、または1GB到達時）
- [ ] ファイル命名規則（{roomId}_{trackId}_{timestamp}.webm）
- [ ] 一時保存先管理（/tmp/recordings/）

**コミットメッセージ例**: `feat(recording): implement recording specifications`

#### Task 4.2.3: ストレージの実装

- [ ] `internal/recording/storage/interface.go` - ストレージインターフェース
- [ ] `internal/recording/storage/local.go` - ローカル保存（開発用）
- [ ] `internal/recording/storage/gcs.go` - GCS保存（本番用、標準）
- [ ] `internal/recording/storage/s3.go` - S3保存（オプション、AWS環境向け）
- [ ] 保持期間管理（30日、設定可能）

> **注記**: deployment.mdに従い、本番環境ではGCSを標準とする。S3はAWS環境向けのオプション。

**コミットメッセージ例**: `feat(recording): implement recording storage with GCS/S3 support`

#### Task 4.2.4: 録画対象選択の実装

- [ ] ルーム全体の録画
- [ ] 特定参加者のみ録画
- [ ] 録画対象トラック選択

**コミットメッセージ例**: `feat(recording): implement recording target selection`

#### Task 4.2.5: 録画法的要件の実装

- [ ] 録画開始時の参加者への通知（recordingStarted）
- [ ] 録画同意フラグの管理（RecordingConsent）
- [ ] 録画メタデータの保存（開始時刻、参加者リスト、同意状況）
- [ ] recordingStarted/recordingStopped 通知

**コミットメッセージ例**: `feat(recording): implement recording legal requirements`

#### Task 4.2.6: 録画APIの実装

- [ ] POST `/api/v1/rooms/{id}/recording` - 録画開始
- [ ] DELETE `/api/v1/rooms/{id}/recording` - 録画停止
- [ ] GET `/api/v1/rooms/{id}/recording` - 録画状態取得
- [ ] GET `/api/v1/rooms/{id}/recordings` - 録画一覧取得
- [ ] GET `/api/v1/recordings/{recordingId}` - 録画詳細取得

**コミットメッセージ例**: `feat(recording): implement recording REST API`

#### Task 4.2.7: 録画アップロードワーカーの実装

- [ ] `internal/recording/uploader.go` - アップロードワーカー
- [ ] 録画ファイルの非同期アップロード
- [ ] metadata.json作成とアップロード（参加者リスト、開始時刻、同意状況）
- [ ] アップロード失敗時のリトライ（指数バックオフ）
- [ ] アップロードキュー管理

> **注記**: GCSストレージ実装はTask 4.2.3に統合。本タスクはアップロードワーカーとメタデータ管理に特化。ファイル分割はTask 4.2.2で実装。

**コミットメッセージ例**: `feat(recording): implement upload worker with retry`

### 4.3 クライアントSDK

#### Task 4.3.1: TypeScript SDK基盤

- [ ] パッケージ初期化（npm init）
- [ ] TypeScript設定
- [ ] ビルド設定（rollup/esbuild）
- [ ] 型定義ファイル生成

**コミットメッセージ例**: `feat(sdk): initialize TypeScript SDK project`

#### Task 4.3.2: SFUClientの実装

- [ ] SFUClient クラス
- [ ] 接続管理
- [ ] 自動再接続（ReconnectConfig対応）

**コミットメッセージ例**: `feat(sdk): implement SFUClient with auto-reconnect`

#### Task 4.3.3: シグナリングクライアントの実装

- [ ] SignalingClient クラス
- [ ] WebSocket通信
- [ ] JSON-RPC処理

**コミットメッセージ例**: `feat(sdk): implement SignalingClient with JSON-RPC`

#### Task 4.3.4: Roomクラスの実装

- [ ] Room クラス
- [ ] 参加者管理
- [ ] イベントエミッター（RoomEvents対応）

**コミットメッセージ例**: `feat(sdk): implement Room class with event emitter`

#### Task 4.3.5: トラック管理の実装

- [ ] LocalTrack/RemoteTrack クラス
- [ ] メディアストリーム管理
- [ ] Simulcastレイヤー制御（setPreferredLayer）

**コミットメッセージ例**: `feat(sdk): implement LocalTrack and RemoteTrack classes`

#### Task 4.3.6: React Hooks（オプション）

- [ ] useSFUClient
- [ ] useRoom
- [ ] useLocalMedia
- [ ] useRemoteTrack
- [ ] useParticipants
- [ ] useScreenShare

**コミットメッセージ例**: `feat(sdk): implement React hooks for SFU client`

#### Task 4.3.7: SDK追加モジュールの実装

- [ ] `Participant` クラス（LocalParticipant/RemoteParticipant）
- [ ] `JsonRpcClient` クラス（JSON-RPC通信）
- [ ] `MediaDevices` クラス（デバイス管理）
- [ ] `SDPUtils` クラス（SDP操作ユーティリティ）
- [ ] `ICEManager` クラス（ICE管理）
- [ ] `EventEmitter` クラス（イベント基盤）
- [ ] `SFUError` クラス（エラー定義）
- [ ] リトライロジック（retry.ts）

**コミットメッセージ例**: `feat(sdk): implement additional SDK modules`

#### Task 4.3.8: SDK接続品質機能の実装

- [ ] サーバーからのconnectionQualityChanged通知の受信・イベント発火
- [ ] 接続品質に基づくSimulcastレイヤー自動調整（オプション機能）
- [ ] ConnectionQuality型の定義とエクスポート

> **注記**: 接続品質はサーバー側で計算されて通知される。SDKはサーバー通知を受け取りイベントとして発火する。

**コミットメッセージ例**: `feat(sdk): implement connection quality event handling`

### 4.4 ドキュメント整備

#### Task 4.4.1: OpenAPI仕様書

- [ ] `api/openapi.yaml` - REST API仕様
- [ ] エンドポイント定義
- [ ] リクエスト/レスポンススキーマ
- [ ] lock/unlock API追加（POST/DELETE `/api/v1/rooms/{id}/lock`）
- [ ] 録画一覧/詳細API追加（GET `/api/v1/rooms/{id}/recordings`、GET `/api/v1/recordings/{recordingId}`）

**コミットメッセージ例**: `docs(api): add OpenAPI specification`

#### Task 4.4.2: デプロイメントガイド

- [ ] Dockerイメージ作成
- [ ] docker-compose.yaml
- [ ] Kubernetesマニフェスト
- [ ] 環境変数一覧

**コミットメッセージ例**: `docs(deploy): add deployment guide with Docker and Kubernetes`

#### Task 4.4.3: JSON-RPC Schema維持・検証

- [ ] `api/jsonrpc-schema.json` の更新・維持
- [ ] 新規メソッド/通知のスキーマ追加（lock/unlock等）
- [ ] スキーマバリデーションの統合テスト
- [ ] スキーマとコード実装の整合性チェック

> **注記**: api/jsonrpc-schema.jsonは既存ファイル。新機能追加時にスキーマを更新し、整合性を維持する。

**コミットメッセージ例**: `docs(api): update JSON-RPC schema definitions`

#### Task 4.4.4: 本番設定ファイル

- [ ] `configs/config.production.yaml` - 本番環境設定例
- [ ] セキュリティ設定のベストプラクティス
- [ ] パフォーマンスチューニング設定

**コミットメッセージ例**: `chore(config): add production configuration example`

### 4.5 分散システム対応

#### Task 4.5.1: Redisルームストアの実装

- [ ] `internal/store/room_store.go` - 分散ルームレジストリ
- [ ] ルーム情報のRedis保存
- [ ] server_idマッピング（ロードバランシング用）
- [ ] ルーム検索・一覧取得
- [ ] 明示的なルーム削除（TTLなし、ADR-0005準拠）

> **注記**: ADR-0005に従い、room:{room_id}はTTLを設定せず明示的に削除する。セッションストアとは異なる方針。

**コミットメッセージ例**: `feat(store): implement Redis-backed distributed room registry`

### 4.6 将来対応機能（オプション）

#### Task 4.6.1: SVC対応（VP9/AV1）

- [ ] VP9 SVCレイヤー管理
- [ ] temporal/spatial レイヤー選択
- [ ] SVC有効/無効の機能フラグ

**コミットメッセージ例**: `feat(media): implement SVC support for VP9/AV1`

#### Task 4.6.2: E2EEクライアント連携

- [ ] E2EE対応のメディア処理（RTPヘッダーのみ参照）
- [ ] クライアントSDKへのE2EEフック提供
- [ ] Insertable Streams API対応ガイド

**コミットメッセージ例**: `feat(sdk): add E2EE client integration hooks`

## テストタスク

各Phaseの実装完了後、以下のテストを実施する。

### Phase 1 テスト

#### Task T1.1: シグナリングテスト

- [ ] JSON-RPCメッセージのシリアライズ/デシリアライズテスト
- [ ] 基本メソッドハンドラのユニットテスト（join/leave/offer/answer/candidate）
- [ ] メディアメソッドハンドラのユニットテスト（publish/unpublish/subscribe/unsubscribe/setPreferredLayer）
- [ ] 通知送信のユニットテスト（joined/left/participantJoined/trackPublished等）
- [ ] WebSocket接続/切断の統合テスト

**コミットメッセージ例**: `test(signaling): add signaling handler unit and integration tests`

#### Task T1.2: SDP/ICEテスト

- [ ] SDPパース・生成のユニットテスト
- [ ] Unified Plan対応の検証テスト
- [ ] ICE候補生成のユニットテスト
- [ ] ICE候補優先順位のテスト（host > srflx > relay）
- [ ] IPv4/IPv6デュアルスタック優先制御のテスト
- [ ] 複数ICEサーバーフォールバックのテスト
- [ ] joinレスポンスiceServers組み立てのテスト
- [ ] Safari互換性のテスト（SDP正規化）

**コミットメッセージ例**: `test(webrtc): add SDP and ICE unit tests`

#### Task T1.3: RTP/RTCP基盤テスト

- [ ] RTPパケット処理のユニットテスト
- [ ] SSRC管理のユニットテスト
- [ ] シーケンス番号書き換えのテスト
- [ ] タイムスタンプ正規化のテスト
- [ ] RTP拡張ヘッダー処理のテスト（MID/RID）
- [ ] パケットペーシングのテスト
- [ ] ジッタバッファ動作のテスト

**コミットメッセージ例**: `test(media): add RTP processing unit tests`

### Phase 2 テスト

#### Task T2.1: 再接続フローテスト

- [ ] セッション保存・復元の統合テスト
- [ ] 再接続時の状態復元テスト
- [ ] セッションタイムアウトのテスト
- [ ] 指数バックオフのテスト

**コミットメッセージ例**: `test(session): add reconnection flow integration tests`

### Phase 3 テスト

#### Task T3.1: RTCP処理テスト

- [ ] TWCCフィードバック生成・処理のテスト
- [ ] REMBフィードバック処理のテスト
- [ ] NACK処理・再送のテスト
- [ ] PLI/FIR転送のテスト

**コミットメッセージ例**: `test(rtcp): add RTCP processing unit tests`

#### Task T3.2: Simulcastテスト

- [ ] レイヤー選択ロジックのユニットテスト
- [ ] 自動レイヤー切り替えのテスト
- [ ] layerChanged通知のテスト

**コミットメッセージ例**: `test(simulcast): add simulcast layer selection tests`

### E2E・負荷・セキュリティテスト

#### Task T4.1: E2Eテスト基盤の整備

- [ ] Playwright/Puppeteerによるブラウザ自動化環境構築
- [ ] テスト用固定メディアファイル準備（fixtures/media/）
- [ ] 2者間通話シナリオテスト
- [ ] 多人数会議シナリオテスト（5人）
- [ ] 途中参加・途中退出シナリオテスト
- [ ] ネットワーク切断復旧シナリオテスト
- [ ] クロスブラウザテスト（Chrome/Firefox/Safari）

**コミットメッセージ例**: `test(e2e): implement end-to-end test infrastructure`

#### Task T4.2: 負荷テストの実装

- [ ] k6によるREST API負荷テスト環境構築
- [ ] WebSocket/WebRTC負荷テストクライアント実装（Go + pion）
- [ ] 同時接続数テスト（100接続/ルーム）
- [ ] ルーム数スケールテスト（50ルーム同時稼働）
- [ ] メッセージスループットテスト（1000msg/秒）
- [ ] メディアスループットテスト（1Gbps）
- [ ] 長時間稼働テスト（24時間連続）

**コミットメッセージ例**: `test(load): implement load testing infrastructure`

#### Task T4.3: セキュリティテストの実装

- [ ] 認証バイパス試行テスト
- [ ] 権限昇格試行テスト
- [ ] トークン改ざんテスト
- [ ] 入力バリデーションテスト
- [ ] JSONインジェクションテスト
- [ ] WebSocket DDoSテスト
- [ ] 不正SDPインジェクションテスト
- [ ] パストラバーサルテスト

**コミットメッセージ例**: `test(security): implement security tests`

#### Task T4.4: 互換性テストの実装

- [ ] ブラウザ互換性テスト（Chrome/Firefox/Safari/Edge/iOS Safari/Android Chrome）
- [ ] コーデック互換性テスト（VP8/VP9/H.264/Opus）
- [ ] Safari互換性テスト（H.264優先）

**コミットメッセージ例**: `test(compatibility): implement browser compatibility tests`

## 依存関係図

```text
Phase 1: 基本機能
├── 1.1 プロジェクト初期設定 ← 最初に実施
│   ├── 1.1.1 プロジェクト構造
│   ├── 1.1.2 設定管理
│   └── 1.1.3 ログ基盤
├── 1.2 HTTPサーバー基盤 ← 1.1に依存
│   ├── 1.2.1 HTTPサーバー
│   ├── 1.2.2 ミドルウェア
│   └── 1.2.3 レート制限
├── 1.3 認証機能 ← 1.2に依存
│   ├── 1.3.1 JWT検証
│   ├── 1.3.2 JWKS取得
│   ├── 1.3.3 JWT失効管理
│   └── 1.3.4 権限管理
├── 1.4 シグナリング基盤 ← 1.2, 1.3に依存
│   ├── 1.4.1 WebSocketハンドラ
│   ├── 1.4.2 WebSocketレート制限
│   ├── 1.4.3 JSON-RPCプロトコル
│   ├── 1.4.4 メッセージディスパッチャ
│   ├── 1.4.5 基本メソッドハンドラ
│   ├── 1.4.6 メディアメソッドハンドラ
│   ├── 1.4.7 サーバー通知
│   └── 1.4.8 追加サーバー通知
├── 1.5 WebRTC基盤 ← 1.1に依存
│   ├── 1.5.1 PeerConnection管理
│   ├── 1.5.2 SDP処理
│   ├── 1.5.3 必須RTCPフィードバック設定
│   ├── 1.5.4 コーデック設定
│   ├── 1.5.5 RTPヘッダー拡張設定
│   ├── 1.5.6 ICE設定
│   ├── 1.5.7 ICE再起動
│   └── 1.5.8 TURN資格情報管理
├── 1.6 メディア基盤 ← 1.5に依存
│   ├── 1.6.1 トラック管理基盤
│   ├── 1.6.2 基本メディアルーター
│   ├── 1.6.3 RTP処理基盤
│   └── 1.6.4 RTP拡張処理
└── 1.7 1:1接続 ← 1.4, 1.5, 1.6に依存
    ├── 1.7.1 Offer/Answer交換
    ├── 1.7.2 メディアトラック受信
    └── 1.7.3 メディアトラック転送

Phase 2: ルーム機能 ← Phase 1完了後
├── 2.1 ルーム管理
├── 2.2 セッション管理
├── 2.3 再接続機能 ← 2.2に依存
└── 2.4 REST API ← 2.1に依存

Phase 3: 品質最適化 ← Phase 2完了後
├── 3.1 Simulcast対応
├── 3.2 RTCP処理
│   ├── 3.2.1 RTCPハンドラ
│   ├── 3.2.2 TWCC処理
│   ├── 3.2.3 REMB処理
│   ├── 3.2.4 NACK処理
│   └── 3.2.5 PLI/FIR処理
├── 3.3 品質制御 ← 3.2に依存
└── 3.4 メディアルーター（拡張） ← 3.1, 3.3に依存

Phase 4: 運用機能 ← Phase 3完了後
├── 4.1 メトリクス
├── 4.2 録画機能（オプション）
│   ├── 4.2.1 レコーダー
│   ├── 4.2.2 録画仕様
│   ├── 4.2.3 ストレージ
│   ├── 4.2.4 録画対象選択
│   ├── 4.2.5 録画法的要件
│   └── 4.2.6 録画API
├── 4.3 クライアントSDK
└── 4.4 ドキュメント整備
```

## 見積もり参考情報

| Phase                         | タスク数 | 複雑度 |
| ----------------------------- | -------- | ------ |
| Phase 1                       | 32       | 高     |
| Phase 2                       | 12       | 中     |
| Phase 3                       | 12       | 高     |
| Phase 4                       | 24       | 中     |
| テストタスク（ユニット/統合） | 5        | 中     |
| テストタスク（E2E/負荷等）    | 4        | 高     |
| 将来対応機能（オプション）    | 2        | 中     |
| 合計                          | 91       | -      |

## 注意事項

1. 各タスクは独立してテスト可能な単位で分割されている
2. コミットはタスク単位で行い、レビュー後にマージする
3. Phase間の依存関係を守り、順序どおりに実装する
4. オプション機能（録画、React Hooks等）は必要に応じてスキップ可能
5. セキュリティに関わる実装（認証、認可、TLS）は特に慎重にレビューする
6. 要件定義書/設計書との整合性を常に確認する
