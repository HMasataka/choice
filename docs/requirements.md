# WebRTC SFU 要件定義書

## 1. 概要

本ドキュメントはGo言語を用いたWebRTC SFU（Selective Forwarding Unit）の要件を定義する。

### 1.1 目的

- 複数参加者間のリアルタイム映像・音声通信を実現する
- スケーラブルで低遅延なメディア中継サーバーを構築する

### 1.2 用語定義

| 用語       | 説明                                                                     |
| ---------- | ------------------------------------------------------------------------ |
| SFU        | Selective Forwarding Unit - メディアストリームを選択的に転送するサーバー |
| Publisher  | メディアストリームを送信するクライアント                                 |
| Subscriber | メディアストリームを受信するクライアント                                 |
| Room       | 複数の参加者が接続する論理的な空間                                       |
| Track      | 単一の映像または音声ストリーム                                           |
| SSRC       | Synchronization Source - RTPストリームの識別子                           |
| MID        | Media ID - SDPにおけるメディア記述の識別子                               |
| RID        | Restriction ID - Simulcastレイヤーの識別子                               |

## 2. 機能要件

### 2.1 シグナリング

#### 2.1.1 基本機能

- [ ] WebSocket経由でのシグナリング通信
- [ ] SDP（Session Description Protocol）のOffer/Answer交換
- [ ] ICE Candidate交換（Trickle ICE対応）
- [ ] JSON-RPC 2.0形式のメッセージプロトコル

#### 2.1.2 メッセージスキーマ

**設計方針:**

- roomIdはJWTトークン内に含まれるため、リクエストパラメータには含めない
- 各メソッドはフラットなparamsを使用
- リクエストには必ずidを含め、レスポンスを受け取る

**リクエスト形式:**

```json
{
  "jsonrpc": "2.0",
  "id": "string (必須: リクエストID、UUIDv4形式)",
  "method": "string (必須: メソッド名)",
  "params": "object (メソッド固有のパラメータ)"
}
```

**成功レスポンス形式:**

```json
{
  "jsonrpc": "2.0",
  "id": "string (リクエストIDと同一)",
  "result": "object (メソッド固有の結果)"
}
```

**エラーレスポンス形式:**

```json
{
  "jsonrpc": "2.0",
  "id": "string (リクエストIDと同一)",
  "error": {
    "code": "number (エラーコード)",
    "message": "string (エラーメッセージ)",
    "data": "object (任意: 追加情報)"
  }
}
```

**サーバー通知形式（id無し）:**

```json
{
  "jsonrpc": "2.0",
  "method": "string (通知メソッド名)",
  "params": "object (通知パラメータ)"
}
```

#### 2.1.3 エラーコード

| コード | 名称             | 説明                   |
| ------ | ---------------- | ---------------------- |
| -32700 | Parse error      | JSONパースエラー       |
| -32600 | Invalid Request  | 不正なリクエスト形式   |
| -32601 | Method not found | 未知のメソッド         |
| -32602 | Invalid params   | 不正なパラメータ       |
| -32603 | Internal error   | サーバー内部エラー     |
| 1001   | Room not found   | ルームが存在しない     |
| 1002   | Room full        | ルームが満員           |
| 1003   | Unauthorized     | 認証エラー             |
| 1004   | Already joined   | 既にルームに参加済み   |
| 1005   | Not in room      | ルームに参加していない |
| 1006   | Track not found  | トラックが存在しない   |
| 1007   | Invalid SDP      | 不正なSDP              |
| 1008   | ICE failure      | ICE接続失敗            |
| 1009   | Session expired  | セッション期限切れ     |
| 1010   | Room locked      | ルームがロック中       |

#### 2.1.4 状態遷移

```text
[Disconnected] --> (connect) --> [Connected]
[Connected] --> (join) --> [Joined]
[Joined] --> (publish) --> [Publishing]
[Joined] --> (subscribe) --> [Subscribing]
[Publishing/Subscribing] --> (leave) --> [Connected]
[Connected] --> (disconnect) --> [Disconnected]
[Any State] --> (error/timeout) --> [Disconnected]
```

#### 2.1.5 再接続処理

- [ ] セッションIDによる再接続識別
- [ ] 再接続時のメディアストリーム自動復元
- [ ] 再接続タイムアウト: 30秒
- [ ] 指数バックオフによるリトライ（初期1秒、最大30秒）

#### 2.1.6 メソッド別スキーマ詳細

**join リクエスト:**

```json
{
  "jsonrpc": "2.0",
  "id": "uuid-v4",
  "method": "join",
  "params": {
    "token": "string (必須: JWTトークン)",
    "sessionId": "string (任意: 再接続時のセッションID)",
    "metadata": "object (任意: クライアントメタデータ)"
  }
}
```

**join レスポンス (成功):**

```json
{
  "jsonrpc": "2.0",
  "id": "uuid-v4",
  "result": {
    "sessionId": "string (セッションID)",
    "roomId": "string (ルームID)",
    "participantId": "string (参加者ID)",
    "participants": [
      {
        "id": "string",
        "metadata": "object",
        "tracks": [{ "trackId": "string", "kind": "video|audio" }]
      }
    ],
    "iceServers": [{ "urls": ["stun:stun.example.com:3478"] }]
  }
}
```

