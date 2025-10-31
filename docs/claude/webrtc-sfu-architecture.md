# WebRTC SFU のアーキテクチャと実装詳解

## 1. WebRTC SFU とは

### 1.1 SFU (Selective Forwarding Unit) の基本概念

SFU（Selective Forwarding Unit）は、WebRTCにおける多対多の通信を実現するためのアーキテクチャの一つです。複数の参加者間でメディアストリームを効率的に配信するために、サーバー側でメディアパケットを選択的に転送します。

**主要な3つのWebRTC配信モデル：**

1. **Mesh（P2P）**: 各クライアントが他のすべてのクライアントと直接接続
   - 利点: レイテンシが低い、サーバー不要
   - 欠点: 参加者数に比例して帯域幅とCPU負荷が増大（N-1接続）

2. **MCU (Multipoint Control Unit)**: サーバーで全ストリームをデコード・ミキシング・再エンコード
   - 利点: クライアント負荷が低い
   - 欠点: サーバー側の計算コストが非常に高い

3. **SFU (Selective Forwarding Unit)**: サーバーがメディアパケットを選択的に転送
   - 利点: 計算コストが低い、スケーラブル、柔軟な品質制御
   - 欠点: Meshより帯域幅消費が多い（アップリンク1本、ダウンリンクN-1本）

### 1.2 SFUの動作原理

SFUは以下の特徴を持ちます：

- **メディアの非加工転送**: 受信したRTPパケットを再エンコードせずに転送
- **選択的転送**: 各受信者に対して最適なストリーム（品質レイヤー）を選択
- **帯域制御**: 各クライアントの帯域幅に応じて適切な品質を配信
- **シグナリング制御**: WebRTCのネゴシエーション（SDP交換）を仲介

## 2. LiveKit SFU の全体アーキテクチャ

### 2.1 主要コンポーネント

LiveKitのSFU実装は、以下の主要コンポーネントで構成されています：

```
┌─────────────────────────────────────────────────────────────┐
│                      LiveKit Room                            │
│  ┌─────────────────────────────────────────────────────┐   │
│  │         Room (pkg/rtc/room.go)                      │   │
│  │  - 参加者管理                                        │   │
│  │  - トラック管理                                      │   │
│  │  - ルーム状態管理                                    │   │
│  └────────────┬────────────────────────────────────────┘   │
│               │                                              │
│  ┌────────────┴─────────────────┬──────────────────────┐   │
│  │ Publisher (WebRTCReceiver)   │ Subscriber           │   │
│  │ ┌──────────────────────┐    │ ┌─────────────────┐  │   │
│  │ │ pkg/sfu/receiver.go  │    │ │ pkg/sfu/        │  │   │
│  │ │                      │    │ │ downtrack.go    │  │   │
│  │ │ - RTP受信           │    │ │                 │  │   │
│  │ │ - バッファリング     │    │ │ - RTP送信       │  │   │
│  │ │ - レイヤー管理       │◄───┼─┤ - 品質適応      │  │   │
│  │ └──────────────────────┘    │ │ - 帯域制御      │  │   │
│  │                              │ └─────────────────┘  │   │
│  └──────────────────────────────┴──────────────────────┘   │
└─────────────────────────────────────────────────────────────┘
```

### 2.2 データフローの概要

```
Publisher → Receiver → Buffer → Forwarder → DownTrack → Subscriber
   (送信)    (受信)    (蓄積)   (選択/変換)  (配信)      (受信)
```

## 3. 主要コンポーネントの詳細

### 3.1 Room（pkg/rtc/room.go）

Roomは会議空間を表す最上位のコンポーネントです。

**主な責務:**

- 参加者（Participant）のライフサイクル管理
- トラック（音声/映像ストリーム）の管理
- ルームメタデータの管理
- イベントの配信

**キー機能:**

```go
type Room struct {
    protoRoom    *livekit.Room
    participants map[ParticipantIdentity]LocalParticipant
    trackManager *RoomTrackManager
    bufferFactory *buffer.FactoryOfBufferFactory
}
```

### 3.2 WebRTCReceiver（pkg/sfu/receiver.go）

パブリッシャーからメディアを受信するコンポーネント。

**主な責務:**

