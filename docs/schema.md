# BigQuery スキーマ

Overload Party Analytics - BigQuery テーブル定義

> PostgreSQL 側のソーススキーマは各サービスリポジトリの `db/schema.sql` が SSoT。
> 本ドキュメントは BigQuery 側のテーブル定義を記載。

## Dataset

**Dataset ID:** `analytics`
**Location:** `asia-northeast1`
**Default table expiration:** なし (永続)

## エクスポートする列の方針

分析に使う列だけを取り込む。次の二種は取り込まない。

- **認証プロバイダの識別子** (`account.players.firebase_uid`): 分析に使わず、漏れたときの影響が大きい
- **課金プラットフォームのトークン** (`shop.apple_*_tokens` / `shop.google_*_tokens`): 外部決済の識別子で分析に使わない

プレイヤーの同一性は `account.players.player_id` (UUID、主キー) が担う。特定プレイヤーの対戦を
集計して勝率を出すといった分析はこの列だけで足りる。表示名 (`account.players.name`,
`card.decks.deck_name`) は利用者が自由に入力するため、分析に使わない値として取り込まない。

---

## Tables

### games (append-only)

対戦のメタデータと結果。ステータス遷移のたびに新しい行が追加されます。最新状態は `games_latest` VIEW を使用。

取得元: `battle.games`

| Column | Type | Mode | Description |
|--------|------|------|-------------|
| game_id | STRING | REQUIRED | ゲーム ID (ULID) |
| status | STRING | REQUIRED | ゲーム状態 (waiting, playing, finished) |
| first_player | INT64 | REQUIRED | 先攻プレイヤー番号 (1 or 2) |
| winning_player_num | INT64 | NULLABLE | 勝者のスロット番号 (NULL: 進行中, 0: 引分, 1 or 2: 勝者) |
| win_reason | STRING | NULLABLE | 決着理由 (budget_zero, turn_timeout 等) |
| engine_version | STRING | REQUIRED | ゲーム作成時のバトルエンジンバージョン |
| card_data_version | STRING | REQUIRED | ゲーム作成時のカードデータバージョン |
| created_at | TIMESTAMP | REQUIRED | ゲーム作成日時 |
| updated_at | TIMESTAMP | REQUIRED | 最終更新日時 |
| finished_at | TIMESTAMP | NULLABLE | ゲーム終了日時 |

**Partitioning:** updated_at (DAY)
**Clustering:** status, winning_player_num

**分析用途:**
- 決着理由の分布
- ゲーム完了率 (completion rate)
- 平均ゲーム時間 (avg duration = finished_at - created_at)
- エンジン・カードデータのバージョン別の傾向比較

battle はスロット番号だけを扱いプレイヤー ID を知らないため、プレイヤー単位の勝率は
`game_players` を JOIN して求めます。

---

### game_events

ゲーム内のすべてのアクション・イベントログ。

取得元: `battle.game_events`

| Column | Type | Mode | Description |
|--------|------|------|-------------|
| game_id | STRING | REQUIRED | ゲーム ID |
| sequence_number | INT64 | REQUIRED | イベントのシーケンス番号 |
| event_type | STRING | REQUIRED | イベントタイプ |
| player_num | INT64 | NULLABLE | イベントを起こしたスロット番号 (NULL: システムイベント) |
| event_data | JSON | REQUIRED | イベント詳細データ |
| created_at | TIMESTAMP | REQUIRED | イベント発生日時 |

**Partitioning:** created_at (DAY)
**Clustering:** game_id, event_type

**分析用途:**
- カード使用率 (card play frequency)
- アクション頻度分析 (action distribution)
- ターンごとの行動パターン
- カードバランス調整データ

---

### game_players

プレイヤーとゲームスロットの対応。プレイヤー単位の集計はこのテーブルを起点にします。

取得元: `gateway.game_players` (時刻列を持たないため `battle.games.updated_at` を JOIN して取得)

| Column | Type | Mode | Description |
|--------|------|------|-------------|
| game_id | STRING | REQUIRED | ゲーム ID |
| player_num | INT64 | REQUIRED | 人間が座っているスロット番号 (1 or 2) |
| player_id | STRING | REQUIRED | プレイヤー ID |
| updated_at | TIMESTAMP | REQUIRED | 対応する対戦の最終更新日時 |

**Partitioning:** updated_at (DAY)
**Clustering:** player_id, game_id

**分析用途:**
- プレイヤー別勝率 (`games.winning_player_num = game_players.player_num`)
- プレイヤーごとの対戦数
- 課金状態・オンボーディング進行と成績の突き合わせ

