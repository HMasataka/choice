# Bandwidth Estimation (BWE) 処理の詳細解説

## 概要

LiveKitのBandwidth Estimation（帯域幅推定）は、ネットワークの輻輳を検出し、利用可能な帯域幅を動的に推定することで、WebRTC通信の品質を最適化する仕組みです。

BWEは `pkg/sfu/bwe/` パッケージで実装されており、以下の3つのタイプがあります。

## BWEタイプ

### 1. NullBWE (pkg/sfu/bwe/null_bwe.go)

何も処理を行わないダミー実装です。BWE機能を無効化する場合に使用されます。

### 2. RemoteBWE (pkg/sfu/bwe/remotebwe/)

クライアント側からのフィードバック（REMB: Receiver Estimated Maximum Bitrate）を使用した帯域幅推定です。

### 3. SendSideBWE (pkg/sfu/bwe/sendsidebwe/)

サーバー側でTWCC（Transport Wide Congestion Control）フィードバックを使用した帯域幅推定です。

## BWEインターフェース (pkg/sfu/bwe/bwe.go)

### 基本定数

```go
DefaultRTT         = 70ms  // デフォルトRTT
RTTSmoothingFactor = 0.5   // RTT平滑化係数
```

### CongestionState（輻輳状態）

pkg/sfu/bwe/bwe.go:57-76

- **CongestionStateNone**: 輻輳なし
- **CongestionStateEarlyWarning**: 早期警告（輻輳の兆候）
- **CongestionStateCongested**: 輻輳状態

### 主要インターフェース

pkg/sfu/bwe/bwe.go:80-115

- `Reset()`: BWEをリセット
- `HandleREMB()`: REMBフィードバックを処理（RemoteBWE用）
- `RecordPacketSendAndGetSequenceNumber()`: パケット送信を記録しTWCCシーケンス番号を取得
- `HandleTWCCFeedback()`: TWCCフィードバックを処理（SendSideBWE用）
- `UpdateRTT()`: RTT（Round Trip Time）を更新
- `CongestionState()`: 現在の輻輳状態を取得
- プローブ関連メソッド: 帯域幅を積極的に測定するためのプローブ機能

## RemoteBWE（リモート側BWE）の詳細

### アーキテクチャ

RemoteBWEは受信側（クライアント）からのフィードバックを基に帯域幅を推定します。

#### 主要コンポーネント

1. **ChannelObserver** (pkg/sfu/bwe/remotebwe/channel_observer.go)
2. **NackTracker** (pkg/sfu/bwe/remotebwe/nack_tracker.go)
3. **ProbeController** (pkg/sfu/bwe/remotebwe/probe_controller.go)

### ChannelObserver（チャネル観測器）

pkg/sfu/bwe/remotebwe/channel_observer.go:117-203

#### チャネルトレンド

- **channelTrendInconclusive**: 不明
- **channelTrendClearing**: 回復中
- **channelTrendCongesting**: 輻輳中

#### 輻輳判定理由

- **channelCongestionReasonNone**: 輻輳なし
- **channelCongestionReasonEstimate**: 推定値による輻輳検出
- **channelCongestionReasonLoss**: パケットロスによる輻輳検出

#### 動作原理

pkg/sfu/bwe/remotebwe/channel_observer.go:170-185

1. **Estimate Trend（推定トレンド）**: 帯域幅推定値の変化を監視
   - 下降トレンド → 輻輳中
   - 上昇トレンド → 回復中

2. **NACK Tracker（NACKトラッカー）**: パケットロスを監視
   - NACK比率が閾値を超えると輻輳を検出

### RemoteBWEの輻輳検出フロー

pkg/sfu/bwe/remotebwe/remote_bwe.go:123-152

#### HandleREMB処理

```
1. REMBフィードバックを受信
2. ChannelObserverに推定値とNACK情報を追加
3. プローブ中かつ輻輳状態の場合、状態を凍結
4. 輻輳検出ステートマシンを実行
5. 状態変化があればリスナーに通知
```