- RTPパケットの受信
- 複数の品質レイヤー（Simulcast/SVC）の管理
- パケットバッファリングとNACK処理
- RTCPフィードバックの処理

**アーキテクチャ:**

```go
type WebRTCReceiver struct {
    buffers [DefaultMaxLayerSpatial + 1]*buffer.Buffer
    upTracks [DefaultMaxLayerSpatial + 1]TrackRemote
    streamTrackerManager *StreamTrackerManager
    downTrackSpreader *DownTrackSpreader
}
```

**品質レイヤー管理:**

- Simulcastの場合: 各レイヤーが独立したトラック（低・中・高品質）
- SVCの場合: 1つのトラックに複数のレイヤーが含まれる
- Dependency Descriptorを使用した詳細なレイヤー情報の解析

### 3.3 Buffer（pkg/sfu/buffer/buffer.go）

RTPパケットをバッファリングし、再送制御を行うコンポーネント。

**主な責務:**

- パケットの順序制御
- NACK（Negative Acknowledgement）による再送要求処理
- RTCPフィードバック生成
- パケットロス検出

**キー機能:**

```go
type Buffer struct {
    bucket *bucket.Bucket[uint64]    // パケットストレージ
    nacker *nack.NackQueue            // NACK管理
    extPackets deque.Deque[*ExtPacket] // 拡張パケットキュー
    rtpStats *rtpstats.RTPStatsReceiver
}
```

**パケット再送メカニズム:**

1. ダウンストリームからNACK要求を受信
2. Bufferから該当パケットを検索
3. 見つかった場合は再送、見つからない場合はアップストリームに要求

### 3.4 Forwarder（pkg/sfu/forwarder.go）

受信したメディアを適切に変換・転送するコンポーネント。

**主な責務:**

- RTPヘッダーの書き換え（Sequence Number、Timestamp、SSRC）
- レイヤースイッチング制御
- 帯域幅適応
- キーフレーム要求管理

**レイヤースイッチング:**

```go
type Forwarder struct {
    muted bool
    pubMuted bool
    targetLayer VideoLayer
    currentLayer VideoLayer
    maxLayer VideoLayer

    // RTPヘッダー変換用
    lastSSRC uint32
    snOffset int64
    tsOffset int64
}
```

**スイッチング戦略:**

- キーフレーム待機: 高品質への切り替えはキーフレームで実行
- シームレスダウンスイッチ: 低品質への切り替えは即座に実行可能
- Temporal Layer活用: 同一Spatialレイヤー内でのスムーズな変更

### 3.5 DownTrack（pkg/sfu/downtrack.go）

各サブスクライバーへの配信を管理するコンポーネント。

**主な責務:**

- RTPパケットの送信
- サブスクライバー固有の品質制御
- RTCPフィードバック処理
- 統計情報収集

**キー機能:**

```go
type DownTrack struct {
    forwarder *Forwarder
    receiver TrackReceiver

    // 送信制御
    writable atomic.Bool
    pacer pacer.Pacer

    // 統計
    rtpStats *rtpstats.RTPStatsSender
    connectionStats *connectionquality.ConnectionStats
}
```

**品質適応メカニズム:**

- RTCPフィードバック（REMB、Transport-CC）に基づく帯域推定
- パケットロス率に基づく品質調整
- RTT（Round Trip Time）を考慮した制御

### 3.6 StreamAllocator（pkg/sfu/streamallocator/streamallocator.go）

帯域幅をトラック間で効率的に配分するコンポーネント。

**主な責務:**

- 利用可能な帯域幅の推定
- トラック間の優先度付け
- 動的な品質調整
- プローブ（帯域探索）の実行

**アルゴリズム:**

```go
// 帯域配分の優先順位
1. オーディオトラック（最優先）
2. スクリーンシェア
3. ビデオ（カメラ）

// 配分戦略
- Cooperative: 全トラックで均等に品質を下げる
- Greedy: 可能な限り高品質を維持
- Priority-based: 優先度に基づいた配分
```

## 4. データフローの詳細

### 4.1 アップストリーム（Publisher → SFU）

