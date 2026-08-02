variable "project_id" {
  description = "Google Cloud Project ID for analytics infrastructure"
  type        = string
}

variable "region" {
  description = "Google Cloud Region"
  type        = string
  default     = "asia-northeast1"
}

variable "env" {
  description = "Environment (dev, stg, prod)"
  type        = string
}

variable "bq_dataset_id" {
  description = "BigQuery dataset ID"
  type        = string
}

variable "source_archive" {
  description = "GCS object name for function source code zip"
  type        = string
  default     = "export-function-latest.zip"
}

variable "cloudsql_instance_name" {
  description = "Cloud SQL instance name owned by overload-party-infra"
  type        = string
}

variable "cloudsql_connection_name" {
  description = "Cloud SQL connection name (project:region:instance)"
  type        = string
}

variable "database_name" {
  description = "PostgreSQL database name holding the service schemas"
  type        = string
}
