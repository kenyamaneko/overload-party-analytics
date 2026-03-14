output "hourly_job_name" {
  description = "Name of the hourly export job"
  value       = google_cloud_scheduler_job.export_hourly.name
}

output "daily_job_name" {
  description = "Name of the daily export job"
  value       = google_cloud_scheduler_job.export_daily.name
}
