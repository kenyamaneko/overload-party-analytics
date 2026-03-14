# BigQuery スキーマ

Overload Party Analytics - BigQuery テーブル定義

## Dataset

**Dataset ID:** `analytics`
**Location:** `asia-northeast1`
**Default table expiration:** なし (永続)

---

## Tables

### 1. games

ゲームのメタデータと結果。

| Column | Type | Mode | Description |
|--------|------|------|-------------|
| game_id | STRING | REQUIRED | ゲーム ID (UUID) |
| player1_id | STRING | REQUIRED | Player 1 ID |
| player2_id | STRING | REQUIRED | Player 2 ID |
| player1_deck_snapshot | JSON | NULLABLE | Player 1 のデッキスナップショット |
| player2_deck_snapshot | JSON | NULLABLE | Player 2 のデッキスナップショット |
| status | STRING | REQUIRED | ゲーム状態 (waiting, selecting, playing, finished) |
| winner_id | STRING | NULLABLE | 勝者の player_id |
| created_at | TIMESTAMP | REQUIRED | ゲーム作成日時 |
| updated_at | TIMESTAMP | REQUIRED | 最終更新日時 |
| finished_at | TIMESTAMP | NULLABLE | ゲーム終了日時 |

**Partitioning:** created_at (DAY)
**Clustering:** status, winner_id

**分析用途:**
- 勝率分析 (win rate by player, deck, faction)
- ゲーム完了率 (completion rate)
- 平均ゲーム時間 (avg duration = finished_at - created_at)

---

### 2. game_events

ゲーム内のすべてのアクション・イベントログ。

| Column | Type | Mode | Description |
|--------|------|------|-------------|
| game_id | STRING | REQUIRED | ゲーム ID |
| sequence_number | INT64 | REQUIRED | イベントのシーケンス番号 |
| event_type | STRING | REQUIRED | イベントタイプ (play_card, attack, scale_up等) |
| player_id | STRING | NULLABLE | アクションを実行したプレイヤー ID |
| event_data | JSON | REQUIRED | イベント詳細データ |
| created_at | TIMESTAMP | REQUIRED | イベント発生日時 |

**Partitioning:** created_at (DAY)
**Clustering:** game_id, event_type

**分析用途:**
- カード使用率 (card play frequency)
- アクション頻度分析 (action distribution)
- ターンごとの行動パターン
- カードバランス調整データ

**主要 event_type:**
- `play_card` - カードプレイ
- `attack` - 攻撃
- `scale_up` - スケールアップ
- `distribute_dv` - DV分配
- `end_phase` - フェーズ終了
- `game_over` - ゲーム終了

---

### 3. players

プレイヤーのプロフィールデータ。

| Column | Type | Mode | Description |
|--------|------|------|-------------|
| player_id | STRING | REQUIRED | プレイヤー ID |
| username | STRING | REQUIRED | ユーザー名 |
| level | INT64 | NULLABLE | レベル |
| exp | INT64 | NULLABLE | 経験値 |
| wins | INT64 | NULLABLE | 勝利数 |
| losses | INT64 | NULLABLE | 敗北数 |
| is_premium | BOOL | REQUIRED | プレミアム会員フラグ |
| selected_faction | STRING | NULLABLE | 選択中の陣営 |
| created_at | TIMESTAMP | REQUIRED | アカウント作成日時 |
| updated_at | TIMESTAMP | REQUIRED | 最終更新日時 |

**Partitioning:** created_at (DAY)
**Clustering:** is_premium, selected_faction

**分析用途:**
- DAU (Daily Active Users) - `COUNT(DISTINCT player_id) WHERE DATE(updated_at) = @date`
- 課金ユーザー割合 - `SUM(is_premium) / COUNT(*)`
- 勝率分析 - `wins / (wins + losses)`
- ユーザー定着率 (retention)

---

### 4. matches

マッチメイキングの記録。

| Column | Type | Mode | Description |
|--------|------|------|-------------|
| match_id | STRING | REQUIRED | マッチ ID |
| game_id | STRING | REQUIRED | ゲーム ID |
| created_at | TIMESTAMP | REQUIRED | マッチ作成日時 |

**Partitioning:** created_at (DAY)
**Clustering:** game_id

**分析用途:**
- マッチ頻度分析
- ゲームとの紐付け

---

### 5. subscriptions

サブスクリプション (継続課金) データ。

| Column | Type | Mode | Description |
|--------|------|------|-------------|
| player_id | STRING | REQUIRED | プレイヤー ID |
| subscription_id | STRING | REQUIRED | サブスクリプション ID |
| product_id | STRING | REQUIRED | 商品 ID |
| platform | STRING | REQUIRED | プラットフォーム (ios, android) |
| purchase_token | STRING | REQUIRED | 購入トークン |
| status | STRING | REQUIRED | ステータス (active, canceled, expired) |
| current_period_start | TIMESTAMP | REQUIRED | 現在の期間開始日時 |
| current_period_end | TIMESTAMP | REQUIRED | 現在の期間終了日時 |
| created_at | TIMESTAMP | REQUIRED | サブスクリプション作成日時 |
| updated_at | TIMESTAMP | REQUIRED | 最終更新日時 |

**Partitioning:** created_at (DAY)
**Clustering:** player_id, status

**分析用途:**
- MRR (Monthly Recurring Revenue)
- チャーン率 (Churn Rate)
- LTV (Lifetime Value)
- サブスクリプション継続率

---

### 6. purchases

ワンタイム課金データ。

| Column | Type | Mode | Description |
|--------|------|------|-------------|
| player_id | STRING | REQUIRED | プレイヤー ID |
| purchase_id | STRING | REQUIRED | 購入 ID |
| product_id | STRING | REQUIRED | 商品 ID |
| platform | STRING | REQUIRED | プラットフォーム (ios, android) |
| purchase_token | STRING | REQUIRED | 購入トークン |
| purchased_at | TIMESTAMP | REQUIRED | 購入日時 |

**Partitioning:** purchased_at (DAY)
**Clustering:** player_id, product_id

**分析用途:**
- ARPU (Average Revenue Per User)
- 購入頻度分析
- 商品別売上
- コンバージョン率

---

## コスト最適化

### パーティショニング
すべてのテーブルは timestamp カラムで日次パーティション化されています。

**例: 過去7日間のクエリ**
```sql
SELECT * FROM analytics.game_events
WHERE created_at >= TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 7 DAY)
```
→ 7日分のパーティションのみスキャン (コスト 95% 削減)

### クラスタリング
頻繁にフィルタする列でクラスタ化。

**例: 特定プレイヤーのイベント**
```sql
SELECT * FROM analytics.game_events
WHERE game_id = @game_id
  AND created_at >= @start_date
```
→ game_id クラスタにより高速化

---

## データ更新頻度

| Table | 更新頻度 | 遅延 |
|-------|---------|------|
| game_events | 1時間ごと | < 1時間 |
| games | 1日1回 | < 24時間 |
| players | 1日1回 | < 24時間 |
| matches | 1日1回 | < 24時間 |
| subscriptions | 1日1回 | < 24時間 |
| purchases | 1日1回 | < 24時間 |
