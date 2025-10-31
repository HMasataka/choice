# RTPストリームの多重化メカニズム

## 概要

WebRTC SFUでは、1つのPeerConnection（UDP接続）上で複数のメディアストリーム（音声・映像）を同時に送受信します。この多重化は**RTPヘッダーに含まれる識別子**を使って実現されています。

## 1. PeerConnectionの構成

### 基本構造

```
Client ←→ [1つのPeerConnection] ←→ SFU

PeerConnection内:
  - Transceiver 1: カメラ映像（send）
  - Transceiver 2: マイク音声（send）
  - Transceiver 3: 他の参加者Aの映像（receive）
  - Transceiver 4: 他の参加者Aの音声（receive）
  - Transceiver 5: 他の参加者Bの映像（receive）
  - Transceiver 6: 他の参加者Bの音声（receive）
  ...
```

### ポイント

- **1つのPeerConnectionで双方向通信**
- ICE、DTLS、SRTP接続は1回の確立で共有
- トランシーバー（Transceiver）による多重化
- 同じUDP接続上で複数のRTPストリームを多重化

## 2. RTPヘッダーの構造

### RTPパケットフォーマット

```
 0                   1                   2                   3
 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|V=2|P|X|  CC   |M|     PT      |       Sequence Number         |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|                           Timestamp                           |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|                          SSRC (★重要)                         |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|                             CSRC                              |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+

V (Version): RTPバージョン（常に2）
P (Padding): パディングの有無
X (Extension): 拡張ヘッダーの有無
CC (CSRC Count): CSRCの数
M (Marker): フレーム境界などのマーカー
PT (Payload Type): メディアタイプ識別子（例: VP8=96, Opus=111）
Sequence Number: パケットの連番（ロス検出用）
Timestamp: メディアのタイムスタンプ
SSRC: ストリームの一意識別子（最重要）
CSRC: 複数のソースからのミックス時に使用
```

## 3. ストリーム識別の主要要素

### 3.1 SSRC (Synchronization Source)

**最も重要な識別子**

- **32bitの一意な識別子**
- 各RTPストリームごとに異なる値
- ランダムに生成されるか、意図的に割り当てられる
- 受信側はSSRCを見てストリームを振り分ける

### 3.2 PT (Payload Type)

メディアタイプとコーデックを識別

- 7bitの値（0-127）
- SDPネゴシエーション時に動的に決定
- 例：
  - 96: VP8
  - 97: VP9
  - 111: Opus
  - 122: RTX (再送)

### 3.3 その他の識別要素

- **Sequence Number**: パケットの順序と欠損検出
- **Timestamp**: メディアのタイミング同期
- **MID/RID**: RTP拡張ヘッダーでの追加識別

## 4. 多重化の具体例

### 同じUDP接続上でのパケット送信

```
┌─────────────────────────────────────────┐
│ UDP Packet 1                            │
│ ├─ RTP Header                           │
│ │  ├─ SSRC: 0x12345678 ← 参加者Aの映像 │
│ │  ├─ PT: 96 (VP8)                     │
│ │  ├─ SeqNum: 1001                     │
│ │  └─ Timestamp: 90000                 │
│ └─ Payload: [Video Data]                │
└─────────────────────────────────────────┘

┌─────────────────────────────────────────┐
│ UDP Packet 2                            │
│ ├─ RTP Header                           │
│ │  ├─ SSRC: 0x87654321 ← 参加者Aの音声 │
│ │  ├─ PT: 111 (Opus)                   │
│ │  ├─ SeqNum: 5001                     │
│ │  └─ Timestamp: 48000                 │
│ └─ Payload: [Audio Data]                │
└─────────────────────────────────────────┘

┌─────────────────────────────────────────┐
│ UDP Packet 3                            │
│ ├─ RTP Header                           │
│ │  ├─ SSRC: 0xABCDEF00 ← 参加者Bの映像 │
│ │  ├─ PT: 96 (VP8)                     │
│ │  ├─ SeqNum: 2001                     │
│ │  └─ Timestamp: 90000                 │
│ └─ Payload: [Video Data]                │
└─────────────────────────────────────────┘
```

## 5. LiveKitの実装

### 5.1 受信側（デマルチプレクシング）

