resource "ripeatlas_measurement" "test" {
  name     = "test-ping"
  target   = "8.8.8.8"
  msm_type = "ping"
  af       = 4
  cohorts = [
    {
      name                = "default"
      probe_count         = 5
      max_probes_per_cell = 2
      interval_seconds    = 240
    }
  ]
}
