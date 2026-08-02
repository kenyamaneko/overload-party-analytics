resource "google_project_service" "bigquery" {
  project            = var.project_id
  service            = "bigquery.googleapis.com"
  disable_on_destroy = false
}

# BigQuery Dataset
resource "google_bigquery_dataset" "analytics" {
  project       = var.project_id
  dataset_id    = var.dataset_id
  friendly_name = "Overload Party Analytics"
  description   = "Analytics data exported from PostgreSQL"
  location      = var.region

  default_table_expiration_ms = null

  labels = {
    env = var.env
  }

  depends_on = [google_project_service.bigquery]
}

# Games Table (append-only, use games_latest view for current state)
resource "google_bigquery_table" "games" {
  project    = var.project_id
  dataset_id = google_bigquery_dataset.analytics.dataset_id
  table_id   = "games"

  time_partitioning {
    type  = "DAY"
    field = "updated_at"
  }

  clustering = ["status", "winning_player_num"]

  schema = jsonencode([
    { name = "game_id", type = "STRING", mode = "REQUIRED" },
    { name = "status", type = "STRING", mode = "REQUIRED" },
    { name = "first_player", type = "INT64", mode = "REQUIRED" },
    { name = "winning_player_num", type = "INT64", mode = "NULLABLE" },
    { name = "win_reason", type = "STRING", mode = "NULLABLE" },
    { name = "engine_version", type = "STRING", mode = "REQUIRED" },
    { name = "card_data_version", type = "STRING", mode = "REQUIRED" },
    { name = "created_at", type = "TIMESTAMP", mode = "REQUIRED" },
    { name = "updated_at", type = "TIMESTAMP", mode = "REQUIRED" },
    { name = "finished_at", type = "TIMESTAMP", mode = "NULLABLE" },
  ])

  labels = {
    env = var.env
  }
}

# GameEvents Table
resource "google_bigquery_table" "game_events" {
  project    = var.project_id
  dataset_id = google_bigquery_dataset.analytics.dataset_id
  table_id   = "game_events"

  time_partitioning {
    type  = "DAY"
    field = "created_at"
  }

  clustering = ["game_id", "event_type"]

  schema = jsonencode([
    { name = "game_id", type = "STRING", mode = "REQUIRED" },
    { name = "sequence_number", type = "INT64", mode = "REQUIRED" },
    { name = "event_type", type = "STRING", mode = "REQUIRED" },
    { name = "player_num", type = "INT64", mode = "NULLABLE" },
    { name = "event_data", type = "JSON", mode = "REQUIRED" },
    { name = "created_at", type = "TIMESTAMP", mode = "REQUIRED" },
  ])

  labels = {
    env = var.env
  }
}

# Players Table (append-only, use players_latest view for current state)
resource "google_bigquery_table" "players" {
  project    = var.project_id
  dataset_id = google_bigquery_dataset.analytics.dataset_id
  table_id   = "players"

  time_partitioning {
    type  = "DAY"
    field = "updated_at"
  }

  clustering = ["is_premium", "onboarding_status"]

  schema = jsonencode([
    { name = "player_id", type = "STRING", mode = "REQUIRED" },
    { name = "is_premium", type = "BOOL", mode = "REQUIRED" },
    { name = "equipped_icon_no", type = "INT64", mode = "NULLABLE" },
    { name = "onboarding_status", type = "STRING", mode = "REQUIRED" },
    { name = "premium_expires_at", type = "TIMESTAMP", mode = "NULLABLE" },
    { name = "created_at", type = "TIMESTAMP", mode = "REQUIRED" },
    { name = "updated_at", type = "TIMESTAMP", mode = "REQUIRED" },
  ])

  labels = {
    env = var.env
  }
}

# Game Players Table (プレイヤーとゲームスロットの対応。games と JOIN して
# プレイヤー単位の勝敗を集計する経路になる)
resource "google_bigquery_table" "game_players" {
  project    = var.project_id
  dataset_id = google_bigquery_dataset.analytics.dataset_id
  table_id   = "game_players"

  time_partitioning {
    type  = "DAY"
    field = "updated_at"
  }

  clustering = ["player_id", "game_id"]

  schema = jsonencode([
    { name = "game_id", type = "STRING", mode = "REQUIRED" },
    { name = "player_num", type = "INT64", mode = "REQUIRED" },
    { name = "player_id", type = "STRING", mode = "REQUIRED" },
    { name = "updated_at", type = "TIMESTAMP", mode = "REQUIRED" },
  ])

  labels = {
    env = var.env
  }
}

