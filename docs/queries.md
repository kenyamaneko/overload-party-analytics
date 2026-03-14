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
  DATE(created_at) as date,
  COUNT(*) as total_games,
  COUNT(DISTINCT player1_id) + COUNT(DISTINCT player2_id) as unique_players,
  ROUND(COUNT(*) / (COUNT(DISTINCT player1_id) + COUNT(DISTINCT player2_id)), 2) as avg_games_per_player
FROM `overload-party-dev.analytics.games_latest`
WHERE created_at >= TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 30 DAY)
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

---

## カードバランス分析

### カード使用率 (トップ 20)

```sql
SELECT
  JSON_EXTRACT_SCALAR(event_data, '$.cardNo') as card_no,
  COUNT(*) as play_count,
  COUNT(DISTINCT game_id) as games_used_in,
  ROUND(COUNT(*) / SUM(COUNT(*)) OVER() * 100, 2) as usage_percentage
FROM `overload-party-dev.analytics.game_events`
WHERE event_type = 'play_card'
  AND created_at >= TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 7 DAY)
GROUP BY card_no
ORDER BY play_count DESC
LIMIT 20;
```

### カード勝率分析

```sql
WITH card_games AS (
  SELECT
    ge.game_id,
    JSON_EXTRACT_SCALAR(ge.event_data, '$.cardNo') as card_no,
    ge.player_id
  FROM `overload-party-dev.analytics.game_events` ge
  WHERE ge.event_type = 'play_card'
    AND ge.created_at >= TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 7 DAY)
  GROUP BY ge.game_id, card_no, ge.player_id
)
SELECT
  cg.card_no,
  COUNT(DISTINCT cg.game_id) as games_used,
  COUNTIF(g.winner_id = cg.player_id) as wins,
  ROUND(COUNTIF(g.winner_id = cg.player_id) / COUNT(DISTINCT cg.game_id) * 100, 2) as win_rate_percentage
FROM card_games cg
JOIN `overload-party-dev.analytics.games_latest` g ON cg.game_id = g.game_id
WHERE g.status = 'finished'
GROUP BY cg.card_no
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
    COUNTIF(status = 'canceled' OR status = 'expired') as churned_subscriptions
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
  platform,
  COUNT(*) as purchase_count,
  -- 実際の価格は products テーブルから JOIN して取得
  ROUND(COUNT(*) / SUM(COUNT(*)) OVER() * 100, 2) as percentage
FROM `overload-party-dev.analytics.purchases`
WHERE purchased_at >= TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 30 DAY)
GROUP BY product_id, platform
ORDER BY purchase_count DESC;
```

---

## ゲームバランス分析（詳細）

### ファクション別勝率

```sql
SELECT
  JSON_EXTRACT_SCALAR(player1_deck_snapshot, '$.faction') AS faction,
  COUNT(*) AS total_games,
  COUNTIF(winner_id = player1_id) AS wins,
  ROUND(COUNTIF(winner_id = player1_id) / COUNT(*) * 100, 2) AS win_rate
FROM `overload-party-dev.analytics.games_latest`
WHERE status = 'finished'
  AND created_at >= TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 30 DAY)
GROUP BY faction
ORDER BY win_rate DESC;
```

### ファクション対戦マトリクス

```sql
WITH faction_matchups AS (
  SELECT
    JSON_EXTRACT_SCALAR(player1_deck_snapshot, '$.faction') AS faction1,
    JSON_EXTRACT_SCALAR(player2_deck_snapshot, '$.faction') AS faction2,
    winner_id,
    player1_id
  FROM `overload-party-dev.analytics.games_latest`
  WHERE status = 'finished'
    AND created_at >= TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 30 DAY)
)
SELECT
  faction1,
  faction2,
  COUNT(*) AS total_games,
  COUNTIF(winner_id = player1_id) AS faction1_wins,
  ROUND(COUNTIF(winner_id = player1_id) / COUNT(*) * 100, 2) AS faction1_win_rate
FROM faction_matchups
GROUP BY faction1, faction2
ORDER BY faction1, faction2;
```

### 勝利条件の分布

```sql
SELECT
  JSON_EXTRACT_SCALAR(ge.event_data, '$.winCondition') AS win_condition,
  COUNT(*) AS count,
  ROUND(COUNT(*) / SUM(COUNT(*)) OVER() * 100, 2) AS percentage
FROM `overload-party-dev.analytics.game_events` ge
WHERE ge.event_type = 'game_end'
  AND ge.created_at >= TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 30 DAY)
GROUP BY win_condition
ORDER BY count DESC;
```

### デッキ内カード採用率（カード別）

```sql
SELECT
  dc.card_no,
  cd.card_name,
  cd.faction,
  cd.card_type,
  COUNT(DISTINCT CONCAT(dc.player_id, '-', dc.deck_id)) AS decks_including,
  ROUND(COUNT(DISTINCT CONCAT(dc.player_id, '-', dc.deck_id)) /
    (SELECT COUNT(DISTINCT CONCAT(player_id, '-', deck_id)) FROM `overload-party-dev.analytics.deck_cards`) * 100, 2
  ) AS adoption_rate
FROM `overload-party-dev.analytics.deck_cards` dc
JOIN `overload-party-dev.analytics.card_definitions_latest` cd ON dc.card_no = cd.card_no
GROUP BY dc.card_no, cd.card_name, cd.faction, cd.card_type
ORDER BY adoption_rate DESC;
```

### 平均ターン数

```sql
SELECT
  DATE(created_at) AS date,
  AVG(CAST(JSON_EXTRACT_SCALAR(
    (SELECT ge.event_data FROM `overload-party-dev.analytics.game_events` ge
     WHERE ge.game_id = g.game_id AND ge.event_type = 'game_end' LIMIT 1),
    '$.turnCount') AS INT64)) AS avg_turns
FROM `overload-party-dev.analytics.games_latest` g
WHERE status = 'finished'
  AND created_at >= TIMESTAMP_SUB(CURRENT_TIMESTAMP(), INTERVAL 30 DAY)
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
    COUNTIF(g.winner_id = p.player_id) as wins,
    COUNT(g.game_id) as total_games
  FROM `overload-party-dev.analytics.players_latest` p
  LEFT JOIN `overload-party-dev.analytics.games_latest` g
    ON (g.player1_id = p.player_id OR g.player2_id = p.player_id)
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
