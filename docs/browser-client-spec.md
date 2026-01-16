# ブラウザクライアント仕様書

Choice SFUのブラウザクライアント実装仕様。WebRTC接続、シグナリングプロトコル、メディア処理の実装要件を定義する。

## 1. 概要

### 1.1 対応ブラウザ

| ブラウザ | バージョン | 備考                          |
| -------- | ---------- | ----------------------------- |
| Chrome   | 90+        | 推奨ブラウザ                  |
| Firefox  | 85+        | -                             |
| Safari   | 14+        | H.264対応、WebRTC API差異あり |
| Edge     | 90+        | Chromiumベース                |

### 1.2 必須Web API

| API                | 用途                   |
| ------------------ | ---------------------- |
| WebSocket          | シグナリング通信       |
| RTCPeerConnection  | WebRTC接続             |
| MediaDevices       | カメラ・マイクアクセス |
| MediaStream        | メディアストリーム管理 |
| Screen Capture API | 画面共有               |

## 2. 接続フロー

### 2.1 全体フロー

```text
┌─────────────────────────────────────────────────────────────────────┐
│                        Connection Flow                               │
├─────────────────────────────────────────────────────────────────────┤
│                                                                     │
│  1. WebSocket接続                                                   │
│     └─> wss://sfu.example.com/ws                                   │
│                                                                     │
│  2. ルーム参加 (join)                                               │
│     └─> JWTトークンを送信、セッションID取得                        │
│                                                                     │
│  3. PeerConnection作成                                              │
│     └─> ICEサーバー設定、コーデック設定                            │
│                                                                     │
│  4. メディア配信 (publish)                                          │
│     └─> トラック追加、SDP Offer/Answer交換                         │
│                                                                     │
│  5. メディア購読 (subscribe)                                        │
│     └─> サーバーからのOffer受信、Answer送信                        │
│                                                                     │
│  6. ICE候補交換                                                     │
│     └─> 双方向でcandidate交換                                      │
│                                                                     │
│  7. メディア通信開始                                                │
│     └─> RTP/RTCP送受信                                             │
│                                                                     │
└─────────────────────────────────────────────────────────────────────┘
```

### 2.2 状態遷移図

```text
                    ┌──────────────┐
                    │ disconnected │
                    └──────┬───────┘
                           │ connect()
                           ▼
                    ┌──────────────┐
                    │  connecting  │
                    └──────┬───────┘
                           │ WebSocket open
                           ▼
                    ┌──────────────┐
        ┌─────────▶│  connected   │◀─────────┐
        │          └──────┬───────┘          │
        │                 │ join()           │
        │                 ▼                  │
        │          ┌──────────────┐          │
        │          │   joining    │          │
        │          └──────┬───────┘          │
        │                 │ join result      │
        │                 ▼                  │
        │          ┌──────────────┐          │
        │          │    joined    │──────────┤ reconnected
        │          └──────┬───────┘          │
        │                 │ disconnect       │
        │                 ▼                  │
        │          ┌──────────────┐          │
        │          │   leaving    │          │
        │          └──────┬───────┘          │
        │                 │                  │
        │                 ▼                  │
        │          ┌──────────────┐          │
        │          │ disconnected │          │
        │          └──────────────┘          │
        │                                    │
        │          ┌──────────────┐          │
        └──────────│ reconnecting │──────────┘
                   └──────────────┘
```

## 3. シグナリングプロトコル

### 3.1 WebSocket接続

```javascript
const ws = new WebSocket("wss://sfu.example.com/ws");

ws.onopen = () => {
    console.log("WebSocket connected");
};

ws.onmessage = (event) => {
    const message = JSON.parse(event.data);
    handleMessage(message);
};

ws.onclose = (event) => {
    console.log("WebSocket closed:", event.code, event.reason);
};

ws.onerror = (error) => {
    console.error("WebSocket error:", error);
};
```

### 3.2 JSON-RPC 2.0メッセージ形式

#### リクエスト

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "メソッド名",
  "params": { ... }
}
```

#### レスポンス（成功）

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": { ... }
}
```

