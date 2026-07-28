terraform {
  required_providers {
    ripeatlas = {
      source  = "supabase/ripe-atlas"
      version = "~> 0.1"
    }
  }
}

provider "ripeatlas" {
  # api_key    = "..."  # or set RIPE_ATLAS_API_KEY
  # snapshot   = "..."  # or set RIPE_ATLAS_SNAPSHOT
  # namespace = "..."  # optional; distinguishes measurements across states/workspaces
}

resource "ripeatlas_measurement" "dns_canary" {
  name     = "dns-canary"
  target   = "canary.example.com"
  msm_type = "dns"
  af       = 4

  cohorts = [
    {
      name                = "high-freq"
      probe_count         = 30
      max_probes_per_cell = 1
      interval_seconds    = 60
      cfg = {
        exclude_tags = ["broken", "system-flakey-connection"]
        asn          = { "7018" = 10, "7922" = 8 }
        tags         = { "office" = 5, "fibre" = 2 }
        stability    = { "system-ipv4-stable-90d" = 5 }
      }
    },
    {
      name                = "low-freq"
      probe_count         = 100
      max_probes_per_cell = 2
      interval_seconds    = 3600
      cfg = {
        exclude_tags  = ["broken"]
        h3_resolution = 5
      }
    },
  ]
}

resource "ripeatlas_measurement" "http_check" {
  name         = "http-check"
  target       = "api.example.com"
  msm_type     = "http"
  af           = 4
  http_method  = "HEAD"
  http_path    = "/health"
  http_port    = 443
  http_version = "1.1"

  cohorts = [
    {
      name                = "global"
      probe_count         = 50
      max_probes_per_cell = 2
      interval_seconds    = 300
    },
  ]
}

output "msm_ids" {
  value = [for c in ripeatlas_measurement.dns_canary.cohorts : c.msm_id]
}

output "probe_ids" {
  value = [for c in ripeatlas_measurement.dns_canary.cohorts : c.probe_ids]
}

output "credit_burn" {
  value = {
    hourly = ripeatlas_measurement.dns_canary.total_hourly_credits
    daily  = ripeatlas_measurement.dns_canary.total_daily_credits
  }
}