**publish リクエスト:**

```json
{
  "jsonrpc": "2.0",
  "id": "uuid-v4",
  "method": "publish",
  "params": {
    "kind": "string (必須: video|audio)",
    "simulcast": "boolean (任意: Simulcast有効化、デフォルトtrue)",
    "metadata": "object (任意: トラックメタデータ)",
    "label": "string (任意: クライアント側識別用ラベル)"
  }
}
```

※ trackIdはサーバーが生成・割り当て、レスポンスで返却

**subscribe リクエスト:**

```json
{
  "jsonrpc": "2.0",
  "id": "uuid-v4",
  "method": "subscribe",
  "params": {
    "publisherId": "string (必須: 配信者ID)",
    "trackId": "string (必須: トラックID)",
    "preferredLayer": "string (任意: h|m|l、デフォルトh)"
  }
}
```

**setPreferredLayer リクエスト:**

```json
{
  "jsonrpc": "2.0",
  "id": "uuid-v4",
  "method": "setPreferredLayer",
  "params": {
    "trackId": "string (必須: トラックID)",
    "layer": "string (必須: h|m|l)"
  }
}
```

**publish レスポンス (成功):**

```json
{
  "jsonrpc": "2.0",
  "id": "uuid-v4",
  "result": {
    "trackId": "string (サーバー割り当てトラックID)",
    "mid": "string (SDPのメディアID)"
  }
}
```

**subscribe レスポンス (成功):**

```json
{
  "jsonrpc": "2.0",
  "id": "uuid-v4",
  "result": {
    "subscriptionId": "string (購読ID、unsubscribe時に使用)",
    "trackId": "string",
    "publisherId": "string"
  }
}
```

**offer リクエスト:**

```json
{
  "jsonrpc": "2.0",
  "id": "uuid-v4",
  "method": "offer",
  "params": {
    "sdp": "string (必須: SDP offer文字列)"
  }
}
```

**offer レスポンス (成功):**

```json
{
  "jsonrpc": "2.0",
  "id": "uuid-v4",
  "result": {
    "sdp": "string (SDP answer文字列)"
  }
}
```

**answer リクエスト:**

```json
{
  "jsonrpc": "2.0",
  "id": "uuid-v4",
  "method": "answer",
  "params": {
    "sdp": "string (必須: SDP answer文字列)"
  }
}
```

**answer レスポンス (成功):**

```json
{
  "jsonrpc": "2.0",
  "id": "uuid-v4",
  "result": {}
}
```

**candidate リクエスト:**

```json
{
  "jsonrpc": "2.0",
  "id": "uuid-v4",
  "method": "candidate",
  "params": {
    "candidate": "string (必須: ICE candidate文字列)",
    "sdpMid": "string (任意: SDPメディアID)",
    "sdpMLineIndex": "number (任意: SDPメディア行インデックス)"
  }
}
```

**candidate レスポンス (成功):**

```json
{
  "jsonrpc": "2.0",
  "id": "uuid-v4",
  "result": {}
}
```

**leave リクエスト:**

```json
{
  "jsonrpc": "2.0",
  "id": "uuid-v4",
  "method": "leave",
  "params": {}
}
```

**leave レスポンス (成功):**

```json
{
  "jsonrpc": "2.0",
  "id": "uuid-v4",
  "result": {}
}
```

**unpublish リクエスト:**

```json
{
  "jsonrpc": "2.0",
  "id": "uuid-v4",
  "method": "unpublish",
  "params": {
    "trackId": "string (必須: トラックID)"
  }
}
```

**unpublish レスポンス (成功):**

```json
{
  "jsonrpc": "2.0",
  "id": "uuid-v4",
  "result": {}
}
```

**unsubscribe リクエスト:**

```json
{
  "jsonrpc": "2.0",
  "id": "uuid-v4",
  "method": "unsubscribe",
  "params": {
    "subscriptionId": "string (必須: 購読ID)"
  }
}
```

**unsubscribe レスポンス (成功):**

```json
{
  "jsonrpc": "2.0",
  "id": "uuid-v4",
  "result": {}
}
```

#### 2.1.7 サーバー通知スキーマ

**trackPublished 通知:**

```json
{
  "jsonrpc": "2.0",
  "method": "trackPublished",
  "params": {
    "publisherId": "string",
    "trackId": "string",
    "kind": "video|audio",
    "simulcast": "boolean",
    "metadata": "object"
  }
}
```

**trackUnpublished 通知:**

```json
{
  "jsonrpc": "2.0",
  "method": "trackUnpublished",
  "params": {
    "publisherId": "string",
    "trackId": "string"
  }
}
```

**participantJoined 通知:**

```json
{
  "jsonrpc": "2.0",
  "method": "participantJoined",
  "params": {
    "participantId": "string",
    "metadata": "object"
  }
}
```

**participantLeft 通知:**

```json
{
  "jsonrpc": "2.0",
  "method": "participantLeft",
  "params": {
    "participantId": "string",
    "reason": "string (leave|timeout|kicked)"
  }
}
```