### 輻輳検出ステートマシン

pkg/sfu/bwe/remotebwe/remote_bwe.go:161-195

#### 状態遷移

```
CongestionStateNone → CongestionStateCongested
  条件: チャネルトレンドが輻輳中 かつ
        (プローブ中 または 利用可能チャネル容量を推定)

CongestionStateCongested → CongestionStateCongested
  条件: 継続的に輻輳中

CongestionStateCongested → CongestionStateNone
  条件: 輻輳が解消
```

### 利用可能チャネル容量の推定

pkg/sfu/bwe/remotebwe/remote_bwe.go:197-226

#### 計算方法

輻輳理由による分岐:

- **Loss（パケットロス）の場合**:

```
推定容量 = 期待帯域使用量 × (1.0 - NACK比率減衰係数 × NACK比率)
```

- デフォルトのNACK比率減衰係数: 0.4

- **Estimate（推定値）の場合**:

```
推定容量 = 受信した推定値
```

#### 適用条件

```
コミット閾値 = 期待帯域使用量 × 期待使用閾値（デフォルト: 0.95）

推定容量がコミット閾値以下の場合のみ適用
```

### プローブ機能

pkg/sfu/bwe/remotebwe/remote_bwe.go:264-342

#### プローブクラスターの開始

1. ProbeControllerにプローブ開始を通知
2. 新しいChannelObserverを作成（プローブ用設定）
3. 期待帯域使用量を更新

#### プローブの終了判定

pkg/sfu/bwe/remotebwe/remote_bwe.go:288-299

```
プローブ目標達成の条件:
- プローブ中である
- 輻輳状態ではない
- 十分な推定サンプルがある
- 最高推定値がプローブ目標を達成
```

#### プローブ結果の確定

pkg/sfu/bwe/remotebwe/remote_bwe.go:301-342

プローブシグナルの決定:

- **ProbeSignalCongesting**: プローブ中に輻輳検出
- **ProbeSignalInconclusive**: サンプル不足
- **ProbeSignalNotCongesting**: 正常、容量を更新

### 設定パラメータ

pkg/sfu/bwe/remotebwe/remote_bwe.go:40-47

```yaml
nack_ratio_attenuator: 0.4 # NACK比率減衰係数
expected_usage_threshold: 0.95 # 期待使用閾値
channel_observer_probe: # プローブ用チャネル観測器設定
channel_observer_non_probe: # 非プローブ用チャネル観測器設定
probe_controller: # プローブコントローラー設定
```

## SendSideBWE（送信側BWE）の詳細

### アーキテクチャ

SendSideBWEは、JitterPath論文（簡略化/修正版）に基づいています。

参考: <https://homepage.iis.sinica.edu.tw/papers/lcs/2114-F.pdf>

### 基本原理

pkg/sfu/bwe/sendsidebwe/send_side_bwe.go:28-54

TWCCフィードバックを使用してデルタ片方向遅延（delta one-way-delay）を計算し、キューイング遅延を蓄積/伝播させることで、パケットグループがどのリージョンで動作しているかを判定します。

#### キューイングリージョン

- **JQR (Join Queuing Region)**: チャネルが輻輳状態
- **DQR (Disjoint Queuing Region)**: チャネルが正常状態
- **Indeterminate**: 不明

### 主要コンポーネント

1. **CongestionDetector** (pkg/sfu/bwe/sendsidebwe/congestion_detector.go)
2. **PacketTracker** (pkg/sfu/bwe/sendsidebwe/packet_tracker.go)
3. **TWCCFeedback** (pkg/sfu/bwe/sendsidebwe/twcc_feedback.go)
4. **PacketGroup** (pkg/sfu/bwe/sendsidebwe/packet_group.go)
5. **TrafficStats** (pkg/sfu/bwe/sendsidebwe/traffic_stats.go)

