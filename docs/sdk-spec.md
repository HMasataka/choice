# クライアントSDK仕様書

WebRTC SFU クライアントSDKの仕様書。TypeScript SDKおよびReact Hooksの設計・API・使用方法を定義する。

## 1. 概要

### 1.1 パッケージ構成

| パッケージ名      | 説明                  | 必須/オプション |
| ----------------- | --------------------- | --------------- |
| `@sfu/client-sdk` | コアSDK（TypeScript） | 必須            |
| `@sfu/react-sdk`  | React Hooks           | オプション      |

### 1.2 対応環境

| 環境       | バージョン                                    |
| ---------- | --------------------------------------------- |
| Node.js    | 18+                                           |
| TypeScript | 5.0+                                          |
| ブラウザ   | Chrome 90+, Firefox 85+, Safari 14+, Edge 90+ |
| React      | 18+ （React SDK使用時）                       |

### 1.3 依存関係

```json
{
  "peerDependencies": {
    "typescript": ">=5.0.0"
  },
  "devDependencies": {
    "@types/node": "^20.0.0"
  }
}
```

## 2. アーキテクチャ

### 2.1 レイヤー構成

```text
┌─────────────────────────────────────────────┐
│              Application Layer              │
│         (User Code / React Components)      │
├─────────────────────────────────────────────┤
│              React SDK (optional)           │
│    useSFUClient, useRoom, useLocalMedia     │
├─────────────────────────────────────────────┤
│               Core SDK Layer                │
│   SFUClient, Room, Participant, Track       │
├─────────────────────────────────────────────┤
│             Signaling Layer                 │
│      SignalingClient, JsonRpcClient         │
├─────────────────────────────────────────────┤
│              WebRTC Layer                   │
│    PeerConnection, SDPUtils, ICEManager     │
├─────────────────────────────────────────────┤
│           Browser WebRTC APIs               │
└─────────────────────────────────────────────┘
```

### 2.2 ディレクトリ構成

```text
@sfu/client-sdk/
├── src/
│   ├── index.ts                    # エントリーポイント
│   ├── client/
│   │   ├── SFUClient.ts            # メインクライアント
│   │   ├── Room.ts                 # ルーム管理
│   │   ├── Participant.ts          # 参加者
│   │   └── Connection.ts           # 接続管理
│   ├── signaling/
│   │   ├── SignalingClient.ts      # WebSocket通信
│   │   ├── JsonRpcClient.ts        # JSON-RPC処理
│   │   └── types.ts                # プロトコル型定義
│   ├── media/
│   │   ├── LocalTrack.ts           # ローカルトラック
│   │   ├── RemoteTrack.ts          # リモートトラック
│   │   ├── MediaDevices.ts         # デバイス管理
│   │   └── Simulcast.ts            # Simulcast制御
│   ├── webrtc/
│   │   ├── PeerConnection.ts       # PC管理
│   │   ├── SDPUtils.ts             # SDP操作
│   │   └── ICEManager.ts           # ICE管理
│   ├── events/
│   │   ├── EventEmitter.ts         # イベント基盤
│   │   └── RoomEvents.ts           # ルームイベント型
│   ├── errors/
│   │   └── SFUError.ts             # エラー定義
│   └── utils/
│       ├── retry.ts                # リトライロジック
│       ├── logger.ts               # ログ出力
│       └── types.ts                # 共通型
├── package.json
├── tsconfig.json
└── README.md
```

## 3. Core SDK API

### 3.1 SFUClient

メインのクライアントクラス。WebSocket接続とルーム参加を管理する。

#### コンストラクタ

```typescript
constructor(config: SFUClientConfig)
```

#### SFUClientConfig

| プロパティ      | 型                | 必須 | デフォルト   | 説明                                 |
| --------------- | ----------------- | ---- | ------------ | ------------------------------------ |
| `url`           | `string`          | Yes  | -            | シグナリングサーバーURL（WebSocket） |
| `autoReconnect` | `boolean`         | No   | `true`       | 切断時の自動再接続                   |
| `reconnect`     | `ReconnectConfig` | No   | 下記参照     | 再接続設定                           |
| `logger`        | `LoggerConfig`    | No   | -            | ログ設定                             |
| `iceServers`    | `RTCIceServer[]`  | No   | サーバー提供 | ICEサーバー設定                      |

