resource "ripeatlas_measurement" "test" {
  name     = "test-traceroute"
  target   = "8.8.8.8"
  msm_type = "traceroute"
  af       = 6
  snapshot = "SNAPSHOT_PATH"

  cohort = {
    name                = "default"
    probe_count         = 5
    max_probes_per_cell = 2
    interval_seconds    = 3600
    cfg = {
      countries = { "US" = 5, "GB" = 3, "DE" = 3 }
    }
  }
}