#### レスポンス（エラー）

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "error": {
    "code": 1001,
    "message": "Room not found",
    "data": { ... }
  }
}
```

#### 通知（サーバープッシュ）

```json
{
  "jsonrpc": "2.0",
  "method": "通知名",
  "params": { ... }
}
```

### 3.3 クライアントからサーバーへのメソッド

#### join - ルーム参加

```json
// リクエスト
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "join",
  "params": {
    "token": "eyJhbGciOiJSUzI1NiIs...",
    "sessionId": "session-123"  // 再接続時のみ
  }
}

// レスポンス
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "sessionId": "session-456",
    "participantId": "participant-789",
    "roomId": "room-abc",
    "iceServers": [
      {
        "urls": ["stun:stun.example.com:3478"]
      },
      {
        "urls": ["turn:turn.example.com:3478"],
        "username": "user",
        "credential": "pass"
      }
    ],
    "participants": [
      {
        "id": "participant-001",
        "metadata": { "displayName": "Alice" },
        "tracks": [
          {
            "id": "track-001",
            "kind": "video",
            "simulcast": true,
            "metadata": { "source": "camera" }
          }
        ]
      }
    ],
    "restored": false
  }
}
```

#### leave - ルーム退出

```json
// リクエスト
{
  "jsonrpc": "2.0",
  "id": 2,
  "method": "leave",
  "params": {}
}

// レスポンス
{
  "jsonrpc": "2.0",
  "id": 2,
  "result": {}
}
```

#### publish - トラック配信

```json
// リクエスト
{
  "jsonrpc": "2.0",
  "id": 3,
  "method": "publish",
  "params": {
    "kind": "video",
    "simulcast": true,
    "name": "camera",
    "metadata": { "source": "camera" }
  }
}

// レスポンス
{
  "jsonrpc": "2.0",
  "id": 3,
  "result": {
    "trackId": "track-123",
    "mid": "0"
  }
}
```

#### unpublish - 配信停止

```json
// リクエスト
{
  "jsonrpc": "2.0",
  "id": 4,
  "method": "unpublish",
  "params": {
    "trackId": "track-123"
  }
}

// レスポンス
{
  "jsonrpc": "2.0",
  "id": 4,
  "result": {}
}
```

#### subscribe - トラック購読

```json
// リクエスト
{
  "jsonrpc": "2.0",
  "id": 5,
  "method": "subscribe",
  "params": {
    "publisherId": "participant-001",
    "trackId": "track-001",
    "preferredLayer": "h"
  }
}

// レスポンス
{
  "jsonrpc": "2.0",
  "id": 5,
  "result": {
    "subscriptionId": "sub-456"
  }
}
```

#### unsubscribe - 購読解除

```json
// リクエスト
{
  "jsonrpc": "2.0",
  "id": 6,
  "method": "unsubscribe",
  "params": {
    "subscriptionId": "sub-456"
  }
}

// レスポンス
{
  "jsonrpc": "2.0",
  "id": 6,
  "result": {}
}
```

#### setPreferredLayer - Simulcastレイヤー設定

```json
// リクエスト
{
  "jsonrpc": "2.0",
  "id": 7,
  "method": "setPreferredLayer",
  "params": {
    "trackId": "track-001",
    "layer": "m"
  }
}

// レスポンス
{
  "jsonrpc": "2.0",
  "id": 7,
  "result": {}
}
```

#### offer - SDPオファー送信

```json
// リクエスト
{
  "jsonrpc": "2.0",
  "id": 8,
  "method": "offer",
  "params": {
    "sdp": "v=0\r\no=- 123456789 2 IN IP4 127.0.0.1\r\n..."
  }
}

// レスポンス
{
  "jsonrpc": "2.0",
  "id": 8,
  "result": {
    "sdp": "v=0\r\no=- 987654321 2 IN IP4 192.168.1.1\r\n..."
  }
}
```

#### answer - SDPアンサー送信

```json
// リクエスト
{
  "jsonrpc": "2.0",
  "id": 9,
  "method": "answer",
  "params": {
    "sdp": "v=0\r\no=- 123456789 2 IN IP4 127.0.0.1\r\n..."
  }
}

