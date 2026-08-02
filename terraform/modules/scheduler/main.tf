resource "google_project_service" "cloudscheduler" {
  project            = var.project_id
  service            = "cloudscheduler.googleapis.com"
  disable_on_destroy = false
}

# Hourly export for high-frequency tables (game_events)
resource "google_cloud_scheduler_job" "export_hourly" {
  project          = var.project_id
  name             = "export-game-events-hourly-${var.env}"
  description      = "Export game events to BigQuery every hour"
  schedule         = "0 * * * *" # Every hour at minute 0
  time_zone        = "Asia/Tokyo"
  region           = var.region
  attempt_deadline = "600s"

  http_target {
    http_method = "POST"
    uri         = var.function_uri

    body = base64encode(jsonencode({
      tables = ["game_events"]
      mode   = "incremental"
    }))

    oidc_token {
      service_account_email = var.function_sa_email
    }
  }

  retry_config {
    retry_count          = 3
    max_retry_duration   = "600s"
    min_backoff_duration = "5s"
    max_backoff_duration = "3600s"
  }

  depends_on = [google_project_service.cloudscheduler]
}

# Daily export for slower-changing tables
resource "google_cloud_scheduler_job" "export_daily" {
  project          = var.project_id
  name             = "export-analytics-daily-${var.env}"
  description      = "Export all analytics tables to BigQuery daily"
  schedule         = "0 3 * * *" # Daily at 3 AM JST
  time_zone        = "Asia/Tokyo"
  region           = var.region
  attempt_deadline = "600s"

  http_target {
    http_method = "POST"
    uri         = var.function_uri

    body = base64encode(jsonencode({
      tables = [
        "games",
        "game_players",
        "players",
        "subscriptions",
        "purchases",
        "card_definitions",
        "deck_cards"
      ]
      mode = "incremental"
    }))

    oidc_token {
      service_account_email = var.function_sa_email
    }
  }

  retry_config {
    retry_count          = 3
    max_retry_duration   = "600s"
    min_backoff_duration = "5s"
    max_backoff_duration = "3600s"
  }

  depends_on = [google_project_service.cloudscheduler]
}
