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
    latency = {
      name    = "API p95 latency by route (seconds, bucket estimate)"
      program = "histogram('ukinfo_api_request_duration_seconds').percentile(pct=95, by=['route']).publish(label='p95 seconds')"
    }
    phase_latency = {
      name    = "Search p95 stage latency (seconds, bucket estimate)"
      program = "histogram('ukinfo_search_phase_duration_seconds').percentile(pct=95, by=['phase']).publish(label='Stage p95 seconds')"
    }
    search_backend = {
      name    = "Search plan selection / second"
      program = "data('ukinfo_address_plans_total', rollup='rate').sum(by=['plan']).publish(label='Plans/s')"
    }
    parade_pool = {
      name    = "Derived search pool utilisation (%)"
      program = "used = data('ukinfo_parade_pool_acquired').sum()\nmaximum = data('ukinfo_parade_pool_max').sum()\n(100 * used / maximum).publish(label='Derived pool utilisation')"
    }
    parade_waits = {
      name    = "Derived search pool waits / second"
      program = "data('ukinfo_parade_pool_waits_total', rollup='rate').sum().publish(label='Waited acquires/s')\ndata('ukinfo_parade_pool_canceled_acquires_total', rollup='rate').sum().publish(label='Canceled acquires/s')"
    }
    errors = {
      name    = "Search error percentage"
      program = "total = data('ukinfo_query_requests_total', rollup='rate').sum()\nfailed = data('ukinfo_query_requests_total', filter=filter('status', '5*'), rollup='rate').sum().fill(0)\n(100 * failed / total).publish(label='Search errors %')"
    }
    timeouts = {
      name    = "Classified API failures / second"
      program = "data('ukinfo_api_failures_total', rollup='rate').sum(by=['reason']).publish(label='Failures/s')"
    }
    waits = {
      name    = "Pool waits and canceled acquisitions / second"
      program = "data('ukinfo_pool_waits_total', rollup='rate').sum().publish(label='Successful waited acquires/s')\ndata('ukinfo_pool_canceled_acquires_total', rollup='rate').sum().publish(label='Canceled acquires/s')"
    }
    wait_latency = {
      name    = "Mean successful pool wait (seconds)"
      program = "seconds = data('ukinfo_pool_wait_seconds_total', rollup='rate').sum()\nwaits = data('ukinfo_pool_waits_total', rollup='rate').sum()\n(seconds / waits).publish(label='Mean waited acquire seconds')"
    }
    heartbeat = {
      name    = "Telemetry heartbeat (expected pool size)"
      program = "data('ukinfo_pool_max', extrapolation='null').sum().publish(label='Pool maximum')"
    }
    regression_status = {
      name    = "Private regression status (last pass / running)"
      program = "data('ukinfo_regression_last_run_passed').max().publish(label='Last run passed')\ndata('ukinfo_regression_running').max().publish(label='Running')"
    }
    regression_cold = {
      name    = "Private regression source-cold maximum (seconds)"
      program = "data('ukinfo_regression_source_cold_max_seconds').max().publish(label='Original source-cold maximum')\ndata('ukinfo_regression_coverage_source_cold_max_seconds').max().publish(label='Expanded nearby source-cold maximum')"
    }
    regression_c4 = {
      name    = "Private regression C4 p95 (seconds)"
      program = "data('ukinfo_regression_candidate_c4_p95_seconds').max().publish(label='Candidate C4 p95')\ndata('ukinfo_regression_baseline_c4_p95_seconds').max().publish(label='Previous build C4 p95')"
    }
    regression_overdue = {
      name    = "Private regression overdue (seconds)"
      program = "data('ukinfo_regression_overdue_seconds').max().publish(label='Overdue seconds')"
    }
    decoded = {
      name    = "Decoded record blocks / second"
      program = "data('ukinfo_record_blocks_decoded_total', rollup='rate').sum().publish(label='Decoded blocks/s')"
    }
  }
}

resource "signalfx_detector" "regression_failed" {
  name         = "UK-Info private regression failed"
  description  = "The private NAS correctness/performance protocol failed or exceeded one hour. Inspect aggregate state before opening private evidence."
  program_text = "failed = data('ukinfo_regression_failed').max()\ndetect(when(failed > 0, '5m')).publish('Regression failed')"
  rule {
    detect_label  = "Regression failed"
    severity      = "Warning"
    notifications = var.notifications
  }
}

resource "signalfx_detector" "regression_overdue" {
  name         = "UK-Info private regression overdue"
  description  = "The weekly Sunday 03:00 UTC run has been deferred for over two hours. Check public traffic, free space, memory and the private fixture lock."
  program_text = "overdue = data('ukinfo_regression_overdue_seconds').max()\ndetect(when(overdue > 7200, '10m')).publish('Regression overdue')"
  rule {
    detect_label  = "Regression overdue"
    severity      = "Warning"
    notifications = var.notifications
  }
}

resource "signalfx_detector" "regression_telemetry" {
  name         = "UK-Info private regression telemetry missing"
  description  = "The isolated regression coordinator has not reported for fifteen minutes."
  program_text = <<-SIGNALFLOW
    from signalfx.detectors.not_reporting import not_reporting
    heartbeat = data('ukinfo_regression_heartbeat', extrapolation='null')
    not_reporting.detector(heartbeat, duration=duration('15m')).publish('Missing regression telemetry')
  SIGNALFLOW
  rule {
    detect_label  = "Missing regression telemetry"
    severity      = "Warning"
    notifications = var.notifications
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

resource "signalfx_detector" "telemetry" {
  name         = "UK-Info telemetry stopped"
  description  = "No pool heartbeat for five minutes after a stream has been observed. Does not distinguish API, Collector, network or NAS failure."
  program_text = <<-SIGNALFLOW
    from signalfx.detectors.not_reporting import not_reporting
    heartbeat = data('ukinfo_pool_max', extrapolation='null')
    not_reporting.detector(heartbeat, duration=duration('5m')).publish('Missing telemetry')
  SIGNALFLOW
  rule {
    detect_label  = "Missing telemetry"
    severity      = "Critical"
    notifications = var.notifications
  }
}