// レスポンス
{
  "jsonrpc": "2.0",
  "id": 9,
  "result": {}
}
```

#### candidate - ICE候補送信

```json
// リクエスト
{
  "jsonrpc": "2.0",
  "id": 10,
  "method": "candidate",
  "params": {
    "candidate": "candidate:1 1 UDP 2122252543 192.168.1.100 52000 typ host",
    "sdpMid": "0",
    "sdpMLineIndex": 0
  }
}

// レスポンス
{
  "jsonrpc": "2.0",
  "id": 10,
  "result": {}
}
```

### 3.4 サーバーからクライアントへの通知

#### participantJoined - 参加者入室

```json
{
    "jsonrpc": "2.0",
    "method": "participantJoined",
    "params": {
        "id": "participant-002",
        "metadata": { "displayName": "Bob" }
    }
}
```

#### participantLeft - 参加者退出

```json
{
    "jsonrpc": "2.0",
    "method": "participantLeft",
    "params": {
        "id": "participant-002",
        "reason": "leave"
    }
}
```

| reason    | 説明               |
| --------- | ------------------ |
| `leave`   | 自発的な退出       |
| `timeout` | 接続タイムアウト   |
| `kicked`  | 管理者によるキック |

#### trackPublished - トラック公開

```json
{
    "jsonrpc": "2.0",
    "method": "trackPublished",
    "params": {
        "publisherId": "participant-002",
        "trackId": "track-002",
        "kind": "video",
        "simulcast": true,
        "metadata": { "source": "camera" }
    }
}
```

#### trackUnpublished - トラック削除

```json
{
    "jsonrpc": "2.0",
    "method": "trackUnpublished",
    "params": {
        "publisherId": "participant-002",
        "trackId": "track-002"
    }
}
```

#### offer - サーバーからのSDPオファー

```json
{
    "jsonrpc": "2.0",
    "method": "offer",
    "params": {
        "sdp": "v=0\r\no=- 987654321 2 IN IP4 192.168.1.1\r\n...",
        "reason": "track_added"
    }
}
```

| reason              | 説明              |
| ------------------- | ----------------- |
| `track_added`       | トラック追加      |
| `track_removed`     | トラック削除      |
| `simulcast_changed` | Simulcast構成変更 |
| `codec_changed`     | コーデック変更    |
| `ice_restart`       | ICE再起動         |

#### candidate - サーバーからのICE候補

```json
{
    "jsonrpc": "2.0",
    "method": "candidate",
    "params": {
        "candidate": "candidate:1 1 UDP 2122252543 203.0.113.1 10000 typ host",
        "sdpMid": "0",
        "sdpMLineIndex": 0
    }
}
```

#### layerChanged - Simulcastレイヤー変更

```json
{
    "jsonrpc": "2.0",
    "method": "layerChanged",
    "params": {
        "trackId": "track-001",
        "requestedLayer": "h",
        "actualLayer": "m",
        "reason": "bandwidth"
    }
}
```

| reason        | 説明                     |
| ------------- | ------------------------ |
| `bandwidth`   | 帯域幅不足による切り替え |
| `unavailable` | 要求レイヤーが存在しない |

#### error - サーバーエラー通知

```json
{
    "jsonrpc": "2.0",
    "method": "error",
    "params": {
        "code": 1008,
        "message": "ICE connection failed",
        "fatal": true
    }
}
```

#### reconnect - 再接続要求

```json
{
    "jsonrpc": "2.0",
    "method": "reconnect",
    "params": {
        "reason": "ice_disconnected",
        "retryAfterMs": 1000
    }
}
```

| reason             | 説明           |
| ------------------ | -------------- |
| `ice_disconnected` | ICE接続切断    |
| `server_restart`   | サーバー再起動 |

#### recordingStarted - 録画開始通知

```json
{
    "jsonrpc": "2.0",
    "method": "recordingStarted",
    "params": {
        "recordingId": "rec-001",
        "startedBy": "participant-admin"
    }
}
```

#### recordingStopped - 録画停止通知

```json
{
    "jsonrpc": "2.0",
    "method": "recordingStopped",
    "params": {
        "recordingId": "rec-001",
        "stoppedBy": "participant-admin"
    }
}
```

### 3.5 エラーコード

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

## 4. WebRTC実装

### 4.1 PeerConnection設定

```javascript
const config = {
    iceServers: [
        { urls: "stun:stun.example.com:3478" },
        {
            urls: "turn:turn.example.com:3478",
            username: "user",
            credential: "pass",
        },
    ],
    iceTransportPolicy: "all",
    bundlePolicy: "max-bundle",
    rtcpMuxPolicy: "require",
    sdpSemantics: "unified-plan",
};

