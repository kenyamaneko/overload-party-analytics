output "dataset_id" {
  description = "BigQuery dataset ID"
  value       = google_bigquery_dataset.analytics.dataset_id
}

output "dataset_location" {
  description = "BigQuery dataset location"
  value       = google_bigquery_dataset.analytics.location
}

output "table_ids" {
  description = "Map of table names to table IDs"
  value = {
    games         = google_bigquery_table.games.table_id
    game_events   = google_bigquery_table.game_events.table_id
    players       = google_bigquery_table.players.table_id
    matches       = google_bigquery_table.matches.table_id
    subscriptions = google_bigquery_table.subscriptions.table_id
    purchases     = google_bigquery_table.purchases.table_id
  }
}