NPC 戦では NPC 側の行が存在しません (NPC はプレイヤー ID を持たないため)。

---

### players (append-only)

プレイヤーのプロフィールデータ。増分エクスポートにより `updated_at` が変わるたびに新しい行が追加されます。最新状態の取得には `players_latest` VIEW を使用してください。

取得元: `account.players`

| Column | Type | Mode | Description |
|--------|------|------|-------------|
| player_id | STRING | REQUIRED | プレイヤー ID |
| is_premium | BOOL | REQUIRED | プレミアム会員フラグ |
| equipped_icon_no | INT64 | NULLABLE | 装備中アイコン番号 |
| onboarding_status | STRING | REQUIRED | オンボーディング進行状態 (not_started, name_set, faction_set, completed) |
| premium_expires_at | TIMESTAMP | NULLABLE | プレミアム有効期限 |
| created_at | TIMESTAMP | REQUIRED | アカウント作成日時 |
| updated_at | TIMESTAMP | REQUIRED | 最終更新日時 |

**Partitioning:** updated_at (DAY)
**Clustering:** is_premium, onboarding_status

**分析用途:**
- DAU (Daily Active Users) - raw テーブルの `updated_at` から集計
- 課金ユーザー割合 - `players_latest` VIEW 経由
- オンボーディング離脱地点 (`onboarding_status` の分布)
- ユーザー定着率 (retention)

レベルと経験値は `account.player_progression` が持ちます (バトルごとの高頻度更新を
`players` から分離する設計のため)。現在はエクスポート対象外です。

---

### card_definitions (append-only)

カードマスタデータ。マスタ更新時に新しい行が追加されます。最新状態は `card_definitions_latest` VIEW を使用。

取得元: `card.card_definitions`

| Column | Type | Mode | Description |
|--------|------|------|-------------|
| card_id | STRING | REQUIRED | カード ID (例: SH-0001) |
| card_name | STRING | REQUIRED | カード名 |
| resource_label | STRING | REQUIRED | リソースラベル |
| faction | STRING | REQUIRED | 所属陣営 |
| card_type | STRING | REQUIRED | カード種別 (Resource / Support) |
| subtype | STRING | NULLABLE | サブタイプ (VM / Container / Database 等) |
| resizable | BOOL | REQUIRED | Resizable 属性 |
| elastic | BOOL | REQUIRED | Elastic 属性 |
| stats | JSON | REQUIRED | ステータス定義 |
| effect_text | STRING | NULLABLE | 効果テキスト |
| effects | JSON | NULLABLE | 効果定義 |
| restriction | STRING | REQUIRED | 制限区分 (unlimited / semi_limited / limited / forbidden) |
| is_active | BOOL | REQUIRED | 有効フラグ |
| created_at | TIMESTAMP | REQUIRED | 作成日時 |
| updated_at | TIMESTAMP | REQUIRED | 最終更新日時 |

**Partitioning:** updated_at (DAY)
**Clustering:** faction, card_type

**分析用途:**
- カードバランス分析 (JOIN 用マスタ)
- ファクション別カード構成
- メタゲーム分析

---

### deck_cards (append-only)

プレイヤーのデッキ構成データ。デッキを編集するたびに、その時点の構成全体が新しい世代として追加されます。最新状態は `deck_cards_latest` VIEW を使用。

取得元: `card.deck_cards` (デッキ属性と更新日時は `card.decks` を JOIN して取得)

| Column | Type | Mode | Description |
|--------|------|------|-------------|
| player_id | STRING | REQUIRED | プレイヤー ID |
| deck_id | INT64 | REQUIRED | デッキ ID |
| card_id | STRING | REQUIRED | カード ID (例: SH-0001) |
| art_no | INT64 | REQUIRED | アート番号 |
| count | INT64 | REQUIRED | 枚数 |
| faction | STRING | REQUIRED | デッキの宣言陣営 |
| created_at | TIMESTAMP | REQUIRED | デッキ作成日時 |
| updated_at | TIMESTAMP | REQUIRED | デッキ更新日時 |

**Partitioning:** updated_at (DAY)
**Clustering:** player_id, card_id

**分析用途:**
- カード採用率
- デッキ構成トレンド
- ファクション別人気カード

デッキ編集は `card.decks.updated_at` だけを動かすため、増分の基準にこの列を使います。

