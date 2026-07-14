---
layout: "ripe-atlas"
page_title: "ripe-atlas: ripeatlas_probe_selection"
sidebar_current: "docs-ripe-atlas-datasource-probe-selection"
description: |-
  Selects RIPE Atlas probes from a local snapshot using cohort definitions in atlasctl.yaml.
---

# ripeatlas_probe_selection

Runs the atlasctl probe selection algorithm during `terraform plan` and exposes the results as probe ID sets keyed by cohort name. The sets can be passed directly to `ripeatlas_measurement` resources.

Selection is holistic: all cohorts defined in the config file are assigned in a single pass and the resulting sets are non-overlapping. Run `atlasctl refresh` to update the snapshot before planning when you want to pick up changes in the probe pool.

## Example Usage

`atlasctl.yaml`:

```yaml
cohorts:
  - name: high-freq
    count: 30
    max_probes_per_cell: 1
  - name: low-freq
    count: 100
    max_probes_per_cell: 3

scoring:
  asn:
    7018: 10   # AT&T
    7922: 8    # Comcast
  tags:
    office: 5
    fibre: 2
  stability:
    system-ipv4-stable-90d: 5

exclude_tags:
  - broken
  - system-flakey-connection
```

`main.tf`:

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
  interval_seconds = 60
  probe_ids        = data.ripeatlas_probe_selection.selected.probe_ids["high-freq"]
}

resource "ripeatlas_measurement" "dns_canary_low" {
  name             = "dns-canary"
  cohort           = "low-freq"
  target           = "canary.example.com"
  msm_type         = "dns"
  interval_seconds = 900
  probe_ids        = data.ripeatlas_probe_selection.selected.probe_ids["low-freq"]
}
```

## Refreshing the Snapshot

The data source reads probes from a local `snapshot.json` file. This file is managed outside Terraform using `atlasctl`:

```bash
atlasctl refresh   # fetch connected probes from the RIPE Atlas API
terraform plan     # selection runs here, probe IDs propagate to measurements
```

Committing `snapshot.json` to version control gives reproducible probe selection until you explicitly refresh.

## Argument Reference

* `snapshot` - (Required) Path to a local `snapshot.json` file produced by `atlasctl refresh`.

* `config` - (Required) Path to an `atlasctl.yaml` configuration file. The `cohorts`, `scoring`, and `exclude_tags` stanzas are used. Any `measurements` stanzas in the file are ignored; declare measurements as `ripeatlas_measurement` resources in HCL instead.

## Attributes Reference

* `probe_ids` - Map from cohort name to a set of RIPE Atlas probe IDs. Keys match the cohort `name` values defined in `atlasctl.yaml`. Example: `probe_ids["high-freq"]` returns the probe IDs assigned to the `high-freq` cohort.

## Selection Algorithm

Probe selection uses scoring bands, continental interleaving, and H3-based geographic diversity to produce a globally distributed, stable cohort. Full details are in the [atlasctl probe selection documentation](https://github.com/supabase/atlasctl/blob/main/README.md#probe-selection).