**offer 通知 (サーバー主導の再ネゴシエーション):**

```json
{
  "jsonrpc": "2.0",
  "method": "offer",
  "params": {
    "sdp": "string (SDP offer文字列)",
    "reason": "string (track_added|track_removed|ice_restart)"
  }
}
```

**candidate 通知:**

```json
{
  "jsonrpc": "2.0",
  "method": "candidate",
  "params": {
    "candidate": "string",
    "sdpMid": "string",
    "sdpMLineIndex": "number"
  }
}
```

**joined 通知（自身の参加完了確認）:**

```json
{
  "jsonrpc": "2.0",
  "method": "joined",
  "params": {
    "participantId": "string",
    "roomId": "string"
  }
}
```

**left 通知（自身の退出完了確認）:**

```json
{
  "jsonrpc": "2.0",
  "method": "left",
  "params": {
    "reason": "string (voluntary|kicked|timeout)"
  }
}
```

**error 通知:**

```json
{
  "jsonrpc": "2.0",
  "method": "error",
  "params": {
    "code": "number (エラーコード)",
    "message": "string (エラーメッセージ)",
    "fatal": "boolean (接続を切断すべきかどうか)"
  }
}
```

**reconnect 通知:**

```json
{
  "jsonrpc": "2.0",
  "method": "reconnect",
  "params": {
    "reason": "string (ice_disconnected|server_restart)",
    "retryAfterMs": "number (再接続までの待機時間)"
  }
}
```

**recordingStarted 通知:**

```json
{
  "jsonrpc": "2.0",
  "method": "recordingStarted",
  "params": {
    "recordingId": "string",
    "startedBy": "string (参加者ID)"
  }
}
```

**recordingStopped 通知:**

```json
{
  "jsonrpc": "2.0",
  "method": "recordingStopped",
  "params": {
    "recordingId": "string",
    "stoppedBy": "string (参加者ID)"
  }
}
```

### 2.2 ルーム管理

#### 2.2.1 基本機能

- [ ] ルームの作成・削除
- [ ] 参加者のルームへの参加・退出
- [ ] ルーム内参加者一覧の取得
- [ ] 最大参加者数の制限設定（デフォルト: 100）

#### 2.2.2 ルーム状態

| 状態      | 説明                       |
| --------- | -------------------------- |
| `created` | ルーム作成済み、参加者なし |
| `active`  | 1人以上の参加者が存在      |
| `locked`  | 新規参加を禁止             |
| `closing` | クローズ処理中             |
| `closed`  | ルーム終了                 |

#### 2.2.3 参加者状態

| 状態          | 説明           |
| ------------- | -------------- |
| `joining`     | 参加処理中     |
| `joined`      | 参加完了       |
| `publishing`  | メディア配信中 |
| `subscribing` | メディア受信中 |
| `leaving`     | 退出処理中     |
| `left`        | 退出完了       |

### 2.3 メディア処理

#### 2.3.1 基本機能

- [ ] 複数のPublisherからのメディアストリーム受信
- [ ] Subscriberへのメディアストリーム転送（再エンコードなし）
- [ ] Simulcast対応（複数解像度での配信）を優先実装
- [ ] SVC（Scalable Video Coding）対応はVP9/AV1使用時のみ

#### 2.3.2 Simulcast仕様

| レイヤー | RID | 解像度   | ビットレート目安 |
| -------- | --- | -------- | ---------------- |
| High     | `h` | 1280x720 | 2.5 Mbps         |
| Medium   | `m` | 640x360  | 500 Kbps         |
| Low      | `l` | 320x180  | 150 Kbps         |

#### 2.3.3 トラック制限

| 項目                          | 上限                        |
| ----------------------------- | --------------------------- |
| 1参加者あたりの映像トラック数 | 3（カメラ、画面共有、追加） |
| 1参加者あたりの音声トラック数 | 2（マイク、システム音声）   |
| 1ルームあたりの総トラック数   | 500                         |

### 2.4 SDP・相互接続要件

#### 2.4.1 SDP仕様

- [ ] Unified Plan必須（Plan B非対応）
- [ ] BUNDLE必須（全メディアを単一トランスポートで送受信）
- [ ] rtcp-mux必須（RTPとRTCPを同一ポートで送受信）

#### 2.4.2 必須RTPヘッダー拡張

| 拡張              | URI                                                                         | 用途                  |
| ----------------- | --------------------------------------------------------------------------- | --------------------- |
| mid               | urn:ietf:params:rtp-hdrext:sdes:mid                                         | メディアID識別        |
| rid               | urn:ietf:params:rtp-hdrext:sdes:rtp-stream-id                               | Simulcastレイヤー識別 |
| transport-wide-cc | `http://www.ietf.org/id/draft-holmer-rmcat-transport-wide-cc-extensions-01` | 輻輳制御              |
| abs-send-time     | `http://www.webrtc.org/experiments/rtp-hdrext/abs-send-time`                | 送信時刻              |

#### 2.4.3 再ネゴシエーショントリガー

- トラックの追加・削除時
- Simulcastレイヤー構成変更時
- コーデック変更時
- ICE再起動時

#### 2.4.4 Safari互換性対応