#### ReconnectConfig

| プロパティ     | 型       | デフォルト | 説明                       |
| -------------- | -------- | ---------- | -------------------------- |
| `maxAttempts`  | `number` | `5`        | 最大リトライ回数           |
| `initialDelay` | `number` | `1000`     | 初回リトライ待機時間（ms） |
| `maxDelay`     | `number` | `30000`    | 最大リトライ待機時間（ms） |
| `factor`       | `number` | `2`        | 指数バックオフ係数         |

#### メソッド

| メソッド                                     | 戻り値          | 説明                 |
| -------------------------------------------- | --------------- | -------------------- |
| `connect(url?: string)`                      | `Promise<void>` | WebSocket接続を確立  |
| `join(token: string, options?: JoinOptions)` | `Promise<Room>` | ルームに参加         |
| `disconnect()`                               | `void`          | 接続を切断           |
| `on(event: ClientEvent, handler: Function)`  | `void`          | イベントリスナー登録 |
| `off(event: ClientEvent, handler: Function)` | `void`          | イベントリスナー解除 |

#### JoinOptions

| プロパティ      | 型                        | 必須 | デフォルト | 説明                   |
| --------------- | ------------------------- | ---- | ---------- | ---------------------- |
| `sessionId`     | `string`                  | No   | -          | 再接続用セッションID   |
| `metadata`      | `Record<string, unknown>` | No   | -          | クライアントメタデータ |
| `autoSubscribe` | `boolean`                 | No   | `false`    | 全トラック自動購読     |

#### クライアントイベント

| イベント       | ペイロード         | 説明       |
| -------------- | ------------------ | ---------- |
| `connecting`   | -                  | 接続開始   |
| `connected`    | -                  | 接続完了   |
| `disconnected` | `DisconnectReason` | 切断       |
| `reconnecting` | -                  | 再接続中   |
| `reconnected`  | -                  | 再接続完了 |
| `error`        | `SFUError`         | エラー発生 |

### 3.2 Room

ルームを表すクラス。参加者とトラックを管理する。

#### プロパティ

| プロパティ         | 型                        | 説明                 |
| ------------------ | ------------------------- | -------------------- |
| `id`               | `string`                  | ルームID             |
| `state`            | `RoomState`               | クライアント接続状態 |
| `serverState`      | `ServerRoomState`         | サーバー側ルーム状態 |
| `localParticipant` | `LocalParticipant`        | 自分自身             |
| `participants`     | `RemoteParticipant[]`     | 他の参加者           |
| `metadata`         | `Record<string, unknown>` | ルームメタデータ     |

#### RoomState

```typescript
type RoomState =
  | "disconnected" // 未接続
  | "connecting" // 接続中
  | "joined" // 参加完了
  | "reconnecting"; // 再接続中
```

#### メソッド

| メソッド                                   | 戻り値                           | 説明                 |
| ------------------------------------------ | -------------------------------- | -------------------- |
| `leave()`                                  | `Promise<void>`                  | ルームから退出       |
| `getParticipant(id: string)`               | `RemoteParticipant \| undefined` | 参加者を取得         |
| `on(event: RoomEvent, handler: Function)`  | `void`                           | イベントリスナー登録 |
| `off(event: RoomEvent, handler: Function)` | `void`                           | イベントリスナー解除 |

#### ルームイベント

