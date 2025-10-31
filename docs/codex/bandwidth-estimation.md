# Bandwidth Estimation (BWE) in LiveKit Server

本ドキュメントは、LiveKit サーバーにおける帯域推定（Bandwidth Estimation: BWE）の仕組みと実装構成、信号の流れ、チューニング観点をまとめた技術解説です。対象は SFU の下り方向（サブスクライバ PC へ転送）における帯域推定と、それに連動する配信レイヤ選択・プロービング制御です。

- 実装パッケージ: `pkg/sfu/bwe`, `pkg/sfu/streamallocator`, `pkg/sfu/pacer`, `pkg/sfu/ccutils`
- 切替/設定: `pkg/config/config.go` 内 `CongestionControl`
- 利用箇所: `pkg/rtc/transport.go`（Subscriber PC 構築時）

---

## 全体像

LiveKit の SFU は 2 系統の BWE をサポートします。

1. Remote BWE（REMB/NACK ベース）

- 受信側（ブラウザなど）からの `RTCP REMB` と NACK 比率を用い、チャンネル容量のトレンドを監視して混雑検知・容量確定を行います。
- 実装: `pkg/sfu/bwe/remotebwe/*`

1. Send-Side BWE（TWCC ベース、JitterPath 簡略）

- `RTCP Transport-CC (TWCC)` を用いてパケット単位の送受遅延増分からキューイング遅延傾向を推定し、JQR（混雑）/DQR（非混雑）判定や損失指標により容量推定・混雑検知を行います。
- 実装: `pkg/sfu/bwe/sendsidebwe/*`

どちらも共通インタフェース `BWE`（`pkg/sfu/bwe/bwe.go`）を実装し、

- パケット送信記録（TWCC 拡張付与）
- RTCP 受信処理（REMB/TWCC）
- RTT 更新
- 混雑状態遷移通知（BWEListener）
- プローブクラスターの開始/終了/判定
  を提供します。BWE の推定結果は `StreamAllocator`（レイヤ割当・プロービング制御）と `Pacer`（送信スケジューリング）に反映されます。

切替は設定で制御します。

- `congestion_control.use_send_side_bwe: true` で自前の Send-Side BWE を使用
- `congestion_control.use_send_side_bwe_interceptor: true` で Pion GCC Interceptor を使用（簡易）
- いずれも false の場合は Remote BWE（REMB/NACK）

関連設定は `pkg/config/config.go` の `CongestionControlConfig` を参照。

---

## 信号の流れ（概要）

- 送信経路
  - `pacer.Base.patchRTPHeaderExtensions` が AbsSendTime/TWCC 拡張ヘッダを付加
  - TWCC シーケンス番号は `BWE.RecordPacketSendAndGetSequenceNumber` で払い出し
- 受信経路（RTCP）
  - REMB: `StreamAllocator.OnREMB` → 推定値イベント → `BWE.HandleREMB`（RemoteBWE）
  - TWCC: `StreamAllocator.OnTransportCCFeedback` → `BWE.HandleTWCCFeedback`（Send-Side BWE）
- RTT
  - `StreamAllocator` が定期的に `BWE.UpdateRTT` を実行
- 混雑通知
  - `BWE` 実装の状態遷移で `BWEListener.OnCongestionStateChange` が呼ばれ、`StreamAllocator` が容量更新・再割当
- プロービング
  - `StreamAllocator` が `ccutils.Prober` と `Pacer` を駆動
  - `BWE` 実装は Probe 開始/進捗/終了に応じて成功・失敗・不明を判定し、容量確定を行う

---

## Remote BWE（REMB/NACK ベース）

実装ファイル: `pkg/sfu/bwe/remotebwe/*`

- 入力
  - `HandleREMB(receivedEstimate, expectedUsage, sentPackets, repeatedNacks)`
    - `receivedEstimate`: REMB の受信推定（bps）
    - `expectedUsage`: プローブ時の期待使用帯域（bps）
    - `sentPackets`/`repeatedNacks`: 送出パケット数/繰り返し NACK 数
  - `UpdateRTT(rtt)`: RTT 平滑化（デフォルト 70ms, `RTTSmoothingFactor=0.5`）
