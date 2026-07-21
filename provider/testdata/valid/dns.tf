resource "ripeatlas_measurement" "test" {
  name     = "test-dns"
  target   = "example.com"
  msm_type = "dns"
  af       = 4

  exclude_tags = ["broken"]

  cohorts = [
    {
      name                = "default"
      probe_count         = 5
      max_probes_per_cell = 2
      interval_seconds    = 60
      cfg = {
        asn       = { "7018" = 10, "7922" = 8 }
        stability = { "system-ipv4-stable-90d" = 5 }
      }
    }
  ]
}
