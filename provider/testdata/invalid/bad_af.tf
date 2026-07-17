resource "ripeatlas_measurement" "test" {
  name     = "test"
  target   = "example.com"
  msm_type = "dns"
  af       = 8
  snapshot = "SNAPSHOT_PATH"

  cohort = {
    name                = "default"
    probe_count         = 5
    max_probes_per_cell = 2
    interval_seconds    = 60
  }
}
