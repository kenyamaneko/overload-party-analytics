# Overload Party Analytics

Cloud SQL PostgreSQL → BigQuery データエクスポート基盤

[テスト観点カタログ](https://kenyamaneko.github.io/overload-party-analytics/): テスト名から生成した、テスト済みの観点の一覧。

## 概要

ゲームサーバーの Cloud SQL PostgreSQL データベースから BigQuery にデータをエクスポートし、ゲームバランス分析・ダッシュボード構築を行うための基盤。

カード使用率・勝率・課金メトリクスの分析基盤。Cloud SQL から増分エクスポート（Firestore チェックポイント）し、Cloud Scheduler で自動実行。

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
│   ├── cmd/local/         # ローカル起動用エントリポイント
│   ├── handler/           # HTTP リクエスト処理
│   └── exporter/          # エクスポートロジック
│       ├── model/         # 設定・エクスポート結果モデル
│       ├── service/       # エクスポート処理本体
│       └── adapter/       # PostgreSQL / GCS / BigQuery / Firestore 接続
├── terraform/              # インフラ定義
│   ├── modules/
│   │   ├── bigquery/
│   │   ├── export-function/
│   │   └── scheduler/
│   └── environments/
│       └── dev/
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

他のサービスと同じ変数名・同じ接続方式（IAM データベース認証）を使う。

| 変数名 | 説明 | 例 |
|--------|------|-----|
| `DATABASE_CONN` | PostgreSQL 接続文字列（libpq キーワード形式） | `user=export-function-dev@overload-party-dev.iam dbname=overload_party sslmode=disable` |
| `DATABASE_IAM_AUTH_ENABLED` | IAM データベース認証の使用可否（`true` / `false`） | `true` |
| `CLOUDSQL_CONNECTION_NAME` | Cloud SQL インスタンス接続名（IAM 認証時のみ必須） | `overload-party-dev:asia-northeast1:overload-party-db` |
| `BQ_PROJECT_ID` | BigQuery プロジェクト ID | `overload-party-dev` |
| `BQ_DATASET_ID` | BigQuery データセット ID | `analytics` |
| `GCS_BUCKET` | ステージング用 GCS バケット | `overload-party-dev-bq-staging` |

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

# Cloud SQL Auth Proxy 起動（別ターミナル）。proxy が IAM 認証を代行する
cloud-sql-proxy overload-party-dev:asia-northeast1:overload-party-db --port=5432 --auto-iam-authn

# 環境変数設定。IAM 認証を proxy に任せるため DATABASE_IAM_AUTH_ENABLED は false
export DATABASE_CONN="host=localhost port=5432 dbname=overload_party user=<自分の Google アカウント> sslmode=disable"
export DATABASE_IAM_AUTH_ENABLED="false"
export BQ_PROJECT_ID="overload-party-dev"
export BQ_DATASET_ID="analytics"
export GCS_BUCKET="overload-party-dev-bq-staging"

# ローカル起動用。デプロイ時と同じくエントリポイントを "/" で受けるため FUNCTION_TARGET を渡す
export PORT="8080"
export FUNCTION_TARGET="ExportPostgresToBigQuery"

# ローカル実行
go run ./cmd/local

# 動作確認 (別ターミナル)
curl -X POST http://localhost:8080 \
  -H "Content-Type: application/json" \
  -d '{"tables": ["games"], "mode": "incremental"}'
```

### 結合テスト

各サービスリポジトリの `db/schema.sql` と card のマスタデータを PostgreSQL コンテナに投入し、
エクスポート対象 8 テーブルを実際に読んで JSONL に変換できることを確かめる。Docker が要る。

```bash
cd export-function
go test -tags=integration ./...
```

各サービスリポジトリが本リポジトリと同じ階層に無い場合は、それらが並ぶディレクトリを
`OVERLOAD_PARTY_WORKSPACE_DIR` で指定する。

## エクスポート対象テーブル

| テーブル | 取得元 | 用途 | 更新頻度 |
|---------|--------|------|---------|
| games | `battle.games` | ゲーム結果、勝敗理由の分布 | Daily |
| game_events | `battle.game_events` | カード使用率、行動分析 | Hourly |
| game_players | `gateway.game_players` | プレイヤーとゲームスロットの対応（勝率分析の起点） | Daily |
| players | `account.players` | DAU、オンボーディング進行、課金状態 | Daily |
| card_definitions | `card.card_definitions` | カードマスタ（メタ分析の JOIN 用） | Daily |
| deck_cards | `card.deck_cards` | デッキ構成分析、カード採用率 | Daily |
| subscriptions | `shop.subscriptions` | MRR、チャーン率 | Daily |
| purchases | `shop.one_time_purchases` | ARPU、購入頻度 | Daily |

append-only テーブルの最新状態は `*_latest` VIEW で取得する。

`battle.games` はスロット番号 (1/2) だけを持ちプレイヤー ID を知らないため、プレイヤー単位の集計は
`game_players` を JOIN して行う。認証プロバイダの識別子 (`firebase_uid`) と課金トークンは
分析に不要なためエクスポートしない。プレイヤーの同一性は `player_id` が担う。

## BigQuery 分析例

```sql
-- プレイヤー別勝率
SELECT
  gp.player_id,
  COUNT(*) AS total_games,
  COUNTIF(g.winning_player_num = gp.player_num) AS wins,
  ROUND(COUNTIF(g.winning_player_num = gp.player_num) / COUNT(*) * 100, 2) AS win_rate
FROM `overload-party-dev.analytics.game_players` gp
JOIN `overload-party-dev.analytics.games_latest` g USING (game_id)
WHERE g.status = 'finished'
  AND g.created_at >= TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 7 DAY)
GROUP BY gp.player_id
ORDER BY win_rate DESC;

-- カード使用率 (トップ 20)
SELECT
  JSON_EXTRACT_SCALAR(event_data, '$.cardId') AS card_id,
  COUNT(*) AS play_count,
  COUNT(DISTINCT game_id) AS games_used_in
FROM `overload-party-dev.analytics.game_events`
WHERE event_type = 'play_card'
  AND created_at >= TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 7 DAY)
GROUP BY card_id
ORDER BY play_count DESC
LIMIT 20;

-- 決着理由の分布
SELECT
  win_reason,
  COUNT(*) AS count
FROM `overload-party-dev.analytics.games_latest`
WHERE status = 'finished'
  AND created_at >= TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 30 DAY)
GROUP BY win_reason
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
gsutil ls gs://overload-party-dev-bq-staging/exports/
```