### TWCCFeedback処理

pkg/sfu/bwe/sendsidebwe/twcc_feedback.go:68-115

#### 処理フロー

1. フィードバックレポートを受信
2. リファレンスタイムのラップアラウンド処理
3. アウトオブオーダーの検出
4. フィードバック間隔の推定（平滑化）

#### リファレンスタイム

```
リファレンスタイム解像度: 64ms
マスク: (1 << 24) - 1
```

### CongestionDetector（輻輳検出器）

pkg/sfu/bwe/sendsidebwe/congestion_detector.go:526-556

#### データ構造

- `packetTracker`: パケット送信情報を追跡
- `twccFeedback`: TWCCフィードバック処理
- `packetGroups`: パケットグループのリスト
- `probePacketGroup`: プローブ用パケットグループ
- `estimatedAvailableChannelCapacity`: 推定利用可能チャネル容量
- `congestionState`: 輻輳状態
- `congestedCTRTrend`: 輻輳時のCTR（Captured Traffic Ratio）トレンド

### TWCCフィードバック処理の詳細

pkg/sfu/bwe/sendsidebwe/congestion_detector.go:610-744

#### 処理ステップ

1. **フィードバックレポートの処理**
   - TWCCフィードバックからリファレンスタイムを取得
   - アウトオブオーダーを検出

2. **パケットグループの初期化**
   - 最初のパケットグループを作成

3. **各パケットの処理**
   - パケット情報（送信時刻、受信時刻、ロス情報）を記録
   - 送信デルタと受信デルタを計算
   - CTRトレンドを更新
   - プローブパケットグループに追加
   - 通常のパケットグループに追加

4. **パケットグループの完了処理**
   - グループが完了したら新しいグループを作成
   - 伝播キューイング遅延を次のグループに引き継ぎ

5. **古いパケットグループの削除**

6. **輻輳検出ステートマシンの実行**

### 輻輳検出ステートマシン

pkg/sfu/bwe/sendsidebwe/congestion_detector.go:954-1004

#### 状態遷移

```
CongestionStateNone → CongestionStateEarlyWarning
  条件: 早期警告シグナルがJQR

CongestionStateEarlyWarning → CongestionStateCongested
  条件: 輻輳シグナルがJQR

CongestionStateEarlyWarning → CongestionStateNone
  条件: 早期警告シグナルがDQR

CongestionStateCongested → CongestionStateNone
  条件: 輻輳シグナルがDQR
```

#### CTRトレンド監視

輻輳状態時、CTR（Captured Traffic Ratio）の下降トレンドを検出すると、チャネル容量を再推定します。

```
輻輳時の確認済みビットレート < 推定利用可能チャネル容量
→ チャネル容量を更新
```

### キューイング遅延測定

pkg/sfu/bwe/sendsidebwe/congestion_detector.go:181-326

#### QDMeasurement（キューイング遅延測定）

##### 処理フロー

```
各パケットグループを処理:
  伝播キューイング遅延を取得

  if 遅延 < DQR最大遅延:
    DQRグループ数をインクリメント
    DQR設定の条件を満たす → リージョンをDQRに設定、シール

  else if 遅延 > JQR最小遅延:
    JQRグループ数をインクリメント
    JQR設定の条件を満たす かつ トレンド係数 > 閾値
      → リージョンをJQRに設定、シール

  else:
    不確定リージョン、連続性が途切れた場合はシール
```

##### トレンド係数の計算

pkg/sfu/bwe/sendsidebwe/congestion_detector.go:298-325

Kendallの順位相関係数に基づく計算:

```
一致ペア: 新しいサンプル > 古いサンプル（遅延増加）
不一致ペア: 新しいサンプル < 古いサンプル（遅延減少）

トレンド係数 = (一致ペア - 不一致ペア) / (一致ペア + 不一致ペア)

値の範囲: -1.0（下降）～ 1.0（上昇）
```

