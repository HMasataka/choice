# デプロイメントガイド

WebRTC SFU のデプロイメント手順、環境構成、インフラ要件を定義する。

## 1. 概要

### 1.1 対象環境

| 環境         | 用途           | インフラ                       |
| ------------ | -------------- | ------------------------------ |
| ローカル     | 開発・テスト   | Docker Compose                 |
| CI           | 自動テスト     | Docker Compose                 |
| ステージング | 統合テスト     | GKE (Google Kubernetes Engine) |
| 本番         | プロダクション | GKE + Cloudflare               |

### 1.2 クラウドサービス

| サービス               | 用途                         | プロバイダー                   |
| ---------------------- | ---------------------------- | ------------------------------ |
| Kubernetes             | コンテナオーケストレーション | GKE (Google Kubernetes Engine) |
| ロードバランサー       | L4/L7 負荷分散               | Cloud Load Balancing           |
| CDN/DDoS防御           | エッジ保護                   | Cloudflare                     |
| オブジェクトストレージ | 録画保存                     | Cloud Storage                  |
| KVストア               | セッション管理               | Memorystore for Redis          |
| DNS                    | ドメイン管理                 | Cloudflare DNS                 |
| 証明書                 | TLS                          | Cloudflare (Origin CA)         |
| シークレット管理       | 機密情報                     | Secret Manager                 |
| ログ                   | ログ集約                     | Cloud Logging                  |
| 監視                   | メトリクス・アラート         | Cloud Monitoring + Prometheus  |

## 2. 環境変数

### 2.1 必須環境変数

| 変数名             | 説明                         | 例                                               |
| ------------------ | ---------------------------- | ------------------------------------------------ |
| `SFU_PUBLIC_IP`    | SFUの公開IPアドレス          | `203.0.113.10`                                   |
| `SFU_HTTP_PORT`    | HTTPサーバーポート           | `8080`                                           |
| `SFU_UDP_PORT_MIN` | WebRTC UDPポート範囲（開始） | `10000`                                          |
| `SFU_UDP_PORT_MAX` | WebRTC UDPポート範囲（終了） | `20000`                                          |
| `REDIS_URL`        | Redis接続URL                 | `redis://10.0.0.5:6379`                          |
| `JWT_JWKS_URL`     | JWKSエンドポイントURL        | `https://auth.example.com/.well-known/jwks.json` |
| `JWT_ISSUER`       | JWTの発行者 (iss)            | `https://auth.example.com`                       |
| `JWT_AUDIENCE`     | JWTの対象者 (aud)            | `sfu.example.com`                                |

### 2.2 オプション環境変数

| 変数名                          | 説明                               | デフォルト                     |
| ------------------------------- | ---------------------------------- | ------------------------------ |
| `SFU_LOG_LEVEL`                 | ログレベル                         | `info`                         |
| `SFU_LOG_FORMAT`                | ログフォーマット                   | `json`                         |
| `SFU_MAX_ROOMS`                 | 最大ルーム数                       | `1000`                         |
| `SFU_MAX_PARTICIPANTS_PER_ROOM` | ルームあたり最大参加者数           | `100`                          |
| `SFU_MAX_TRACKS_PER_ROOM`       | ルームあたり最大トラック数         | `500`                          |
| `SFU_RECONNECT_TIMEOUT`         | 再接続タイムアウト（秒）           | `30`                           |
| `SFU_ICE_LITE`                  | ICE Liteモード有効化               | `true`                         |
| `SFU_STUN_SERVERS`              | STUNサーバーリスト（カンマ区切り） | `stun:stun.l.google.com:19302` |
| `SFU_TURN_SERVERS`              | TURNサーバーリスト（JSON形式）     | `[]`                           |
| `STORAGE_BUCKET`                | 録画保存先バケット                 | -                              |
| `STORAGE_PREFIX`                | 録画保存パスプレフィックス         | `recordings/`                  |
| `METRICS_ENABLED`               | Prometheusメトリクス有効化         | `true`                         |

### 2.3 環境変数の優先順位

1. 環境変数
2. 設定ファイル (`config.yaml`)
3. デフォルト値

## 3. 設定ファイル

### 3.1 config.yaml