```go
// pkg/sfu/receiver.go
type WebRTCReceiver struct {
    mediaSSRC uint32  // このReceiverが処理するSSRC
    buffers [DefaultMaxLayerSpatial + 1]*buffer.Buffer
    upTracks [DefaultMaxLayerSpatial + 1]TrackRemote
}

// パケット受信時の処理
func (w *WebRTCReceiver) forwardRTP(layer int32, buff *buffer.Buffer) {
    for {
        pkt, err := buff.ReadExtended(pktBuf)
        if err == io.EOF {
            return
        }
        
        // SSRCでストリームを識別済み（バッファ作成時に紐付け）
        // PayloadTypeでメディアタイプを確認
        if pkt.Packet.PayloadType != uint8(w.codec.PayloadType) {
            // 期待と異なるPayloadTypeなので破棄
            continue
        }
        
        // 各ダウンストリームに配信
        w.downTrackSpreader.Broadcast(func(dt TrackSender) {
            _ = dt.WriteRTP(pkt, spatialLayer)
        })
    }
}

// バッファへのパケット書き込み
func (b *Buffer) Write(pkt *rtp.Packet) {
    // このバッファのSSRCと一致するかチェック
    if pkt.SSRC != b.mediaSSRC {
        return // 別のストリームなので無視
    }
    
    // パケットを保存
    b.bucket.AddPacket(&pkt)
}
```

### 5.2 送信側（SSRC書き換え）

```go
// pkg/sfu/downtrack.go
type DownTrack struct {
    ssrc    uint32  // このダウンストリーム用のSSRC
    ssrcRTX uint32  // 再送用のSSRC（別のストリーム）
    
    payloadType    atomic.Uint32  // メインストリームのPT
    payloadTypeRTX atomic.Uint32  // 再送ストリームのPT
}

// RTPパケット送信
func (d *DownTrack) WriteRTP(extPkt *buffer.ExtPacket, layer int32) error {
    // Forwarderから変換パラメータを取得
    tp, err := d.forwarder.GetTranslationParams(extPkt, layer)
    
    // RTPヘッダーを構築（新しいSSRCに変換）
    hdr := &rtp.Header{
        Version:        extPkt.Packet.Version,
        Padding:        extPkt.Packet.Padding,
        PayloadType:    d.getTranslatedPayloadType(extPkt.Packet.PayloadType),
        SequenceNumber: uint16(tp.rtp.extSequenceNumber),
        Timestamp:      uint32(tp.rtp.extTimestamp),
        SSRC:           d.ssrc,  // ★ ダウンストリーム固有のSSRCに変換
    }
    
    // パケット送信
    d.pacer.Enqueue(&pacer.Packet{
        Header:      hdr,
        Payload:     payload,
        WriteStream: d.writeStream,
    })
    
    return nil
}
```

## 6. Simulcastでの多重化

Simulcastでは、同じコンテンツの異なる品質を**別々のSSRC**で送信します。

### Publisher側

```
同じトランシーバー内で複数のRTPストリーム:

├─ SSRC: 0x11111111, RID: "q" → 低品質 (160x90 @ 150kbps)
├─ SSRC: 0x22222222, RID: "h" → 中品質 (320x180 @ 500kbps)
└─ SSRC: 0x33333333, RID: "f" → 高品質 (640x360 @ 1500kbps)
```

### SFU側の処理

```go
// pkg/sfu/receiver.go
type WebRTCReceiver struct {
    // 各レイヤーごとに別のバッファとトラック
    buffers[0] *buffer.Buffer  // SSRC: 0x11111111用（低品質）
    buffers[1] *buffer.Buffer  // SSRC: 0x22222222用（中品質）
    buffers[2] *buffer.Buffer  // SSRC: 0x33333333用（高品質）
    
    upTracks[0] TrackRemote    // 低品質トラック
    upTracks[1] TrackRemote    // 中品質トラック
    upTracks[2] TrackRemote    // 高品質トラック
}

// レイヤー追加時
func (w *WebRTCReceiver) AddUpTrack(track TrackRemote, buff *buffer.Buffer) error {
    // RIDから空間レイヤーを決定
    layer := buffer.GetSpatialLayerForRid(w.Mime(), track.RID(), w.trackInfo.Load())
    
    // 各レイヤーのSSRCを個別のバッファに格納
    w.upTracks[layer] = track
    w.buffers[layer] = buff
}
```

### Subscriber側

```
選択されたレイヤーを新しいSSRCで送信:

SFU → Subscriber:
└─ SSRC: 0xAAAAAAAA → 中品質レイヤー（元のSSRC: 0x22222222から変換）
```

## 7. RTX（再送ストリーム）

再送パケットも**別のSSRCを使った独立したストリーム**として送信されます。

### RTXの仕組み

