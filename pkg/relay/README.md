# Relay（リレー）

`ion-sfu`は、ORTC APIを使用して他のion-SFUや他のサービスにトラックをリレーすることをサポートしています。

このAPIを使用することで、単一のリクエストをシグナリングするだけでストリームを他のサービスに素早く送信でき、その後のすべてのネゴシエーションは内部で処理されます。

## API

### Relay Peer（リレーピア）

リレーピアはWebRTC PeerConnectionと共通のメソッドを共有しているため、使用は簡単です。新しいリレーピアを作成するには、以下の例に従ってください：

```go
 // Metaはリレーしたいピアの関連情報をすべて保持します。
meta := PeerMeta{
    PeerID : "super-villain-1",
    SessionID : "world-domination",
}

// configは接続に必要なpion/webrtc関連の構造体を保持します。
// 要件に応じて設定するか、デフォルトのままにしてください。
config := &PeerConfig{}
peer, err := NewPeer(meta, config)
handleErr(err)

// ピアを操作する前に、リモートサーバーにピアをシグナリングする必要があります。
// シグナリングは任意の方法（gRPC、REST、pubsubなど）で行えます。
signalFunc = func (meta PeerMeta, signal []byte) ([]byte, error){
    if meta.session == "world-domination"{
        return RelayToLegionOfDoom(meta, signal)
    }
    return nil, errors.New("not supported")
}

// リモートピアはメタデータを使って新しいRelay Peerを作成し、Answerを呼び出す必要があります。
if err:= peer.Offer(signalFunc); err!=nil{
   handleErr(err)
}

// エラーがなければ、リレーピアはリレーされたピアと通信するための
// 便利なメソッドを提供します。

// Emitはリクエストイベントをfire-and-forgetで送信します
peer.Emit("evil-plan-1", data)

// Requestはリモートからの応答を待ちます。ピアが応答しない場合に
// 永続的にブロックしないよう、タイムアウト付きのcontextを使用してください
ans,err:= peer.Request(ctx, "evil-plan-2", data)

// リモートイベントを監視するには、ピアにコールバックをアタッチします
peer.OnRequest( func (event string, msg Message){
   // リクエストデータにアクセスするには
   msg.Paylod()
   // リクエストに返信するには
   msg.Reply(...)
})

// Relay Peerは、ピアのライフスパンを管理するための便利なコールバックも提供します。

// Peer OnCloseは、リモートピア接続が閉じられたとき、またはCloseメソッドが呼ばれたときに呼び出されます
peer.OnClose(func())

// Peer OnReadyは、リレーピアがトラック、データチャネル、リクエストのネゴシエーションを
// 開始する準備ができたときに呼び出されます。すべての初期化ロジックを
// このコールバックにアタッチすることを強く推奨します
peer.OnReady(func())

// トラックやデータチャネルを追加または受信するには、APIはWebRTC Peer Connectionと似ており、必要なコールバックをリスンするだけです
peer.OnDataChannel(f func(channel *webrtc.DataChannel))
peer.OnTrack(f func(track *webrtc.TrackRemote, receiver *webrtc.RTPReceiver))

// OnReadyコールバックが発火した後に、以下のメソッドを呼び出すようにしてください。
peer.CreateDataChannel(label string)
peer.AddTrack(receiver *webrtc.RTPReceiver, remoteTrack *webrtc.TrackRemote,
localTrack webrtc.TrackLocal) (*webrtc.RTPSender, error)
```

### ION-SFU統合

ION-SFUは、非常にシンプルな方法でピアをリレーするための便利なメソッドを提供しています。

ピアをリレーするには、単に`Peer.Publisher().Relay(...)`を呼び出し、データをリモートSFUにシグナリングして、以下を使用してデータを取り込みます：

`session.AddRelayPeer(peerID string, signalData []byte) ([]byte, error)`

このメソッドからの[]byte応答をシグナリングの応答として設定します。これで準備完了です。新しいSFUにピアが参加するたびに、リレーされたストリームがネゴシエートされます。