const pc = new RTCPeerConnection(config);
```

### 4.2 SDP要件

#### 必須設定

| 設定          | 値           | 説明               |
| ------------- | ------------ | ------------------ |
| sdpSemantics  | unified-plan | Plan B非対応       |
| bundlePolicy  | max-bundle   | 単一トランスポート |
| rtcpMuxPolicy | require      | RTP/RTCP多重化必須 |

#### RTPヘッダー拡張

SDPに以下のRTPヘッダー拡張を含める:

```text
a=extmap:1 urn:ietf:params:rtp-hdrext:sdes:mid
a=extmap:2 urn:ietf:params:rtp-hdrext:sdes:rtp-stream-id
a=extmap:3 http://www.ietf.org/id/draft-holmer-rmcat-transport-wide-cc-extensions-01
a=extmap:4 http://www.webrtc.org/experiments/rtp-hdrext/abs-send-time
```

#### RTCPフィードバック

映像トラックには以下のRTCPフィードバックを設定:

```text
a=rtcp-fb:96 nack
a=rtcp-fb:96 nack pli
a=rtcp-fb:96 ccm fir
a=rtcp-fb:96 goog-remb
a=rtcp-fb:96 transport-cc
```

### 4.3 コーデック設定

#### 映像コーデック優先順位

1. VP8 - 最も広くサポート
2. H.264 - Safari対応に必要
3. VP9 - 高圧縮率（オプション）

#### H.264プロファイル

```text
# High Profile Level 5.0（高品質、デスクトップ向け）
a=fmtp:96 profile-level-id=640032;packetization-mode=1;level-asymmetry-allowed=1

# Constrained Baseline Level 3.1（Safari・モバイル互換）
a=fmtp:97 profile-level-id=42e01f;packetization-mode=1;level-asymmetry-allowed=1
```

#### 音声コーデック

Opusを使用:

```text
a=fmtp:111 minptime=10;useinbandfec=1;stereo=1
```

### 4.4 Simulcast設定

```javascript
// Simulcastエンコーディング設定
const encodings = [
    {
        rid: "h",
        maxBitrate: 2500000,
        maxFramerate: 30,
        scaleResolutionDownBy: 1,
    },
    {
        rid: "m",
        maxBitrate: 500000,
        maxFramerate: 30,
        scaleResolutionDownBy: 2,
    },
    {
        rid: "l",
        maxBitrate: 150000,
        maxFramerate: 15,
        scaleResolutionDownBy: 4,
    },
];

// トラック追加時にSimulcast設定
const sender = pc.addTrack(videoTrack, stream);
const params = sender.getParameters();
params.encodings = encodings;
await sender.setParameters(params);
```

#### Simulcastレイヤー仕様

| レイヤー | RID | 解像度   | ビットレート | FPS |
| -------- | --- | -------- | ------------ | --- |
| High     | h   | 1280x720 | 2.5 Mbps     | 30  |
| Medium   | m   | 640x360  | 500 kbps     | 30  |
| Low      | l   | 320x180  | 150 kbps     | 15  |

### 4.5 ICE処理

```javascript
// ICE候補収集
pc.onicecandidate = (event) => {
    if (event.candidate) {
        sendToServer({
            jsonrpc: "2.0",
            id: nextId(),
            method: "candidate",
            params: {
                candidate: event.candidate.candidate,
                sdpMid: event.candidate.sdpMid,
                sdpMLineIndex: event.candidate.sdpMLineIndex,
            },
        });
    }
};

