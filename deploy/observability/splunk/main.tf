terraform {
  required_version = ">= 1.11.0, < 2.0.0"
  required_providers {
    signalfx = {
      source  = "splunk-terraform/signalfx"
      version = "9.35.1"
    }
  }
}

variable "realm" {
  type        = string
  description = "Splunk Observability Cloud realm, for example eu0."
  validation {
    condition     = can(regex("^[a-z]{2}[0-9]+$", var.realm))
    error_message = "Use a Splunk realm such as eu0 or us1."
  }
}
variable "api_token" {
  type        = string
  sensitive   = true
  description = "API-scoped management token, supplied as TF_VAR_api_token; distinct from Collector ingest token."
}
variable "notifications" {
  type        = list(string)
  default     = []
  description = "Explicitly approved detector destinations. Empty means no messages are sent."
}
provider "signalfx" {
  auth_token = var.api_token
  api_url    = "https://api.${var.realm}.observability.splunkcloud.com"
}

resource "signalfx_dashboard_group" "ukinfo" {
  name        = "UK-Info"
  description = "Managed in ukinfo-reader/deploy/observability/splunk."
}
locals {
  charts = {
    active = {
      name    = "Active API requests"
      program = "data('ukinfo_api_active').sum().publish(label='Active requests')"
    }
    pool = {
      name    = "Database pool utilisation (%)"
      program = "used = data('ukinfo_pool_acquired').sum()\nmaximum = data('ukinfo_pool_max').sum()\n(100 * used / maximum).publish(label='Pool utilisation')"
    }
    queries = {
      name    = "Search requests / second by category"
      program = "data('ukinfo_query_requests_total', rollup='rate').sum(by=['class']).publish(label='Search requests/s')"
    }
    failures = {
      name    = "Search 503 responses / second"
      program = "data('ukinfo_query_requests_total', filter=filter('status', '503'), rollup='rate').sum().publish(label='503 responses/s')"
    }
    decoded = {
      name    = "Decoded record blocks / second"
      program = "data('ukinfo_record_blocks_decoded_total', rollup='rate').sum().publish(label='Decoded blocks/s')"
    }
  }
}
resource "signalfx_time_chart" "ukinfo" {
  for_each     = local.charts
  name         = each.value.name
  program_text = each.value.program
  time_range   = 3600
  plot_type    = "LineChart"
}
resource "signalfx_dashboard" "ukinfo" {
  name            = "UK-Info service health"
  dashboard_group = signalfx_dashboard_group.ukinfo.id
  time_range      = "-1h"
  dynamic "chart" {
    for_each = signalfx_time_chart.ukinfo
    content {
      chart_id = chart.value.id
      width    = 12
      height   = 1
    }
  }
}
resource "signalfx_detector" "pool" {
  name         = "UK-Info sustained pool saturation"
  description  = "Warning after five minutes above 80%; see docs/observability.md."
  program_text = <<-SIGNALFLOW
    used = data('ukinfo_pool_acquired').sum()
    maximum = data('ukinfo_pool_max').sum()
    detect(when(used / maximum > 0.8, '5m')).publish('Pool saturation')
  SIGNALFLOW
  rule {
    detect_label  = "Pool saturation"
    severity      = "Warning"
    notifications = var.notifications
  }
}
output "dashboard_id" {
  value = signalfx_dashboard.ukinfo.id
}
