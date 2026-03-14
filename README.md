# Overload Party Analytics

Cloud SQL PostgreSQL → BigQuery データエクスポート基盤

## 概要

ゲームサーバーの Cloud SQL PostgreSQL データベースから BigQuery にデータをエクスポートし、ゲームバランス分析・ダッシュボード構築を行うための基盤。

**目的:**
- カード使用率・勝率の分析によるゲームバランス調整
- ファクション別・カード別の統計分析
- 勝利条件の分布分析
- 課金・収益メトリクスの追跡

**特徴:**
- 完全独立: メインゲームサーバーのコードに一切依存しない
- 低コスト: Cloud SQL の読み取りは最小限（増分エクスポート）
- 増分更新: Firestore チェックポイントで差分のみエクスポート
- 自動実行: Cloud Scheduler で定期実行

## アーキテクチャ

```
Cloud Scheduler (hourly/daily)
  ↓
Cloud Function (export-postgres-to-bigquery)
  ├─ Firestore (checkpoint management)
  ├─ Cloud SQL PostgreSQL (read-only, incremental query)
  ├─ Cloud Storage (JSONL staging)
  └─ BigQuery (batch load)
  ↓
Looker Studio Dashboard
```

## ディレクトリ構成

```
overload-party-analytics/
├── export-function/        # Cloud Function コード
│   ├── go.mod
│   ├── main.go
│   ├── config.yaml        # テーブル定義・クエリ
│   └── exporter/          # エクスポートロジック
│       ├── config.go      # 設定ロード
│       ├── exporter.go    # メインエクスポート処理
│       ├── postgres.go    # PostgreSQL クエリ実行
│       ├── bigquery.go    # BigQuery ロード
│       ├── gcs.go         # GCS ステージング
│       └── checkpoint.go  # Firestore チェックポイント
├── terraform/              # インフラ定義
│   ├── modules/
│   │   ├── bigquery/
│   │   ├── export-function/
│   │   └── scheduler/
│   └── environments/
│       ├── dev/
│       └── prod/
├── scripts/
│   └── backfill.sh        # 履歴データバックフィル
└── docs/
    ├── schema.md          # BigQuery スキーマ
    └── queries.md         # サンプルクエリ集
```

## セットアップ

### 前提条件

- gcloud CLI (認証済み)
- Terraform v1.5+
- Go 1.22+
- Cloud SQL PostgreSQL への read-only アクセス
- Cloud SQL Auth Proxy（ローカル開発時）

### 環境変数

| 変数名 | 説明 | 例 |
|--------|------|-----|
| `INSTANCE_CONNECTION_NAME` | Cloud SQL インスタンス接続名（Cloud Functions 用） | `overload-party-dev:asia-northeast1:overload-party-db` |
| `DB_HOST` | PostgreSQL ホスト（ローカル開発用） | `localhost` |
| `DB_USER` | PostgreSQL ユーザー | `analytics-reader` |
| `DB_PASSWORD` | PostgreSQL パスワード | (Secret Manager 経由) |
| `DB_NAME` | データベース名 | `overload_party` |
| `BQ_PROJECT_ID` | BigQuery プロジェクト ID | `overload-party-dev` |
| `BQ_DATASET_ID` | BigQuery データセット ID | `analytics` |
| `GCS_BUCKET` | ステージング用 GCS バケット | `op-bq-staging-dev` |

### 初回デプロイ

```bash
# 1. Terraform でインフラ構築
cd terraform/environments/dev
terraform init
terraform apply

# 2. Cloud Function デプロイ (main ブランチへの push で CI が自動デプロイ)
git push origin main

# 3. 履歴データのバックフィル (オプション)
./scripts/backfill.sh dev games 2024-01-01
```

## ローカル開発