| イベント                   | ペイロード                                  | 説明                                    |
| -------------------------- | ------------------------------------------- | --------------------------------------- |
| `stateChanged`             | `RoomState`                                 | クライアント接続状態変化                |
| `serverStateChanged`       | `ServerRoomState`                           | サーバー側ルーム状態変化                |
| `participantJoined`        | `RemoteParticipant`                         | 参加者が参加                            |
| `participantLeft`          | `RemoteParticipant, ParticipantLeaveReason` | 参加者が退出                            |
| `trackPublished`           | `RemoteTrack, RemoteParticipant`            | トラック公開                            |
| `trackUnpublished`         | `RemoteTrack, RemoteParticipant`            | トラック削除                            |
| `trackSubscribed`          | `RemoteTrack, RemoteParticipant`            | トラック購読完了                        |
| `trackSubscriptionFailed`  | `string, SFUError`                          | トラック購読失敗                        |
| `layerChanged`             | `LayerChangedEvent`                         | Simulcastレイヤー変更                   |
| `connectionQualityChanged` | `ConnectionQuality, Participant`            | 接続品質変化                            |
| `reconnecting`             | -                                           | 再接続中                                |
| `reconnected`              | -                                           | 再接続完了                              |
| `disconnected`             | `DisconnectReason`                          | 切断                                    |
| `joined`                   | `string, string`                            | 自身の参加完了（participantId, roomId） |
| `left`                     | `LeaveReason`                               | 自身の退出完了                          |
| `error`                    | `ServerError`                               | サーバーエラー通知                      |
| `reconnectRequested`       | `ReconnectReason, number`                   | 再接続要求                              |
| `recordingStarted`         | `string, string`                            | 録画開始（recordingId, startedBy）      |
| `recordingStopped`         | `string, string`                            | 録画停止（recordingId, stoppedBy）      |

### 3.3 LocalParticipant

自分自身を表すクラス。メディア配信を管理する。

#### プロパティ

| プロパティ | 型                        | 説明             |
| ---------- | ------------------------- | ---------------- |
| `id`       | `string`                  | 参加者ID         |
| `tracks`   | `LocalTrack[]`            | 配信中のトラック |
| `metadata` | `Record<string, unknown>` | メタデータ       |

#### メソッド

| メソッド                                                     | 戻り値                | 説明                     |
| ------------------------------------------------------------ | --------------------- | ------------------------ |
| `publish(track: MediaStreamTrack, options?: PublishOptions)` | `Promise<LocalTrack>` | トラックを配信           |
| `unpublish(track: LocalTrack)`                               | `Promise<void>`       | 配信を停止               |
| `setMicrophoneEnabled(enabled: boolean)`                     | `Promise<void>`       | マイクのミュート切り替え |
| `setCameraEnabled(enabled: boolean)`                         | `Promise<void>`       | カメラのミュート切り替え |

#### PublishOptions

| プロパティ      | 型                        | 必須 | デフォルト                      | 説明               |
| --------------- | ------------------------- | ---- | ------------------------------- | ------------------ |
| `name`          | `string`                  | No   | -                               | トラック名/ラベル  |
| `simulcast`     | `boolean`                 | No   | `true`（映像）, `false`（音声） | Simulcast有効化    |
| `metadata`      | `Record<string, unknown>` | No   | -                               | トラックメタデータ |
| `videoEncoding` | `VideoEncodingOptions`    | No   | -                               | 映像エンコード設定 |
| `audioEncoding` | `AudioEncodingOptions`    | No   | -                               | 音声エンコード設定 |

#### VideoEncodingOptions

| プロパティ     | 型                            | デフォルト | 説明                    |
| -------------- | ----------------------------- | ---------- | ----------------------- |
| `maxBitrate`   | `number`                      | -          | 最大ビットレート（bps） |
| `maxFramerate` | `number`                      | -          | 最大フレームレート      |
| `priority`     | `"low" \| "medium" \| "high"` | `"medium"` | エンコード優先度        |

#### AudioEncodingOptions

| プロパティ   | 型        | デフォルト | 説明                    |
| ------------ | --------- | ---------- | ----------------------- |
| `maxBitrate` | `number`  | -          | 最大ビットレート（bps） |
| `stereo`     | `boolean` | `false`    | ステレオ有効化          |
| `dtx`        | `boolean` | `true`     | DTX（不連続送信）有効化 |

### 3.4 RemoteParticipant

他の参加者を表すクラス。メディア購読を管理する。

#### プロパティ

| プロパティ          | 型                        | 説明             |
| ------------------- | ------------------------- | ---------------- |
| `id`                | `string`                  | 参加者ID         |
| `tracks`            | `RemoteTrack[]`           | 公開中のトラック |
| `metadata`          | `Record<string, unknown>` | メタデータ       |
| `connectionQuality` | `ConnectionQuality`       | 接続品質         |

#### メソッド