- 観測器
  - `channelObserver`
    - `TrendDetector`（Kendall’s Tau）で推定値のトレンド監視（`channelTrendClearing/Congesting`）
    - `nackTracker` で短時間窓の繰り返し NACK 比率を監視
    - トレンド低下または NACK 比率上昇で「混雑傾向」を出し分け（理由: Estimate/Loss）
- 状態機械（`congestionDetectionStateMachine`）
  - `NONE` → `CONGESTED`: 混雑傾向時に容量見積りを試行（詳細下記）
  - `CONGESTED` → `NONE`: クリア傾向（トレンド回復）
- 容量確定（`estimateAvailableChannelCapacity`）
  - Loss が理由: `estimate = expectedUsage * (1 - nackRatioAttenuator * nackRatio)`
  - それ以外: `estimate = receivedEstimate`
  - `estimate` は `lastReceivedEstimate` を上限、かつ `expectedUsageThreshold * expectedUsage` より小さい時のみコミット
  - コミット時に `BWEListener` へ通知
- プローブ連携（`probeController`）
  - プローブ中に混雑検出した場合は観測をフリーズし、プローブ結果を汚染しない
  - プローブ終了時に `ProbeClusterFinalize`
    - 充分なサンプルがあり混雑なし→ 最高推定値を `committedChannelCapacity` に引き上げ
    - 混雑または不十分→ 増速せず終了
  - `SettleWaitNumRTT` などでプローブ後の収束待ち制御

主な設定（`RemoteBWEConfig`）

- `nack_ratio_attenuator`（デフォルト 0.4）
- `expected_usage_threshold`（デフォルト 0.95）
- `channel_observer_probe/non_probe`（トレンド/窓など）
- `probe_controller`（RTT 依存の待機など）

---

## Send-Side BWE（TWCC + JitterPath 簡略）

実装ファイル: `pkg/sfu/bwe/sendsidebwe/*`

- 入力
  - `RecordPacketSendAndGetSequenceNumber`: 送信時に TWCC 番号を払い出し
  - `HandleTWCCFeedback(*rtcp.TransportLayerCC)`: 受信側の到着時刻/TWCC から遅延増分を復元
  - `UpdateRTT(rtt)`: RTT 平滑化
- コア概念
  - パケットを時間窓の「グループ」に集約し、グループ内の
    - 伝搬キューイング遅延（propagated queuing delay）
    - 加重損失（weighted loss）
      を計測
  - 判定領域
    - JQR: キュー蓄積（混雑）
    - DQR: 非混雑
    - しきい値（hysteresis）で遷移をなだらかに
  - `qdMeasurement`/`lossMeasurement` が連続したグループでのしきい値到達、持続時間条件等で
    - Early Warning / Congested を段階的に確定
    - 併せて `congestionReason`（QueueingDelay/Loss）を確定
- 状態機械（`congestionDetector`）
  - `NONE`/`EARLY_WARNING`/`CONGESTED` の遷移
  - コンジェスト中は `Captured Traffic Ratio (CTR)` のトレンド監視（改善/悪化傾向）
  - 容量推定（`estimateAvailableChannelCapacity`）
    - 混雑理由に応じて「貢献グループ」の統計のみで `AcknowledgedBitrate()` を算出
    - 理由不明確な場合は時間窓（`EstimationWindowDuration`）で代用
  - 変化時は `BWEListener` に通知
- プローブ連携
  - `ProbeSignalConfig` で最小バイト/時間、JQR/DQR の遅延・損失の閾値から
    - `Congesting` / `NotCongesting` / `Inconclusive` を判定
  - プローブ完了で `committedChannelCapacity` を更新（非混雑かつ容量増）

主な設定（`SendSideBWEConfig` → `CongestionDetectorConfig`）

- グループ閾値（`PacketGroup`）、混雑検知条件（連続グループ数・持続時間）
- 加重損失の係数、JQR/DQR の遅延・損失しきい値
- `EstimationWindowDuration`（時間窓による代替推定）
- `ProbeRegulator`（プローブ間隔/量の制御）