- Safari 14+でUnified Planをサポート
- Safari固有のSDP属性の正規化処理
- H.264 Constrained Baseline Profile優先（Safari互換）

#### 2.4.5 必須RTCPフィードバック

SDPに以下のrtcp-fb属性を含める必要がある:

| フィードバック | 用途                    | 対象       |
| -------------- | ----------------------- | ---------- |
| `nack`         | パケット再送要求        | 映像       |
| `nack pli`     | Picture Loss Indication | 映像       |
| `ccm fir`      | Full Intra Request      | 映像       |
| `goog-remb`    | REMB帯域幅推定          | 映像       |
| `transport-cc` | TWCC輻輳制御            | 映像・音声 |

**SDP例（映像）:**

```text
a=rtcp-fb:96 nack
a=rtcp-fb:96 nack pli
a=rtcp-fb:96 ccm fir
a=rtcp-fb:96 goog-remb
a=rtcp-fb:96 transport-cc
```

#### 2.4.6 コーデックパラメータ要件

**H.264:**

SFUは以下の両プロファイルをSDPで提示し、クライアントとネゴシエーションを行う：

| プロファイル                   | profile-level-id | 用途                                         |
| ------------------------------ | ---------------- | -------------------------------------------- |
| High Profile Level 5.0         | 640032           | 高品質（1080p30対応）、デスクトップ向け      |
| Constrained Baseline Level 3.1 | 42e01f           | Safari・モバイル互換、低スペックデバイス向け |

**共通パラメータ:**

| パラメータ              | 値  | 説明                         |
| ----------------------- | --- | ---------------------------- |
| packetization-mode      | 1   | Non-interleaved mode（必須） |
| level-asymmetry-allowed | 1   | 送受信で異なるレベル許可     |

※ クライアントのデバイス性能・帯域に応じて適切なプロファイルが選択される

**SDP例（H.264 High Profile）:**

```text
a=fmtp:96 profile-level-id=640032;packetization-mode=1;level-asymmetry-allowed=1
```

**SDP例（H.264 Constrained Baseline）:**

```text
a=fmtp:96 profile-level-id=42e01f;packetization-mode=1;level-asymmetry-allowed=1
```

**VP8/VP9:**

- 特別なパラメータは不要
- SDPにはprofile-id等を含めない（デフォルト値を使用）

**Opus:**

| パラメータ   | 値  | 説明                     |
| ------------ | --- | ------------------------ |
| minptime     | 10  | 最小パケット化時間（ms） |
| useinbandfec | 1   | インバンドFEC有効化      |
| stereo       | 1   | ステレオ対応             |

**SDP例（Opus）:**

```text
a=fmtp:111 minptime=10;useinbandfec=1;stereo=1
```

### 2.5 RTP/RTCP要件

#### 2.5.1 RTP処理

- [ ] SSRC管理（Publisher/Subscriber間のマッピング）
- [ ] MID/RID拡張ヘッダー処理
- [ ] シーケンス番号の書き換え
- [ ] タイムスタンプの正規化
- [ ] パケットペーシング（バースト送信の平滑化）
- [ ] ジッタバッファ（50ms、適応的）

#### 2.5.2 RTCP処理

- [ ] Receiver Report (RR) の集約と転送
- [ ] TWCC (Transport-Wide Congestion Control) を優先使用
- [ ] REMB (Receiver Estimated Maximum Bitrate) はTWCC非対応クライアント向けフォールバック
- [ ] NACK処理（パケットロス検出から10ms以内に再送要求）
- [ ] PLI (Picture Loss Indication) 転送
- [ ] FIR (Full Intra Request) 転送
- [ ] RTX (Retransmission) 対応

#### 2.5.3 輻輳制御方式

- TWCCを優先（より正確な帯域幅推定）
- クライアントがTWCC非対応の場合のみREMBを使用
- 帯域幅推定の更新間隔: 100ms

**SFU側の挙動:**

| 受信状況       | SFUの処理                                          |
| -------------- | -------------------------------------------------- |
| TWCCのみ受信   | TWCCで帯域幅推定                                   |
| REMBのみ受信   | REMBで帯域幅推定（レガシークライアント対応）       |
| 両方受信       | TWCCを採用、REMBは無視                             |
| どちらも未受信 | デフォルト帯域幅を使用、品質低下時に段階的に下げる |

#### 2.5.4 品質制御トリガー

| 条件                                 | アクション               |
| ------------------------------------ | ------------------------ |
| パケットロス率 > 5%                  | 低レイヤーへ切り替え     |
| RTT > 300ms                          | 低レイヤーへ切り替え     |
| 帯域幅推定 < 現在のビットレート      | 低レイヤーへ切り替え     |
| パケットロス率 < 1% かつ帯域幅に余裕 | 高レイヤーへ切り替え検討 |

#### 2.5.5 レイヤー制御の優先順位

- クライアントの`setPreferredLayer`要求を最優先
- 自動品質制御はクライアント要求の範囲内で動作
- クライアント要求が満たせない場合の挙動:
  - 要求レイヤーが存在しない: 次に近いレイヤーを選択
  - 帯域幅不足: 自動的に低レイヤーへ切り替え、`layerChanged`通知を送信