```
1. WebRTC PeerConnection (Publisher側)
   ↓ RTP packets
2. Transport (WebRTC)
   ↓ デマルチプレクシング (SSRC, PayloadType)
3. WebRTCReceiver.forwardRTP()
   ↓ パケット解析、拡張ヘッダー抽出
4. Buffer.write()
   ↓ バッファリング、統計更新
5. Buffer.ReadExtended()
   ↓ キューから取り出し
6. DownTrackSpreader.Broadcast()
   ↓ 全サブスクライバーへ配信
7. DownTrack.WriteRTP()
```

### 4.2 ダウンストリーム（SFU → Subscriber）

```
1. DownTrack.WriteRTP()
   ↓ 品質レイヤー判定
2. Forwarder.GetTranslationParams()
   ↓ RTPヘッダー変換パラメータ生成
3. ヘッダー書き換え
   - Sequence Number: 連続性を保つよう変換
   - Timestamp: レイヤースイッチ時に調整
   - SSRC: DownTrack固有のSSRCに変換
4. Pacer.Enqueue()
   ↓ ペーシング（送信レート制御）
5. WriteStream
   ↓ WebRTC Transport
6. WebRTC PeerConnection (Subscriber側)
```

### 4.3 RTCP フィードバックループ

```
Subscriber
   ↓ RTCP Receiver Report, NACK, PLI, REMB, Transport-CC
DownTrack.handleRTCP()
   ↓ 統計更新、品質推定
StreamAllocator
   ↓ 帯域配分計算
DownTrack.AllocateOptimal()
   ↓ レイヤー選択
Forwarder.SetMaxLayer()
   ↓ 必要に応じてキーフレーム要求
WebRTCReceiver.SendPLI()
   ↓ RTCP PLI (Picture Loss Indication)
Publisher
```

## 5. 重要な技術要素

### 5.1 Simulcast

Simulcastは、パブリッシャーが同じコンテンツを複数の品質で同時に送信する技術です。

**LiveKitでの実装:**

- 3つの品質レイヤー（低・中・高）を送信
- 各レイヤーは独立したRTPストリーム（異なるSSRC）
- サーバー側で適切なレイヤーを選択して転送

**品質レイヤーの例:**

```
Low:    160x90  @ 150 kbps
Medium: 320x180 @ 500 kbps
High:   640x360 @ 1500 kbps
```

### 5.2 SVC (Scalable Video Coding)

SVCは、1つのストリームに複数の品質レイヤーを埋め込む技術です。

**特徴:**

- Spatial Layer: 解像度の違い
- Temporal Layer: フレームレートの違い
- 依存関係を持つレイヤー構造
- Dependency Descriptor拡張により詳細な制御

**VP9 SVCの例:**

```
S2T2: 720p @ 30fps (full)
S2T1: 720p @ 15fps (depends on S2T0)
S2T0: 720p @ 7.5fps (base)
S1T2: 360p @ 30fps (depends on S1T0, S2T2)
S1T1: 360p @ 15fps (depends on S1T0)
S1T0: 360p @ 7.5fps (base)
S0T0: 180p @ 7.5fps (base)
```

### 5.3 帯域推定 (BWE: Bandwidth Estimation)

**2つの方式:**

1. **REMB (Receiver Estimated Maximum Bitrate)**
   - 受信側が帯域を推定してRTCPで通知
   - パケットロス、遅延、ジッターから計算
   - やや古い方式

2. **Transport-CC (Transport-wide Congestion Control)**
   - 各パケットに連番を付与
   - 受信時刻をRTCPで返送
   - 送信側で詳細な帯域推定
   - より正確で推奨される方式

**LiveKitの実装:**

```go
// pkg/sfu/bwe/
- remotebwe/: REMB方式の実装
- sendsidebwe/: Transport-CC方式の実装
```

### 5.4 パケットペーシング（Pacing）

パケットを均等なレートで送信し、バースト送信を避ける技術。

**効果:**

- ネットワーク輻輳の軽減
- スムーズな配信
- 帯域推定の精度向上

**実装:**

```go
// pkg/sfu/pacer/
type Pacer interface {
    Enqueue(packet *Packet)
    SetTargetBitrate(bitrate int64)
}
```

### 5.5 NACK と再送制御

**NACKメカニズム:**

1. 受信側がパケット欠損を検出（Sequence Number の穴）
2. RTCPでNACKを送信
3. 送信側がバッファから該当パケットを再送