| メソッド                                                 | 戻り値                     | 説明           |
| -------------------------------------------------------- | -------------------------- | -------------- |
| `subscribe(trackId: string, options?: SubscribeOptions)` | `Promise<RemoteTrack>`     | トラックを購読 |
| `unsubscribe(track: RemoteTrack)`                        | `Promise<void>`            | 購読を解除     |
| `getTrack(trackId: string)`                              | `RemoteTrack \| undefined` | トラックを取得 |

#### SubscribeOptions

| プロパティ       | 型                 | 必須 | デフォルト | 説明                      |
| ---------------- | ------------------ | ---- | ---------- | ------------------------- |
| `preferredLayer` | `SimulcastLayer`   | No   | `"h"`      | 希望するSimulcastレイヤー |
| `autoAttach`     | `HTMLMediaElement` | No   | -          | 自動アタッチ先要素        |

### 3.5 LocalTrack

ローカルトラックを表すクラス。

#### プロパティ

| プロパティ         | 型                        | 説明                           |
| ------------------ | ------------------------- | ------------------------------ |
| `id`               | `string`                  | トラックID（サーバー割り当て） |
| `kind`             | `TrackKind`               | トラック種別                   |
| `name`             | `string \| undefined`     | トラック名                     |
| `mediaStreamTrack` | `MediaStreamTrack`        | ブラウザのMediaStreamTrack     |
| `simulcast`        | `boolean`                 | Simulcast有効かどうか          |
| `muted`            | `boolean`                 | ミュート状態                   |
| `metadata`         | `Record<string, unknown>` | メタデータ                     |

#### メソッド

| メソッド   | 戻り値 | 説明         |
| ---------- | ------ | ------------ |
| `mute()`   | `void` | ミュート     |
| `unmute()` | `void` | ミュート解除 |
| `stop()`   | `void` | トラック停止 |

### 3.6 RemoteTrack

リモートトラックを表すクラス。

#### プロパティ

| プロパティ         | 型                         | 説明                       |
| ------------------ | -------------------------- | -------------------------- |
| `id`               | `string`                   | トラックID                 |
| `kind`             | `TrackKind`                | トラック種別               |
| `publisherId`      | `string`                   | 配信者ID                   |
| `subscriptionId`   | `string \| undefined`      | 購読ID（購読後に設定）     |
| `mediaStreamTrack` | `MediaStreamTrack \| null` | ブラウザのMediaStreamTrack |
| `simulcast`        | `boolean`                  | Simulcast有効かどうか      |
| `currentLayer`     | `SimulcastLayer \| null`   | 現在のSimulcastレイヤー    |
| `metadata`         | `Record<string, unknown>`  | メタデータ                 |

#### メソッド

| メソッド                                   | 戻り値          | 説明                     |
| ------------------------------------------ | --------------- | ------------------------ |
| `attach(element: HTMLMediaElement)`        | `void`          | メディア要素にアタッチ   |
| `detach()`                                 | `void`          | メディア要素からデタッチ |
| `setPreferredLayer(layer: SimulcastLayer)` | `Promise<void>` | 希望レイヤーを設定       |

## 4. 型定義

### 4.1 基本型

```typescript
/** トラック種別 */
type TrackKind = "audio" | "video";

/** Simulcastレイヤー */
type SimulcastLayer = "h" | "m" | "l";

/** 接続状態 */
type ConnectionState =
  | "disconnected"
  | "connecting"
  | "connected"
  | "reconnecting";

/** サーバー側ルーム状態 */
type ServerRoomState =
  | "created" // ルーム作成済み、参加者なし
  | "active" // アクティブ、参加者あり
  | "locked" // ロック中、新規参加不可
  | "closing" // クローズ処理中
  | "closed"; // クローズ完了

/** 接続品質 */
type ConnectionQuality = "excellent" | "good" | "fair" | "poor";

/** 自身の退出理由（left通知用） */
type LeaveReason = "voluntary" | "timeout" | "kicked";

/** 他参加者の退出理由（participantLeft通知用） */
type ParticipantLeaveReason = "leave" | "timeout" | "kicked";

/** 切断理由 */
type DisconnectReason =
  | "client_initiated"
  | "server_shutdown"
  | "room_closed"
  | "kicked"
  | "connection_error";

/** 再接続理由 */
type ReconnectReason = "ice_disconnected" | "server_restart";

/** レイヤー変更理由 */
type LayerChangeReason = "bandwidth" | "unavailable";

/** クライアントイベント名 */
type ClientEvent =
  | "connecting"
  | "connected"
  | "disconnected"
  | "reconnecting"
  | "reconnected"
  | "error";

/** ルームイベント名 */
type RoomEvent =
  | "stateChanged"
  | "serverStateChanged"
  | "participantJoined"
  | "participantLeft"
  | "trackPublished"
  | "trackUnpublished"
  | "trackSubscribed"
  | "trackSubscriptionFailed"
  | "layerChanged"
  | "connectionQualityChanged"
  | "reconnecting"
  | "reconnected"
  | "disconnected"
  | "joined"
  | "left"
  | "error"
  | "reconnectRequested"
  | "recordingStarted"
  | "recordingStopped";

/** 参加者の共通インターフェース */
interface Participant {
  id: string;
  metadata: Record<string, unknown>;
}
```