**layerChanged 通知:**

```json
{
  "jsonrpc": "2.0",
  "method": "layerChanged",
  "params": {
    "trackId": "string",
    "requestedLayer": "string (クライアント要求)",
    "actualLayer": "string (実際に選択されたレイヤー)",
    "reason": "string (bandwidth|unavailable)"
  }
}
```

### 2.6 コーデック対応

**映像コーデック（優先順位順）:**

- [ ] VP8（必須、全クライアント対応）
- [ ] H.264（必須、ハードウェアデコード対応）
- [ ] VP9（推奨、SVC対応）
- [ ] AV1（オプション、将来対応）

**音声コーデック:**

- [ ] Opus（必須、48kHz、ステレオ対応）
- [ ] G.711（オプション、レガシー互換）

### 2.7 NAT越え・接続確立

#### 2.7.1 ICEサーバー構成

- [ ] 内蔵STUNサーバー機能
- [ ] 外部STUN/TURNサーバー連携
- [ ] 複数ICEサーバーのフォールバック

#### 2.7.2 TURN設定

| 項目                     | 仕様                                                                                           |
| ------------------------ | ---------------------------------------------------------------------------------------------- |
| 認証方式                 | TURN REST API方式（RFC 5389のLong-term認証メカニズムを利用、HMAC-SHA1による動的資格情報生成） |
| 資格情報の有効期限       | 24時間                                                                                         |
| 資格情報のローテーション | 12時間ごとに更新                                                                               |
| 対応プロトコル           | UDP, TCP, TLS                                                                                  |

**認証方式の詳細:**
- ユーザー名形式: `timestamp:participantID` (有効期限のUnixタイムスタンプ + 参加者ID)
- 資格情報: `base64(HMAC-SHA1(secret, username))` (共有秘密鍵によるHMAC署名)
- coturnサーバーとの互換性: `--use-auth-secret` および `--static-auth-secret` オプションに対応
- セキュリティ: 短命トークンによる動的認証、秘密鍵のみサーバー側で管理

#### 2.7.3 ICE処理

- [ ] ICE Lite対応（SFUはサーバーとして動作）
- [ ] ICE再起動（ネットワーク切り替え時）
- [ ] ICE候補の優先順位付け（host > srflx > relay）
- [ ] 接続確立タイムアウト: 30秒

#### 2.7.4 ICE Lite動作前提

- SFUはICE Liteとして動作（controlled role固定、候補収集を簡略化）
- クライアント側はフルICEを実行し、ICE Liteサーバーとの接続をサポート
- 全モダンブラウザはフルICE実装のためICE Liteサーバーと接続可能
- SFUは自身のhost候補のみを通知、srflx/relay候補は生成しない

#### 2.7.5 SFUネットワーク到達性要件

**ICE Liteを採用するため、SFUは以下のいずれかの条件を満たす必要がある:**

| 構成             | 説明                                     | 推奨度 |
| ---------------- | ---------------------------------------- | ------ |
| 公開IPアドレス   | SFUが直接公開IPを持つ                    | 推奨   |
| 1:1 NAT          | 静的NAT変換でSFUポートが外部から到達可能 | 可     |
| ロードバランサー | L4ロードバランサー経由でUDP転送          | 可     |

**運用要件:**

- SFUのUDPポート（10000-20000）はファイアウォールで開放する必要がある
- クライアントがNAT/ファイアウォール配下の場合はTURNサーバーを使用
- SFUが直接到達できない環境（NAT配下等）ではICE Liteは使用不可
- クラウド環境では Elastic IP または 静的IP の割り当てが必要

**TURN利用時の注意:**

- ICE LiteのSFU + TURNの組み合わせでは、クライアントがTURNを経由してSFUに接続
- TURNサーバーはSFUとは別のインスタンスで運用を推奨

#### 2.7.6 ネットワークポリシー

- [ ] IPv4/IPv6デュアルスタック対応
- [ ] IPv4優先（IPv6フォールバック）
- [ ] UDPポート範囲: 10000-20000（設定可能）

### 2.8 録画機能

#### 2.8.1 基本機能

- [ ] WebM形式での録画（VP8/VP9 + Opus）- 再エンコード不要
- [ ] MP4形式での録画（H.264 + AAC）オプション - 音声トランスコード必要
- [ ] 録画の開始・停止API

※ MP4形式はWebRTCのOpus音声をAACにトランスコードする必要があるため、サーバー負荷が増加する

#### 2.8.2 録画仕様

| 項目              | 仕様                                  |
| ----------------- | ------------------------------------- |
| 音声/映像同期精度 | ±50ms                                 |
| ファイル分割      | 1時間ごと、または1GB到達時            |
| ファイル命名規則  | `{roomId}_{trackId}_{timestamp}.webm` |
| 一時保存先        | `/tmp/recordings/`                    |
| 最終保存先        | Cloud Storage（GCP）                  |
| 保持期間          | 30日（設定可能）                      |

#### 2.8.3 録画対象選択

- [ ] ルーム全体の録画
- [ ] 特定参加者のみ録画
- [ ] 合成録画（全参加者をグリッド表示）はPhase 2以降

