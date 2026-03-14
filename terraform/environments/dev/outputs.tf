output "bigquery_dataset_id" {
  description = "BigQuery dataset ID"
  value       = module.bigquery.dataset_id
}

output "function_name" {
  description = "Export function name"
  value       = module.export_function.function_name
}

output "function_uri" {
  description = "Export function URI"
  value       = module.export_function.function_uri
}

output "staging_bucket" {
  description = "GCS staging bucket for exports"
  value       = module.export_function.staging_bucket
}

output "scheduler_jobs" {
  description = "Cloud Scheduler job names"
  value = {
    hourly = module.scheduler.hourly_job_name
    daily  = module.scheduler.daily_job_name
  }
}