---

## StreamAllocator と Pacer の役割

- `StreamAllocator`（`pkg/sfu/streamallocator`）
  - `BWEListener` を実装し、混雑状態変化で `committedChannelCapacity` を更新
  - 容量に基づき各 `DownTrack` の層（空間/時間）を割当（公平準最適）
    - まず低層を広く配り、余力があれば高層へ
    - 余力がなければ低優先度から一時停止（AllowPause=true の場合）
  - `Probe` 制御
    - 状態が Deficient で `BWE.CanProbe()` が true ならクラスターを起動
    - `Pacer` でパディング/プローブ送出、BWE に開始/終了を通告し、ゴール到達チェック
    - 結果 `NotCongesting` かつ推定増で `committedChannelCapacity` を引上げ、足りないトラックをブースト
  - 定期タスク
    - パケット送出が停止し混雑中のまま長時間経過時に BWE を `Reset`（プローブ再開を促す）
    - RTT を定期更新して BWE へ供給

- `Pacer`（`pkg/sfu/pacer`）
  - RTP 送出直前に AbsSendTime / TWCC を付加
  - 送信種別
    - `pass-through`: 既存トラフィックに追随（既定）
    - `no-queue`/`leaky-bucket`: キュー/リーキーバケット（構成可）
  - プローブ観測（`ProbeObserver`）でクラスターの進捗を通知

---

## 設定と切替

`config.yaml`（`congestion_control`）例:

```yaml
congestion_control:
  enabled: true
  allow_pause: true
  # 送信側 BWE（自前）を使う場合
  use_send_side_bwe: true
  send_side_bwe_pacer: no-queue # pass-through | no-queue | leaky-bucket
  send_side_bwe: { ... } # sendsidebwe の詳細設定

  # Pion Interceptor を使う場合（簡易）
  # use_send_side_bwe_interceptor: true

  # 受信側（REMB/NACK）BWE の設定
  remote_bwe: { ... }

  stream_allocator:
    probe_mode: padding
    probe_overage_pct: 120
    probe_min_bps: 200000
    paused_min_wait: 5s
```

- 運用の目安
  - 環境が TWCC を確実にサポートするなら send-side BWE が応答性・安定性で有利
  - 古いクライアントや簡略構成では Remote BWE（REMB）でも運用可能
  - リージョン/RTT が大きい場合は `ProbeController`/`ProbeSignal` の閾値や `EstimationWindowDuration` を調整

---

## 関連ファイル早見表

- インタフェース/共通: `pkg/sfu/bwe/bwe.go`
- Remote BWE: `pkg/sfu/bwe/remotebwe/*`（`remote_bwe.go`, `channel_observer.go`, `nack_tracker.go`, `probe_controller.go`）
- Send-Side BWE: `pkg/sfu/bwe/sendsidebwe/*`（`send_side_bwe.go`, `congestion_detector.go`, `packet_*`, `traffic_stats.go`, `twcc_feedback.go`）
- プロービング/補助: `pkg/sfu/ccutils/*`（`prober.go`, `probe_regulator.go`, `trenddetector.go` など）
- Pacer: `pkg/sfu/pacer/*`
- 割当: `pkg/sfu/streamallocator/*`（BWE 連携・層割当・プローブ駆動）
- WebRTC 取付: `pkg/rtc/transport.go`（BWE/Pacer/StreamAllocator の生成と配線）

---

## 参考と実装上の注意

- TWCC と AbsSendTime 拡張が有効であること（送信方向に適用）
- REMB は gratuitous（値不変でも送られる）可能性があり、トレンド判定は短絡しない（TrendDetector/NackTracker）
- プローブは `ProbeRegulator`/`ProbeSignal` により自己輻輳を避けつつ短時間で判断
- 混雑中の CTR トレンド監視で割当の妥当性を継続評価（過/不足を自動調整）

以上。運用環境（RTT・クライアント多様性・ネットワーク特性）に応じて、Remote/Send-Side の選択と各種の閾値を調整してください。