// ICE接続状態監視
pc.oniceconnectionstatechange = () => {
    console.log("ICE state:", pc.iceConnectionState);

    switch (pc.iceConnectionState) {
        case "connected":
        case "completed":
            // 接続成功
            break;
        case "failed":
            // ICE再起動を試行
            restartICE();
            break;
        case "disconnected":
            // 一時的な切断、再接続待機
            scheduleReconnect();
            break;
    }
};

// ICE再起動
async function restartICE() {
    const offer = await pc.createOffer({ iceRestart: true });
    await pc.setLocalDescription(offer);
    // サーバーにofferを送信
}
```

### 4.6 メディアトラック処理

```javascript
// ローカルトラック追加
async function publishTrack(track, options = {}) {
    const sender = pc.addTrack(track);

    if (track.kind === "video" && options.simulcast) {
        const params = sender.getParameters();
        params.encodings = [
            { rid: "h", maxBitrate: 2500000 },
            { rid: "m", maxBitrate: 500000, scaleResolutionDownBy: 2 },
            { rid: "l", maxBitrate: 150000, scaleResolutionDownBy: 4 },
        ];
        await sender.setParameters(params);
    }

    // サーバーにpublishリクエスト
    const result = await sendRequest("publish", {
        kind: track.kind,
        simulcast: options.simulcast,
        name: options.name,
        metadata: options.metadata,
    });

    // SDP再ネゴシエーション
    await renegotiate();

    return result.trackId;
}

// リモートトラック受信
pc.ontrack = (event) => {
    const track = event.track;
    const stream = event.streams[0];

    console.log("Track received:", track.kind, track.id);

    // トラックをメディア要素にアタッチ
    const element = document.createElement(track.kind);
    element.srcObject = stream;
    element.autoplay = true;
    element.playsInline = true;

    document.body.appendChild(element);
};
```

## 5. メディア取得

### 5.1 カメラ・マイク

```javascript
async function getLocalMedia(constraints = {}) {
    const defaultConstraints = {
        video: {
            width: { ideal: 1280 },
            height: { ideal: 720 },
            frameRate: { ideal: 30 },
        },
        audio: {
            echoCancellation: true,
            noiseSuppression: true,
            autoGainControl: true,
        },
    };

    const mergedConstraints = {
        ...defaultConstraints,
        ...constraints,
    };

    try {
        return await navigator.mediaDevices.getUserMedia(mergedConstraints);
    } catch (error) {
        handleMediaError(error);
        throw error;
    }
}

function handleMediaError(error) {
    switch (error.name) {
        case "NotAllowedError":
            console.error("Permission denied");
            break;
        case "NotFoundError":
            console.error("No camera/microphone found");
            break;
        case "NotReadableError":
            console.error("Device in use by another application");
            break;
        case "OverconstrainedError":
            console.error("Constraints cannot be satisfied");
            break;
        default:
            console.error("Unknown error:", error);
    }
}
```

### 5.2 画面共有

```javascript
async function getScreenShare() {
    const constraints = {
        video: {
            cursor: "always",
            displaySurface: "monitor",
        },
        audio: {
            echoCancellation: false,
            noiseSuppression: false,
            autoGainControl: false,
        },
    };

    try {
        const stream =
            await navigator.mediaDevices.getDisplayMedia(constraints);

        // 画面共有停止検知
        stream.getVideoTracks()[0].onended = () => {
            console.log("Screen sharing stopped");
            handleScreenShareEnd();
        };

        return stream;
    } catch (error) {
        if (error.name === "NotAllowedError") {
            console.log("User cancelled screen sharing");
        }
        throw error;
    }
}
```

### 5.3 デバイス列挙

```javascript
async function enumerateDevices() {
    const devices = await navigator.mediaDevices.enumerateDevices();

    const cameras = devices.filter((d) => d.kind === "videoinput");
    const microphones = devices.filter((d) => d.kind === "audioinput");
    const speakers = devices.filter((d) => d.kind === "audiooutput");

    return { cameras, microphones, speakers };
}

// デバイス変更監視
navigator.mediaDevices.ondevicechange = async () => {
    const devices = await enumerateDevices();
    console.log("Devices changed:", devices);
};
```

## 6. 再接続処理

### 6.1 再接続戦略

```javascript
class ReconnectManager {
    constructor(options = {}) {
        this.maxAttempts = options.maxAttempts || 5;
        this.initialDelay = options.initialDelay || 1000;
        this.maxDelay = options.maxDelay || 30000;
        this.factor = options.factor || 2;

        this.attempts = 0;
        this.sessionId = null;
    }

