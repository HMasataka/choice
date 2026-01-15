# WebRTC SFU 用語集

本ドキュメントはWebRTC SFUに関連する用語の詳細な説明を提供する。

## 目次

- [WebRTC SFU 用語集](#webrtc-sfu-用語集)
  - [目次](#目次)
  - [アーキテクチャ関連](#アーキテクチャ関連)
    - [SFU (Selective Forwarding Unit)](#sfu-selective-forwarding-unit)
    - [Room](#room)
    - [Publisher](#publisher)
    - [Subscriber](#subscriber)
    - [Track](#track)
  - [プロトコル関連](#プロトコル関連)
    - [SDP (Session Description Protocol)](#sdp-session-description-protocol)
    - [ICE (Interactive Connectivity Establishment)](#ice-interactive-connectivity-establishment)
    - [STUN (Session Traversal Utilities for NAT)](#stun-session-traversal-utilities-for-nat)
    - [TURN (Traversal Using Relays around NAT)](#turn-traversal-using-relays-around-nat)
    - [RTP (Real-time Transport Protocol)](#rtp-real-time-transport-protocol)
    - [RTCP (RTP Control Protocol)](#rtcp-rtp-control-protocol)
  - [識別子関連](#識別子関連)
    - [SSRC (Synchronization Source)](#ssrc-synchronization-source)
    - [MID (Media ID)](#mid-media-id)
    - [RID (Restriction ID)](#rid-restriction-id)
  - [映像技術関連](#映像技術関連)
    - [Simulcast](#simulcast)
    - [SVC (Scalable Video Coding)](#svc-scalable-video-coding)
  - [輻輳制御関連](#輻輳制御関連)
    - [TWCC (Transport-Wide Congestion Control)](#twcc-transport-wide-congestion-control)
    - [REMB (Receiver Estimated Maximum Bitrate)](#remb-receiver-estimated-maximum-bitrate)
  - [RTCPフィードバック関連](#rtcpフィードバック関連)
    - [NACK (Negative Acknowledgement)](#nack-negative-acknowledgement)
    - [PLI (Picture Loss Indication)](#pli-picture-loss-indication)
    - [FIR (Full Intra Request)](#fir-full-intra-request)
  - [RTP再送関連](#rtp再送関連)
    - [RTX (Retransmission)](#rtx-retransmission)
  - [コーデック関連](#コーデック関連)
    - [VP8](#vp8)
    - [VP9](#vp9)
    - [H.264](#h264)
    - [AV1](#av1)
    - [Opus](#opus)
  - [SDP関連](#sdp関連)
    - [Unified Plan](#unified-plan)
    - [BUNDLE](#bundle)
    - [rtcp-mux](#rtcp-mux)
  - [セキュリティ関連](#セキュリティ関連)
    - [DTLS (Datagram Transport Layer Security)](#dtls-datagram-transport-layer-security)
    - [SRTP (Secure Real-time Transport Protocol)](#srtp-secure-real-time-transport-protocol)
    - [E2EE (End-to-End Encryption)](#e2ee-end-to-end-encryption)
  - [参考文献](#参考文献)

## アーキテクチャ関連

### SFU (Selective Forwarding Unit)

メディアストリームを選択的に転送するサーバーアーキテクチャ。

SFUは受信したメディアストリームを再エンコードせずに、必要な参加者にのみ転送する。これにより、MCU（Multipoint Control Unit）と比較して以下の利点がある。

- **低遅延**: 再エンコードが不要なため、処理遅延が最小化される
- **スケーラビリティ**: サーバーのCPU負荷が低く、多くの接続を処理可能
- **品質維持**: オリジナルの映像品質が保持される

```text
[Publisher A] ----> [SFU] ----> [Subscriber B]
                      |
                      +-------> [Subscriber C]
```

SFUの主な機能:

1. 複数のPublisherからメディアストリームを受信
2. 各Subscriberに必要なストリームのみを転送
3. Simulcastレイヤーの選択と切り替え
4. RTCPフィードバックの処理と転送

### Room

複数の参加者が接続する論理的な空間。

Roomは以下の状態を持つ:

| 状態      | 説明                       |
| --------- | -------------------------- |
| `created` | ルーム作成済み、参加者なし |
| `active`  | 1人以上の参加者が存在      |
| `locked`  | 新規参加を禁止             |
| `closing` | クローズ処理中             |
| `closed`  | ルーム終了                 |

Roomの制限:

- 最大参加者数: 100（設定可能）
- 最大トラック数: 500/ルーム

### Publisher

メディアストリームを送信するクライアント。

Publisherは以下の操作を行う:

1. カメラ/マイクからのメディアキャプチャ
2. WebRTC PeerConnectionの確立
3. メディアトラックのSFUへの送信
4. Simulcast有効時は複数レイヤーの同時送信

1参加者あたりの制限:

- 映像トラック: 最大3（カメラ、画面共有、追加）
- 音声トラック: 最大2（マイク、システム音声）

### Subscriber

メディアストリームを受信するクライアント。

Subscriberは以下の操作を行う:

1. 目的のPublisherのトラックを購読
2. SFUからメディアストリームを受信
3. 希望するSimulcastレイヤーの指定
4. ネットワーク状況に応じた品質調整要求

### Track

単一の映像または音声ストリーム。

Trackは以下の属性を持つ:

| 属性       | 説明                               |
| ---------- | ---------------------------------- |
| `trackId`  | サーバー割り当ての一意識別子       |
| `kind`     | `video` または `audio`             |
| `mid`      | SDPにおけるメディアID              |
| `simulcast`| Simulcastの有効/無効               |
| `metadata` | クライアント定義のメタデータ       |

## プロトコル関連

### SDP (Session Description Protocol)

セッション記述プロトコル。WebRTCにおけるメディアセッションのネゴシエーションに使用される。

SDPは以下の情報を含む:

- セッションメタデータ（バージョン、タイミング等）
- メディア記述（音声/映像のコーデック、ポート等）
- 接続情報（IPアドレス、ポート）
- 属性（帯域幅制限、暗号化パラメータ等）

SDP Offer/Answerモデル:

```text
[Client] --Offer--> [SFU]
[Client] <-Answer-- [SFU]
```

本システムで必須のSDP要件:

- Unified Plan（必須）
- BUNDLE（必須）
- rtcp-mux（必須）

### ICE (Interactive Connectivity Establishment)

NAT越えを実現するための接続確立プロトコル。

ICEは以下のステップで接続を確立する:

1. **候補収集（Gathering）**: 可能な接続経路を収集
2. **接続性チェック（Connectivity Check）**: 各経路の到達性を確認
3. **候補選択（Nomination）**: 最適な経路を選択

候補タイプ（優先順位順）:

| タイプ | 説明                     | 優先度 |
| ------ | ------------------------ | ------ |
| host   | ローカルIPアドレス       | 最高   |
| srflx  | STUNで取得した外部IP     | 中     |
| relay  | TURNリレー経由           | 最低   |

本システムではICE Liteモードを採用:

- SFUはcontrolled role固定
- host候補のみ通知
- クライアントがフルICEを実行

### STUN (Session Traversal Utilities for NAT)

NAT越え支援サーバー。クライアントが自身の公開IPアドレスを取得するために使用される。

STUNの動作:

```text
[Client] --Binding Request--> [STUN Server]
[Client] <-Binding Response-- [STUN Server]
                               (公開IP/ポート情報)
```

STUNは以下の情報を提供:

- クライアントの公開IPアドレス
- クライアントの公開ポート番号
- NATタイプの推測

### TURN (Traversal Using Relays around NAT)

リレーサーバー。直接接続が確立できない場合にメディアを中継する。

TURNの使用シナリオ:

- シンメトリックNAT環境
- 企業ファイアウォール配下
- UDP通信がブロックされる環境

対応プロトコル:

| プロトコル | ポート例 | 用途                  |
| ---------- | -------- | --------------------- |
| UDP        | 3478     | 標準、最も効率的      |
| TCP        | 3478     | UDP制限環境向け       |
| TLS        | 443      | ファイアウォール越え  |

認証方式: Long-term credentials（RFC 8656）

### RTP (Real-time Transport Protocol)

リアルタイムメディア転送プロトコル。音声/映像データの転送に使用される。

RTPパケット構造:

```text
 0                   1                   2                   3
 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|V=2|P|X|  CC   |M|     PT      |       sequence number         |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|                           timestamp                           |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|           synchronization source (SSRC) identifier            |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
```

SFUでのRTP処理:

- SSRC管理とマッピング
- シーケンス番号の書き換え
- タイムスタンプの正規化
- パケットペーシング

### RTCP (RTP Control Protocol)

RTPの制御プロトコル。メディア品質のフィードバックや統計情報の交換に使用される。

RTCPパケットタイプ:

| タイプ | 名称                | 用途                   |
| ------ | ------------------- | ---------------------- |
| SR     | Sender Report       | 送信者統計             |
| RR     | Receiver Report     | 受信者統計             |
| SDES   | Source Description  | ソース情報             |
| BYE    | Goodbye             | セッション終了通知     |
| APP    | Application-Defined | アプリケーション固有   |

## 識別子関連

### SSRC (Synchronization Source)

RTPストリームの識別子。32ビットの乱数で、各RTPストリームを一意に識別する。

SSRCの用途:

- 同一セッション内の複数ストリーム識別
- Simulcastの各レイヤー識別
- 送信者と受信者のマッピング

SFUでは、PublisherからのSSRCをSubscriber向けに再マッピングする。

### MID (Media ID)

SDPにおけるメディア記述の識別子。

MIDはRTPヘッダー拡張として送信され、以下の目的で使用される:

- BUNDLEグループ内のメディア識別
- Unified Planでの複数トラック管理

URN: `urn:ietf:params:rtp-hdrext:sdes:mid`

### RID (Restriction ID)

Simulcastレイヤーの識別子。

本システムで使用するRID:

| RID | レイヤー | 解像度    | ビットレート目安 |
| --- | -------- | --------- | ---------------- |
| `h` | High     | 1280x720  | 2.5 Mbps         |
| `m` | Medium   | 640x360   | 500 Kbps         |
| `l` | Low      | 320x180   | 150 Kbps         |

URN: `urn:ietf:params:rtp-hdrext:sdes:rtp-stream-id`

## 映像技術関連

### Simulcast

同一ソースを複数の解像度/ビットレートで同時配信する技術。

Simulcastの利点:

- **適応的品質**: 受信者の帯域に応じたレイヤー選択
- **低遅延**: SFUでの再エンコード不要
- **柔軟性**: 各Subscriberが独立してレイヤー選択可能

Simulcastの動作:

```text
[Publisher]
    |
    +-- Layer H (720p) --+
    +-- Layer M (360p) --+--> [SFU] --> [Subscriber A: H]
    +-- Layer L (180p) --+          --> [Subscriber B: M]
                                    --> [Subscriber C: L]
```

レイヤー切り替えトリガー:

| 条件                    | アクション           |
| ----------------------- | -------------------- |
| パケットロス率 > 5%     | 低レイヤーへ切り替え |
| RTT > 300ms             | 低レイヤーへ切り替え |
| 帯域幅不足              | 低レイヤーへ切り替え |
| 帯域幅余裕あり          | 高レイヤーへ切り替え |

### SVC (Scalable Video Coding)

単一ストリームで複数品質を実現する技術。

SVCの構造:

- **基本層（Base Layer）**: 最低品質、必須
- **拡張層（Enhancement Layer）**: 品質向上、オプション

Simulcastとの比較:

| 特性             | Simulcast        | SVC              |
| ---------------- | ---------------- | ---------------- |
| 帯域効率         | 低（重複あり）   | 高（増分のみ）   |
| コーデック対応   | 広い             | VP9/AV1のみ      |
| SFU実装複雑度    | 低               | 高               |
| クライアント対応 | 広い             | 限定的           |

本システムではSimulcastを優先実装し、SVCはVP9/AV1使用時のオプションとする。

## 輻輳制御関連

### TWCC (Transport-Wide Congestion Control)

トランスポート全体の輻輳制御メカニズム。

TWCCの特徴:

- 全パケットにシーケンス番号を付与
- 受信側で詳細な到着時刻を計測
- 送信側で帯域幅を正確に推定

TWCCのフィードバック間隔: 100ms

URN: `http://www.ietf.org/id/draft-holmer-rmcat-transport-wide-cc-extensions-01`

※ RFC 8888で標準化されているが、多くの実装ではdraft URIが使用されている。

### REMB (Receiver Estimated Maximum Bitrate)

受信者が推定する最大ビットレート。

REMBはTWCC非対応クライアント向けのフォールバックとして使用される。

SFUの輻輳制御方式選択:

| 受信状況         | 処理                   |
| ---------------- | ---------------------- |
| TWCCのみ         | TWCCを使用（優先）     |
| REMBのみ         | REMBを使用             |
| 両方             | TWCCを採用、REMB無視   |
| どちらもなし     | デフォルト帯域幅使用   |

## RTCPフィードバック関連

### NACK (Negative Acknowledgement)

パケット再送要求。受信側がパケットロスを検出した際に送信する。

NACK処理要件:

- パケットロス検出から10ms以内に再送要求
- RTXを使用した再送

### PLI (Picture Loss Indication)

映像フレーム損失の通知。デコーダーが映像を正しく表示できない場合に送信する。

PLI受信時の動作:

- Publisherに即時転送
- 新しいキーフレームの生成をトリガー

### FIR (Full Intra Request)

完全なイントラフレーム（キーフレーム）の要求。

FIRの使用シナリオ:

- 新しいSubscriberの参加時
- 映像品質の大幅な劣化時
- Simulcastレイヤー切り替え時

## RTP再送関連

### RTX (Retransmission)

再送専用のRTPストリーム。NACKに応答してパケットを再送する。

RTXはRTCPフィードバックではなく、RTPの再送メカニズムである。SDPでは`ssrc-group:FID`属性でオリジナルストリームと再送ストリームを関連付ける。

RTXの特徴:

- 独自のSSRCとペイロードタイプを持つ
- `apt`（associated payload type）でオリジナルコーデックを指定
- 元のストリームと分離された再送
- 効率的なパケット再送管理

SDP例:

```text
a=ssrc-group:FID 12345 67890
a=rtpmap:97 rtx/90000
a=fmtp:97 apt=96
```

## コーデック関連

### VP8

Googleが開発したオープンなビデオコーデック。

VP8の特徴:

- ロイヤリティフリー
- 全WebRTCクライアントで対応
- Simulcast対応

本システムでの優先順位: 1位（必須）

### VP9

VP8の後継コーデック。

VP9の特徴:

- 高圧縮効率（VP8比約50%向上）
- SVC対応
- 4K解像度サポート

本システムでの優先順位: 3位（推奨）

### H.264

広く普及しているビデオコーデック。

H.264の特徴:

- ハードウェアデコード対応が広い
- モバイルデバイスで効率的
- ライセンスが必要な場合あり

本システムで対応するプロファイル:

| プロファイル             | profile-level-id | 用途             |
| ------------------------ | ---------------- | ---------------- |
| High Profile Level 5.0   | 640032           | デスクトップ向け |
| Constrained Baseline 3.1 | 42e01f           | モバイル/Safari  |

本システムでの優先順位: 2位（必須）

### AV1

次世代オープンビデオコーデック。

AV1の特徴:

- 最高の圧縮効率
- ロイヤリティフリー
- SVC対応
- ハードウェア対応は限定的

本システムでの優先順位: 4位（オプション、将来対応）

### Opus

WebRTC標準の音声コーデック。

Opusの特徴:

- 低遅延
- 可変ビットレート
- ステレオ対応
- インバンドFEC対応

推奨パラメータ:

| パラメータ   | 値    | 説明                 |
| ------------ | ----- | -------------------- |
| minptime     | 10    | 最小パケット化時間   |
| useinbandfec | 1     | インバンドFEC有効    |
| stereo       | 1     | ステレオ対応         |
| サンプリング | 48kHz | 標準サンプリングレート|

## SDP関連

### Unified Plan

SDPの構造仕様。各メディアトラックを独立したm=行で記述する。

Unified Planの特徴:

- 1トラック = 1 m=行
- 柔軟なトラック追加/削除
- 全モダンブラウザで対応

本システムではUnified Plan必須（Plan B非対応）。

### BUNDLE

複数のメディアを単一のトランスポートで送受信するSDP拡張。

BUNDLEの利点:

- ICE候補の削減
- 接続確立の高速化
- リソース使用量の削減

本システムではBUNDLE必須。

### rtcp-mux

RTPとRTCPを同一ポートで送受信するSDP拡張。

rtcp-muxの利点:

- ポート使用量の削減
- NAT越えの簡略化
- ファイアウォール設定の簡略化

本システムではrtcp-mux必須。

## セキュリティ関連

### DTLS (Datagram Transport Layer Security)

UDPベースのTLSプロトコル。WebRTCのキー交換に使用される。

DTLSの用途:

- SRTPキーの交換
- ピア認証
- 暗号化パラメータのネゴシエーション

本システムではDTLS 1.2を使用。

### SRTP (Secure Real-time Transport Protocol)

暗号化されたRTPプロトコル。

SRTPの特徴:

- メディアデータの暗号化
- 認証とメッセージ完全性
- リプレイ攻撃の防止

本システムではDTLS-SRTPを使用。

### E2EE (End-to-End Encryption)

エンドツーエンド暗号化。SFUを含む中間者がメディアを復号できない。

E2EEの実装方式:

- Insertable Streams APIを使用
- クライアント側で暗号化/復号
- SFUはRTPヘッダーのみ参照

本システムではE2EEはオプション機能（クライアント実装依存）。

## 参考文献

- [WebRTC 1.0: Real-Time Communication Between Browsers](https://www.w3.org/TR/webrtc/)
- [RFC 8825 - Overview: Real-Time Protocols for Browser-Based Applications](https://datatracker.ietf.org/doc/html/rfc8825)
- [RFC 5389 - STUN](https://datatracker.ietf.org/doc/html/rfc5389)
- [RFC 8656 - TURN](https://datatracker.ietf.org/doc/html/rfc8656)
- [RFC 4585 - Extended RTP Profile for RTCP-Based Feedback](https://datatracker.ietf.org/doc/html/rfc4585)
- [RFC 3550 - RTP](https://datatracker.ietf.org/doc/html/rfc3550)
- [Pion WebRTC](https://github.com/pion/webrtc)