```yaml
# config.yaml（design.md セクション3.10 準拠）
server:
  http:
    host: "0.0.0.0"
    port: 8080
    read_timeout: 30s
    write_timeout: 30s
  websocket:
    read_buffer_size: 1024
    write_buffer_size: 1024
    handshake_timeout: 10s
    ping_interval: 30s

webrtc:
  ice_lite: true
  ice_servers:
    - urls:
        - "stun:stun.l.google.com:19302"
    # TURNサーバー（オプション）
    # - urls:
    #     - "turn:turn.example.com:3478"
    #   username: ""
    #   credential: ""
  port_range:
    min: 10000
    max: 20000
  public_ip: "" # 環境変数 SFU_PUBLIC_IP で上書き

room:
  max_participants: 100
  max_tracks_per_room: 500
  max_tracks_per_participant:
    video: 3
    audio: 2
  empty_timeout: 5m
  reconnect_timeout: 30s

auth:
  jwt:
    algorithm: "RS256"
    jwks_url: "https://auth.example.com/.well-known/jwks.json"
    issuer: "https://auth.example.com"
    audience: "sfu.example.com"
    cache_ttl: 1h
  rate_limit:
    websocket_connections: 10 # per second per IP
    signaling_messages: 100 # per second per connection
    api_requests: 100 # per minute per token

redis:
  url: redis://localhost:6379
  pool_size: 10
  dial_timeout: 5s
  read_timeout: 3s
  write_timeout: 3s

recording:
  enabled: false
  format: "webm" # webm | mp4
  storage:
    type: "gcs"
    bucket: ""
    prefix: "recordings/"

logging:
  level: info # debug | info | warn | error
  format: json # json | text
  # PII マスキング
  mask_ip: true
  hash_user_id: false

metrics:
  enabled: true
  path: /metrics
```

## 4. Docker

### 4.1 Dockerfile

```dockerfile
# Dockerfile
FROM golang:1.22-alpine AS builder

WORKDIR /app

# 依存関係のキャッシュ
COPY go.mod go.sum ./
RUN go mod download

# ソースコードのコピーとビルド
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /sfu ./cmd/sfu

# 実行用イメージ
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /sfu /sfu
COPY config.yaml /etc/sfu/config.yaml

USER nonroot:nonroot

EXPOSE 8080
EXPOSE 10000-20000/udp

ENTRYPOINT ["/sfu"]
CMD ["--config", "/etc/sfu/config.yaml"]
```

### 4.2 Docker Compose（開発環境）

```yaml
# docker-compose.yaml
version: "3.8"

services:
  sfu:
    build: .
    ports:
      - "8080:8080"
      - "10000-10100:10000-10100/udp"
    environment:
      - SFU_PUBLIC_IP=127.0.0.1
      - SFU_LOG_LEVEL=debug
      - REDIS_URL=redis://redis:6379
      # 開発環境: ローカルJWKSモックサーバーまたは直接公開鍵を使用
      - JWT_JWKS_URL=http://localhost:8081/.well-known/jwks.json
      - JWT_ISSUER=http://localhost:8081
      - JWT_AUDIENCE=localhost
    volumes:
      - ./config.yaml:/etc/sfu/config.yaml:ro
    depends_on:
      - redis
    networks:
      - sfu-network

  redis:
    image: redis:7-alpine
    ports:
      - "6379:6379"
    volumes:
      - redis-data:/data
    networks:
      - sfu-network

  # 開発用: Prometheus
  prometheus:
    image: prom/prometheus:v2.47.0
    ports:
      - "9090:9090"
    volumes:
      - ./prometheus.yaml:/etc/prometheus/prometheus.yml:ro
    networks:
      - sfu-network

  # 開発用: Grafana
  grafana:
    image: grafana/grafana:10.1.0
    ports:
      - "3000:3000"
    environment:
      - GF_SECURITY_ADMIN_PASSWORD=admin
    volumes:
      - grafana-data:/var/lib/grafana
    networks:
      - sfu-network

volumes:
  redis-data:
  grafana-data:

networks:
  sfu-network:
    driver: bridge
```

### 4.3 prometheus.yaml