    async reconnect() {
        while (this.attempts < this.maxAttempts) {
            const delay = this.calculateDelay();
            console.log(
                `Reconnecting in ${delay}ms (attempt ${this.attempts + 1})`,
            );

            await this.sleep(delay);

            try {
                await this.connect();
                await this.rejoin();
                this.attempts = 0;
                return true;
            } catch (error) {
                this.attempts++;
                console.error("Reconnect failed:", error);
            }
        }

        console.error("Max reconnection attempts reached");
        return false;
    }

    calculateDelay() {
        const delay = this.initialDelay * Math.pow(this.factor, this.attempts);
        return Math.min(delay, this.maxDelay);
    }

    async rejoin() {
        const result = await sendRequest("join", {
            token: this.token,
            sessionId: this.sessionId,
        });

        if (result.restored) {
            console.log("Session restored");
            // 状態復元
        } else {
            console.log("Session expired, fresh join");
            this.sessionId = result.sessionId;
        }
    }

    sleep(ms) {
        return new Promise((resolve) => setTimeout(resolve, ms));
    }
}
```

### 6.2 再接続パラメータ

| パラメータ     | デフォルト | 説明                 |
| -------------- | ---------- | -------------------- |
| maxAttempts    | 5          | 最大リトライ回数     |
| initialDelay   | 1000ms     | 初回リトライ待機時間 |
| maxDelay       | 30000ms    | 最大リトライ待機時間 |
| factor         | 2          | 指数バックオフ係数   |
| sessionTimeout | 30000ms    | セッション保持時間   |

## 7. ブラウザ互換性

### 7.1 Safari対応

```javascript
// Safari判定
const isSafari = /^((?!chrome|android).)*safari/i.test(navigator.userAgent);

// Safari用SDP調整
function normalizeSafariSDP(sdp) {
    if (!isSafari) return sdp;

    // Safari固有のSDP調整が必要な場合はここで処理
    return sdp;
}

// Safari用コーデック設定
function getSafariCodecPreferences() {
    return {
        video: ["H264"], // SafariはH.264を優先
        audio: ["opus"],
    };
}
```

### 7.2 Firefox対応

```javascript
const isFirefox = navigator.userAgent.includes("Firefox");

// Firefox用設定
function getFirefoxConfig() {
    if (!isFirefox) return {};

    return {
        // Firefox固有の設定
    };
}
```

### 7.3 機能検出

```javascript
function checkWebRTCSupport() {
    const support = {
        webRTC: !!window.RTCPeerConnection,
        getUserMedia: !!(
            navigator.mediaDevices && navigator.mediaDevices.getUserMedia
        ),
        getDisplayMedia: !!(
            navigator.mediaDevices && navigator.mediaDevices.getDisplayMedia
        ),
        webSocket: !!window.WebSocket,
    };

    const missing = Object.entries(support)
        .filter(([, supported]) => !supported)
        .map(([feature]) => feature);

    if (missing.length > 0) {
        throw new Error(`Missing features: ${missing.join(", ")}`);
    }

    return support;
}
```

## 8. 実装例

### 8.1 完全な接続フロー

```javascript
class SFUClient {
    constructor(url) {
        this.url = url;
        this.ws = null;
        this.pc = null;
        this.pendingRequests = new Map();
        this.nextId = 1;
        this.sessionId = null;
    }

    // WebSocket接続
    async connect() {
        return new Promise((resolve, reject) => {
            this.ws = new WebSocket(this.url);

            this.ws.onopen = () => resolve();
            this.ws.onerror = (error) => reject(error);
            this.ws.onmessage = (event) =>
                this.handleMessage(JSON.parse(event.data));
            this.ws.onclose = () => this.handleClose();
        });
    }

