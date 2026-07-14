---
layout: "ripe-atlas"
page_title: "ripe-atlas: ripeatlas_measurement"
sidebar_current: "docs-ripe-atlas-resource-measurement"
description: |-
  Manages a RIPE Atlas measurement.
---

# ripeatlas_measurement

Creates and manages a single RIPE Atlas measurement. Each resource maps to one `(name, cohort)` pair and one measurement ID on the RIPE Atlas platform.

Structural attributes (`name`, `cohort`, `target`, `msm_type`, `af`, `interval_seconds`) are immutable. Changing any of them destroys the old measurement and creates a new one. `probe_ids` is mutable in place: changing the set adds or removes probe participants on the running measurement without recreating it.

## Example Usage

```hcl
data "ripeatlas_probe_selection" "selected" {
  snapshot = "${path.module}/snapshot.json"
  config   = "${path.module}/atlasctl.yaml"
}

resource "ripeatlas_measurement" "dns_canary_high" {
  name             = "dns-canary"
  cohort           = "high-freq"
  target           = "canary.example.com"
  msm_type         = "dns"
  af               = 4
  interval_seconds = 60
  probe_ids        = data.ripeatlas_probe_selection.selected.probe_ids["high-freq"]
}

output "msm_id" {
  value = ripeatlas_measurement.dns_canary_high.msm_id
}
```

## Argument Reference

The following arguments are supported:

* `name` - (Required, Forces new resource) Logical measurement name, for example `dns-canary`. Used together with `cohort` to identify the measurement within atlasctl state.

* `cohort` - (Required, Forces new resource) Probe group name, for example `high-freq`. Must match a cohort name defined in `atlasctl.yaml` when probe IDs come from the `ripeatlas_probe_selection` data source.

* `target` - (Required, Forces new resource) The measurement target. A DNS name or IP address depending on `msm_type`.

* `msm_type` - (Required, Forces new resource) Measurement type. One of `dns`, `ping`, `tls`, or `traceroute`.

* `interval_seconds` - (Required, Forces new resource) Measurement interval in seconds. Minimum value is `60`. All measurements created by this provider are periodic.

* `probe_ids` - (Required) Set of RIPE Atlas probe IDs to include in the measurement. This attribute is mutable: adding or removing probe IDs calls `AddParticipants` or `RemoveParticipants` on the running measurement rather than recreating it.

* `af` - (Optional, Forces new resource) Address family. `4` for IPv4, `6` for IPv6. Defaults to `4`.

## Attributes Reference

In addition to the arguments above, the following attributes are exported:

* `msm_id` - The RIPE Atlas measurement ID assigned at creation. Stable for the lifetime of the resource.

## Credit Costs

Credits are consumed per result per measurement cycle. Approximate costs:

| Type | Credits per result |
|------|-------------------|
| `dns` | 10 |
| `tls` | 10 |
| `ping` | 3 |
| `traceroute` | 30 |

Total hourly cost is `(3600 / interval_seconds) * credits_per_result * len(probe_ids)`.
