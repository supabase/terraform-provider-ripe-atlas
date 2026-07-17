---
layout: "ripe-atlas"
page_title: "ripe-atlas: ripeatlas_measurement"
sidebar_current: "docs-ripe-atlas-resource-measurement"
description: |-
  Manages a RIPE Atlas measurement.
---

# ripeatlas_measurement

Creates and manages a single RIPE Atlas measurement. Each resource maps to one `(name, cohort)` pair and one measurement ID on the RIPE Atlas platform.

Probe selection is declared directly in the resource using the `cohort` block. The provider reads a local `snapshot.json` produced by `atlasctl refresh`, scores and filters probes according to the cohort configuration, and stores the selected probe IDs in state.

Immutable attributes (`name`, `target`, `msm_type`, `af`, `cohort.name`, `cohort.interval_seconds`) trigger replacement when changed. All other attributes are mutable in place: changes to `snapshot`, `exclude_tags`, or cohort selection fields re-run probe selection on the next plan, and the resulting diff drives `AddParticipants` or `RemoveParticipants` on the running measurement without recreating it.

## Example Usage

```hcl
resource "ripeatlas_measurement" "dns_canary_high" {
  name     = "dns-canary"
  target   = "canary.example.com"
  msm_type = "dns"
  af       = 4
  snapshot = "${path.module}/snapshot.json"

  exclude_tags = ["broken", "system-flakey-connection"]

  cohort = {
    name                = "high-freq"
    probe_count         = 30
    max_probes_per_cell = 1
    interval_seconds    = 60
    include_probe_ids   = [1001]
    exclude_probe_ids   = [9999]

    cfg = {
      asn       = { "7018" = 10, "7922" = 8 }
      tags      = { "office" = 5, "fibre" = 2 }
      stability = { "system-ipv4-stable-90d" = 5 }
    }
  }
}

output "msm_id" {
  value = ripeatlas_measurement.dns_canary_high.msm_id
}

output "probe_ids" {
  value = ripeatlas_measurement.dns_canary_high.probe_ids
}
```

## Argument Reference

### Resource-level

* `name` - (Required, Forces new resource) Measurement name. Combined with `cohort.name` as the atlasctl identity key.

* `target` - (Required, Forces new resource) DNS name or IP address for the measurement target.

* `msm_type` - (Required, Forces new resource) Measurement type. One of `dns`, `ping`, `tls`, or `traceroute`.

* `snapshot` - (Required) Path to a `snapshot.json` produced by `atlasctl refresh`. Updated on each plan to pick up probe pool changes.

* `af` - (Optional, Forces new resource) Address family. `4` for IPv4, `6` for IPv6. Defaults to `4`.

* `exclude_tags` - (Optional) List of probe tags that hard-exclude a probe from selection. A probe carrying any listed tag is never selected.

### `cohort` block (required, exactly one)

* `name` - (Required, Forces new resource) Cohort tier name, for example `high-freq`.

* `probe_count` - (Required) Number of probes to select.

* `max_probes_per_cell` - (Required) Maximum probes per H3 geographic cell.

* `interval_seconds` - (Required, Forces new resource) Measurement interval in seconds. Minimum `60`.

* `include_probe_ids` - (Optional) Set of probe IDs always included in the cohort, regardless of scoring or the H3 cell cap.

* `exclude_probe_ids` - (Optional) Set of probe IDs never selected in this cohort.

### `cohort.cfg` block (optional)

Additive scoring weights applied on top of each probe's base score of 1. A probe matching multiple criteria accumulates all matching scores.

* `asn` - (Optional) Map of ASN (string key) to score bonus.

* `tags` - (Optional) Map of probe tag string to score bonus.

* `countries` - (Optional) Map of ISO 3166-1 alpha-2 country code to score bonus.

* `stability` - (Optional) Map of RIPE Atlas stability tag to score bonus.

## Attributes Reference

In addition to the arguments above, the following computed attributes are exported:

* `msm_id` - The RIPE Atlas measurement ID assigned at creation. Stable for the lifetime of the resource.

* `probe_ids` - Set of probe IDs selected and participating in the measurement. Computed from probe selection at plan time.

## Credit Costs

Credits are consumed per result per measurement cycle. Approximate costs:

| Type | Credits per result |
|------|-------------------|
| `dns` | 10 |
| `tls` | 10 |
| `ping` | 3 |
| `traceroute` | 30 |

Total hourly cost is `(3600 / interval_seconds) * credits_per_result * len(probe_ids)`.