### パケットロス測定

pkg/sfu/bwe/sendsidebwe/congestion_detector.go:328-433

#### LossMeasurement（ロス測定）

##### 処理フロー

```
各パケットグループを処理:
  トラフィック統計をマージ

  JQR条件を満たす:
    加重ロス > JQR最小ロス → JQRリージョン、シール

  DQR条件を満たす:
    加重ロス < DQR最大ロス → DQRリージョン、シール
```

##### 加重ロス（Weighted Loss）

パケットロスを時間的な重み付けで計算し、最近のロスをより重視します。

### 輻輳シグナルの更新

pkg/sfu/bwe/sendsidebwe/congestion_detector.go:884-952

#### 早期警告シグナル

```yaml
queuing_delay_early_warning_jqr:
  min_number_of_groups: 2
  min_duration: 200ms

loss_early_warning_jqr:
  min_number_of_groups: 3
  min_duration: 300ms
```

#### 輻輳シグナル

```yaml
queuing_delay_congested_jqr:
  min_number_of_groups: 4
  min_duration: 400ms

loss_congested_jqr:
  min_number_of_groups: 6
  min_duration: 600ms
```

### 利用可能チャネル容量の推定

pkg/sfu/bwe/sendsidebwe/congestion_detector.go:1071-1114

#### 推定方法

##### 輻輳時

輻輳の原因となったパケットグループの範囲を使用:

```
if 輻輳理由 == キューイング遅延:
  QDMeasurementのグループ範囲を使用

else if 輻輳理由 == パケットロス:
  LossMeasurementのグループ範囲を使用
```

##### 非輻輳時

時間ウィンドウ測定を使用（デフォルト: 1秒）:

```
推定ウィンドウ期間内のパケットグループを集約
```

##### 容量計算

```
推定利用可能チャネル容量 = 集約トラフィック統計の確認済みビットレート
```

### CTRトレンド監視

pkg/sfu/bwe/sendsidebwe/congestion_detector.go:1029-1069

#### 動作原理

輻輳状態時、CTR（Captured Traffic Ratio）の変化を監視:

```
CTR = 確認済みビットレート / 送信ビットレート
```

#### 処理フロー

```
1. 輻輳状態に入る → CTRトレンドを作成
2. パケットグループが完了 → CTRを計算、トレンドに追加
3. CTRが下降トレンド → チャネル容量を再推定
4. 輻輳状態から脱出 → CTRトレンドをクリア
```

#### 量子化

pkg/sfu/bwe/sendsidebwe/congestion_detector.go:1058

小さな変化をフィルタリングするため、CTRを量子化:

```
epsilon = 0.05（デフォルト）
量子化CTR = int((CTR + epsilon/2) / epsilon) * epsilon
```

### プローブ機能

#### プローブシグナル設定

pkg/sfu/bwe/sendsidebwe/congestion_detector.go:85-131

```yaml
probe_signal:
  min_bytes_ratio: 0.5 # 最小バイト比率
  min_duration_ratio: 0.5 # 最小期間比率
  jqr_min_delay: 50ms # JQR最小遅延
  dqr_max_delay: 20ms # DQR最大遅延
  jqr_min_weighted_loss: 0.25 # JQR最小加重ロス
  dqr_max_weighted_loss: 0.1 # DQR最大加重ロス
```

#### プローブ結果の判定

pkg/sfu/bwe/sendsidebwe/congestion_detector.go:101-117

```
if 伝播キューイング遅延 > JQR最小遅延 または 加重ロス > JQR最小加重ロス:
  → ProbeSignalCongesting（輻輳中）

else if 伝播キューイング遅延 < DQR最大遅延 かつ 加重ロス < DQR最大加重ロス:
  → ProbeSignalNotCongesting（正常）

else:
  → ProbeSignalInconclusive（不明）
```

### デフォルト設定

pkg/sfu/bwe/sendsidebwe/congestion_detector.go:470-517