**LiveKitの最適化:**

- Bufferでのパケット保持期間の調整
- 再送パケット数の制限
- RTXストリーム（専用の再送ストリーム）のサポート

### 5.6 キーフレーム要求

**PLI (Picture Loss Indication):**

- デコーダーがリカバリー不能な状態を検出
- キーフレーム（I-frame）を要求
- レイヤースイッチング時にも使用

**FIR (Full Intra Request):**

- より強力なキーフレーム要求
- LiveKitではPLIと同様に処理

**最適化:**

```go
// pkg/sfu/receiver.go
// PLIのスロットリング（過度な要求を防ぐ）
pliThrottleConfig := PLIThrottleConfig{
    LowQuality:  500 * time.Millisecond,
    MidQuality:  time.Second,
    HighQuality: time.Second,
}
```

## 6. スケーラビリティとパフォーマンス

### 6.1 並列処理

**DownTrackSpreader:**

- 多数のサブスクライバーへの配信を並列化
- しきい値を超えるとワーカープールを使用

```go
type DownTrackSpreader struct {
    threshold int // 並列化しきい値（デフォルト: 無効）
}
```

### 6.2 メモリ管理

**Packet Factory:**

```go
var PacketFactory = &sync.Pool{
    New: func() interface{} {
        b := make([]byte, 1460) // MTU考慮
        return &b
    },
}
```

- sync.Poolによるバッファ再利用
- GC圧力の軽減

### 6.3 バッファサイズの最適化

```go
// Video: 300パケット（約10秒分）
InitPacketBufferSizeVideo = 300

// Audio: 70パケット（約1.4秒分）
InitPacketBufferSizeAudio = 70
```

### 6.4 統計情報とモニタリング

**RTPStats:**

- パケット数、バイト数
- ロス率、ジッター
- RTT（往復遅延）

**ConnectionQuality:**

- スコアリングシステム（0-5）
- 品質低下の早期検出
- ユーザー体験の可視化

## 7. LiveKit SFU の特徴的な実装

### 7.1 Dependency Descriptor サポート

AV1やVP9の高度なSVC制御のための拡張。

**機能:**

- フレーム間依存関係の詳細な記述
- 柔軟なレイヤー選択
- より効率的なスイッチング

### 7.2 RED (Redundant Encoding)

音声パケットの冗長送信によるロバスト性向上。

**実装:**

```go
// pkg/sfu/redreceiver.go
// pkg/sfu/redprimaryreceiver.go
```

- Opus + REDのサポート
- パケットロスへの耐性向上

### 7.3 データチャネル対応

WebRTCのデータチャネルもサポート。

**用途:**

- チャットメッセージ
- メタデータ送信
- カスタムシグナリング

### 7.4 エージェント統合

AI/ML処理のためのエージェント機能。

**機能:**

- 音声認識
- リアルタイム翻訳
- コンテンツモデレーション

## 8. まとめ

LiveKitのSFU実装は、以下の特徴を持つ高度なメディアルーターです：

### 8.1 主要な強み

1. **高いスケーラビリティ**
   - 効率的なパケット転送
   - 最小限のトランスコーディング
   - 並列処理の最適化

2. **柔軟な品質制御**
   - Simulcast/SVCの完全サポート
   - 動的な帯域推定
   - 個別の品質適応

3. **信頼性**
   - 堅牢な再送メカニズム
   - パケットロス対策
   - 接続品質モニタリング

4. **最新技術対応**
   - Dependency Descriptor
   - Transport-wide CC
   - 最新コーデック（AV1, VP9, Opus）

### 8.2 アーキテクチャの利点

- **モジュール性**: 各コンポーネントが明確に分離
- **拡張性**: 新機能の追加が容易
- **テスタビリティ**: 単体テストが可能な設計
- **パフォーマンス**: メモリとCPUの効率的な使用

### 8.3 ユースケース

LiveKit SFUは以下のようなアプリケーションに適しています：

- ビデオ会議システム
- ライブストリーミング
- オンライン教育
- テレヘルス
- リモートコラボレーション

WebRTC SFUは、リアルタイムメディア配信において、品質・スケーラビリティ・コストのバランスが取れた優れたアーキテクチャであり、LiveKitはその実装のベストプラクティスを示しています。
