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

  # Cloud SQL インスタンスと DB は overload-party-infra の state が所有する。
  # state をまたぐため名前で参照する。
  cloudsql_instance_name   = "overload-party-db"
  cloudsql_connection_name = "overload-party-dev:asia-northeast1:overload-party-db"
  database_name            = "overload_party"
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

  project_id               = local.project_id
  region                   = local.region
  env                      = local.env
  bq_dataset_id            = module.bigquery.dataset_id
  source_archive           = "export-function-latest.zip"
  cloudsql_instance_name   = local.cloudsql_instance_name
  cloudsql_connection_name = local.cloudsql_connection_name
  database_name            = local.database_name

  depends_on = [module.bigquery]
}

# Cloud Scheduler Jobs
module "scheduler" {
  source = "../../modules/scheduler"

  project_id        = local.project_id
  region            = local.region
  env               = local.env
  function_uri      = module.export_function.function_uri
  function_sa_email = module.export_function.service_account_email

  depends_on = [module.export_function]
}