### 4.2 イベント型

```typescript
/** Simulcastレイヤー変更イベント */
interface LayerChangedEvent {
  trackId: string;
  requestedLayer: SimulcastLayer;
  actualLayer: SimulcastLayer;
  reason: LayerChangeReason;
}

/** サーバーエラー通知 */
interface ServerError {
  code: number;
  message: string;
  fatal: boolean;
}
```

### 4.3 エラー

```typescript
/** SDKエラークラス */
class SFUError extends Error {
  constructor(
    public code: number,
    message: string,
    public data?: unknown
  );
}

/** エラーコード */
const ErrorCodes = {
  // JSON-RPC標準エラー
  PARSE_ERROR: -32700,
  INVALID_REQUEST: -32600,
  METHOD_NOT_FOUND: -32601,
  INVALID_PARAMS: -32602,
  INTERNAL_ERROR: -32603,

  // アプリケーションエラー
  ROOM_NOT_FOUND: 1001,
  ROOM_FULL: 1002,
  UNAUTHORIZED: 1003,
  ALREADY_JOINED: 1004,
  NOT_IN_ROOM: 1005,
  TRACK_NOT_FOUND: 1006,
  INVALID_SDP: 1007,
  ICE_FAILURE: 1008,
  SESSION_EXPIRED: 1009,
} as const;
```

## 5. React SDK API

### 5.1 useSFUClient

SFUクライアントの接続管理Hook。

```typescript
function useSFUClient(options: UseSFUClientOptions): UseSFUClientReturn;
```

#### UseSFUClientOptions

| プロパティ    | 型        | 必須 | デフォルト | 説明                    |
| ------------- | --------- | ---- | ---------- | ----------------------- |
| `url`         | `string`  | Yes  | -          | シグナリングサーバーURL |
| `autoConnect` | `boolean` | No   | `false`    | マウント時に自動接続    |

#### UseSFUClientReturn

| プロパティ        | 型                    | 説明                     |
| ----------------- | --------------------- | ------------------------ |
| `client`          | `SFUClient \| null`   | クライアントインスタンス |
| `connectionState` | `ConnectionState`     | 接続状態                 |
| `connect`         | `() => Promise<void>` | 接続関数                 |
| `disconnect`      | `() => void`          | 切断関数                 |

### 5.2 useRoom

ルーム参加・管理Hook。

```typescript
function useRoom(options: UseRoomOptions): UseRoomReturn;
```

#### UseRoomOptions

| プロパティ | 型          | 必須 | デフォルト | 説明                 |
| ---------- | ----------- | ---- | ---------- | -------------------- |
| `client`   | `SFUClient` | Yes  | -          | SFUクライアント      |
| `token`    | `string`    | Yes  | -          | 参加トークン         |
| `autoJoin` | `boolean`   | No   | `false`    | マウント時に自動参加 |

#### UseRoomReturn

| プロパティ         | 型                         | 説明               |
| ------------------ | -------------------------- | ------------------ |
| `room`             | `Room \| null`             | ルームインスタンス |
| `state`            | `RoomState`                | ルーム状態         |
| `participants`     | `RemoteParticipant[]`      | 参加者一覧         |
| `localParticipant` | `LocalParticipant \| null` | 自分自身           |
| `join`             | `() => Promise<void>`      | 参加関数           |
| `leave`            | `() => Promise<void>`      | 退出関数           |