#### 2.8.4 法的要件

- [ ] 録画開始時の参加者への通知
- [ ] 録画同意フラグの管理
- [ ] 録画メタデータの保存（開始時刻、参加者リスト、同意状況）

## 3. 非機能要件

### 3.1 性能要件

| 項目                 | 要件                      | 測定方法               |
| -------------------- | ------------------------- | ---------------------- |
| 同時接続数           | 1ルームあたり最大100接続  | Prometheusメトリクス   |
| メディア転送遅延     | 片道50ms以下（SFU内処理） | RTCPタイムスタンプ差分 |
| エンドツーエンド遅延 | 片道200ms以下（目標）     | クライアント側計測     |
| スループット         | 1サーバーあたり1Gbps以上  | ネットワークI/O監視    |
| CPU使用率            | 80%以下で安定動作         | システムメトリクス     |
| メモリ使用量         | 1接続あたり10MB以下       | システムメトリクス     |

#### 3.1.1 映像品質上限

| 解像度    | fps | ビットレート上限 |
| --------- | --- | ---------------- |
| 1920x1080 | 30  | 4 Mbps           |
| 1280x720  | 30  | 2.5 Mbps         |
| 640x360   | 30  | 500 Kbps         |
| 320x180   | 15  | 150 Kbps         |

#### 3.1.2 同時配信能力

| 構成                      | サポート |
| ------------------------- | -------- |
| 100人ルーム、全員視聴のみ | 対応     |
| 50人ルーム、10人配信      | 対応     |
| 20人ルーム、全員配信      | 対応     |

### 3.2 可用性

- [ ] Graceful shutdown対応（進行中セッションの完了待機）
- [ ] ヘルスチェックエンドポイント（`/health`、`/ready`）
- [ ] 自動再接続サポート（クライアント側実装ガイド提供）
- [ ] 99.9%の可用性目標（月間ダウンタイム43分以内）

### 3.3 スケーラビリティ

#### 3.3.1 水平スケーリング

- [ ] ステートレス設計（セッション状態は外部ストアに保存）
- [ ] ルーム単位でのサーバー固定（Consistent Hashing）
- [ ] ロードバランサー連携（HTTP/WebSocket対応）

#### 3.3.2 ルーム分散戦略

- [ ] 単一SFUにルーム固定（基本モード）
- [ ] カスケード接続（大規模ルーム対応、将来実装）
- [ ] 共有状態ストア: Redis

#### 3.3.3 フェイルオーバー

- [ ] SFU障害時のルーム移行（接続維持は不可、再接続で復旧）
- [ ] ヘルスチェック失敗時の自動除外
- [ ] フェイルオーバー検知時間: 10秒以内

### 3.4 セキュリティ

#### 3.4.1 通信暗号化

- [ ] WebSocket: TLS 1.3（TLS 1.2も許可）
- [ ] メディア: DTLS 1.2 + SRTP
- [ ] E2EE: Insertable Streams対応（オプション、クライアント実装依存）

#### 3.4.2 認証・認可

| 項目                 | 仕様                                         |
| -------------------- | -------------------------------------------- |
| トークン形式         | JWT (RS256)                                  |
| トークン有効期限     | 1時間                                        |
| リフレッシュトークン | 7日間                                        |
| 必須クレーム         | `sub`, `iat`, `exp`, `room_id`, `iss`, `aud` |
| 任意クレーム         | `role`, `permissions`                        |

#### 3.4.3 JWTキー運用

| 項目               | 仕様                                    |
| ------------------ | --------------------------------------- |
| 署名アルゴリズム   | RS256（RSA 2048bit以上）                |
| キーID (kid)       | 必須、JWTヘッダーに含める               |
| 公開鍵配布         | JWKS (JSON Web Key Set) エンドポイント  |
| キーローテーション | 90日ごと、旧キーは7日間の猶予期間       |
| 失効管理           | 短TTL（1時間）+ ブラックリスト（Redis） |
| iss (発行者)       | 設定ファイルで指定、検証必須            |
| aud (対象者)       | SFUサーバー識別子、検証必須             |

#### 3.4.4 権限モデル

| ロール       | 権限                     |
| ------------ | ------------------------ |
| `admin`      | ルーム作成、削除、全操作 |
| `moderator`  | 参加者のミュート、キック |
| `publisher`  | メディア配信、受信       |
| `subscriber` | メディア受信のみ         |

#### 3.4.5 レート制限

| 対象                   | 制限              |
| ---------------------- | ----------------- |
| WebSocket接続          | 10回/秒/IP        |
| シグナリングメッセージ | 100回/秒/接続     |
| REST API               | 100回/分/トークン |
| ルーム作成             | 10回/分/ユーザー  |

#### 3.4.6 DoS対策

- [ ] 接続数上限（IPあたり、グローバル）
- [ ] 帯域幅上限（接続あたり、ルームあたり）
- [ ] 不正パケット検出・遮断

### 3.5 監視・運用

#### 3.5.1 メトリクス（Prometheus形式）

