# Implementation Plan

## Overview

A Terraform provider that manages RIPE Atlas measurements as first-class resources. Operators
declare measurements in HCL, run `terraform plan` to see what will change (including which probes
will be added or removed), and `terraform apply` to execute against the RIPE Atlas API.

The provider is an adapter layer over `atlasctl/pkg/`. No domain logic lives here.

## Framework

`hashicorp/terraform-plugin-framework` (same as `terraform-provider-supabase`). The provider
binary speaks the Terraform plugin protocol over gRPC; the framework handles that entirely.

## Future path

This Terraform provider is intended as the canonical implementation. A Pulumi provider will be
derived from it using the Pulumi Terraform bridge (`pulumi-terraform-bridge`), which wraps the
provider binary and generates a Pulumi SDK automatically. No manual translation is needed.

## Repo layout

```
terraform-provider-ripe-atlas/
  main.go
  go.mod
  go.sum
  docs/
    plan.md             (this file)
  internal/
    provider/
      provider.go       provider struct and Configure
      measurement.go    ripe_atlas_measurement resource
      selection.go      ripe_atlas_probe_selection data source
```

`internal/provider/` keeps framework types out of `main.go` and away from any future test helpers.

## Provider configuration

```hcl
provider "ripe_atlas" {
  api_key = var.ripe_atlas_api_key   # or RIPE_ATLAS_API_KEY env var
}
```

The API key is the only provider-level config needed to construct `atlasapi.NewApplyClient` and
`atlasapi.MsmClient`. Everything else (snapshot path, config path) is per-resource or per-data-source.

## Data source: `ripe_atlas_probe_selection`

Runs `pkg/selection.Select` during `terraform plan`. Selection is holistic: cohorts are assigned
in definition order and are non-overlapping, so one data source invocation covers all cohorts
defined in the config file.

### Schema

| Attribute  | Type   | Required | Description                          |
|------------|--------|----------|--------------------------------------|
| `snapshot` | string | yes      | Path to local `snapshot.json`        |
| `config`   | string | yes      | Path to `atlasctl.yaml`              |
| `probe_ids`| map(set(number)) | computed | Probe IDs keyed by cohort name |

### Read logic

```
config.Load(inputs.config)
snapshot.Load(inputs.snapshot)
selection.Select(ctx, snap, cfg)
→ map each SelectedCohort.Cohort.Name → []uint32 probe IDs
→ write to state as probe_ids
```

### Example

```hcl
data "ripe_atlas_probe_selection" "all" {
  snapshot = "/path/to/snapshot.json"
  config   = "/path/to/atlasctl.yaml"
}
```

The snapshot is managed outside Terraform. Run `atlasctl refresh` to update it before planning
when you want a fresh probe pool.

## Resource: `ripe_atlas_measurement`

One resource maps to one `(name, cohort)` pair, which is exactly one RIPE Atlas measurement ID.

### Schema

| Attribute         | Type        | Required | Immutable | Description                              |
|-------------------|-------------|----------|-----------|------------------------------------------|
| `name`            | string      | yes      | yes       | Measurement name (e.g. `dns-canary`)     |
| `cohort`          | string      | yes      | yes       | Cohort name (e.g. `high-freq`)           |
| `target`          | string      | yes      | yes       | Measurement target host or IP            |
| `type`            | string      | yes      | yes       | One of `dns`, `ping`, `tls`, `traceroute`|
| `af`              | number      | no       | yes       | Address family: 4 or 6 (default 4)       |
| `interval_seconds`| number      | yes      | yes       | Measurement interval in seconds          |
| `probe_ids`       | set(number) | yes      | no        | Probe IDs to participate                 |
| `msm_id`          | number      | computed | n/a       | RIPE Atlas measurement ID (set on create)|

Immutable attributes use `RequiresReplace()` plan modifiers. Changing any of them triggers
Delete then Create (Terraform handles the sequencing).

`probe_ids` is mutable in-place. The Update method diffs the new set against state and calls
`AddParticipants`/`RemoveParticipants` on the running measurement.

### Typical HCL

```hcl
resource "ripe_atlas_measurement" "dns_canary_high" {
  name             = "dns-canary"
  cohort           = "high-freq"
  target           = "canary.supabase.co"
  type             = "dns"
  interval_seconds = 60
  probe_ids        = data.ripe_atlas_probe_selection.all.probe_ids["high-freq"]
}
```

### Create

```
atlasapi.NewApplyClient(apiKey)
client.CreateMeasurement(ctx, plan.MsmSpec{
    Key:      plan.MsmKey{Name: name, Cohort: cohort},
    Target:   target,
    Type:     plan.MsmType(type),
    AF:       af,
    Interval: interval_seconds,
    ProbeIDs: probe_ids,
})
→ store msm_id and probe_ids in state
```

### Read (drift detection)

```
msmClient.GetMeasurement(ctx, msm_id)
→ if plan.ErrMsmNotFound: signal resource must be recreated
→ otherwise: reconcile probe_ids from live API into state
```

### Update (probe_ids changed only)

```
added   = new_probe_ids - state_probe_ids
removed = state_probe_ids - new_probe_ids

if len(added) > 0:   client.AddParticipants(ctx, msm_id, added)
if len(removed) > 0: client.RemoveParticipants(ctx, msm_id, removed)
→ update probe_ids in state
```

Update is only reachable when `probe_ids` changes. All other attributes are immutable and trigger
replacement instead.

### Delete

```
client.StopMeasurement(ctx, msm_id)
```

## atlasctl dependency

`go.mod` will include a `replace` directive for local development:

```
require github.com/supabase/atlasctl v0.0.0

replace github.com/supabase/atlasctl => ../atlasctl
```

## Packages imported from atlasctl

| Package                              | Used for                                        |
|--------------------------------------|-------------------------------------------------|
| `pkg/config`                         | `config.Load` in the data source                |
| `pkg/snapshot`                       | `snapshot.Load` in the data source              |
| `pkg/selection`                      | `selection.Select` in the data source           |
| `pkg/plan`                           | `MsmSpec`, `MsmKey`, `MsmType`, `ErrMsmNotFound`|
| `pkg/atlasapi`                       | `ApplyClient`, `MsmClient`, `NewApplyClient`    |

## Implementation order

1. `go.mod` and module scaffold
2. `main.go` with provider server entry point
3. `internal/provider/provider.go` (provider struct, Configure, API key handling)
4. `internal/provider/selection.go` (data source: Read only)
5. `internal/provider/measurement.go` (resource: Create, Read, Update, Delete)
6. Manual smoke test against the RIPE Atlas API with `RIPE_ATLAS_API_KEY` set

## Out of scope (for now)

- S3-backed snapshot caching (can be added later; carries through the Pulumi bridge unchanged)
- `for_each` over measurements (callers do this at the HCL layer)
- Acceptance tests (requires a live API key and credits)
- Published registry release