```go
// pkg/sfu/downtrack.go

type DownTrack struct {
    ssrc    uint32  // メインストリーム: 0x12345678
    ssrcRTX uint32  // 再送ストリーム: 0x87654321
}

// 再送パケットの送信
func (d *DownTrack) retransmitPacket(epm *extPacketMeta, sourcePkt []byte, isProbe bool) (int, error) {
    // 元のパケットを解析
    var pkt rtp.Packet
    pkt.Unmarshal(sourcePkt)
    
    // RTX用のヘッダーを構築
    hdr := &rtp.Header{
        Version:        pkt.Header.Version,
        Padding:        pkt.Header.Padding,
        Marker:         epm.marker,
        PayloadType:    uint8(d.payloadTypeRTX.Load()), // RTX専用PT
        SequenceNumber: uint16(rtxExtSequenceNumber),    // RTX独自のSeqNum
        Timestamp:      epm.timestamp,                    // 元のTimestamp
        SSRC:           d.ssrcRTX,  // ★ 再送専用SSRC
    }
    
    // ペイロードの先頭2バイトに元のSequenceNumberを埋め込む
    payload := make([]byte, 2 + len(pkt.Payload))
    binary.BigEndian.PutUint16(payload[0:2], epm.targetSeqNo)  // OSN
    copy(payload[2:], pkt.Payload)
    
    // 送信
    d.pacer.Enqueue(&pacer.Packet{
        Header:  hdr,
        Payload: payload,
        IsRTX:   true,
    })
}
```

### RTXパケットの構造

```
メインストリーム（SSRC: 0x12345678）:
┌─────────────────────────────┐
│ RTP Header                  │
│ ├─ SSRC: 0x12345678         │
│ ├─ PT: 96 (VP8)             │
│ └─ SeqNum: 1000             │
├─────────────────────────────┤
│ Payload: [Original Data]    │
└─────────────────────────────┘

RTXストリーム（SSRC: 0x87654321）:
┌─────────────────────────────┐
│ RTP Header                  │
│ ├─ SSRC: 0x87654321         │
│ ├─ PT: 122 (RTX)            │
│ └─ SeqNum: 5001 (RTX独自)  │
├─────────────────────────────┤
│ OSN: 1000 (元のSeqNum)      │ ← 2バイト追加
│ Payload: [Original Data]    │
└─────────────────────────────┘
```

## 8. 実際のパケットフロー全体図

```
┌──────────────┐
│  Publisher   │
└──────┬───────┘
       │ 同じUDP接続で複数のSSRCを多重化
       │
       ├─ SSRC: 0x1000 (自分の映像 - 低品質)
       ├─ SSRC: 0x2000 (自分の映像 - 中品質)
       ├─ SSRC: 0x3000 (自分の映像 - 高品質)
       ├─ SSRC: 0x4000 (自分の音声)
       │
       ↓ WebRTC PeerConnection
       │
┌──────┴───────┐
│     SFU      │
│              │
│ ┌──────────────────────────┐
│ │ デマルチプレクシング      │
│ │ if (ssrc == 0x1000) → VideoBuffer[0]
│ │ if (ssrc == 0x2000) → VideoBuffer[1]
│ │ if (ssrc == 0x3000) → VideoBuffer[2]
│ │ if (ssrc == 0x4000) → AudioBuffer
│ └──────────────────────────┘
│              │
│ ┌──────────────────────────┐
│ │ 品質選択 & SSRC書き換え   │
│ │ 中品質を選択 (0x2000)     │
│ │ → 新SSRC: 0xA001に変換   │
│ └──────────────────────────┘
│              │
│ ┌──────────────────────────┐
│ │ 再マルチプレクシング      │
│ │ 複数のSubscriberに配信    │
│ └──────────────────────────┘
└──────┬───────┘
       │ 同じUDP接続で複数のSSRCを多重化
       │
       ├─ SSRC: 0xA001 (参加者Aの映像)
       ├─ SSRC: 0xB001 (参加者Aの音声)
       ├─ SSRC: 0xA002 (参加者Bの映像)
       ├─ SSRC: 0xB002 (参加者Bの音声)
       ├─ SSRC: 0xC001 (参加者Cの映像)
       ├─ SSRC: 0xD001 (参加者Cの音声)
       │
       ↓ WebRTC PeerConnection
       │
┌──────┴───────┐
│ Subscriber   │
│              │
│ ┌──────────────────────────┐
│ │ デマルチプレクシング      │
│ │ SSRCごとに別トラックへ    │
│ └──────────────────────────┘
└──────────────┘
```

## 9. RTP拡張ヘッダーによる追加識別

### 9.1 MID (Media Stream Identification)

