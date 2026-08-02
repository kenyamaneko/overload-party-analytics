# サンプルクエリ集

Overload Party Analytics - よく使う分析クエリ

## 目次

- [ユーザー分析](#ユーザー分析)
- [ゲームプレイ分析](#ゲームプレイ分析)
- [カードバランス分析](#カードバランス分析)
- [収益分析](#収益分析)
- [ゲームバランス分析（詳細）](#ゲームバランス分析詳細)

---

## ユーザー分析

### DAU (Daily Active Users)

```sql
SELECT
  DATE(updated_at) as date,
  COUNT(DISTINCT player_id) as dau
FROM `overload-party-dev.analytics.players`
WHERE updated_at >= TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 30 DAY)
GROUP BY date
ORDER BY date DESC;
```

### MAU (Monthly Active Users)

```sql
SELECT
  FORMAT_DATE('%Y-%m', DATE(updated_at)) as month,
  COUNT(DISTINCT player_id) as mau
FROM `overload-party-dev.analytics.players`
WHERE updated_at >= TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 12 MONTH)
GROUP BY month
ORDER BY month DESC;
```

### 課金ユーザー割合

```sql
SELECT
  is_premium,
  COUNT(*) as user_count,
  ROUND(COUNT(*) / SUM(COUNT(*)) OVER() * 100, 2) as percentage
FROM `overload-party-dev.analytics.players_latest`
GROUP BY is_premium;
```

### オンボーディングの離脱地点

```sql
SELECT
  onboarding_status,
  COUNT(*) as player_count,
  ROUND(COUNT(*) / SUM(COUNT(*)) OVER() * 100, 2) as percentage
FROM `overload-party-dev.analytics.players_latest`
GROUP BY onboarding_status
ORDER BY player_count DESC;
```

### 新規登録ユーザー (週次)

```sql
SELECT
  FORMAT_DATE('%Y-W%V', DATE(created_at)) as week,
  COUNT(*) as new_users
FROM `overload-party-dev.analytics.players_latest`
WHERE created_at >= TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 12 WEEK)
GROUP BY week
ORDER BY week DESC;
```

> **Note:** DAU/MAU は `players` テーブル (raw) の `updated_at` から集計します。
> 「最新のプレイヤー状態」を参照する場合は `players_latest` VIEW を使用してください。

---

## ゲームプレイ分析

### 1日あたりの平均バトル数

```sql
SELECT
  DATE(g.created_at) as date,
  COUNT(DISTINCT g.game_id) as total_games,
  COUNT(DISTINCT gp.player_id) as unique_players,
  ROUND(COUNT(DISTINCT g.game_id) / COUNT(DISTINCT gp.player_id), 2) as avg_games_per_player
FROM `overload-party-dev.analytics.games_latest` g
JOIN `overload-party-dev.analytics.game_players` gp USING (game_id)
WHERE g.created_at >= TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 30 DAY)
GROUP BY date
ORDER BY date DESC;
```

### ゲーム完了率

```sql
SELECT
  status,
  COUNT(*) as count,
  ROUND(COUNT(*) / SUM(COUNT(*)) OVER() * 100, 2) as percentage
FROM `overload-party-dev.analytics.games_latest`
WHERE created_at >= TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 7 DAY)
GROUP BY status;
```

### 平均ゲーム時間

```sql
SELECT
  DATE(created_at) as date,
  AVG(TIMESTAMP_DIFF(finished_at, created_at, MINUTE)) as avg_duration_minutes,
  MIN(TIMESTAMP_DIFF(finished_at, created_at, MINUTE)) as min_duration_minutes,
  MAX(TIMESTAMP_DIFF(finished_at, created_at, MINUTE)) as max_duration_minutes
FROM `overload-party-dev.analytics.games_latest`
WHERE finished_at IS NOT NULL
  AND created_at >= TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 7 DAY)
GROUP BY date
ORDER BY date DESC;
```

### 時間帯別のゲーム数

```sql
SELECT
  EXTRACT(HOUR FROM created_at) as hour_of_day,
  COUNT(*) as game_count
FROM `overload-party-dev.analytics.games_latest`
WHERE created_at >= TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 7 DAY)
GROUP BY hour_of_day
ORDER BY hour_of_day;
```

### プレイヤー別勝率

```sql
SELECT
  gp.player_id,
  COUNT(*) as total_games,
  COUNTIF(g.winning_player_num = gp.player_num) as wins,
  ROUND(COUNTIF(g.winning_player_num = gp.player_num) / COUNT(*) * 100, 2) as win_rate_percentage
FROM `overload-party-dev.analytics.game_players` gp
JOIN `overload-party-dev.analytics.games_latest` g USING (game_id)
WHERE g.status = 'finished'
  AND g.created_at >= TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 30 DAY)
GROUP BY gp.player_id
HAVING total_games >= 5
ORDER BY win_rate_percentage DESC;
```

### 先攻・後攻の勝率

```sql
SELECT
  first_player,
  COUNT(*) as total_games,
  COUNTIF(winning_player_num = first_player) as first_player_wins,
  ROUND(COUNTIF(winning_player_num = first_player) / COUNT(*) * 100, 2) as first_player_win_rate
FROM `overload-party-dev.analytics.games_latest`
WHERE status = 'finished'
  AND created_at >= TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 30 DAY)
GROUP BY first_player;
```

---

## カードバランス分析

### カード使用率 (トップ 20)

```sql
SELECT
  JSON_EXTRACT_SCALAR(event_data, '$.cardId') as card_id,
  COUNT(*) as play_count,
  COUNT(DISTINCT game_id) as games_used_in,
  ROUND(COUNT(*) / SUM(COUNT(*)) OVER() * 100, 2) as usage_percentage
FROM `overload-party-dev.analytics.game_events`
WHERE event_type = 'play_card'
  AND created_at >= TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 7 DAY)
GROUP BY card_id
ORDER BY play_count DESC
LIMIT 20;
```

### カード勝率分析

```sql
WITH card_plays AS (
  SELECT DISTINCT
    game_id,
    JSON_EXTRACT_SCALAR(event_data, '$.cardId') as card_id,
    player_num
  FROM `overload-party-dev.analytics.game_events`
  WHERE event_type = 'play_card'
    AND player_num IS NOT NULL
    AND created_at >= TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 7 DAY)
)
SELECT
  cp.card_id,
  COUNT(DISTINCT cp.game_id) as games_used,
  COUNTIF(g.winning_player_num = cp.player_num) as wins,
  ROUND(COUNTIF(g.winning_player_num = cp.player_num) / COUNT(DISTINCT cp.game_id) * 100, 2) as win_rate_percentage
FROM card_plays cp
JOIN `overload-party-dev.analytics.games_latest` g USING (game_id)
WHERE g.status = 'finished'
GROUP BY cp.card_id
HAVING games_used >= 10  -- 最低10ゲーム使用されたカードのみ
ORDER BY win_rate_percentage DESC;
```

### イベントタイプ別の頻度

```sql
SELECT
  event_type,
  COUNT(*) as event_count,
  COUNT(DISTINCT game_id) as games_with_event,
  ROUND(COUNT(*) / COUNT(DISTINCT game_id), 2) as avg_per_game
FROM `overload-party-dev.analytics.game_events`
WHERE created_at >= TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 7 DAY)
GROUP BY event_type
ORDER BY event_count DESC;
```

---

## 収益分析

### MRR (Monthly Recurring Revenue)

```sql
-- 簡易版: アクティブなサブスクリプション数 × 平均価格
WITH active_subs AS (
  SELECT
    FORMAT_DATE('%Y-%m', DATE(current_period_start)) as month,
    COUNT(*) as active_subscriptions
  FROM `overload-party-dev.analytics.subscriptions_latest`
  WHERE status = 'active'
    AND current_period_start >= TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 12 MONTH)
  GROUP BY month
)
SELECT
  month,
  active_subscriptions,
  -- 仮に月額 ¥980 とする (実際は products テーブルから取得)
  active_subscriptions * 980 as estimated_mrr_jpy
FROM active_subs
ORDER BY month DESC;
```

### チャーン率 (月次)

```sql
WITH monthly_subs AS (
  SELECT
    FORMAT_DATE('%Y-%m', DATE(created_at)) as month,
    COUNT(*) as new_subscriptions,
    COUNTIF(status IN ('cancelled', 'expired', 'revoked')) as churned_subscriptions
  FROM `overload-party-dev.analytics.subscriptions_latest`
  WHERE created_at >= TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 12 MONTH)
  GROUP BY month
)
SELECT
  month,
  new_subscriptions,
  churned_subscriptions,
  ROUND(churned_subscriptions / NULLIF(new_subscriptions, 0) * 100, 2) as churn_rate_percentage
FROM monthly_subs
ORDER BY month DESC;
```

### ARPU (Average Revenue Per User)

```sql
WITH revenue AS (
  SELECT
    FORMAT_DATE('%Y-%m', DATE(purchased_at)) as month,
    COUNT(*) as total_purchases,
    -- 仮に平均単価 ¥1200 とする (実際は products テーブルから取得)
    COUNT(*) * 1200 as total_revenue_jpy
  FROM `overload-party-dev.analytics.purchases`
  WHERE purchased_at >= TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 12 MONTH)
  GROUP BY month
),
users AS (
  SELECT
    FORMAT_DATE('%Y-%m', DATE(created_at)) as month,
    COUNT(DISTINCT player_id) as total_users
  FROM `overload-party-dev.analytics.players_latest`
  WHERE created_at >= TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 12 MONTH)
  GROUP BY month
)
SELECT
  r.month,
  r.total_purchases,
  r.total_revenue_jpy,
  u.total_users,
  ROUND(r.total_revenue_jpy / NULLIF(u.total_users, 0), 2) as arpu_jpy
FROM revenue r
JOIN users u ON r.month = u.month
ORDER BY r.month DESC;
```

### 商品別売上

```sql
SELECT
  product_id,
  COUNT(*) as purchase_count,
  -- 実際の価格は products テーブルから JOIN して取得
  ROUND(COUNT(*) / SUM(COUNT(*)) OVER() * 100, 2) as percentage
FROM `overload-party-dev.analytics.purchases`
WHERE purchased_at >= TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 30 DAY)
GROUP BY product_id
ORDER BY purchase_count DESC;
```

---

## ゲームバランス分析（詳細）

### ファクション別勝率

デッキの宣言陣営は `deck_cards` が持つため、プレイヤーの直近デッキと突き合わせます。

```sql
WITH player_faction AS (
  SELECT DISTINCT player_id, deck_id, faction
  FROM `overload-party-dev.analytics.deck_cards`
)
SELECT
  pf.faction,
  COUNT(*) AS total_games,
  COUNTIF(g.winning_player_num = gp.player_num) AS wins,
  ROUND(COUNTIF(g.winning_player_num = gp.player_num) / COUNT(*) * 100, 2) AS win_rate
FROM `overload-party-dev.analytics.game_players` gp
JOIN `overload-party-dev.analytics.games_latest` g USING (game_id)
JOIN player_faction pf ON pf.player_id = gp.player_id
WHERE g.status = 'finished'
  AND g.created_at >= TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 30 DAY)
GROUP BY pf.faction
ORDER BY win_rate DESC;
```

### 決着理由の分布

```sql
SELECT
  win_reason,
  COUNT(*) AS count,
  ROUND(COUNT(*) / SUM(COUNT(*)) OVER() * 100, 2) AS percentage
FROM `overload-party-dev.analytics.games_latest`
WHERE status = 'finished'
  AND created_at >= TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 30 DAY)
GROUP BY win_reason
ORDER BY count DESC;
```

### エンジンバージョン別の平均ゲーム時間

```sql
SELECT
  engine_version,
  card_data_version,
  COUNT(*) AS total_games,
  ROUND(AVG(TIMESTAMP_DIFF(finished_at, created_at, MINUTE)), 2) AS avg_duration_minutes
FROM `overload-party-dev.analytics.games_latest`
WHERE status = 'finished'
  AND finished_at IS NOT NULL
  AND created_at >= TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 30 DAY)
GROUP BY engine_version, card_data_version
ORDER BY total_games DESC;
```

### デッキ内カード採用率（カード別）

```sql
SELECT
  dc.card_id,
  cd.card_name,
  cd.faction,
  cd.card_type,
  COUNT(DISTINCT CONCAT(dc.player_id, '-', CAST(dc.deck_id AS STRING))) AS decks_including,
  ROUND(COUNT(DISTINCT CONCAT(dc.player_id, '-', CAST(dc.deck_id AS STRING))) /
    (SELECT COUNT(DISTINCT CONCAT(player_id, '-', CAST(deck_id AS STRING))) FROM `overload-party-dev.analytics.deck_cards`) * 100, 2
  ) AS adoption_rate
FROM `overload-party-dev.analytics.deck_cards` dc
JOIN `overload-party-dev.analytics.card_definitions_latest` cd ON dc.card_id = cd.card_id
GROUP BY dc.card_id, cd.card_name, cd.faction, cd.card_type
ORDER BY adoption_rate DESC;
```

### 平均ターン数

```sql
SELECT
  DATE(g.created_at) AS date,
  ROUND(AVG(turns.max_turn), 2) AS avg_turns
FROM `overload-party-dev.analytics.games_latest` g
JOIN (
  SELECT game_id, MAX(CAST(JSON_EXTRACT_SCALAR(event_data, '$.turn') AS INT64)) AS max_turn
  FROM `overload-party-dev.analytics.game_events`
  GROUP BY game_id
) turns USING (game_id)
WHERE g.status = 'finished'
  AND g.created_at >= TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 30 DAY)
GROUP BY date
ORDER BY date DESC;
```

---

## 複合分析

### 課金ユーザーの勝率

```sql
WITH player_stats AS (
  SELECT
    p.player_id,
    p.is_premium,
    COUNTIF(g.winning_player_num = gp.player_num) as wins,
    COUNT(g.game_id) as total_games
  FROM `overload-party-dev.analytics.players_latest` p
  LEFT JOIN `overload-party-dev.analytics.game_players` gp ON gp.player_id = p.player_id
  LEFT JOIN `overload-party-dev.analytics.games_latest` g
    ON g.game_id = gp.game_id
    AND g.status = 'finished'
    AND g.created_at >= TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 30 DAY)
  GROUP BY p.player_id, p.is_premium
)
SELECT
  is_premium,
  COUNT(*) as player_count,
  ROUND(AVG(wins / NULLIF(total_games, 0)) * 100, 2) as avg_win_rate_percentage
FROM player_stats
WHERE total_games >= 5
GROUP BY is_premium;
```

---

## Tips

### パフォーマンス最適化

1. **パーティションフィルタを必ず使う**
```sql
WHERE created_at >= TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 7 DAY)
```

2. **LIMIT を活用**
```sql
ORDER BY created_at DESC LIMIT 1000
```

3. **集計はサブクエリで**
```sql
WITH aggregated AS (
  SELECT ... GROUP BY ...
)
SELECT * FROM aggregated
```

### コスト確認

クエリ実行前に「DRY RUN」でスキャン量を確認:
```bash
bq query --use_legacy_sql=false --dry_run 'SELECT ...'
```
