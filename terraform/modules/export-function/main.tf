locals {
  # Cloud SQL の IAM データベース認証では、SA メールから .gserviceaccount.com を
  # 落とした文字列がそのまま DB ユーザー名になる。
  db_user = trimsuffix(google_service_account.export_function.email, ".gserviceaccount.com")
}

resource "google_project_service" "cloudfunctions" {
  project            = var.project_id
  service            = "cloudfunctions.googleapis.com"
  disable_on_destroy = false
}

resource "google_project_service" "cloudbuild" {
  project            = var.project_id
  service            = "cloudbuild.googleapis.com"
  disable_on_destroy = false
}

# Service Account for Export Function
resource "google_service_account" "export_function" {
  project      = var.project_id
  account_id   = "export-function-${var.env}"
  display_name = "Export Function Service Account (${var.env})"
  description  = "Service account for PostgreSQL to BigQuery export function"
}

# GCS Bucket for Function Source Code
resource "google_storage_bucket" "function_source" {
  project       = var.project_id
  name          = "${var.project_id}-function-source"
  location      = var.region
  force_destroy = var.env == "dev" ? true : false

  uniform_bucket_level_access = true

  lifecycle_rule {
    condition {
      age = 30
    }
    action {
      type = "Delete"
    }
  }

  labels = {
    env = var.env
  }
}

# GCS Bucket for Data Staging
resource "google_storage_bucket" "staging" {
  project       = var.project_id
  name          = "${var.project_id}-bq-staging"
  location      = var.region
  force_destroy = var.env == "dev" ? true : false

  uniform_bucket_level_access = true

  lifecycle_rule {
    condition {
      age = 7 # Delete exported files after 7 days
    }
    action {
      type = "Delete"
    }
  }

  labels = {
    env = var.env
  }
}

# Cloud Function (Gen 2)
resource "google_cloudfunctions2_function" "export" {
  project  = var.project_id
  name     = "export-postgres-to-bigquery-${var.env}"
  location = var.region

  build_config {
    runtime     = "go122"
    entry_point = "ExportPostgresToBigQuery"
    source {
      storage_source {
        bucket = google_storage_bucket.function_source.name
        object = var.source_archive
      }
    }
  }

  service_config {
    max_instance_count    = 10
    min_instance_count    = 0
    available_memory      = "1Gi"
    timeout_seconds       = 540 # 9 minutes
    service_account_email = google_service_account.export_function.email

    environment_variables = {
      DATABASE_CONN             = "user=${local.db_user} dbname=${var.database_name} sslmode=disable"
      DATABASE_IAM_AUTH_ENABLED = "true"
      CLOUDSQL_CONNECTION_NAME  = var.cloudsql_connection_name
      BQ_PROJECT_ID             = var.project_id
      BQ_DATASET_ID             = var.bq_dataset_id
      GCS_BUCKET                = google_storage_bucket.staging.name
    }
  }

  labels = {
    env = var.env
  }

  # CI deploys the function source directly via gcloud.
  # Prevent Terraform from reverting the build config on subsequent applies.
  lifecycle {
    ignore_changes = [build_config]
  }

  depends_on = [
    google_project_service.cloudfunctions,
    google_project_service.cloudbuild,
  ]
}

# Cloud SQL: IAM database user (instance itself is owned by overload-party-infra)
resource "google_sql_user" "export_function" {
  name     = local.db_user
  instance = var.cloudsql_instance_name
  project  = var.project_id
  type     = "CLOUD_IAM_SERVICE_ACCOUNT"
}

# IAM: Cloud SQL Client (connect through the Cloud SQL connector)
resource "google_project_iam_member" "cloudsql_client" {
  project = var.project_id
  role    = "roles/cloudsql.client"
  member  = "serviceAccount:${google_service_account.export_function.email}"
}

# IAM: Cloud SQL Instance User (log in with an IAM database account)
resource "google_project_iam_member" "cloudsql_instance_user" {
  project = var.project_id
  role    = "roles/cloudsql.instanceUser"
  member  = "serviceAccount:${google_service_account.export_function.email}"
}

# IAM: GCS Object Creator (staging bucket)
resource "google_storage_bucket_iam_member" "gcs_writer" {
  bucket = google_storage_bucket.staging.name
  role   = "roles/storage.objectCreator"
  member = "serviceAccount:${google_service_account.export_function.email}"
}

# IAM: BigQuery Job User
resource "google_project_iam_member" "bq_job_user" {
  project = var.project_id
  role    = "roles/bigquery.jobUser"
  member  = "serviceAccount:${google_service_account.export_function.email}"
}

# IAM: BigQuery Data Editor (for the analytics dataset)
resource "google_bigquery_dataset_iam_member" "bq_data_editor" {
  project    = var.project_id
  dataset_id = var.bq_dataset_id
  role       = "roles/bigquery.dataEditor"
  member     = "serviceAccount:${google_service_account.export_function.email}"
}

# IAM: Firestore User (for checkpoint management)
resource "google_project_iam_member" "firestore_user" {
  project = var.project_id
  role    = "roles/datastore.user"
  member  = "serviceAccount:${google_service_account.export_function.email}"
}

# IAM: Cloud Functions Invoker (for Cloud Scheduler)
resource "google_cloudfunctions2_function_iam_member" "invoker" {
  project        = var.project_id
  location       = var.region
  cloud_function = google_cloudfunctions2_function.export.name
  role           = "roles/cloudfunctions.invoker"
  member         = "serviceAccount:${google_service_account.export_function.email}"
}
