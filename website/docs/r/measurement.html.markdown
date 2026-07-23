---
layout: "ripe-atlas"
page_title: "ripe-atlas: ripeatlas_measurement"
sidebar_current: "docs-ripe-atlas-resource-measurement"
description: |-
  Manages a set of RIPE Atlas measurements grouped by cohort.
---

# ripeatlas_measurement

Creates and manages one or more RIPE Atlas measurements under a single resource. Each cohort in the `cohorts` list corresponds to exactly one RIPE Atlas measurement ID.

Probe selection runs locally during `terraform plan`. All cohorts are passed to the selection algorithm in a single call. Probes selected for cohort N are excluded from cohort N+1, so successive cohorts draw from a diminishing pool. Cohort ordering in the list determines selection priority.

Immutable attributes (`name`, `target`, `msm_type`, `af`, and all cohort structural fields) trigger replacement of the affected measurement when changed. Adding or removing a cohort creates or stops only that cohort's measurement without affecting others. Changes to `exclude_tags` or cohort scoring fields re-run probe selection on the next plan; the resulting diff drives `AddParticipants` or `RemoveParticipants` on the running measurement without recreating it.

## Example Usage

```hcl
resource "ripeatlas_measurement" "dns_canary" {
  name     = "dns-canary"
  target   = "canary.example.com"
  msm_type = "dns"
  af       = 4

  exclude_tags = ["broken", "system-flakey-connection"]

  cohorts = [
    {
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
    },
    {
      name                = "low-freq"
      probe_count         = 100
      max_probes_per_cell = 2
      interval_seconds    = 3600
    },
  ]
}

output "msm_ids" {
  value = [for c in ripeatlas_measurement.dns_canary.cohorts : c.msm_id]
}

output "probe_ids" {
  value = [for c in ripeatlas_measurement.dns_canary.cohorts : c.probe_ids]
}
```

## Argument Reference

### Resource-level

* `name` - (Required, Forces new resource) Measurement name. Used together with each cohort name as the atlasctl identity key.

* `target` - (Required, Forces new resource) DNS name or IP address for the measurement target.

* `msm_type` - (Required, Forces new resource) Measurement type. One of `dns`, `ping`, `tls`, or `traceroute`.

* `af` - (Optional, Forces new resource) Address family. `4` for IPv4, `6` for IPv6. Defaults to `4`.

* `exclude_tags` - (Optional) List of probe tags that hard-exclude a probe from selection. A probe carrying any listed tag is never selected across any cohort.

### `cohorts` list (required, one or more items)

Each item in the list defines one cohort and creates one RIPE Atlas measurement.

* `name` - (Required, Forces new resource) Cohort tier name, for example `high-freq`.

* `probe_count` - (Required, Forces new resource) Number of probes to select.

* `max_probes_per_cell` - (Required, Forces new resource) Maximum probes per H3 geographic cell.

* `interval_seconds` - (Required, Forces new resource) Measurement interval in seconds. Minimum `60`.

* `include_probe_ids` - (Optional) Set of probe IDs always included in the cohort, regardless of scoring or the H3 cell cap.

* `exclude_probe_ids` - (Optional) Set of probe IDs never selected in this cohort.

### `cohorts[*].cfg` block (optional)

Additive scoring weights applied on top of each probe's base score of 1. A probe matching multiple criteria accumulates all matching scores.

* `asn` - (Optional) Map of ASN (string key) to score bonus.

* `tags` - (Optional) Map of probe tag string to score bonus.

* `countries` - (Optional) Map of ISO 3166-1 alpha-2 country code to score bonus.

* `stability` - (Optional) Map of RIPE Atlas stability tag to score bonus.

## Attributes Reference

The following computed attributes are exported on each cohort item:

* `cohorts[*].msm_id` - The RIPE Atlas measurement ID assigned at creation. Stable for the lifetime of the cohort.

* `cohorts[*].probe_ids` - Set of probe IDs selected and participating in the measurement. Computed from probe selection at plan time.

## Provider Configuration

The `ripeatlas` provider accepts the following arguments:

* `api_key` - (Optional) RIPE Atlas API key. Sensitive; never written in plain text to state. Falls back to `RIPE_ATLAS_API_KEY` environment variable.

* `snapshot` - (Optional) Path to `snapshot.json` produced by `atlasctl refresh`. Falls back to `RIPE_ATLAS_SNAPSHOT` environment variable.

* `namespace` - (Optional) Identifier embedded in each measurement description on the RIPE Atlas API. Used to distinguish measurements created by different Terraform states or workspaces. Defaults to `terraform-provider-ripe-atlas`.

```hcl
provider "ripeatlas" {
  # api_key    = "..."  # or set RIPE_ATLAS_API_KEY
  # snapshot   = "..."  # or set RIPE_ATLAS_SNAPSHOT
  # namespace = "..."  # optional
}
```

## Credit Costs

Credits are consumed per result per measurement cycle. Approximate costs:

| Type | Credits per result |
|------|-------------------|
| `dns` | 10 |
| `tls` | 10 |
| `ping` | 3 |
| `traceroute` | 30 |

Total hourly cost is `(3600 / interval_seconds) * credits_per_result * len(probe_ids)` per cohort.
