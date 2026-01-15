# Architecture Decision Records (ADR)

このディレクトリには、WebRTC SFU プロジェクトの重要なアーキテクチャ決定を記録したドキュメントが含まれます。

## ADRとは

Architecture Decision Records (ADR) は、ソフトウェアアーキテクチャに関する重要な決定を記録するための軽量なドキュメント形式です。各ADRは、特定の決定の背景、検討した選択肢、決定理由、および影響を記録します。

## ADR一覧

| ADR                                        | タイトル                            | ステータス | 日付       |
| ------------------------------------------ | ----------------------------------- | ---------- | ---------- |
| [ADR-0001](0001-sfu-architecture.md)       | SFUアーキテクチャの採用             | Accepted   | 2025-01-15 |
| [ADR-0002](0002-signaling-protocol.md)     | JSON-RPC 2.0 シグナリングプロトコル | Accepted   | 2025-01-15 |
| [ADR-0003](0003-simulcast-strategy.md)     | Simulcast実装戦略                   | Accepted   | 2025-01-15 |
| [ADR-0004](0004-jwt-authentication.md)     | JWT認証方式                         | Accepted   | 2025-01-15 |
| [ADR-0005](0005-session-store.md)          | Redisセッションストア               | Accepted   | 2025-01-15 |
| [ADR-0006](0006-recording-architecture.md) | 録画アーキテクチャ                  | Accepted   | 2025-01-15 |

## ステータス定義

- **Proposed**: 提案中、レビュー待ち
- **Accepted**: 承認済み、実装予定または実装中
- **Deprecated**: 非推奨、新しいADRに置き換え
- **Superseded**: 別のADRに置き換え済み

## ADRテンプレート

新しいADRを作成する場合は、[テンプレート](template.md)を使用してください。

## 参考

- [ADR GitHub Organization](https://adr.github.io/)
- [Michael Nygard's ADR article](https://cognitect.com/blog/2011/11/15/documenting-architecture-decisions)