| メトリクス名                 | 型        | 説明                      |
| ---------------------------- | --------- | ------------------------- |
| `sfu_rooms_total`            | Gauge     | アクティブルーム数        |
| `sfu_connections_total`      | Gauge     | 総接続数                  |
| `sfu_connections_per_room`   | Histogram | ルームあたり接続数分布    |
| `sfu_bytes_received_total`   | Counter   | 受信バイト数              |
| `sfu_bytes_sent_total`       | Counter   | 送信バイト数              |
| `sfu_packets_received_total` | Counter   | 受信パケット数            |
| `sfu_packets_sent_total`     | Counter   | 送信パケット数            |
| `sfu_packets_lost_total`     | Counter   | ロストパケット数          |
| `sfu_rtt_seconds`            | Histogram | RTT分布                   |
| `sfu_jitter_seconds`         | Histogram | ジッタ分布                |
| `sfu_bitrate_bps`            | Gauge     | 現在のビットレート        |
| `sfu_simulcast_layer`        | Gauge     | 選択中のSimulcastレイヤー |

#### 3.5.2 ログ出力

| レベル | 出力内容                           |
| ------ | ---------------------------------- |
| ERROR  | 接続失敗、内部エラー、パニック     |
| WARN   | 再送多発、品質低下、レート制限発動 |
| INFO   | 接続/切断、ルーム作成/削除         |
| DEBUG  | シグナリングメッセージ、RTCP統計   |

#### 3.5.3 ログフォーマット（JSON）

```json
{
  "timestamp": "2025-01-14T10:00:00.000Z",
  "level": "INFO",
  "message": "participant joined",
  "room_id": "room-123",
  "participant_id": "user-456",
  "remote_ip": "[REDACTED]",
  "trace_id": "abc-123"
}
```

#### 3.5.4 ログの機密化範囲

| データ種別    | 処理方法               | DEBUGレベル        | INFO以上          |
| ------------- | ---------------------- | ------------------ | ----------------- |
| IPアドレス    | 最終オクテットをマスク | `192.168.1.xxx`    | `192.168.1.xxx`   |
| JWTトークン   | 先頭8文字のみ表示      | `eyJhbGci...`      | 非出力            |
| SDP           | offer/answer種別のみ   | 種別+行数          | 種別のみ          |
| ICE Candidate | タイプのみ表示         | `host/srflx/relay` | 非出力            |
| ユーザーID    | 設定により選択         | そのまま/ハッシュ  | そのまま/ハッシュ |
| ルームID      | そのまま出力           | そのまま           | そのまま          |

#### 3.5.5 PII取り扱い

- [ ] IPアドレスはログ出力時にマスク（最終オクテットを`xxx`に）
- [ ] ユーザーIDは設定によりハッシュ化可能
- [ ] ログ保持期間: 90日（設定可能）
- [ ] DEBUGログは本番環境で無効化を推奨

#### 3.5.6 アラート条件

| 条件                    | 重要度   | アクション            |
| ----------------------- | -------- | --------------------- |
| CPU使用率 > 80% 5分継続 | Warning  | 通知                  |
| CPU使用率 > 95% 1分継続 | Critical | 通知 + スケールアウト |
| メモリ使用率 > 80%      | Warning  | 通知                  |
| エラー率 > 1%           | Warning  | 通知                  |
| 接続失敗率 > 5%         | Critical | 通知 + 調査           |

### 3.6 クライアント互換性

#### 3.6.1 対応ブラウザ

| ブラウザ | 最小バージョン | 備考                 |
| -------- | -------------- | -------------------- |
| Chrome   | 90+            | 推奨                 |
| Firefox  | 85+            | 対応                 |
| Safari   | 14+            | 対応（一部制限あり） |
| Edge     | 90+            | Chrome準拠           |

#### 3.6.2 対応モバイル

| プラットフォーム  | 最小バージョン |
| ----------------- | -------------- |
| iOS Safari        | 14.0+          |
| Android Chrome    | 90+            |
| WebView (Android) | 90+            |

#### 3.6.3 クライアントSDK

- [ ] JavaScript SDK提供
- [ ] TypeScript型定義提供
- [ ] React Hooks提供（オプション）

## 4. 技術スタック

### 4.1 必須ライブラリ

| ライブラリ        | 用途                  |
| ----------------- | --------------------- |
| pion/webrtc       | WebRTCプロトコル実装  |
| gorilla/websocket | WebSocketシグナリング |

### 4.2 推奨ライブラリ

| ライブラリ               | 用途               |
| ------------------------ | ------------------ |
| go-chi/chi または gin    | HTTPルーティング   |
| zap または slog          | 構造化ログ         |
| prometheus/client_golang | メトリクス収集     |
| golang-jwt/jwt           | JWT処理            |
| redis/go-redis           | セッション状態管理 |

## 5. API設計

### 5.1 REST API

