terraform {
  required_version = ">= 1.5"
  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "~> 5.0"
    }
  }

  backend "gcs" {
    bucket = "keyandnotes-tf-state"
    prefix = "overload-party/analytics/dev"
  }
}

provider "google" {
  project = local.project_id
  region  = local.region
}

locals {
  project_id = "overload-party-dev"
  region     = "asia-northeast1"
  env        = "dev"

}

# BigQuery Dataset & Tables
module "bigquery" {
  source = "../../modules/bigquery"

  project_id = local.project_id
  region     = local.region
  dataset_id = "analytics"
  env        = local.env
}

# Export Function Infrastructure
module "export_function" {
  source = "../../modules/export-function"

  project_id           = local.project_id
  region               = local.region
  env                  = local.env
  bq_dataset_id        = module.bigquery.dataset_id
  source_archive       = "export-function-latest.zip"

  depends_on = [module.bigquery]
}

# Cloud Scheduler Jobs
module "scheduler" {
  source = "../../modules/scheduler"

  project_id         = local.project_id
  region             = local.region
  env                = local.env
  function_uri       = module.export_function.function_uri
  function_sa_email  = module.export_function.service_account_email

  depends_on = [module.export_function]
}
