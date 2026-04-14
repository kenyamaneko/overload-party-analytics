variable "project_id" {
  description = "Google Cloud Project ID"
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

variable "function_uri" {
  description = "Cloud Function URI to invoke"
  type        = string
}

variable "function_sa_email" {
  description = "Service account email for OIDC authentication"
  type        = string
}
