resource "ripeatlas_measurement" "test" {
  name     = "test-tls"
  target   = "example.com"
  msm_type = "tls"
  af       = 4
  cohorts = [
    {
      name                = "default"
      probe_count         = 5
      max_probes_per_cell = 2
      interval_seconds    = 900
    }
  ]
}