# Subscriptions Table (append-only, use subscriptions_latest view)
resource "google_bigquery_table" "subscriptions" {
  project    = var.project_id
  dataset_id = google_bigquery_dataset.analytics.dataset_id
  table_id   = "subscriptions"

  time_partitioning {
    type  = "DAY"
    field = "updated_at"
  }

  clustering = ["player_id", "status"]

  schema = jsonencode([
    { name = "subscription_id", type = "INT64", mode = "REQUIRED" },
    { name = "player_id", type = "STRING", mode = "REQUIRED" },
    { name = "product_id", type = "STRING", mode = "REQUIRED" },
    { name = "status", type = "STRING", mode = "REQUIRED" },
    { name = "current_period_start", type = "TIMESTAMP", mode = "REQUIRED" },
    { name = "current_period_end", type = "TIMESTAMP", mode = "REQUIRED" },
    { name = "created_at", type = "TIMESTAMP", mode = "REQUIRED" },
    { name = "updated_at", type = "TIMESTAMP", mode = "REQUIRED" },
  ])

  labels = {
    env = var.env
  }
}

# Card Definitions Table (append-only, use card_definitions_latest view)
resource "google_bigquery_table" "card_definitions" {
  project    = var.project_id
  dataset_id = google_bigquery_dataset.analytics.dataset_id
  table_id   = "card_definitions"

  time_partitioning {
    type  = "DAY"
    field = "updated_at"
  }

  clustering = ["faction", "card_type"]

  schema = jsonencode([
    { name = "card_id", type = "STRING", mode = "REQUIRED" },
    { name = "card_name", type = "STRING", mode = "REQUIRED" },
    { name = "resource_label", type = "STRING", mode = "REQUIRED" },
    { name = "faction", type = "STRING", mode = "REQUIRED" },
    { name = "card_type", type = "STRING", mode = "REQUIRED" },
    { name = "subtype", type = "STRING", mode = "NULLABLE" },
    { name = "resizable", type = "BOOL", mode = "REQUIRED" },
    { name = "elastic", type = "BOOL", mode = "REQUIRED" },
    { name = "stats", type = "JSON", mode = "REQUIRED" },
    { name = "effect_text", type = "STRING", mode = "NULLABLE" },
    { name = "effects", type = "JSON", mode = "NULLABLE" },
    { name = "restriction", type = "STRING", mode = "REQUIRED" },
    { name = "is_active", type = "BOOL", mode = "REQUIRED" },
    { name = "created_at", type = "TIMESTAMP", mode = "REQUIRED" },
    { name = "updated_at", type = "TIMESTAMP", mode = "REQUIRED" },
  ])

  labels = {
    env = var.env
  }
}

# Deck Cards Table (append-only, use deck_cards_latest view for current state)
resource "google_bigquery_table" "deck_cards" {
  project    = var.project_id
  dataset_id = google_bigquery_dataset.analytics.dataset_id
  table_id   = "deck_cards"

  time_partitioning {
    type  = "DAY"
    field = "updated_at"
  }

  clustering = ["player_id", "card_id"]

  schema = jsonencode([
    { name = "player_id", type = "STRING", mode = "REQUIRED" },
    { name = "deck_id", type = "INT64", mode = "REQUIRED" },
    { name = "card_id", type = "STRING", mode = "REQUIRED" },
    { name = "art_no", type = "INT64", mode = "REQUIRED" },
    { name = "count", type = "INT64", mode = "REQUIRED" },
    { name = "faction", type = "STRING", mode = "REQUIRED" },
    { name = "created_at", type = "TIMESTAMP", mode = "REQUIRED" },
    { name = "updated_at", type = "TIMESTAMP", mode = "REQUIRED" },
  ])

  labels = {
    env = var.env
  }
}