### 5.3 useLocalMedia

ローカルメディア取得Hook。

```typescript
function useLocalMedia(options?: UseLocalMediaOptions): UseLocalMediaReturn;
```

#### UseLocalMediaOptions

| プロパティ | 型                                 | デフォルト | 説明         |
| ---------- | ---------------------------------- | ---------- | ------------ |
| `video`    | `boolean \| MediaTrackConstraints` | `true`     | 映像取得設定 |
| `audio`    | `boolean \| MediaTrackConstraints` | `true`     | 音声取得設定 |

#### UseLocalMediaReturn

| プロパティ   | 型                         | 説明               |
| ------------ | -------------------------- | ------------------ |
| `stream`     | `MediaStream \| null`      | メディアストリーム |
| `videoTrack` | `MediaStreamTrack \| null` | 映像トラック       |
| `audioTrack` | `MediaStreamTrack \| null` | 音声トラック       |
| `isLoading`  | `boolean`                  | 取得中フラグ       |
| `error`      | `Error \| null`            | エラー             |
| `getMedia`   | `() => Promise<void>`      | メディア取得関数   |
| `stopMedia`  | `() => void`               | メディア停止関数   |

### 5.4 useRemoteTrack

リモートトラック管理Hook。

```typescript
function useRemoteTrack(options: UseRemoteTrackOptions): UseRemoteTrackReturn;
```

#### UseRemoteTrackOptions

| プロパティ       | 型               | 必須 | 説明             |
| ---------------- | ---------------- | ---- | ---------------- |
| `track`          | `RemoteTrack`    | Yes  | リモートトラック |
| `preferredLayer` | `SimulcastLayer` | No   | 希望レイヤー     |

#### UseRemoteTrackReturn

| プロパティ          | 型                                                | 説明              |
| ------------------- | ------------------------------------------------- | ----------------- |
| `mediaRef`          | `RefObject<HTMLVideoElement \| HTMLAudioElement>` | メディア要素のref |
| `isSubscribed`      | `boolean`                                         | 購読済みフラグ    |
| `currentLayer`      | `SimulcastLayer \| null`                          | 現在のレイヤー    |
| `setPreferredLayer` | `(layer: SimulcastLayer) => Promise<void>`        | レイヤー設定関数  |

### 5.5 useParticipants（追加API）

参加者一覧管理Hook。

> **Note:** このHookはdesign.mdのReact Hooksセクション（5.5）に定義されていない追加APIです。

```typescript
function useParticipants(room: Room | null): UseParticipantsReturn;
```

#### UseParticipantsReturn

| プロパティ         | 型                                               | 説明           |
| ------------------ | ------------------------------------------------ | -------------- |
| `participants`     | `RemoteParticipant[]`                            | 参加者一覧     |
| `participantCount` | `number`                                         | 参加者数       |
| `getParticipant`   | `(id: string) => RemoteParticipant \| undefined` | 参加者取得関数 |

### 5.6 useScreenShare（追加API）

画面共有Hook。

> **Note:** このHookはdesign.mdのReact Hooksセクション（5.5）に定義されていない追加APIです。

```typescript
function useScreenShare(
  localParticipant: LocalParticipant | null,
): UseScreenShareReturn;
```

#### UseScreenShareReturn

| プロパティ         | 型                                                | 説明         |
| ------------------ | ------------------------------------------------- | ------------ |
| `isSharing`        | `boolean`                                         | 共有中フラグ |
| `track`            | `LocalTrack \| null`                              | 共有トラック |
| `startScreenShare` | `(options?: ScreenShareOptions) => Promise<void>` | 共有開始     |
| `stopScreenShare`  | `() => Promise<void>`                             | 共有停止     |

## 6. 使用例

### 6.1 基本的な使用例（TypeScript）

