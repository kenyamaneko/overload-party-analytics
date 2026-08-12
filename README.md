# overload-party-analytics

カードゲーム Overload Party の Cloud SQL PostgreSQL → BigQuery データエクスポート基盤を担うリポジトリ。カード使用率・勝率・課金メトリクスの分析基盤として使う。Cloud SQL から増分エクスポート（Firestore チェックポイント）し、Cloud Scheduler で自動実行する。

## 技術スタック

| レイヤー | 技術 |
|---|---|
| 言語 | Go |
| 実行基盤 | Cloud Scheduler, Cloud Function |
| データベース (取得元) | Cloud SQL PostgreSQL |
| 出力先 | BigQuery |
| ステージング | Cloud Storage |
| チェックポイント管理 | Firestore |

## ドキュメント

| ドキュメント | 内容 |
|---|---|
| [セットアップ](docs/SETUP.md) | 前提条件・環境変数・デプロイ手順・ローカル開発・トラブルシューティング |
| [BigQuery スキーマ](docs/schema.md) | テーブル定義・エクスポート元・更新頻度 |
| [サンプルクエリ集](docs/queries.md) | よく使う分析クエリ |
| [ADR](https://github.com/kenyamaneko/overload-party-common/tree/main/docs/adr)（commonリポジトリ） | 設計判断の背景・理由・結果 |
| [システム構成図](https://github.com/kenyamaneko/overload-party-common#システム構成図)（commonリポジトリ） | Overload Party 全体の構成図 |
| [テスト観点カタログ](https://kenyamaneko.github.io/overload-party-analytics/) | テスト名から自動生成した、テスト済みの観点一覧 |