```yaml
# prometheus.yaml
global:
  scrape_interval: 15s
  evaluation_interval: 15s

scrape_configs:
  - job_name: "sfu"
    static_configs:
      - targets: ["sfu:8080"] # HTTPサーバーの /metrics
    metrics_path: /metrics
```

## 5. Kubernetes (GKE)

### 5.1 クラスタ構成

| 項目          | ステージング      | 本番                           |
| ------------- | ----------------- | ------------------------------ |
| リージョン    | asia-northeast1   | asia-northeast1 (マルチゾーン) |
| ノードプール  | e2-standard-4 x 2 | n2-standard-8 x 3+             |
| 最小ノード数  | 2                 | 3                              |
| 最大ノード数  | 4                 | 10                             |
| SFUレプリカ数 | 2                 | 3+                             |

### 5.2 Namespace

```yaml
# namespace.yaml
apiVersion: v1
kind: Namespace
metadata:
  name: sfu
  labels:
    name: sfu
```

### 5.3 ConfigMap

```yaml
# configmap.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: sfu-config
  namespace: sfu
data:
  # UDP LoadBalancerの外部IPを設定（デプロイ後に更新）
  public-ip: "YOUR_UDP_LB_EXTERNAL_IP"
  config.yaml: |
    server:
      http:
        host: "0.0.0.0"
        port: 8080
        read_timeout: 30s
        write_timeout: 30s
      websocket:
        read_buffer_size: 1024
        write_buffer_size: 1024
        handshake_timeout: 10s
        ping_interval: 30s
    webrtc:
      ice_lite: true
      ice_servers:
        - urls:
            - "stun:stun.l.google.com:19302"
      port_range:
        min: 10000
        max: 20000
      public_ip: ""  # 環境変数 SFU_PUBLIC_IP で上書き
    room:
      max_participants: 100
      max_tracks_per_room: 500
      max_tracks_per_participant:
        video: 3
        audio: 2
      empty_timeout: 5m
      reconnect_timeout: 30s
    logging:
      level: info
      format: json
      mask_ip: true
    metrics:
      enabled: true
      path: /metrics
```

### 5.4 Secret（External Secrets Operator推奨）

```yaml
# secret.yaml（開発用、本番はExternal Secrets Operatorを使用）
apiVersion: v1
kind: Secret
metadata:
  name: sfu-secrets
  namespace: sfu
type: Opaque
stringData:
  redis-url: "redis://10.0.0.5:6379"
  jwt-jwks-url: "https://auth.example.com/.well-known/jwks.json"
  jwt-issuer: "https://auth.example.com"
  jwt-audience: "sfu.example.com"
---
# External Secrets Operator用（本番推奨）
apiVersion: external-secrets.io/v1beta1
kind: ExternalSecret
metadata:
  name: sfu-secrets
  namespace: sfu
spec:
  refreshInterval: 1h
  secretStoreRef:
    kind: ClusterSecretStore
    name: gcp-secret-manager
  target:
    name: sfu-secrets
    creationPolicy: Owner
  data:
    - secretKey: redis-url
      remoteRef:
        key: sfu-redis-url
    - secretKey: jwt-jwks-url
      remoteRef:
        key: sfu-jwt-jwks-url
    - secretKey: jwt-issuer
      remoteRef:
        key: sfu-jwt-issuer
    - secretKey: jwt-audience
      remoteRef:
        key: sfu-jwt-audience
```

### 5.5 Deployment