```yaml
packet_group:
  min_packets: 5
  max_window_duration: 100ms

packet_group_max_age: 10s

jqr_min_delay: 50ms
jqr_min_trend_coefficient: 0.8
dqr_max_delay: 20ms

weighted_loss:
  # 加重ロス設定

jqr_min_weighted_loss: 0.25
dqr_max_weighted_loss: 0.1

queuing_delay_early_warning_jqr:
  min_number_of_groups: 2
  min_duration: 200ms

loss_early_warning_jqr:
  min_number_of_groups: 3
  min_duration: 300ms

queuing_delay_congested_jqr:
  min_number_of_groups: 4
  min_duration: 400ms

loss_congested_jqr:
  min_number_of_groups: 6
  min_duration: 600ms

congested_ctr_trend:
  required_samples: 4
  required_samples_min: 2
  downward_trend_threshold: -0.5
  downward_trend_max_wait: 2s

estimation_window_duration: 1s
```

## BWEの利用フロー

### 初期化

```go
// RemoteBWEの場合
bwe := remotebwe.NewRemoteBWE(remotebwe.RemoteBWEParams{
    Config: config,
    Logger: logger,
})

// SendSideBWEの場合
bwe := sendsidebwe.NewSendSideBWE(sendsidebwe.SendSideBWEParams{
    Config: config,
    Logger: logger,
})

// リスナーを設定
bwe.SetBWEListener(bweListener)
```

### パケット送信（SendSideBWEの場合）

```go
// パケット送信時にシーケンス番号を取得
seqNum := bwe.RecordPacketSendAndGetSequenceNumber(
    sendTime,
    packetSize,
    isRTX,
    probeClusterId,
    isProbe,
)
```

### フィードバック処理

```go
// RemoteBWE: REMBフィードバックを処理
bwe.HandleREMB(
    receivedEstimate,
    expectedBandwidthUsage,
    sentPackets,
    repeatedNacks,
)

// SendSideBWE: TWCCフィードバックを処理
bwe.HandleTWCCFeedback(twccReport)
```

### 輻輳状態の取得

```go
congestionState := bwe.CongestionState()

switch congestionState {
case bwe.CongestionStateNone:
    // 正常状態
case bwe.CongestionStateEarlyWarning:
    // 早期警告
case bwe.CongestionStateCongested:
    // 輻輳状態
}
```

### プローブの実行

```go
// プローブ可能か確認
if bwe.CanProbe() {
    duration := bwe.ProbeDuration()

    // プローブ開始
    bwe.ProbeClusterStarting(probeClusterInfo)

    // プローブ実行...

    // プローブ終了
    bwe.ProbeClusterDone(probeClusterInfo)

    // 目標達成確認
    if bwe.ProbeClusterIsGoalReached() {
        // 目標達成
    }

    // プローブ結果を確定
    signal, capacity, ok := bwe.ProbeClusterFinalize()
}
```

### リスナーでの通知受信

```go
type MyBWEListener struct {}

func (l *MyBWEListener) OnCongestionStateChange(
    fromState bwe.CongestionState,
    toState bwe.CongestionState,
    estimatedAvailableChannelCapacity int64,
) {
    // 輻輳状態の変化を処理
    // チャネル容量に基づいてビットレート配分を調整
}
```

## まとめ

LiveKitのBWE実装は、WebRTC通信における帯域幅管理の包括的なソリューションを提供します。

### RemoteBWE

- REMBフィードバックとNACKを使用
- シンプルで実装が容易
- クライアント側の推定に依存

### SendSideBWE

- TWCCフィードバックを使用
- より詳細な輻輳検出（JitterPathベース）
- キューイング遅延とパケットロスの両方を監視
- 早期警告機能により予防的な対応が可能
- CTRトレンド監視による継続的な最適化

両方式とも、プローブ機能により積極的に帯域幅を測定し、動的にチャネル容量を最適化します。
