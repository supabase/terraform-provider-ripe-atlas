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

Immutable attributes (`name`, `target`, `msm_type`, `af`, the HTTP-specific fields, and all cohort structural fields) trigger replacement of the affected measurement when changed. Adding or removing a cohort creates or stops only that cohort's measurement without affecting others. Changes to cohort `cfg` fields re-run probe selection on the next plan; the resulting diff drives `AddParticipants` or `RemoveParticipants` on the running measurement without recreating it.

## Example Usage

```hcl
resource "ripeatlas_measurement" "dns_canary" {
  name     = "dns-canary"
  target   = "canary.example.com"
  msm_type = "dns"
  af       = 4

  cohorts = [
    {
      name                = "high-freq"
      probe_count         = 30
      max_probes_per_cell = 1
      interval_seconds    = 60
      include_probe_ids   = [1001]
      exclude_probe_ids   = [9999]
      cfg = {
        exclude_tags = ["broken", "system-flakey-connection"]
        asn          = { "7018" = 10, "7922" = 8 }
        tags         = { "office" = 5, "fibre" = 2 }
        stability    = { "system-ipv4-stable-90d" = 5 }
      }
    },
    {
      name                = "low-freq"
      probe_count         = 100
      max_probes_per_cell = 2
      interval_seconds    = 3600
      cfg = {
        exclude_tags  = ["broken"]
        h3_resolution = 5
      }
    },
  ]
}

resource "ripeatlas_measurement" "http_check" {
  name         = "http-check"
  target       = "api.example.com"
  msm_type     = "http"
  af           = 4
  http_method  = "HEAD"
  http_path    = "/health"

  cohorts = [
    {
      name                = "global"
      probe_count         = 50
      max_probes_per_cell = 2
      interval_seconds    = 300
    },
  ]
}

output "msm_ids" {
  value = [for c in ripeatlas_measurement.dns_canary.cohorts : c.msm_id]
}

output "probe_ids" {
  value = [for c in ripeatlas_measurement.dns_canary.cohorts : c.probe_ids]
}

output "credit_burn" {
  value = {
    hourly = ripeatlas_measurement.dns_canary.total_hourly_credits
    daily  = ripeatlas_measurement.dns_canary.total_daily_credits
  }
}
```

## Argument Reference

### Resource-level

* `name` - (Required, Forces new resource) Measurement name. Used together with each cohort name as the atlasctl identity key.

* `target` - (Required, Forces new resource) DNS name or IP address for the measurement target.

* `msm_type` - (Required, Forces new resource) Measurement type. One of `dns`, `ping`, `tls`, `traceroute`, or `http`.

* `af` - (Optional, Forces new resource) Address family. `4` for IPv4, `6` for IPv6. Defaults to `4`.

* `http_method` - (Optional, Forces new resource) HTTP request method: `GET`, `HEAD` (default), or `POST`. Only valid when `msm_type` is `http`.

* `http_path` - (Optional, Forces new resource) URL path for the HTTP request. Defaults to `/`. Only valid when `msm_type` is `http`.

* `http_port` - (Optional, Forces new resource) TCP port for the HTTP request. Defaults to `80`. Only valid when `msm_type` is `http`.

* `http_version` - (Optional, Forces new resource) HTTP protocol version. One of `"1.0"` or `"1.1"`. Only valid when `msm_type` is `http`.

### Option blocks

Each block is an optional object. Unset attributes are omitted from the RIPE Atlas request and the API default applies. Changing any value in a block forces a new measurement. The `ping`, `traceroute`, `dns`, `tls` and `http` blocks are only valid when `msm_type` matches the block name.

