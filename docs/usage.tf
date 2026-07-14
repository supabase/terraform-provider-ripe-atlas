terraform {
  required_providers {
    ripe-atlas = {
      source  = "supabase/ripe-atlas"
      version = "~> 0.1"
    }
  }
}

provider "ripe-atlas" {
  # api_key = "..."  # or set RIPE_ATLAS_API_KEY
}

data "ripe_atlas_probe_selection" "selected" {
  snapshot = "${path.module}/snapshot.json"
  config   = "${path.module}/atlasctl.yaml"
}

resource "ripe_atlas_measurement" "dns_canary" {
  name             = "dns-canary"
  cohort           = "high-freq"
  target           = "canary.example.com"
  msm_type         = "dns"
  af               = 4
  interval_seconds = 60
  probe_ids        = data.ripe_atlas_probe_selection.selected.probe_ids["high-freq"]
}