# OneTimePurchases Table
resource "google_bigquery_table" "purchases" {
  project    = var.project_id
  dataset_id = google_bigquery_dataset.analytics.dataset_id
  table_id   = "purchases"

  time_partitioning {
    type  = "DAY"
    field = "purchased_at"
  }

  clustering = ["player_id", "product_id"]

  schema = jsonencode([
    { name = "purchase_id", type = "INT64", mode = "REQUIRED" },
    { name = "player_id", type = "STRING", mode = "REQUIRED" },
    { name = "product_id", type = "STRING", mode = "REQUIRED" },
    { name = "purchased_at", type = "TIMESTAMP", mode = "REQUIRED" },
  ])

  labels = {
    env = var.env
  }
}

# Games Latest View (dedup: one row per game_id, most recent updated_at wins)
resource "google_bigquery_table" "games_latest" {
  project    = var.project_id
  dataset_id = google_bigquery_dataset.analytics.dataset_id
  table_id   = "games_latest"

  view {
    query          = <<-SQL
      SELECT * EXCEPT(rn) FROM (
        SELECT *, ROW_NUMBER() OVER (PARTITION BY game_id ORDER BY updated_at DESC) AS rn
        FROM `${var.project_id}.${var.dataset_id}.games`
      ) WHERE rn = 1
    SQL
    use_legacy_sql = false
  }

  labels = {
    env = var.env
  }

  depends_on = [google_bigquery_table.games]
}

# Subscriptions Latest View (dedup: one row per subscription_id)
resource "google_bigquery_table" "subscriptions_latest" {
  project    = var.project_id
  dataset_id = google_bigquery_dataset.analytics.dataset_id
  table_id   = "subscriptions_latest"

  view {
    query          = <<-SQL
      SELECT * EXCEPT(rn) FROM (
        SELECT *, ROW_NUMBER() OVER (PARTITION BY subscription_id ORDER BY updated_at DESC) AS rn
        FROM `${var.project_id}.${var.dataset_id}.subscriptions`
      ) WHERE rn = 1
    SQL
    use_legacy_sql = false
  }

  labels = {
    env = var.env
  }

  depends_on = [google_bigquery_table.subscriptions]
}

# Players Latest View (dedup: one row per player_id, most recent updated_at wins)
resource "google_bigquery_table" "players_latest" {
  project    = var.project_id
  dataset_id = google_bigquery_dataset.analytics.dataset_id
  table_id   = "players_latest"

  view {
    query          = <<-SQL
      SELECT * EXCEPT(rn) FROM (
        SELECT *, ROW_NUMBER() OVER (PARTITION BY player_id ORDER BY updated_at DESC) AS rn
        FROM `${var.project_id}.${var.dataset_id}.players`
      ) WHERE rn = 1
    SQL
    use_legacy_sql = false
  }

  labels = {
    env = var.env
  }

  depends_on = [google_bigquery_table.players]
}

# Deck Cards Latest View (dedup: the newest generation of each deck, kept as a whole)
#
# A deck edit rewrites the whole composition under the deck's single updated_at, so ranking
# per card would keep the cards that the edit removed.
resource "google_bigquery_table" "deck_cards_latest" {
  project    = var.project_id
  dataset_id = google_bigquery_dataset.analytics.dataset_id
  table_id   = "deck_cards_latest"

  view {
    query          = <<-SQL
      SELECT * EXCEPT(rn) FROM (
        SELECT *, RANK() OVER (
          PARTITION BY player_id, deck_id ORDER BY updated_at DESC
        ) AS rn
        FROM `${var.project_id}.${var.dataset_id}.deck_cards`
      ) WHERE rn = 1
    SQL
    use_legacy_sql = false
  }

  labels = {
    env = var.env
  }

  depends_on = [google_bigquery_table.deck_cards]
}

# Card Definitions Latest View (dedup: one row per card_id)
resource "google_bigquery_table" "card_definitions_latest" {
  project    = var.project_id
  dataset_id = google_bigquery_dataset.analytics.dataset_id
  table_id   = "card_definitions_latest"

  view {
    query          = <<-SQL
      SELECT * EXCEPT(rn) FROM (
        SELECT *, ROW_NUMBER() OVER (PARTITION BY card_id ORDER BY updated_at DESC) AS rn
        FROM `${var.project_id}.${var.dataset_id}.card_definitions`
      ) WHERE rn = 1
    SQL
    use_legacy_sql = false
  }

  labels = {
    env = var.env
  }

  depends_on = [google_bigquery_table.card_definitions]
}
