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

  exclude_tags = ["broken", "system-flakey-connection"]

  cohorts = [
    {
      name                = "high-freq"
      probe_count         = 30
      max_probes_per_cell = 1
      interval_seconds    = 60
    },
  ]
}