```typescript
import { SFUClient, SFUError, ErrorCodes } from "@sfu/client-sdk";

// クライアント作成
const client = new SFUClient({
  url: "wss://sfu.example.com/ws",
  autoReconnect: true,
  reconnect: {
    maxAttempts: 5,
    initialDelay: 1000,
    maxDelay: 30000,
    factor: 2,
  },
});

// イベントハンドラ
client.on("reconnecting", () => console.log("Reconnecting..."));
client.on("reconnected", () => console.log("Reconnected"));
client.on("error", (error: SFUError) => {
  console.error(`Error [${error.code}]: ${error.message}`);
});

// ルーム参加
async function joinRoom(token: string) {
  try {
    const room = await client.join(token, {
      autoSubscribe: true,
      metadata: { displayName: "John Doe" },
    });

    // 参加者イベント
    room.on("participantJoined", (participant) => {
      console.log(`${participant.id} joined`);
    });

    room.on("participantLeft", (participant, reason) => {
      console.log(`${participant.id} left: ${reason}`);
    });

    // トラックイベント
    room.on("trackPublished", async (track, participant) => {
      const remoteTrack = await participant.subscribe(track.id, {
        preferredLayer: "h",
      });
      const videoEl = document.getElementById("video") as HTMLVideoElement;
      remoteTrack.attach(videoEl);
    });

    // ローカルメディア配信
    const stream = await navigator.mediaDevices.getUserMedia({
      video: { width: 1280, height: 720 },
      audio: true,
    });

    const videoTrack = stream.getVideoTracks()[0];
    await room.localParticipant.publish(videoTrack, {
      name: "camera",
      simulcast: true,
    });

    const audioTrack = stream.getAudioTracks()[0];
    await room.localParticipant.publish(audioTrack, {
      name: "microphone",
    });

    return room;
  } catch (error) {
    if (error instanceof SFUError) {
      switch (error.code) {
        case ErrorCodes.ROOM_NOT_FOUND:
          console.error("Room does not exist");
          break;
        case ErrorCodes.ROOM_FULL:
          console.error("Room is full");
          break;
        case ErrorCodes.UNAUTHORIZED:
          console.error("Invalid token");
          break;
        default:
          console.error(`SFU Error: ${error.message}`);
      }
    }
    throw error;
  }
}

// 退出
async function leaveRoom(room: Room) {
  await room.leave();
  client.disconnect();
}
```

### 6.2 画面共有

```typescript
async function startScreenShare(room: Room) {
  const stream = await navigator.mediaDevices.getDisplayMedia({
    video: { width: 1920, height: 1080 },
    audio: true,
  });

  const videoTrack = stream.getVideoTracks()[0];
  const localTrack = await room.localParticipant.publish(videoTrack, {
    name: "screen",
    simulcast: true,
    metadata: { source: "screen" },
  });

  // 画面共有停止検知
  videoTrack.onended = async () => {
    await room.localParticipant.unpublish(localTrack);
  };

  return localTrack;
}
```

### 6.3 Simulcastレイヤー制御

```typescript
// 接続品質に応じたレイヤー調整
room.on("connectionQualityChanged", (quality, participant) => {
  if (participant.id === room.localParticipant.id) return;

  participant.tracks.forEach(async (track) => {
    if (track.kind !== "video") return;

    switch (quality) {
      case "excellent":
      case "good":
        await track.setPreferredLayer("h");
        break;
      case "fair":
        await track.setPreferredLayer("m");
        break;
      case "poor":
        await track.setPreferredLayer("l");
        break;
    }
  });
});

// レイヤー変更通知
room.on("layerChanged", (event) => {
  if (event.requestedLayer !== event.actualLayer) {
    console.log(
      `Layer fallback: requested ${event.requestedLayer}, got ${event.actualLayer} (${event.reason})`,
    );
  }
});
```

### 6.4 React使用例

