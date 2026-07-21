resource "ripeatlas_measurement" "test" {
  name     = "test"
  target   = ""
  msm_type = "dns"
  cohorts = [
    {
      name                = "default"
      probe_count         = 5
      max_probes_per_cell = 2
      interval_seconds    = 60
    }
  ]
}
