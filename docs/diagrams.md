# WebRTC SFU 図表集

本ドキュメントはWebRTC SFUシステムの各種図表（状態遷移図、シーケンス図、アーキテクチャ図）を集約したものである。

> **注記**: 本ドキュメントのアーキテクチャ図はGCP/Cloudflareを使用した具体的なデプロイ構成を示している。
> 詳細なデプロイ手順については[deployment.md](deployment.md)を参照。

## 目次

1. [アーキテクチャ図](#1-アーキテクチャ図)
2. [状態遷移図](#2-状態遷移図)
3. [シーケンス図](#3-シーケンス図)
4. [データフロー図](#4-データフロー図)

---

## 1. アーキテクチャ図

### 1.1 システム全体構成

```mermaid
graph TB
    subgraph Clients
        C1[Client 1<br/>Publisher]
        C2[Client 2<br/>Subscriber]
        C3[Client 3<br/>Publisher/Subscriber]
    end

    subgraph CloudFlare
        CF[Cloudflare CDN<br/>DDoS Protection]
    end

    subgraph GCP
        subgraph GKE
            LB[Cloud Load Balancer<br/>L4 UDP/TCP]
            SFU1[SFU Pod 1]
            SFU2[SFU Pod 2]
            SFU3[SFU Pod N]
        end

        Redis[(Memorystore<br/>Redis)]
        GCS[(Cloud Storage<br/>Recordings)]
    end

    subgraph External
        AuthServer[Auth Server<br/>JWT発行]
        JWKS[JWKS Endpoint<br/>公開鍵配布]
        TURN[STUN/TURN Server<br/>NAT越え支援]
    end

    C1 --> CF
    C2 --> CF
    C3 --> CF
    CF --> LB
    LB --> SFU1
    LB --> SFU2
    LB --> SFU3
    SFU1 <--> Redis
    SFU2 <--> Redis
    SFU3 <--> Redis
    SFU1 --> GCS
    SFU2 --> GCS
    SFU3 --> GCS
    SFU1 --> JWKS
    AuthServer --> C1
    AuthServer --> C2
    AuthServer --> C3
    C1 <-.->|ICE| TURN
    C2 <-.->|ICE| TURN
    C3 <-.->|ICE| TURN
    TURN <-.->|TURN Relay| LB
```

### 1.2 SFUコンポーネント構成

```mermaid
graph TB
    subgraph SFU Process
        subgraph HTTP Layer
            HTTP[HTTP Server<br/>:8080]
            WS[WebSocket Handler]
            REST[REST API Handler]
            Metrics[Metrics Endpoint<br/>/metrics]
        end

        subgraph Signaling
            Dispatcher[Message Dispatcher]
            Protocol[JSON-RPC Protocol]
        end

        subgraph Room Management
            RoomMgr[Room Manager]
            Room[Room Entity]
            Participant[Participant Entity]
        end

        subgraph Media
            Router[Media Router]
            Publisher[Publisher]
            Subscriber[Subscriber]
            Simulcast[Simulcast Controller]
        end

        subgraph WebRTC
            PeerConn[PeerConnection Manager]
            ICE[ICE Handler]
            RTCP[RTCP Handler]
            RTP[RTP Processor]
        end

        subgraph Auth
            JWT[JWT Validator]
            JWKS_Cache[JWKS Cache]
            Blacklist[Token Blacklist]
        end

        subgraph Recording
            Recorder[Recorder]
            Storage[Storage Writer]
        end
    end

    HTTP --> WS
    HTTP --> REST
    HTTP --> Metrics
    WS --> Dispatcher
    Dispatcher --> Protocol
    Protocol --> RoomMgr
    RoomMgr --> Room
    Room --> Participant
    Participant --> Router
    Router --> Publisher
    Router --> Subscriber
    Router --> Simulcast
    Participant --> PeerConn
    PeerConn --> ICE
    PeerConn --> RTCP
    PeerConn --> RTP
    Dispatcher --> JWT
    JWT --> JWKS_Cache
    JWT --> Blacklist
    Room --> Recorder
    Recorder --> Storage
```

---

## 2. 状態遷移図

### 2.1 ルーム状態遷移

サーバーサイドのルームライフサイクル状態を示す。

```mermaid
stateDiagram-v2
    [*] --> created: POST /api/v1/rooms

    created --> active: 最初の参加者がjoin
    created --> closed: 空ルームタイムアウト<br/>(設定可能)

    active --> active: 参加者の入退室
    active --> created: 最後の参加者がleave
    active --> locked: lock操作<br/>(admin/moderator)
    active --> closing: DELETE /api/v1/rooms/{id}<br/>(admin)

    locked --> active: unlock操作<br/>(admin/moderator)
    locked --> closing: DELETE /api/v1/rooms/{id}<br/>(admin)

    closing --> closed: 全参加者退出完了

    closed --> [*]

    note right of created
        ルーム作成直後
        参加者なし
    end note

    note right of active
        1人以上の参加者が存在
        メディア通信可能
    end note

    note right of locked
        新規参加を禁止
        既存参加者は継続可能
    end note

    note right of closing
        クローズ処理中
        新規参加不可
        参加者に退出通知
    end note
```

**状態定義:**

| 状態 | 説明 | 許可される操作 |
|------|------|---------------|
| created | ルーム作成直後、参加者なし | join, delete |
| active | 1人以上の参加者が存在 | join, leave, publish, subscribe, lock, delete |
| locked | 新規参加を禁止 | leave, publish, subscribe, unlock, delete |
| closing | クローズ処理中 | leave |
| closed | ルーム終了 | なし |

### 2.2 参加者状態遷移

```mermaid
stateDiagram-v2
    [*] --> joining: join request

    joining --> joined: join成功<br/>joined通知受信
    joining --> [*]: join失敗<br/>(認証エラー/ルーム満員)

    joined --> publishing: publish request
    joined --> subscribing: subscribe request
    joined --> leaving: leave request<br/>または接続断

    publishing --> joined: unpublish完了<br/>(トラックなし)
    publishing --> publishing: 追加publish<br/>unpublish
    publishing --> subscribing: subscribe request
    publishing --> leaving: leave request<br/>または接続断

    subscribing --> joined: unsubscribe完了<br/>(購読なし)
    subscribing --> subscribing: 追加subscribe<br/>unsubscribe
    subscribing --> publishing: publish request
    subscribing --> leaving: leave request<br/>または接続断

    leaving --> left: 退出処理完了

    left --> [*]

    note right of joining
        JWT検証中
        ルーム参加処理中
    end note

    note right of publishing
        1つ以上のトラックを
        パブリッシュ中
    end note

    note right of subscribing
        1つ以上のトラックを
        サブスクライブ中
    end note
```

**状態定義:**

| 状態 | 説明 | 許可される操作 |
|------|------|---------------|
| joining | 参加処理中 | - |
| joined | 参加完了、メディア操作なし | publish, subscribe, leave |
| publishing | 1つ以上のトラックをパブリッシュ中 | publish, unpublish, subscribe, unsubscribe, leave |
| subscribing | 1つ以上のトラックをサブスクライブ中 | publish, unpublish, subscribe, unsubscribe, leave |
| leaving | 退出処理中 | - |
| left | 退出完了 | - |

### 2.3 クライアント接続状態遷移

クライアントSDKの接続状態を示す。

```mermaid
stateDiagram-v2
    [*] --> disconnected

    disconnected --> connecting: connect()呼び出し

    connecting --> connected: WebSocket接続成功
    connecting --> disconnected: 接続失敗

    connected --> joined: join成功
    connected --> disconnected: 接続エラー

    joined --> reconnecting: 接続断検出
    joined --> disconnected: leave()呼び出し

    reconnecting --> joined: 再接続成功<br/>(セッション復元)
    reconnecting --> connecting: セッション期限切れ<br/>(新規接続)
    reconnecting --> disconnected: 再接続失敗<br/>(最大リトライ超過)

    note right of connecting
        WebSocket接続中
    end note

    note right of connected
        WebSocket接続完了
        join前
    end note

    note right of joined
        ルーム参加完了
        メディア操作可能
    end note

    note right of reconnecting
        指数バックオフで
        再接続試行中
        (1s → 30s max)
    end note
```

### 2.4 WebRTC接続状態遷移

PeerConnectionの状態遷移を示す。

```mermaid
stateDiagram-v2
    [*] --> new: PeerConnection作成

    new --> connecting: setLocalDescription<br/>(offer/answer)

    connecting --> connected: ICE接続確立
    connecting --> failed: ICE接続失敗<br/>(タイムアウト30秒)

    connected --> disconnected: ICE接続断
    connected --> connected: ICE再起動成功

    disconnected --> connected: ICE再接続成功
    disconnected --> failed: 再接続失敗

    failed --> connecting: ICE再起動

    connected --> closed: close()呼び出し
    disconnected --> closed: close()呼び出し
    failed --> closed: close()呼び出し

    closed --> [*]
```

### 2.5 録画状態遷移

```mermaid
stateDiagram-v2
    [*] --> idle: ルーム作成

    idle --> starting: POST /recording<br/>(admin)

    starting --> recording: 録画開始成功<br/>recordingStarted通知

    recording --> stopping: DELETE /recording<br/>(admin)
    recording --> stopping: ルームclose

    stopping --> idle: 録画停止完了<br/>recordingStopped通知<br/>ファイルアップロード完了

    stopping --> error: アップロード失敗

    error --> idle: リトライ成功
    error --> [*]: リトライ失敗<br/>(ログ出力)

    note right of recording
        RTPパケットを
        WebMファイルに書き込み中
        参加者に録画中を通知
    end note

    note right of stopping
        ファイルのクローズ
        Cloud Storageへアップロード
        メタデータ保存
    end note
```

### 2.6 Simulcastレイヤー選択状態

```mermaid
stateDiagram-v2
    [*] --> high: デフォルト<br/>(h: 720p)

    high --> medium: 帯域幅不足検出<br/>またはsetPreferredLayer(m)
    high --> low: 深刻な帯域幅不足<br/>またはsetPreferredLayer(l)

    medium --> high: 帯域幅回復<br/>かつクライアント要求範囲内
    medium --> low: 帯域幅不足継続<br/>またはsetPreferredLayer(l)

    low --> medium: 帯域幅回復<br/>かつクライアント要求範囲内
    low --> high: 帯域幅十分回復<br/>かつクライアント要求範囲内

    note right of high
        h: 1280x720
        最大2.5Mbps, 30fps
    end note

    note right of medium
        m: 640x360
        最大500Kbps, 30fps
    end note

    note right of low
        l: 320x180
        最大150Kbps, 15fps
    end note
```

**レイヤー切り替え条件:**

| 切り替え | トリガー条件 |
|---------|-------------|
| 高→中 | パケットロス率 > 5% または RTT > 300ms |
| 中→低 | パケットロス率 > 10% または 帯域幅 < 400Kbps |
| 低→中 | パケットロス率 < 1% かつ 帯域幅 > 600Kbps |
| 中→高 | パケットロス率 < 1% かつ 帯域幅 > 2Mbps |

---

## 3. シーケンス図

### 3.1 認証フロー

```mermaid
sequenceDiagram
    participant Client
    participant AuthServer
    participant SFU
    participant JWKS
    participant Redis

    Client->>AuthServer: POST /auth/login<br/>{credentials}
    AuthServer->>AuthServer: 認証処理
    AuthServer-->>Client: {access_token: JWT}

    Client->>SFU: WebSocket Connect (wss://)
    SFU-->>Client: Connection Established

    Client->>SFU: join {token, sessionId?, metadata?}
    SFU->>SFU: JWTヘッダー解析 (kid取得)

    alt JWKSキャッシュなし または 期限切れ
        SFU->>JWKS: GET /.well-known/jwks.json
        JWKS-->>SFU: {keys: [...]}
        SFU->>SFU: キャッシュ更新 (TTL: 1時間)
    end

    SFU->>SFU: JWT署名検証 (RS256)
    SFU->>SFU: クレーム検証<br/>(iss, aud, exp, room_id)

    SFU->>Redis: GET jwt:blacklist:{jti}
    Redis-->>SFU: null (ブラックリストなし)

    SFU-->>Client: join result<br/>{sessionId, participants, iceServers}
```

### 3.2 参加フロー

```mermaid
sequenceDiagram
    participant Client
    participant WebSocket
    participant Signaling
    participant Auth
    participant RoomMgr
    participant Room
    participant Redis

    Client->>WebSocket: Connect (wss://)
    WebSocket->>Signaling: New Connection

    Client->>Signaling: join {token, sessionId?, metadata?}
    Signaling->>Auth: ValidateJWT(token)
    Auth-->>Signaling: {roomId, userId, role, permissions}

    Signaling->>RoomMgr: GetOrCreateRoom(roomId)

    alt ルームが存在しない
        RoomMgr->>Room: Create(roomId, config)
        RoomMgr->>Redis: HSET room:{roomId} ...
    end

    RoomMgr-->>Signaling: Room

    alt ルームがlocked状態
        Signaling-->>Client: error {code: 1010, message: "Room locked"}
    else ルームが満員
        Signaling-->>Client: error {code: 1002, message: "Room full"}
    else 参加可能
        Signaling->>Room: AddParticipant(userId, metadata)
        Room->>Redis: HSET session:{participantId} ...
        Room-->>Signaling: Participant

        par 参加者への通知
            Room-->>Client: join result {sessionId, participants, iceServers}
        and 他参加者への通知
            Room-->>Room: Broadcast participantJoined
        end
    end
```

### 3.3 パブリッシュフロー

```mermaid
sequenceDiagram
    participant Client
    participant Signaling
    participant Participant
    participant MediaRouter
    participant Room

    Client->>Signaling: publish {kind: "video", simulcast: true, metadata?}
    Signaling->>Signaling: 権限チェック (role: publisher以上)
    Signaling->>Participant: Publish(kind, opts)
    Participant->>Participant: トラック数チェック<br/>(映像3, 音声2/参加者)
    Participant->>MediaRouter: AddTrack(track)
    MediaRouter-->>Participant: trackId
    Participant-->>Signaling: LocalTrack
    Signaling-->>Client: publish result {trackId, mid}

    Note over Client,Signaling: SDP Negotiation (Client-initiated)

    Client->>Client: createOffer()
    Client->>Signaling: offer {sdp}
    Signaling->>Participant: SetRemoteDescription(offer)
    Participant->>Participant: CreateAnswer()
    Signaling-->>Client: offer result {sdp: answer}

    loop ICE Candidates
        Client->>Signaling: candidate {candidate, sdpMid, sdpMLineIndex}
        Signaling->>Participant: AddICECandidate(candidate)
        Signaling-->>Client: candidate result {}
    end

    Note over Client,Room: メディア接続確立後

    Room->>Room: Broadcast trackPublished
    Room-->>Client: trackPublished (to other participants)
```

### 3.4 サブスクライブフロー

```mermaid
sequenceDiagram
    participant Client
    participant Signaling
    participant Participant
    participant MediaRouter
    participant Publisher

    Client->>Signaling: subscribe {publisherId, trackId, preferredLayer?}
    Signaling->>Signaling: 権限チェック (role: subscriber以上)
    Signaling->>Participant: Subscribe(publisherId, trackId, layer)
    Participant->>MediaRouter: Subscribe(trackId)
    MediaRouter->>Publisher: GetTrack(trackId)
    Publisher-->>MediaRouter: RemoteTrack
    MediaRouter->>Participant: AddRemoteTrack(track)

    Note over Participant,Client: Server-initiated SDP Offer

    Participant->>Participant: CreateOffer()
    Signaling-->>Client: offer notification {sdp, reason: "track_added"}
    Client->>Client: setRemoteDescription(offer)
    Client->>Client: createAnswer()
    Client->>Signaling: answer {sdp}
    Signaling->>Participant: SetRemoteDescription(answer)

    loop ICE Candidates (Server → Client)
        Signaling-->>Client: candidate notification {candidate}
        Client->>Client: addIceCandidate(candidate)
    end

    Signaling-->>Client: subscribe result {subscriptionId}

    Note over Client: メディア受信開始
```

### 3.5 再接続フロー

```mermaid
sequenceDiagram
    participant Client
    participant SFU
    participant SessionStore

    Note over Client: 接続断検出

    Client->>Client: 指数バックオフ開始<br/>(初期: 1秒)

    loop リトライ (最大30秒間隔)
        Client->>SFU: WebSocket Connect
        alt 接続成功
            Client->>SFU: join {token, sessionId}
            SFU->>SessionStore: GetSession(sessionId)
            SessionStore-->>SFU: Session

            alt セッション有効 (30秒以内)
                SFU->>SFU: RestoreSession
                SFU->>SFU: Restore published tracks
                SFU->>SFU: Restore subscriptions
                SFU-->>Client: join result {restored: true, ...}

                Note over Client,SFU: SDP再ネゴシエーション
                SFU-->>Client: offer notification {sdp, reason: "reconnect"}
                Client->>SFU: answer {sdp}
            else セッション期限切れ
                SFU-->>Client: error {code: 1009, message: "Session expired"}
                Note over Client: 新規joinとして再開
                Client->>SFU: join {token}
                SFU-->>Client: join result {restored: false, ...}
            end
        else 接続失敗
            Client->>Client: バックオフ時間 × 2<br/>(最大30秒)
        end
    end
```

### 3.6 録画フロー

```mermaid
sequenceDiagram
    participant Admin
    participant SFU
    participant Room
    participant Recorder
    participant GCS

    Admin->>SFU: POST /api/v1/rooms/{id}/recording
    SFU->>SFU: 権限チェック (role: admin)
    SFU->>Room: StartRecording()
    Room->>Recorder: Start(roomId, config)
    Recorder->>Recorder: WebMファイル作成
    Room-->>SFU: recordingId
    SFU-->>Admin: 201 {recordingId, startedAt}

    Room->>Room: Broadcast recordingStarted
    Room-->>Room: recordingStarted (to all participants)

    Note over Recorder: RTPパケット書き込み中

    loop メディアストリーム受信
        Room->>Recorder: WriteRTP(packet)
        Recorder->>Recorder: WebMに書き込み
    end

    Admin->>SFU: DELETE /api/v1/rooms/{id}/recording
    SFU->>Room: StopRecording()
    Room->>Recorder: Stop()
    Recorder->>Recorder: ファイルクローズ
    Recorder->>GCS: Upload media.webm
    GCS-->>Recorder: OK
    Recorder->>GCS: Upload metadata.json
    GCS-->>Recorder: OK
    Room-->>SFU: RecordingInfo
    SFU-->>Admin: 200 {recordingId, duration, fileSize}

    Room->>Room: Broadcast recordingStopped
    Room-->>Room: recordingStopped (to all participants)
```

---

## 4. データフロー図

### 4.1 メディアデータフロー

```mermaid
graph LR
    subgraph Publisher側
        P_Camera[カメラ]
        P_Mic[マイク]
        P_Encoder[エンコーダー<br/>VP8/H.264/Opus]
        P_RTP[RTP Packetizer]
        P_PC[PeerConnection]
    end

    subgraph SFU
        SFU_PC[PeerConnection<br/>ICE Lite]
        SFU_RTP[RTP Processor<br/>SSRC書き換え]
        SFU_Router[Media Router]
        SFU_Simulcast[Simulcast Controller<br/>レイヤー選択]
        SFU_RTCP[RTCP Handler<br/>TWCC/REMB]
    end

    subgraph Subscriber側
        S_PC[PeerConnection]
        S_RTP[RTP Depacketizer]
        S_Decoder[デコーダー]
        S_Video[映像表示]
        S_Audio[音声出力]
    end

    P_Camera --> P_Encoder
    P_Mic --> P_Encoder
    P_Encoder --> P_RTP
    P_RTP --> P_PC
    P_PC -->|RTP/SRTP| SFU_PC

    SFU_PC --> SFU_RTP
    SFU_RTP --> SFU_Router
    SFU_Router --> SFU_Simulcast
    SFU_Simulcast --> S_PC

    S_PC --> S_RTP
    S_RTP --> S_Decoder
    S_Decoder --> S_Video
    S_Decoder --> S_Audio

    SFU_PC <-->|RTCP| SFU_RTCP
    SFU_RTCP -->|フィードバック| P_PC
    S_PC -->|RTCP| SFU_RTCP
```

### 4.2 シグナリングデータフロー

```mermaid
graph TB
    subgraph Client
        SDK[SFU Client SDK]
        WS_Client[WebSocket Client]
    end

    subgraph SFU
        WS_Server[WebSocket Server]
        Dispatcher[JSON-RPC Dispatcher]
        Handler[Method Handler]
        Notifier[Notification Sender]
    end

    subgraph Handlers
        JoinH[join Handler]
        PublishH[publish Handler]
        SubscribeH[subscribe Handler]
        OfferH[offer Handler]
        CandidateH[candidate Handler]
    end

    SDK --> WS_Client
    WS_Client -->|Request| WS_Server
    WS_Server --> Dispatcher
    Dispatcher --> Handler

    Handler --> JoinH
    Handler --> PublishH
    Handler --> SubscribeH
    Handler --> OfferH
    Handler --> CandidateH

    JoinH -->|Response| Dispatcher
    PublishH -->|Response| Dispatcher
    SubscribeH -->|Response| Dispatcher
    OfferH -->|Response| Dispatcher
    CandidateH -->|Response| Dispatcher

    Dispatcher -->|Response| WS_Server
    WS_Server -->|Response| WS_Client
    WS_Client --> SDK

    Notifier -->|Notification| WS_Server
    WS_Server -->|Notification| WS_Client
```

---

## 関連ドキュメント

- [requirements.md](requirements.md) - 要件定義書
- [design.md](design.md) - 設計書
- [api/openapi.yaml](api/openapi.yaml) - REST API仕様
- [api/jsonrpc-schema.json](api/jsonrpc-schema.json) - シグナリングプロトコル仕様
- [sdk-spec.md](sdk-spec.md) - クライアントSDK仕様
- [adr/](adr/) - アーキテクチャ決定記録