```tsx
import React, { useEffect, useRef } from "react";
import {
  useSFUClient,
  useRoom,
  useLocalMedia,
  useRemoteTrack,
} from "@sfu/react-sdk";

function VideoRoom({ token }: { token: string }) {
  const { client, connectionState, connect } = useSFUClient({
    url: "wss://sfu.example.com/ws",
    autoConnect: true,
  });

  const { room, state, participants, localParticipant, join, leave } = useRoom({
    client: client!,
    token,
    autoJoin: true,
  });

  const { stream, videoTrack, audioTrack, getMedia, isLoading } = useLocalMedia(
    {
      video: { width: 1280, height: 720 },
      audio: true,
    },
  );

  const localVideoRef = useRef<HTMLVideoElement>(null);

  // ローカル映像表示
  useEffect(() => {
    if (localVideoRef.current && stream) {
      localVideoRef.current.srcObject = stream;
    }
  }, [stream]);

  // メディア取得とトラック配信
  useEffect(() => {
    getMedia();
  }, []);

  useEffect(() => {
    if (localParticipant && videoTrack && audioTrack) {
      localParticipant.publish(videoTrack, { simulcast: true });
      localParticipant.publish(audioTrack);
    }
  }, [localParticipant, videoTrack, audioTrack]);

  return (
    <div>
      <div>Connection: {connectionState}</div>
      <div>Room: {state}</div>

      <h3>Local Video</h3>
      <video ref={localVideoRef} autoPlay muted playsInline />

      <h3>Participants ({participants.length})</h3>
      {participants.map((p) => (
        <RemoteParticipantView key={p.id} participant={p} />
      ))}

      <button onClick={leave}>Leave</button>
    </div>
  );
}

function RemoteParticipantView({
  participant,
}: {
  participant: RemoteParticipant;
}) {
  return (
    <div>
      <div>{participant.id}</div>
      {participant.tracks.map((track) => (
        <RemoteTrackView key={track.id} track={track} />
      ))}
    </div>
  );
}

function RemoteTrackView({ track }: { track: RemoteTrack }) {
  const { mediaRef, currentLayer, setPreferredLayer } = useRemoteTrack({
    track,
    preferredLayer: "h",
  });

  if (track.kind === "audio") {
    return (
      <audio ref={mediaRef as React.RefObject<HTMLAudioElement>} autoPlay />
    );
  }

  return (
    <div>
      <video
        ref={mediaRef as React.RefObject<HTMLVideoElement>}
        autoPlay
        playsInline
      />
      <div>Layer: {currentLayer}</div>
      <button onClick={() => setPreferredLayer("h")}>High</button>
      <button onClick={() => setPreferredLayer("m")}>Medium</button>
      <button onClick={() => setPreferredLayer("l")}>Low</button>
    </div>
  );
}
```

## 7. エラーハンドリング

### 7.1 エラーの種類

| カテゴリ       | エラーコード             | 対処                       |
| -------------- | ------------------------ | -------------------------- |
| 接続エラー     | -                        | 自動再接続または手動再接続 |
| 認証エラー     | 1003                     | トークン再取得             |
| ルームエラー   | 1001, 1002, 1004         | ユーザーに通知             |
| メディアエラー | 1006, 1007, 1008         | 再試行またはフォールバック |
| 致命的エラー   | `ServerError.fatal=true` | 接続切断、再参加が必要     |

### 7.2 推奨エラーハンドリング

```typescript
room.on("error", (error: ServerError) => {
  if (error.fatal) {
    // 致命的エラー: 再参加が必要
    showErrorDialog("Connection lost. Please rejoin.");
    client.disconnect();
  } else {
    // 非致命的エラー: 通知のみ
    showToast(`Warning: ${error.message}`);
  }
});

room.on("trackSubscriptionFailed", (trackId, error) => {
  console.warn(`Failed to subscribe to track ${trackId}: ${error.message}`);
  // 必要に応じてリトライ
});
```

## 8. ベストプラクティス

### 8.1 パフォーマンス

- Simulcastを有効にして帯域幅を最適化
- 不要なトラックは早めにunsubscribe
- `autoSubscribe: false`で必要なトラックのみ購読

### 8.2 メモリ管理

- コンポーネントアンマウント時に`leave()`と`disconnect()`を呼び出す
- `detach()`でメディア要素から切り離す
- `stopMedia()`でローカルトラックを停止

### 8.3 再接続

- `autoReconnect: true`を推奨
- `sessionId`を保存して再接続時に使用
- `reconnecting`/`reconnected`イベントでUIを更新

### 8.4 デバッグ

```typescript
const client = new SFUClient({
  url: "wss://sfu.example.com/ws",
  logger: {
    level: "debug", // "error" | "warn" | "info" | "debug"
    handler: (level, message, data) => {
      console.log(`[${level}] ${message}`, data);
    },
  },
});
```