```yaml
# deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: sfu
  namespace: sfu
  labels:
    app: sfu
spec:
  replicas: 3
  selector:
    matchLabels:
      app: sfu
  strategy:
    type: RollingUpdate
    rollingUpdate:
      maxSurge: 1
      maxUnavailable: 0
  template:
    metadata:
      labels:
        app: sfu
      annotations:
        prometheus.io/scrape: "true"
        prometheus.io/port: "8080" # メトリクスはHTTPサーバーの /metrics で提供
        prometheus.io/path: "/metrics"
    spec:
      serviceAccountName: sfu
      securityContext:
        runAsNonRoot: true
        runAsUser: 65534
        fsGroup: 65534
      containers:
        - name: sfu
          image: asia-northeast1-docker.pkg.dev/PROJECT_ID/sfu/sfu:latest
          imagePullPolicy: Always
          ports:
            - containerPort: 8080
              name: http
              protocol: TCP
            - containerPort: 10000
              name: webrtc-udp
              protocol: UDP
          env:
            # SFU_PUBLIC_IP: UDP LoadBalancerの外部IPを設定
            # 方法1: ConfigMapで静的に設定（推奨）
            - name: SFU_PUBLIC_IP
              valueFrom:
                configMapKeyRef:
                  name: sfu-config
                  key: public-ip
            # 方法2: Init Containerでメタデータから取得（下記参照）
            - name: REDIS_URL
              valueFrom:
                secretKeyRef:
                  name: sfu-secrets
                  key: redis-url
            - name: JWT_JWKS_URL
              valueFrom:
                secretKeyRef:
                  name: sfu-secrets
                  key: jwt-jwks-url
            - name: JWT_ISSUER
              valueFrom:
                secretKeyRef:
                  name: sfu-secrets
                  key: jwt-issuer
            - name: JWT_AUDIENCE
              valueFrom:
                secretKeyRef:
                  name: sfu-secrets
                  key: jwt-audience
          volumeMounts:
            - name: config
              mountPath: /etc/sfu
              readOnly: true
          resources:
            requests:
              cpu: "1"
              memory: "2Gi"
            limits:
              cpu: "4"
              memory: "8Gi"
          livenessProbe:
            httpGet:
              path: /health
              port: 8080
            initialDelaySeconds: 10
            periodSeconds: 10
            timeoutSeconds: 5
            failureThreshold: 3
          readinessProbe:
            httpGet:
              path: /ready
              port: 8080
            initialDelaySeconds: 5
            periodSeconds: 5
            timeoutSeconds: 3
            failureThreshold: 3
          securityContext:
            allowPrivilegeEscalation: false
            readOnlyRootFilesystem: true
            capabilities:
              drop:
                - ALL
      volumes:
        - name: config
          configMap:
            name: sfu-config
      affinity:
        podAntiAffinity:
          preferredDuringSchedulingIgnoredDuringExecution:
            - weight: 100
              podAffinityTerm:
                labelSelector:
                  matchLabels:
                    app: sfu
                topologyKey: kubernetes.io/hostname
      topologySpreadConstraints:
        - maxSkew: 1
          topologyKey: topology.kubernetes.io/zone
          whenUnsatisfiable: ScheduleAnyway
          labelSelector:
            matchLabels:
              app: sfu
```

### 5.6 Service

```yaml
# service.yaml
apiVersion: v1
kind: Service
metadata:
  name: sfu
  namespace: sfu
  annotations:
    cloud.google.com/neg: '{"ingress": true}'
spec:
  type: ClusterIP
  ports:
    - name: http
      port: 80
      targetPort: 8080
      protocol: TCP
  selector:
    app: sfu
```

> **Note (メトリクス)**: メトリクスは HTTPサーバーの `/metrics` パスで提供されます（design.md準拠）。
> Prometheusは `prometheus.io/port: "8080"` と `prometheus.io/path: "/metrics"` アノテーションでスクレイプします。

> **Note (UDP ポート範囲)**: WebRTCは10000-20000の広いUDPポート範囲を使用します。
> Kubernetesの標準ServiceではUDPポート範囲を扱えないため、以下の方法から選択してください。

### 5.6.1 方法A: hostNetwork使用（推奨）

Podがノードのネットワークを直接使用し、ノードの外部IPでICE候補を広告します。

```yaml
# deployment-hostnetwork.yaml（5.5 Deploymentの spec.template.spec に追加）
spec:
  template:
    spec:
      hostNetwork: true
      dnsPolicy: ClusterFirstWithHostNet
      # SFU_PUBLIC_IPにはノードの外部IPを設定
```

この場合、Firewall Ruleでノードへの直接UDP通信を許可します：

```bash
gcloud compute firewall-rules create allow-sfu-webrtc \
  --direction=INGRESS \
  --priority=1000 \
  --network=default \
  --action=ALLOW \
  --rules=udp:10000-20000 \
  --target-tags=sfu-node
```

### 5.6.2 方法B: 固定ポート範囲でLoadBalancer

使用ポートを制限する場合（小規模向け、最大100ポート程度）：