    // ルーム参加
    async join(token) {
        const result = await this.sendRequest("join", {
            token,
            sessionId: this.sessionId,
        });

        this.sessionId = result.sessionId;

        // PeerConnection作成
        this.pc = new RTCPeerConnection({
            iceServers: result.iceServers,
            bundlePolicy: "max-bundle",
            rtcpMuxPolicy: "require",
        });

        this.setupPeerConnection();

        return result;
    }

    // PeerConnection設定
    setupPeerConnection() {
        this.pc.onicecandidate = (event) => {
            if (event.candidate) {
                this.sendRequest("candidate", {
                    candidate: event.candidate.candidate,
                    sdpMid: event.candidate.sdpMid,
                    sdpMLineIndex: event.candidate.sdpMLineIndex,
                });
            }
        };

        this.pc.ontrack = (event) => {
            this.onTrack?.(event.track, event.streams[0]);
        };

        this.pc.oniceconnectionstatechange = () => {
            if (this.pc.iceConnectionState === "failed") {
                this.restartICE();
            }
        };
    }

    // トラック配信
    async publish(track, options = {}) {
        this.pc.addTrack(track);

        const result = await this.sendRequest("publish", {
            kind: track.kind,
            simulcast: options.simulcast,
            name: options.name,
        });

        await this.renegotiate();

        return result.trackId;
    }

    // トラック購読
    async subscribe(publisherId, trackId, options = {}) {
        const result = await this.sendRequest("subscribe", {
            publisherId,
            trackId,
            preferredLayer: options.preferredLayer || "h",
        });

        return result.subscriptionId;
    }

    // SDP再ネゴシエーション
    async renegotiate() {
        const offer = await this.pc.createOffer();
        await this.pc.setLocalDescription(offer);

        const result = await this.sendRequest("offer", {
            sdp: this.pc.localDescription.sdp,
        });

        await this.pc.setRemoteDescription({
            type: "answer",
            sdp: result.sdp,
        });
    }

    // ICE再起動
    async restartICE() {
        const offer = await this.pc.createOffer({ iceRestart: true });
        await this.pc.setLocalDescription(offer);

        const result = await this.sendRequest("offer", {
            sdp: this.pc.localDescription.sdp,
        });

        await this.pc.setRemoteDescription({
            type: "answer",
            sdp: result.sdp,
        });
    }

    // JSON-RPCリクエスト送信
    sendRequest(method, params) {
        return new Promise((resolve, reject) => {
            const id = this.nextId++;

            this.pendingRequests.set(id, { resolve, reject });

            this.ws.send(
                JSON.stringify({
                    jsonrpc: "2.0",
                    id,
                    method,
                    params,
                }),
            );

            // タイムアウト
            setTimeout(() => {
                if (this.pendingRequests.has(id)) {
                    this.pendingRequests.delete(id);
                    reject(new Error("Request timeout"));
                }
            }, 10000);
        });
    }

    // メッセージ処理
    handleMessage(message) {
        if (message.id) {
            // レスポンス
            const pending = this.pendingRequests.get(message.id);
            if (pending) {
                this.pendingRequests.delete(message.id);
                if (message.error) {
                    pending.reject(new Error(message.error.message));
                } else {
                    pending.resolve(message.result);
                }
            }
        } else if (message.method) {
            // 通知
            this.handleNotification(message.method, message.params);
        }
    }

    // 通知処理
    handleNotification(method, params) {
        switch (method) {
            case "participantJoined":
                this.onParticipantJoined?.(params);
                break;
            case "participantLeft":
                this.onParticipantLeft?.(params);
                break;
            case "trackPublished":
                this.onTrackPublished?.(params);
                break;
            case "trackUnpublished":
                this.onTrackUnpublished?.(params);
                break;
            case "offer":
                this.handleServerOffer(params);
                break;
            case "candidate":
                this.handleServerCandidate(params);
                break;
            case "layerChanged":
                this.onLayerChanged?.(params);
                break;
            case "error":
                this.onError?.(params);
                break;
            case "reconnect":
                this.handleReconnectRequest(params);
                break;
        }
    }

    // サーバーからのOffer処理
    async handleServerOffer(params) {
        await this.pc.setRemoteDescription({
            type: "offer",
            sdp: params.sdp,
        });

        const answer = await this.pc.createAnswer();
        await this.pc.setLocalDescription(answer);

        await this.sendRequest("answer", {
            sdp: this.pc.localDescription.sdp,
        });
    }

