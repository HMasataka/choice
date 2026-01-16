# Choice SFU Web Client

ブラウザで動作するビデオ会議クライアント。

## 機能

- WebRTC経由のビデオ/オーディオ通話
- Simulcastによる品質適応
- 画面共有
- 参加者一覧表示
- カメラ/マイクのミュート切り替え

## 開発

```bash
# 依存関係のインストール
npm install

# 開発サーバー起動 (http://localhost:3000)
npm run dev

# ビルド
npm run build

# プレビュー
npm run preview
```

## 構成

```
web/
├── src/
│   ├── App.tsx              # メインアプリケーション
│   ├── main.tsx             # エントリーポイント
│   ├── components/          # UIコンポーネント
│   │   ├── JoinScreen.tsx   # 参加画面
│   │   ├── Room.tsx         # ルーム画面
│   │   ├── VideoTile.tsx    # ビデオタイル
│   │   └── Icons.tsx        # アイコン
│   ├── hooks/               # React Hooks
│   │   ├── useSFUClient.ts  # SFUクライアント管理
│   │   ├── useLocalMedia.ts # ローカルメディア管理
│   │   └── useScreenShare.ts# 画面共有管理
│   ├── sfu/                 # SFUクライアント実装
│   │   ├── SFUClient.ts     # WebSocket + WebRTC
│   │   └── types.ts         # 型定義
│   └── styles/
│       └── index.css        # スタイル
├── index.html
├── package.json
├── tsconfig.json
└── vite.config.ts
```

## SFUサーバーとの接続

開発時はViteのプロキシ設定により、`/ws` へのWebSocket接続がSFUサーバー (localhost:8080) に転送されます。

本番環境では適切なリバースプロキシ設定が必要です。

## ブラウザ対応

- Chrome 90+
- Firefox 85+
- Safari 14+
- Edge 90+