```bash
cd export-function

# Cloud SQL Auth Proxy 起動（別ターミナル）
cloud-sql-proxy overload-party-dev:asia-northeast1:overload-party-db --port=5432

# 環境変数設定
export DB_HOST="localhost"
export DB_USER="analytics-reader"
export DB_PASSWORD="..."
export DB_NAME="overload_party"
export BQ_PROJECT_ID="overload-party-dev"
export BQ_DATASET_ID="analytics"
export GCS_BUCKET="op-bq-staging-dev"

# ローカル実行
go run main.go

# テスト (別ターミナル)
curl -X POST http://localhost:8080 \
  -H "Content-Type: application/json" \
  -d '{"tables": ["games"], "mode": "incremental"}'
```

## エクスポート対象テーブル

| テーブル | 用途 | 更新頻度 |
|---------|------|---------|
| games | ゲーム結果、勝率分析、勝利条件分布 | Daily |
| game_events | カード使用率、行動分析 | Hourly |
| players | DAU、ファクション選択傾向 | Daily |
| card_definitions | カードマスタ（メタ分析の JOIN 用） | Daily |
| deck_cards | デッキ構成分析、カード採用率 | Daily |
| matches | ゲーム履歴 | Daily |
| subscriptions | MRR、チャーン率 | Daily |
| purchases | ARPU、購入頻度 (source: one_time_purchases) | Daily |

更新があるテーブル (games, players, subscriptions, card_definitions) は append-only で蓄積されます。最新状態は `*_latest` VIEW (`games_latest`, `players_latest`, `subscriptions_latest`, `card_definitions_latest`) で取得してください。

## BigQuery 分析例

```sql
-- ファクション別勝率
SELECT
  JSON_EXTRACT_SCALAR(player1_deck_snapshot, '$.faction') AS faction,
  COUNT(*) AS total_games,
  COUNTIF(winner_id = player1_id) AS wins,
  ROUND(COUNTIF(winner_id = player1_id) / COUNT(*) * 100, 2) AS win_rate
FROM `overload-party-dev.analytics.games_latest`
WHERE status = 'finished'
  AND created_at >= TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 7 DAY)
GROUP BY faction
ORDER BY win_rate DESC;

-- カード使用率 (トップ 20)
SELECT
  JSON_EXTRACT_SCALAR(event_data, '$.cardNo') AS card_no,
  COUNT(*) AS play_count,
  COUNT(DISTINCT game_id) AS games_used_in
FROM `overload-party-dev.analytics.game_events`
WHERE event_type = 'play_card'
  AND created_at >= TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 7 DAY)
GROUP BY card_no
ORDER BY play_count DESC
LIMIT 20;

-- 勝利条件の分布
SELECT
  JSON_EXTRACT_SCALAR(
    (SELECT event_data FROM `overload-party-dev.analytics.game_events` ge
     WHERE ge.game_id = g.game_id AND ge.event_type = 'game_end'
     LIMIT 1),
    '$.winCondition'
  ) AS win_condition,
  COUNT(*) AS count
FROM `overload-party-dev.analytics.games_latest` g
WHERE status = 'finished'
  AND created_at >= TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 30 DAY)
GROUP BY win_condition
ORDER BY count DESC;
```

詳細なクエリは [docs/queries.md](docs/queries.md) を参照。

## コスト見積もり

### 1K DAU の場合

| 項目 | 月額 |
|------|------|
| BigQuery ストレージ | $0.20 |
| BigQuery クエリ | $0 (無料枠内) |
| Cloud Functions | $0 (無料枠内) |
| Cloud Scheduler | $0.10 |
| Cloud SQL 読み取り | 既存インスタンスに含む |
| Cloud Storage | $0.07 |
| **合計** | **$0.37/月** |

### 10K DAU の場合: ~$2.50/月

## トラブルシューティング

### エクスポートが失敗する

```bash
# ログ確認
gcloud functions logs read export-postgres-to-bigquery-dev \
  --region asia-northeast1 \
  --limit 50

# チェックポイント確認
gcloud firestore export gs://backup-bucket \
  --collection-ids=export_checkpoints
```

### BigQuery にデータが反映されない

```bash
# BigQuery load job 確認
bq ls -j -a -n 100 overload-party-dev

# GCS ファイル確認
gsutil ls gs://op-bq-staging-dev/exports/
```

## ライセンス

Proprietary
