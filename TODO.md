# TODO: Overload Party Analytics

## セットアップ手順 (初回デプロイ)

### ✅ 完了済み
- [x] Terraform モジュール作成
- [x] Cloud Function コード実装
- [x] デプロイスクリプト作成
- [x] ドキュメント作成

### 📋 次のステップ

#### 1. Git リポジトリ初期化
```bash
cd /Users/kenyamamoto/Documents/key_and_notes/overload-party-analytics
git init
git add .
git commit -m "Initial commit: Analytics infrastructure

- Terraform modules (BigQuery, Cloud Functions, Scheduler)
- Export function (PostgreSQL → BigQuery)
- Deployment scripts
- Documentation"
```

オプション: GitHub にプッシュ
```bash
gh repo create overload-party-analytics --private
git remote add origin https://github.com/YOUR_ORG/overload-party-analytics.git
git push -u origin main
```

---

#### 2. Terraform State バケット作成
```bash
gsutil mb -p overload-party-dev -l asia-northeast1 gs://overload-party-analytics-terraform-state
gsutil versioning set on gs://overload-party-analytics-terraform-state
```

---

#### 3. Terraform インフラデプロイ
```bash
cd terraform/environments/dev
terraform init
terraform plan
terraform apply
```

確認項目:
- [ ] BigQuery dataset `analytics` が作成された
- [ ] テーブル (games, game_events, players, matches, subscriptions, purchases) が作成された
- [ ] GCS バケット (function-source, bq-staging) が作成された
- [ ] Service Account が作成された
- [ ] IAM 権限が付与された

---

#### 4. Cloud Function デプロイ
```bash
cd /Users/kenyamamoto/Documents/key_and_notes/overload-party-analytics
./scripts/deploy.sh dev
```

出力された `source_archive` 変数をメモ:
```bash
# 例: export-function-20260219-143052.zip
```

Terraform 変数を更新:
```bash
cd terraform/environments/dev
terraform apply -var="source_archive=export-function-YYYYMMDD-HHMMSS.zip"
```

---

#### 5. 動作確認

##### 5.1 手動トリガーでテスト
```bash
gcloud functions call export-postgres-to-bigquery-dev \
  --region asia-northeast1 \
  --data '{"tables": ["games"], "mode": "incremental"}'
```

##### 5.2 ログ確認
```bash
gcloud functions logs read export-postgres-to-bigquery-dev \
  --region asia-northeast1 \
  --limit 50
```

##### 5.3 BigQuery データ確認
```bash
bq query --use_legacy_sql=false \
  'SELECT COUNT(*) FROM `overload-party-dev.analytics.games`'

bq query --use_legacy_sql=false \
  'SELECT * FROM `overload-party-dev.analytics.games` LIMIT 10'
```

---

#### 6. 履歴データのバックフィル (オプション)
```bash
# 特定テーブルのみ
./scripts/backfill.sh dev games 2024-01-01

# すべてのテーブル
./scripts/backfill.sh dev all 2024-01-01
```

---

#### 7. Looker Studio ダッシュボード作成

1. https://lookerstudio.google.com/ にアクセス
2. 「作成」→「レポート」
3. データソース追加:
   - 「BigQuery」を選択
   - プロジェクト: `overload-party-dev`
   - データセット: `analytics`
   - テーブル: `games`, `game_events`, `players` 等
4. 主要指標を追加:
   - DAU (Daily Active Users)
   - カード使用率
   - 課金ユーザー割合
   - 1日平均バトル数
   - 売上推移

参考: `docs/queries.md` のサンプルクエリを活用

---

#### 8. モニタリング設定

##### 8.1 Cloud Monitoring アラート作成
```bash
# Function エラーアラート
gcloud alpha monitoring policies create \
  --notification-channels=CHANNEL_ID \
  --display-name="Export Function Failures" \
  --condition-display-name="Function errors > 2" \
  --condition-threshold-value=2 \
  --condition-threshold-duration=300s
```

##### 8.2 定期的な確認項目
- [ ] Cloud Scheduler ジョブが正常に実行されているか
- [ ] BigQuery テーブルのデータが更新されているか
- [ ] GCS staging バケットのファイルが自動削除されているか (7日後)
- [ ] Firestore checkpoint が更新されているか

---

## 今後の拡張

### Phase 2: 追加テーブルのエクスポート
- [ ] `card_definitions` テーブル追加
- [ ] `decks` / `deck_cards` テーブル追加 (集約版)
- [ ] `player_cards` テーブル追加

### Phase 3: 分析の高度化
- [ ] BigQuery View 作成 (事前集計済みビュー)
- [ ] マテリアライズドビュー検討 (コスト vs 速度)
- [ ] カスタムメトリクスの Cloud Monitoring 連携

### Phase 4: 本番環境デプロイ
- [ ] `terraform/environments/prod/` 作成
- [ ] 本番用 PostgreSQL DB への接続設定
- [ ] 本番用アラート設定

---

## トラブルシューティング

### エクスポートが失敗する場合
```bash
# ログ確認
gcloud functions logs read export-postgres-to-bigquery-dev \
  --region asia-northeast1 \
  --limit 100

# Firestore checkpoint 確認
gcloud firestore export gs://backup-temp-bucket \
  --collection-ids=export_checkpoints
```

### BigQuery にデータが反映されない場合
```bash
# BigQuery load job 確認
bq ls -j -a -n 100 overload-party-dev

# GCS ファイル確認
gsutil ls -r gs://overload-party-dev-bq-staging/exports/
```

### IAM 権限エラーの場合
```bash
# Service Account の権限確認
gcloud projects get-iam-policy overload-party-dev \
  --flatten="bindings[].members" \
  --filter="bindings.members:export-function-dev"
```

---

## コスト管理

### 月次コスト確認
```bash
# BigQuery コスト
bq show --format=prettyjson overload-party-dev:analytics

# Cloud Functions コスト
gcloud functions describe export-postgres-to-bigquery-dev \
  --region asia-northeast1 \
  --format="value(serviceConfig.availableMemory,serviceConfig.timeoutSeconds)"
```

### コスト最適化チェックリスト
- [ ] BigQuery テーブルのパーティショニング有効化確認
- [ ] クラスタリング設定確認
- [ ] GCS ライフサイクルポリシー (7日削除) 確認
- [ ] Cloud Functions のメモリ・タイムアウト設定最適化
- [ ] 不要なログの削減

---

## 関連リンク

- [README](README.md) - プロジェクト概要
- [docs/schema.md](docs/schema.md) - BigQuery スキーマ
- [docs/queries.md](docs/queries.md) - サンプルクエリ集
- [メインゲームサーバー](../overload-party)