```yaml
# service-udp.yaml
apiVersion: v1
kind: Service
metadata:
  name: sfu-udp
  namespace: sfu
  annotations:
    cloud.google.com/l4-rbs: "enabled"
spec:
  type: LoadBalancer
  externalTrafficPolicy: Local
  ports:
    # 必要なポート数に応じて追加（例: 100ポート）
    - name: webrtc-udp-10000
      port: 10000
      targetPort: 10000
      protocol: UDP
    - name: webrtc-udp-10001
      port: 10001
      targetPort: 10001
      protocol: UDP
    # ... 必要に応じて追加
    - name: webrtc-udp-10099
      port: 10099
      targetPort: 10099
      protocol: UDP
  selector:
    app: sfu
```

> **Note**: GKE LoadBalancerは最大5つのポートをサポートするため、
> 大規模な場合は方法Aの hostNetwork を使用してください。

### 5.7 Ingress (GKE Ingress + Cloudflare)

```yaml
# ingress.yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: sfu
  namespace: sfu
  annotations:
    kubernetes.io/ingress.class: "gce"
    kubernetes.io/ingress.global-static-ip-name: "sfu-ip"
    networking.gke.io/managed-certificates: "sfu-cert"
    networking.gke.io/v1beta1.FrontendConfig: "sfu-frontend"
spec:
  rules:
    - host: sfu.example.com
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: sfu
                port:
                  number: 80
---
# ManagedCertificate（Cloudflare Origin CAを使う場合は不要）
apiVersion: networking.gke.io/v1
kind: ManagedCertificate
metadata:
  name: sfu-cert
  namespace: sfu
spec:
  domains:
    - sfu.example.com
---
# FrontendConfig
apiVersion: networking.gke.io/v1beta1
kind: FrontendConfig
metadata:
  name: sfu-frontend
  namespace: sfu
spec:
  redirectToHttps:
    enabled: true
    responseCodeName: MOVED_PERMANENTLY_DEFAULT
```

### 5.8 HorizontalPodAutoscaler

```yaml
# hpa.yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: sfu
  namespace: sfu
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: sfu
  minReplicas: 3
  maxReplicas: 10
  metrics:
    - type: Resource
      resource:
        name: cpu
        target:
          type: Utilization
          averageUtilization: 70
    - type: Resource
      resource:
        name: memory
        target:
          type: Utilization
          averageUtilization: 80
  behavior:
    scaleDown:
      stabilizationWindowSeconds: 300
      policies:
        - type: Pods
          value: 1
          periodSeconds: 60
    scaleUp:
      stabilizationWindowSeconds: 0
      policies:
        - type: Pods
          value: 2
          periodSeconds: 60
```

### 5.9 PodDisruptionBudget

```yaml
# pdb.yaml
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: sfu
  namespace: sfu
spec:
  minAvailable: 2
  selector:
    matchLabels:
      app: sfu
```

### 5.10 ServiceAccount

```yaml
# serviceaccount.yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: sfu
  namespace: sfu
  annotations:
    iam.gke.io/gcp-service-account: sfu@PROJECT_ID.iam.gserviceaccount.com
```

## 6. Cloudflare設定

### 6.1 DNS設定

| レコード              | タイプ | 値             | プロキシ           |
| --------------------- | ------ | -------------- | ------------------ |
| `sfu.example.com`     | A      | GKE Ingress IP | オン（オレンジ雲） |
| `sfu-udp.example.com` | A      | UDP LB IP      | オフ（グレー雲）   |

> **注意**: WebRTC UDP通信はCloudflareプロキシを経由できないため、UDP用エンドポイントはプロキシをオフにする。

### 6.2 SSL/TLS設定

| 項目              | 設定値        |
| ----------------- | ------------- |
| SSL/TLSモード     | Full (strict) |
| 最小TLSバージョン | TLS 1.2       |
| TLS 1.3           | 有効          |
| HSTS              | 有効          |
| 自動HTTPS書き換え | 有効          |

### 6.3 Origin CA証明書

GKE IngressでCloudflare Origin CA証明書を使用する場合:

```bash
# Origin CA証明書の作成（Cloudflareダッシュボードで実行）
# 1. SSL/TLS > Origin Server > Create Certificate
# 2. Private keyとCertificateをダウンロード

# Kubernetes Secretとして保存
kubectl create secret tls sfu-origin-cert \
  --namespace sfu \
  --cert=origin-cert.pem \
  --key=origin-key.pem
```

### 6.4 Firewall Rules

```hcl
# Cloudflare IP + GCLBヘルスチェックIPからのみ許可（HTTP/HTTPS）
# https://www.cloudflare.com/ips/
# https://cloud.google.com/load-balancing/docs/health-check-concepts#ip-ranges

# GCP Firewall Rule
resource "google_compute_firewall" "allow-cloudflare-and-gclb" {
  name    = "allow-cloudflare-and-gclb"
  network = "default"

  allow {
    protocol = "tcp"
    ports    = ["80", "443"]
  }

  source_ranges = [
    # Cloudflare IPs
    "173.245.48.0/20",
    "103.21.244.0/22",
    "103.22.200.0/22",
    "103.31.4.0/22",
    "141.101.64.0/18",
    "108.162.192.0/18",
    "190.93.240.0/20",
    "188.114.96.0/20",
    "197.234.240.0/22",
    "198.41.128.0/17",
    "162.158.0.0/15",
    "104.16.0.0/13",
    "104.24.0.0/14",
    "172.64.0.0/13",
    "131.0.72.0/22",
    # GCP Load Balancer Health Check IPs
    "35.191.0.0/16",
    "130.211.0.0/22"
  ]

  target_tags = ["sfu"]
}
```

### 6.5 WAF/Rate Limiting

Cloudflare WAFルール（推奨）:

| ルール        | アクション | 条件                                |
| ------------- | ---------- | ----------------------------------- |
| WebSocket許可 | Allow      | `http.request.uri.path eq "/ws"`    |
| APIレート制限 | Rate Limit | `/api/*` に対し 100req/min          |
| 管理API保護   | Block      | `/admin/*` かつ IP not in allowlist |

## 7. Memorystore for Redis

### 7.1 インスタンス構成

| 項目       | ステージング    | 本番                 |
| ---------- | --------------- | -------------------- |
| ティア     | Basic           | Standard (HA)        |
| メモリ     | 1 GB            | 5 GB                 |
| バージョン | Redis 7.0       | Redis 7.0            |
| リージョン | asia-northeast1 | asia-northeast1      |
| レプリカ   | なし            | 1 (読み取りレプリカ) |

### 7.2 接続設定

```bash
# VPCネイティブ接続（推奨）
# GKEクラスタとMemorystoreを同じVPCに配置

# 接続URL形式
REDIS_URL=redis://10.0.0.5:6379

# 認証有効時（AUTH）
REDIS_URL=redis://:password@10.0.0.5:6379
```

### 7.3 Terraform設定例

```hcl
resource "google_redis_instance" "sfu" {
  name           = "sfu-redis"
  tier           = "STANDARD_HA"
  memory_size_gb = 5
  region         = "asia-northeast1"

  authorized_network = google_compute_network.vpc.id

  redis_version = "REDIS_7_0"

  display_name = "SFU Session Store"

  labels = {
    app = "sfu"
    env = "production"
  }

  maintenance_policy {
    weekly_maintenance_window {
      day = "SUNDAY"
      start_time {
        hours   = 3
        minutes = 0
      }
    }
  }
}
```

## 8. Cloud Storage

### 8.1 バケット構成

| 項目             | 値                          |
| ---------------- | --------------------------- |
| バケット名       | `PROJECT_ID-sfu-recordings` |
| リージョン       | asia-northeast1             |
| ストレージクラス | Standard                    |
| アクセス制御     | Uniform                     |
| 暗号化           | Google管理キー              |

### 8.2 ライフサイクルルール

```json
{
  "lifecycle": {
    "rule": [
      {
        "action": { "type": "SetStorageClass", "storageClass": "NEARLINE" },
        "condition": { "age": 30 }
      },
      {
        "action": { "type": "SetStorageClass", "storageClass": "COLDLINE" },
        "condition": { "age": 90 }
      },
      {
        "action": { "type": "Delete" },
        "condition": { "age": 365 }
      }
    ]
  }
}
```