* `common` - (Optional, Forces new resource) Valid for every type.
  * `resolve_on_probe` - (Optional) Resolve the target on each probe instead of once centrally.
  * `spread` - (Optional) Seconds over which probe start times are spread within one interval.
  * `skip_dns_check` - (Optional) Skip the platform's DNS check of the target at creation time.
  * `target_update_hours` - (Optional) Re-resolve the target every N hours.
  * `auto_topup` - (Optional) Replace probes that go offline. Implied by auto_topup_prb_days_off and auto_topup_prb_similarity.
  * `auto_topup_prb_days_off` - (Optional) Days a probe is offline before it is replaced.
  * `auto_topup_prb_similarity` - (Optional) How similar a replacement probe must be, between 0 and 1.
  * `start_time`, `stop_time` - (Optional) RFC 3339 timestamps. `stop_time` must be after `start_time`.
  * `bill_to` - (Optional) Charge credits to another RIPE Atlas account instead of your own. That account must have added you to its allowed billing list.

* `ping` - (Optional, Forces new resource) Only valid when `msm_type` is `ping`.
  * `packets` - (Optional) Packets per result. Platform default 3.
  * `packet_interval` - (Optional) Milliseconds between the packets of one result.
  * `include_probe_id` - (Optional) Put the probe ID (as ASCII digits) in the packet payload.

* `traceroute` - (Optional, Forces new resource) Only valid when `msm_type` is `traceroute`.
  * `protocol` - (Optional) ICMP, UDP or TCP. atlasctl defaults to UDP; the platform default is ICMP.
  * `response_timeout` - (Optional) Milliseconds to wait for a reply to each packet. Platform default 4000.
  * `packets` - (Optional) Packets sent per hop. Platform default 3.
  * `paris` - (Optional) Number of distinct flow identifiers to cycle through so load balanced paths are explored consistently (Paris traceroute). Platform default 16.
  * `first_hop` - (Optional) TTL of the first hop probed. Platform default 1.
  * `max_hops` - (Optional) TTL of the last hop probed. Platform default 32.
  * `destination_option_size` - (Optional) Size in bytes of an IPv6 destination options extension header added to each packet. Platform default 0 (none).
  * `hop_by_hop_option_size` - (Optional) Size in bytes of an IPv6 hop by hop options extension header added to each packet. Platform default 0 (none).
  * `dont_fragment` - (Optional) Set the don't fragment bit on outgoing packets. Platform default false.

* `dns` - (Optional, Forces new resource) Only valid when `msm_type` is `dns`.
  * `query_type` - (Optional) Record type: A (default), AAAA, NS, MX, TXT, and so on.
  * `query_class` - (Optional) IN (default) or CHAOS.
  * `protocol` - (Optional) UDP (default) or TCP.
  * `use_macros` - (Optional) Expand $p (probe ID), $r (random string) and $t (timestamp) in the target, for example to build unique names that bypass resolver caches.
  * `set_nsid_bit` - (Optional) Ask the server to identify itself with the EDNS NSID option (RFC 5001), commonly used to see which anycast instance answered.
  * `include_qbuf` - (Optional) Include the query packet sent by the probe in each result.
  * `prepend_probe_id` - (Optional) Prefix the queried name with the probe ID, giving a unique name per probe.
  * `set_rd_bit` - (Optional) Set the recursion desired bit.
  * `set_do_bit` - (Optional) Set the DNSSEC OK bit.
  * `udp_payload_size` - (Optional) Maximum UDP response size in bytes advertised through EDNS0. Platform default 512.
  * `retry` - (Optional) Times a query is retried after a failure. Platform default 0.
  * `timeout` - (Optional) Milliseconds to wait for a response before the query counts as failed. Platform default 5000.

* `tls` - (Optional, Forces new resource) Only valid when `msm_type` is `tls`.
  * `port` - (Optional) TCP port. Platform default 443.
  * `hostname` - (Optional) Server name sent as SNI. Useful when the target is an IP address.