```
RTP Extension Header:
┌─────────────────────────────┐
│ Extension Type: 15 (MID)    │
│ Value: "video0"              │
└─────────────────────────────┘

SDPとの対応:
a=mid:video0   ← カメラ映像
a=mid:audio0   ← マイク音声
a=mid:video1   ← 画面共有
```

### 9.2 RID (Restriction Identifier)

Simulcastのレイヤー識別

```
RTP Extension Header:
┌─────────────────────────────┐
│ Extension Type: ?? (RID)    │
│ Value: "h"                   │
└─────────────────────────────┘

SDPとの対応:
a=rid:q send    ← 低品質 (quarter)
a=rid:h send    ← 中品質 (half)
a=rid:f send    ← 高品質 (full)
```

### LiveKitでのRID処理

```go
// pkg/sfu/buffer/videolayerutils.go

// RIDから空間レイヤーを判定
func GetSpatialLayerForRid(mime MimeType, rid string, ti *livekit.TrackInfo) int32 {
    if ti == nil || len(ti.Layers) == 0 {
        // デフォルトのマッピング
        switch strings.ToLower(rid) {
        case "q":
            return 0  // 低品質
        case "h":
            return 1  // 中品質
        case "f":
            return 2  // 高品質
        }
    }
    
    // TrackInfoから検索
    for _, layer := range ti.Layers {
        if layer.Rid == rid {
            return layer.Spatial
        }
    }
    
    return InvalidLayerSpatial
}
```

## 10. パフォーマンスと効率性

### 10.1 多重化の利点

1. **接続の効率性**
   - ICE/DTLS接続を1回だけ確立
   - ファイアウォール/NATトラバーサルが1回で済む
   - ポート消費が最小限

2. **帯域の効率性**
   - 同じUDP接続を共有
   - パケット化のオーバーヘッドが低い
   - 輻輳制御を統合的に実行

3. **実装の単純性**
   - 接続管理が単純
   - 状態管理が集約される

### 10.2 スケーラビリティ

```
1つのPeerConnectionで扱えるストリーム数:

理論上: 2^32個のSSRC（約43億）
実用上:
  - 送信: 数個〜数十個（Simulcast含む）
  - 受信: 数十個〜数百個（大規模会議）

LiveKitの実績:
  - 100人規模の会議で問題なく動作
  - 各参加者が音声+映像を送受信
  - 合計200〜300ストリームの多重化
```

## 11. デバッグとモニタリング

### 11.1 SSRC追跡

```go
// pkg/sfu/receiver.go

func (w *WebRTCReceiver) DebugInfo() map[string]interface{} {
    upTrackInfo := make([]map[string]interface{}, 0)
    for layer, ut := range w.upTracks {
        if ut != nil {
            upTrackInfo = append(upTrackInfo, map[string]interface{}{
                "Layer": layer,
                "SSRC":  ut.SSRC(),  // ★ SSRC情報
                "RID":   ut.RID(),
                "Msid":  ut.Msid(),
            })
        }
    }
    return map[string]interface{}{
        "UpTracks": upTrackInfo,
    }
}
```

### 11.2 統計情報

```go
// pkg/sfu/rtpstats/

// SSRC単位での統計
type RTPStatsReceiver struct {
    packetsReceived uint64
    bytesReceived   uint64
    packetsLost     uint32
    jitter          uint32
}

// ストリームごとに独立した統計を保持
func (r *RTPStatsReceiver) Update(
    arrival int64,
    packetSize int,
    payloadSize int,
    hdr *rtp.Header,  // SSRCを含む
) {
    // SSRCに紐づけて統計更新
    r.packetsReceived++
    r.bytesReceived += uint64(packetSize)
    // ...
}
```

## まとめ

RTPストリームの多重化メカニズムのキーポイント：

1. **SSRC（32bit識別子）が最も重要**
   - 各ストリームに一意のSSRCを割り当て
   - 受信側はSSRCでストリームを識別・振り分け

2. **1つのUDP接続で多数のストリームを扱える**
   - 効率的な接続管理
   - スケーラブルなアーキテクチャ

3. **LiveKitでの実装**
   - WebRTCReceiver: SSRCでバッファに振り分け
   - DownTrack: 新しいSSRCに書き換えて送信
   - Simulcast: レイヤーごとに異なるSSRC
   - RTX: 専用SSRCで再送ストリームを分離

4. **拡張性**
   - MID/RIDによる明示的な識別
   - Dependency Descriptorによる詳細な制御
   - 将来的な拡張にも対応可能

この多重化メカニズムにより、WebRTC SFUは大規模なリアルタイムメディア配信を効率的に実現しています。