### 8.3 IAM設定

```bash
# SFUサービスアカウントにStorage Object Admin権限を付与
gcloud projects add-iam-policy-binding PROJECT_ID \
  --member="serviceAccount:sfu@PROJECT_ID.iam.gserviceaccount.com" \
  --role="roles/storage.objectAdmin" \
  --condition="expression=resource.name.startsWith('projects/_/buckets/PROJECT_ID-sfu-recordings'),title=sfu-recordings-only"
```

## 9. デプロイ手順

### 9.1 前提条件

```bash
# 必要なツール
gcloud version  # Google Cloud SDK
kubectl version # Kubernetes CLI
docker version  # Docker
terraform version # Terraform (オプション)
```

### 9.2 初期セットアップ

```bash
# 1. GCPプロジェクト設定
export PROJECT_ID=your-project-id
export REGION=asia-northeast1
gcloud config set project $PROJECT_ID
gcloud config set compute/region $REGION

# 2. 必要なAPIを有効化
gcloud services enable \
  container.googleapis.com \
  redis.googleapis.com \
  storage.googleapis.com \
  secretmanager.googleapis.com \
  artifactregistry.googleapis.com

# 3. Artifact Registry リポジトリ作成
gcloud artifacts repositories create sfu \
  --repository-format=docker \
  --location=$REGION \
  --description="SFU Docker images"

# 4. GKEクラスタ作成
gcloud container clusters create sfu-cluster \
  --region=$REGION \
  --num-nodes=1 \
  --machine-type=n2-standard-4 \
  --enable-autoscaling \
  --min-nodes=1 \
  --max-nodes=5 \
  --enable-ip-alias \
  --workload-pool=$PROJECT_ID.svc.id.goog

# 5. kubectl認証設定
gcloud container clusters get-credentials sfu-cluster --region=$REGION
```

### 9.3 イメージビルド・プッシュ

```bash
# Docker認証設定
gcloud auth configure-docker $REGION-docker.pkg.dev

# イメージビルド
docker build -t $REGION-docker.pkg.dev/$PROJECT_ID/sfu/sfu:latest .

# イメージプッシュ
docker push $REGION-docker.pkg.dev/$PROJECT_ID/sfu/sfu:latest

# タグ付け（バージョン管理）
VERSION=$(git describe --tags --always)
docker tag $REGION-docker.pkg.dev/$PROJECT_ID/sfu/sfu:latest \
  $REGION-docker.pkg.dev/$PROJECT_ID/sfu/sfu:$VERSION
docker push $REGION-docker.pkg.dev/$PROJECT_ID/sfu/sfu:$VERSION
```

### 9.4 Kubernetesリソースデプロイ

```bash
# Namespace作成
kubectl apply -f k8s/namespace.yaml

# Secretsデプロイ（Secret Managerから取得する場合はExternal Secrets Operator使用）
kubectl apply -f k8s/secrets.yaml

# ConfigMapデプロイ
kubectl apply -f k8s/configmap.yaml

# ServiceAccountデプロイ
kubectl apply -f k8s/serviceaccount.yaml

# Deploymentデプロイ
kubectl apply -f k8s/deployment.yaml

# Serviceデプロイ
kubectl apply -f k8s/service.yaml

# Ingressデプロイ
kubectl apply -f k8s/ingress.yaml

# HPA/PDBデプロイ
kubectl apply -f k8s/hpa.yaml
kubectl apply -f k8s/pdb.yaml

# デプロイ確認
kubectl get pods -n sfu
kubectl get svc -n sfu
kubectl get ingress -n sfu
```

### 9.5 ローリングアップデート

```bash
# 新バージョンのイメージをデプロイ
kubectl set image deployment/sfu sfu=$REGION-docker.pkg.dev/$PROJECT_ID/sfu/sfu:$NEW_VERSION -n sfu

# ロールアウト状況確認
kubectl rollout status deployment/sfu -n sfu

# ロールバック（問題発生時）
kubectl rollout undo deployment/sfu -n sfu
```

## 10. 監視・アラート

### 10.1 Cloud Monitoring アラートポリシー

