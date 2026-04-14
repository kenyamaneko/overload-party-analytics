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
