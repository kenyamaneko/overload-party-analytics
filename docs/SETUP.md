# セットアップ

## 前提条件

- gcloud CLI (認証済み)
- Terraform v1.5+
- Go 1.22+
- Cloud SQL PostgreSQL への read-only アクセス
- Cloud SQL Auth Proxy（ローカル開発時）

## 環境変数

他のサービスと同じ変数名・同じ接続方式（IAM データベース認証）を使う。

| 変数名 | 説明 | 例 |
|--------|------|-----|
| `DATABASE_CONN` | PostgreSQL 接続文字列（libpq キーワード形式） | `user=export-function-dev@overload-party-dev.iam dbname=overload_party sslmode=disable` |
| `DATABASE_IAM_AUTH_ENABLED` | IAM データベース認証の使用可否（`true` / `false`） | `true` |
| `CLOUDSQL_CONNECTION_NAME` | Cloud SQL インスタンス接続名（IAM 認証時のみ必須） | `overload-party-dev:asia-northeast1:overload-party-db` |
| `BQ_PROJECT_ID` | BigQuery プロジェクト ID | `overload-party-dev` |
| `BQ_DATASET_ID` | BigQuery データセット ID | `analytics` |
| `GCS_BUCKET` | ステージング用 GCS バケット | `overload-party-dev-bq-staging` |

## 初回デプロイ

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

# ローカル起動に必須。どちらか一方でも未設定なら起動しない
# FUNCTION_TARGET はデプロイ時と同じくエントリポイントを "/" で受けるために要る
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
