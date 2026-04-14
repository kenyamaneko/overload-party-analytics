variable "project_id" {
  description = "Google Cloud Project ID"
  type        = string
}

variable "region" {
  description = "Google Cloud Region for BigQuery dataset"
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
