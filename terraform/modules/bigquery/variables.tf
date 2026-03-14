variable "project_id" {
  description = "GCP Project ID"
  type        = string
}

variable "region" {
  description = "GCP Region for BigQuery dataset"
  type        = string
  default     = "asia-northeast1"
}

variable "dataset_id" {
  description = "BigQuery dataset ID"
  type        = string
  default     = "analytics"
}

variable "env" {
  description = "Environment (dev, stg, prod)"
  type        = string
}