対戦で実際に使われたデッキは `battle.game_decks` が持ちますが、現在はエクスポート対象外です。
そのため「対戦のファクション別勝率」はこのテーブルからは求まりません
(1 プレイヤーが複数ファクションのデッキを持てるため、`player_id` だけで結ぶと重複します)。

---

### subscriptions (append-only)

サブスクリプション (継続課金) データ。ステータス変更時に新しい行が追加されます。最新状態は `subscriptions_latest` VIEW を使用。

取得元: `shop.subscriptions`

| Column | Type | Mode | Description |
|--------|------|------|-------------|
| subscription_id | INT64 | REQUIRED | サブスクリプション ID |
| player_id | STRING | REQUIRED | プレイヤー ID |
| product_id | STRING | REQUIRED | 商品 ID |
| status | STRING | REQUIRED | ステータス (active / cancelled / grace_period / expired / revoked) |
| current_period_start | TIMESTAMP | REQUIRED | 現在の期間開始日時 |
| current_period_end | TIMESTAMP | REQUIRED | 現在の期間終了日時 |
| created_at | TIMESTAMP | REQUIRED | サブスクリプション作成日時 |
| updated_at | TIMESTAMP | REQUIRED | 最終更新日時 |

**Partitioning:** updated_at (DAY)
**Clustering:** player_id, status

**分析用途:**
- MRR (Monthly Recurring Revenue) - `subscriptions_latest` VIEW 経由
- チャーン率 (Churn Rate)
- LTV (Lifetime Value)
- サブスクリプション継続率

---

### purchases

ワンタイム課金データ。

取得元: `shop.one_time_purchases`

| Column | Type | Mode | Description |
|--------|------|------|-------------|
| purchase_id | INT64 | REQUIRED | 購入 ID |
| player_id | STRING | REQUIRED | プレイヤー ID |
| product_id | STRING | REQUIRED | 商品 ID |
| purchased_at | TIMESTAMP | REQUIRED | 購入日時 |

**Partitioning:** purchased_at (DAY)
**Clustering:** player_id, product_id

**分析用途:**
- ARPU (Average Revenue Per User)
- 購入頻度分析
- 商品別売上
- コンバージョン率

---

## Views

### games_latest

games テーブルから game_id ごとに最新の 1 行を返す VIEW。

```sql
SELECT * EXCEPT(rn) FROM (
  SELECT *, ROW_NUMBER() OVER (PARTITION BY game_id ORDER BY updated_at DESC) AS rn
  FROM analytics.games
) WHERE rn = 1
```

### subscriptions_latest

subscriptions テーブルから subscription_id ごとに最新の 1 行を返す VIEW。

```sql
SELECT * EXCEPT(rn) FROM (
  SELECT *, ROW_NUMBER() OVER (PARTITION BY subscription_id ORDER BY updated_at DESC) AS rn
  FROM analytics.subscriptions
) WHERE rn = 1
```

### players_latest

players テーブルから player_id ごとに最新の 1 行を返す VIEW。

```sql
SELECT * EXCEPT(rn) FROM (
  SELECT *, ROW_NUMBER() OVER (PARTITION BY player_id ORDER BY updated_at DESC) AS rn
  FROM analytics.players
) WHERE rn = 1
```

### card_definitions_latest

card_definitions テーブルから card_id ごとに最新の 1 行を返す VIEW。

```sql
SELECT * EXCEPT(rn) FROM (
  SELECT *, ROW_NUMBER() OVER (PARTITION BY card_id ORDER BY updated_at DESC) AS rn
  FROM analytics.card_definitions
) WHERE rn = 1
```

### deck_cards_latest

deck_cards テーブルからデッキごとに最新世代のカード構成をまとめて返す VIEW。

デッキ編集は構成全体を書き直し、全行がそのデッキの単一の updated_at を持つため、カード単位で
最新行を選ぶと編集で外したカードが残ります。デッキ単位で順位付けし、同順の行をすべて残すことで
最後に保存された構成と一致します。

```sql
SELECT * EXCEPT(rn) FROM (
  SELECT *, RANK() OVER (
    PARTITION BY player_id, deck_id ORDER BY updated_at DESC
  ) AS rn
  FROM analytics.deck_cards
) WHERE rn = 1
```

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
| game_players | 1日1回 | < 24時間 |
| players | 1日1回 | < 24時間 |
| card_definitions | 1日1回 | < 24時間 |
| deck_cards | 1日1回 | < 24時間 |
| subscriptions | 1日1回 | < 24時間 |
| purchases | 1日1回 | < 24時間 |