| アラート名             | 条件                       | 重要度   | 通知先    |
| ---------------------- | -------------------------- | -------- | --------- |
| SFU High CPU           | CPU > 80% for 5min         | Warning  | Slack     |
| SFU Critical CPU       | CPU > 95% for 1min         | Critical | PagerDuty |
| SFU High Memory        | Memory > 80%               | Warning  | Slack     |
| SFU Pod Restart        | Restart count > 3 in 10min | Warning  | Slack     |
| SFU Error Rate         | Error rate > 1%            | Warning  | Slack     |
| SFU Connection Failure | Failure rate > 5%          | Critical | PagerDuty |
| Redis Down             | Instance unavailable       | Critical | PagerDuty |

### 10.2 Prometheus ServiceMonitor（オプション）

```yaml
# servicemonitor.yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: sfu
  namespace: sfu
  labels:
    app: sfu
spec:
  selector:
    matchLabels:
      app: sfu
  endpoints:
    - port: http # HTTPサーバーの /metrics
      interval: 15s
      path: /metrics
```

## 11. バックアップ・リストア

### 11.1 Redisバックアップ

```bash
# Memorystore RDB エクスポート
gcloud redis instances export gs://$PROJECT_ID-backups/redis/backup-$(date +%Y%m%d).rdb \
  --instance=sfu-redis \
  --region=$REGION

# インポート（リストア）
gcloud redis instances import gs://$PROJECT_ID-backups/redis/backup-20240101.rdb \
  --instance=sfu-redis \
  --region=$REGION
```

### 11.2 設定バックアップ

```bash
# Kubernetes設定のバックアップ
kubectl get configmap sfu-config -n sfu -o yaml > backup/configmap.yaml
kubectl get secret sfu-secrets -n sfu -o yaml > backup/secrets.yaml
```

## 12. トラブルシューティング

### 12.1 よくある問題と対処

| 問題            | 原因                 | 対処                                 |
| --------------- | -------------------- | ------------------------------------ |
| WebRTC接続失敗  | UDP通信がブロック    | Firewall/NAT設定確認、TURN使用       |
| Redis接続エラー | ネットワーク到達不可 | VPC設定、Firewall確認                |
| Pod起動失敗     | リソース不足         | ノード追加、リソース制限見直し       |
| 高レイテンシ    | リージョン不一致     | クライアント近接リージョンにデプロイ |
| メモリ不足      | 大規模ルーム         | HPA設定、ノードスペック見直し        |

### 12.2 デバッグコマンド

```bash
# Pod状態確認
kubectl describe pod -l app=sfu -n sfu

# Podログ確認
kubectl logs -l app=sfu -n sfu --tail=100 -f

# Pod内でシェル実行
kubectl exec -it deployment/sfu -n sfu -- /bin/sh

# Redis接続テスト
kubectl run redis-cli --rm -it --image=redis:7-alpine -- redis-cli -h $REDIS_HOST ping

# ネットワーク到達確認
kubectl run curl --rm -it --image=curlimages/curl -- curl -v http://sfu.sfu.svc.cluster.local/health
```

### 12.3 ログ検索（Cloud Logging）

```bash
# エラーログ検索
gcloud logging read 'resource.type="k8s_container" AND resource.labels.namespace_name="sfu" AND severity>=ERROR' \
  --limit=50 \
  --format="table(timestamp,jsonPayload.message)"

# 特定ルームのログ検索
gcloud logging read 'resource.type="k8s_container" AND jsonPayload.room_id="room-123"' \
  --limit=100
```

## 13. セキュリティチェックリスト

### 13.1 デプロイ前チェック

- [ ] JWT公開鍵が正しく設定されている
- [ ] Redis接続にパスワードが設定されている（本番）
- [ ] TLS証明書が有効である
- [ ] Firewall設定がCloudflare IPのみ許可している
- [ ] ServiceAccountに最小権限が付与されている
- [ ] Podがnon-rootユーザーで実行される
- [ ] Secret ManagerからSecretを取得している（本番）

### 13.2 定期チェック

- [ ] 証明書の有効期限（90日前に更新）
- [ ] JWTキーのローテーション（90日ごと）
- [ ] 依存パッケージの脆弱性スキャン
- [ ] CloudflareのIP範囲更新確認
- [ ] GKEノードのセキュリティパッチ適用