* `http` - (Optional, Forces new resource) Additional HTTP options. Only valid when `msm_type` is `http`. These add to the `http_*` attributes above.
  * `query_string` - (Optional) Appended to the request path (without the leading "?").
  * `header_bytes` - (Optional) Maximum number of response header bytes to record in the result.
  * `extended_timing` - (Optional) Report additional timing detail in each result (for example time to resolve and time to first byte).
  * `more_extended_timing` - (Optional) Report even more timing detail than extended_timing.

Example:

```hcl
resource "ripeatlas_measurement" "dns_mx" {
  name     = "dns-mx"
  target   = "example.com"
  msm_type = "dns"

  common = {
    resolve_on_probe = true
    spread           = 60
  }

  dns = {
    query_type = "MX"
    set_rd_bit = true
  }

  cohorts = [
    { name = "global", probe_count = 50, max_probes_per_cell = 2, interval_seconds = 300 },
  ]
}
```

### `cohorts` list (required, one or more items)

Each item in the list defines one cohort and creates one RIPE Atlas measurement.

* `name` - (Required, Forces new resource) Cohort tier name, for example `high-freq`.

* `description` - (Optional) Free-form description for this cohort, carried through to the measurement output unchanged (for example, to tag downstream systems like Grafana). Defaults to an empty string.

* `probe_count` - (Required, Forces new resource) Number of probes to select.

* `max_probes_per_cell` - (Required, Forces new resource) Maximum probes per H3 geographic cell.

* `interval_seconds` - (Required, Forces new resource) Measurement interval in seconds. Minimum `60`.

* `include_probe_ids` - (Optional) Set of probe IDs always included in the cohort. Pinned probes bypass tag exclusions, H3 cell caps, and inter-cohort drawdown. The only reason a pinned probe is skipped is if it is absent from the snapshot.

* `exclude_probe_ids` - (Optional) Set of probe IDs never selected in this cohort.

### `cohorts[*].cfg` block (optional)

Controls hard exclusion, geographic diversity, and soft scoring weights.

* `exclude_tags` - (Optional) List of probe tags that hard-exclude a probe from this cohort. A probe carrying any listed tag is never selected. Each cohort maintains its own exclusion list, so different cohorts can target different probe populations.

* `h3_resolution` - (Optional) H3 cell resolution for geographic diversity, between `1` and `15`. Defaults to `3` (state or province granularity, roughly 12,000 km2 per cell). Increase for finer geographic spread.

* `asn` - (Optional) Map of ASN (string key) to score bonus.

* `tags` - (Optional) Map of probe tag string to score bonus.

* `countries` - (Optional) Map of ISO 3166-1 alpha-2 country code to score bonus.

* `stability` - (Optional) Map of RIPE Atlas stability tag to score bonus.

Additive scoring weights apply on top of each probe's base score of 1. A probe matching multiple criteria accumulates all matching scores.

## Attributes Reference

### Resource-level computed attributes

* `total_hourly_credits` - Projected RIPE Atlas credit burn per hour, summed across all cohorts. Available as a known value during `pulumi preview` and `terraform plan`.

* `total_daily_credits` - Projected RIPE Atlas credit burn per day, summed across all cohorts. Available as a known value during `pulumi preview` and `terraform plan`.

### Per-cohort computed attributes

The following are exported on each cohort item:

* `cohorts[*].msm_id` - The RIPE Atlas measurement ID assigned at creation. Stable for the lifetime of the cohort.

* `cohorts[*].probe_ids` - Set of probe IDs selected and participating in the measurement. Computed from probe selection at plan time.

* `cohorts[*].hourly_credits` - Projected RIPE Atlas credit burn per hour for this cohort. Computed at plan time.

* `cohorts[*].daily_credits` - Projected RIPE Atlas credit burn per day for this cohort. Computed at plan time.

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
| `http` | 3 |

Total hourly cost is `(3600 / interval_seconds) * credits_per_result * len(probe_ids)` per cohort. Use `total_hourly_credits` and `total_daily_credits` as stack outputs to monitor projected spend.