    // サーバーからのICE候補処理
    async handleServerCandidate(params) {
        await this.pc.addIceCandidate({
            candidate: params.candidate,
            sdpMid: params.sdpMid,
            sdpMLineIndex: params.sdpMLineIndex,
        });
    }

    // 再接続要求処理
    handleReconnectRequest(params) {
        console.log("Reconnect requested:", params.reason);
        setTimeout(() => {
            this.reconnect();
        }, params.retryAfterMs);
    }

    // 切断処理
    handleClose() {
        console.log("WebSocket closed");
        this.onDisconnected?.();
    }

    // 切断
    disconnect() {
        this.pc?.close();
        this.ws?.close();
    }
}
```

### 8.2 使用例

```javascript
// クライアント作成
const client = new SFUClient("wss://sfu.example.com/ws");

// イベントハンドラ設定
client.onParticipantJoined = (params) => {
    console.log("Participant joined:", params.id);
};

client.onTrackPublished = async (params) => {
    console.log("Track published:", params.trackId);
    await client.subscribe(params.publisherId, params.trackId);
};

client.onTrack = (track, stream) => {
    const element = document.createElement(track.kind);
    element.srcObject = stream;
    element.autoplay = true;
    element.playsInline = true;
    document.getElementById("videos").appendChild(element);
};

// 接続
await client.connect();

// ルーム参加
const joinResult = await client.join("your-jwt-token");
console.log("Joined room:", joinResult.roomId);

// ローカルメディア取得と配信
const stream = await navigator.mediaDevices.getUserMedia({
    video: true,
    audio: true,
});

const videoTrackId = await client.publish(stream.getVideoTracks()[0], {
    simulcast: true,
    name: "camera",
});

const audioTrackId = await client.publish(stream.getAudioTracks()[0], {
    name: "microphone",
});

// ローカル映像表示
const localVideo = document.getElementById("local-video");
localVideo.srcObject = stream;
```

## 9. デバッグ

### 9.1 WebRTC統計情報

```javascript
async function getStats(pc) {
    const stats = await pc.getStats();

    stats.forEach((report) => {
        if (report.type === "inbound-rtp") {
            console.log("Inbound RTP:", {
                kind: report.kind,
                packetsReceived: report.packetsReceived,
                bytesReceived: report.bytesReceived,
                packetsLost: report.packetsLost,
                jitter: report.jitter,
            });
        } else if (report.type === "outbound-rtp") {
            console.log("Outbound RTP:", {
                kind: report.kind,
                packetsSent: report.packetsSent,
                bytesSent: report.bytesSent,
            });
        }
    });
}
```

### 9.2 ログ出力

```javascript
const LOG_LEVELS = {
    DEBUG: 0,
    INFO: 1,
    WARN: 2,
    ERROR: 3,
};

class Logger {
    constructor(level = LOG_LEVELS.INFO) {
        this.level = level;
    }

    debug(...args) {
        if (this.level <= LOG_LEVELS.DEBUG) {
            console.debug("[DEBUG]", new Date().toISOString(), ...args);
        }
    }

    info(...args) {
        if (this.level <= LOG_LEVELS.INFO) {
            console.info("[INFO]", new Date().toISOString(), ...args);
        }
    }

    warn(...args) {
        if (this.level <= LOG_LEVELS.WARN) {
            console.warn("[WARN]", new Date().toISOString(), ...args);
        }
    }

    error(...args) {
        if (this.level <= LOG_LEVELS.ERROR) {
            console.error("[ERROR]", new Date().toISOString(), ...args);
        }
    }
}

const logger = new Logger(LOG_LEVELS.DEBUG);
```

## 10. 関連ドキュメント

- [設計書](design.md) - 全体設計
- [SDK仕様書](sdk-spec.md) - TypeScript SDK API
- [シグナリングプロトコルADR](adr/0002-signaling-protocol.md) - JSON-RPC 2.0選定理由
- [Simulcast戦略ADR](adr/0003-simulcast-strategy.md) - Simulcast実装方針
