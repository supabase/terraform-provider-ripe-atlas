resource "ripeatlas_measurement" "test" {
  name     = "test"
  target   = "example.com"
  msm_type = "dns"
}
