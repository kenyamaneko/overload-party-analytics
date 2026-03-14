output "function_name" {
  description = "Cloud Function name"
  value       = google_cloudfunctions2_function.export.name
}

output "function_uri" {
  description = "Cloud Function URI"
  value       = google_cloudfunctions2_function.export.service_config[0].uri
}

output "service_account_email" {
  description = "Service account email for the export function"
  value       = google_service_account.export_function.email
}

output "staging_bucket" {
  description = "GCS staging bucket name"
  value       = google_storage_bucket.staging.name
}