| エンドポイント                    | メソッド | 説明                 | 認証                 |
| --------------------------------- | -------- | -------------------- | -------------------- |
| `/api/v1/rooms`                   | POST     | ルーム作成           | 必須                 |
| `/api/v1/rooms/{id}`              | GET      | ルーム情報取得       | 必須                 |
| `/api/v1/rooms/{id}`              | DELETE   | ルーム削除           | 必須（admin）        |
| `/api/v1/rooms/{id}/participants` | GET      | 参加者一覧取得       | 必須                 |
| `/api/v1/rooms/{id}/token`        | POST     | 参加トークン発行     | 必須                 |
| `/api/v1/rooms/{id}/recording`    | POST     | 録画開始             | 必須（admin）        |
| `/api/v1/rooms/{id}/recording`    | DELETE   | 録画停止             | 必須（admin）        |
| `/health`                         | GET      | ヘルスチェック       | 不要                 |
| `/ready`                          | GET      | レディネスチェック   | 不要                 |
| `/metrics`                        | GET      | Prometheusメトリクス | 不要（内部のみ推奨） |

### 5.2 WebSocket API

#### 5.2.1 クライアント→サーバー

| メソッド            | 説明                  | 必須パラメータ           |
| ------------------- | --------------------- | ------------------------ |
| `join`              | ルームへの参加        | `token`                  |
| `leave`             | ルームからの退出      | -                        |
| `offer`             | SDP Offer送信         | `sdp`                    |
| `answer`            | SDP Answer送信        | `sdp`                    |
| `candidate`         | ICE Candidate送信     | `candidate`              |
| `publish`           | メディア配信開始      | `kind`                   |
| `unpublish`         | メディア配信停止      | `trackId`                |
| `subscribe`         | メディア購読開始      | `publisherId`, `trackId` |
| `unsubscribe`       | メディア購読停止      | `subscriptionId`         |
| `setPreferredLayer` | Simulcastレイヤー指定 | `trackId`, `layer`       |

#### 5.2.2 サーバー→クライアント

| メソッド            | 説明                            |
| ------------------- | ------------------------------- |
| `joined`            | 参加完了通知                    |
| `left`              | 退出完了通知                    |
| `participantJoined` | 他参加者の参加通知              |
| `participantLeft`   | 他参加者の退出通知              |
| `trackPublished`    | トラック配信開始通知            |
| `trackUnpublished`  | トラック配信停止通知            |
| `offer`             | SDP Offer（再ネゴシエーション） |
| `answer`            | SDP Answer                      |
| `candidate`         | ICE Candidate                   |
| `error`             | エラー通知                      |
| `reconnect`         | 再接続要求                      |

## 6. ディレクトリ構成（案）

```text
choicespec/
├── cmd/
│   └── sfu/
│       └── main.go
├── internal/
│   ├── server/
│   │   ├── server.go
│   │   ├── handler.go
│   │   └── middleware.go
│   ├── room/
│   │   ├── room.go
│   │   ├── participant.go
│   │   └── state.go
│   ├── signaling/
│   │   ├── websocket.go
│   │   ├── message.go
│   │   └── protocol.go
│   ├── media/
│   │   ├── track.go
│   │   ├── router.go
│   │   ├── simulcast.go
│   │   └── rtcp.go
│   ├── ice/
│   │   ├── server.go
│   │   └── turn.go
│   ├── recording/
│   │   ├── recorder.go
│   │   └── storage.go
│   └── auth/
│       ├── jwt.go
│       └── permission.go
├── pkg/
│   ├── config/
│   │   └── config.go
│   └── metrics/
│       └── prometheus.go
├── docs/
│   └── requirements.md
├── go.mod
└── go.sum
```

## 7. 開発フェーズ

### Phase 1: 基本機能

- シグナリングサーバー実装
- 1:1接続の確立
- 基本的なメディア転送
- JWT認証

### Phase 2: ルーム機能

- マルチユーザー対応
- ルーム管理機能
- 参加者管理
- 権限モデル実装

### Phase 3: 品質最適化

- Simulcast対応
- RTCP処理実装
- 帯域幅制御
- 品質監視

### Phase 4: 運用機能

- メトリクス・ログ整備
- 録画機能
- クライアントSDK
- ドキュメント整備

## 8. 設計上の決定事項

### 8.1 E2EEについて

- SFUは平文のRTPペイロードを扱う（再エンコードなしの転送のため）
- E2EEが必要な場合はInsertable Streamsをクライアント側で実装
- SFUはRTPヘッダーのみ参照、ペイロードは転送のみ

### 8.2 Simulcast vs SVC

- Simulcastを優先実装（VP8/H.264での広いクライアント互換性）
- SVCはVP9/AV1使用時のオプションとして対応

### 8.3 ルーム分散

- 初期実装は単一SFUにルーム固定
- 大規模対応が必要な場合はカスケード接続を検討

## 9. 参考資料

- [WebRTC 1.0: Real-Time Communication Between Browsers](https://www.w3.org/TR/webrtc/)
- [Pion WebRTC](https://github.com/pion/webrtc)
- [RFC 8825 - Overview: Real-Time Protocols for Browser-Based Applications](https://datatracker.ietf.org/doc/html/rfc8825)
- [RFC 5389 - STUN](https://datatracker.ietf.org/doc/html/rfc5389)
- [RFC 8656 - TURN](https://datatracker.ietf.org/doc/html/rfc8656)
- [RFC 4585 - Extended RTP Profile for RTCP-Based Feedback](https://datatracker.ietf.org/doc/html/rfc4585)
