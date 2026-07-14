# Coding Plan

## Step 1: go.mod scaffold

Create `go.mod` for module `github.com/supabase/terraform-provider-ripe-atlas`.

Required dependencies:

- `github.com/hashicorp/terraform-plugin-framework`
- `github.com/supabase/atlasctl v0.0.0`

Replace directive for local development:

```
replace github.com/supabase/atlasctl => ../atlasctl
```

After writing `go.mod`, run `go mod tidy` to resolve transitive dependencies and
populate `go.sum`.

---

## Step 2: main.go

Standard `terraform-plugin-framework` provider server entry point. Calls
`providerserver.Serve` with a factory function that returns the provider. No
application logic lives here.

```go
func main() {
    providerserver.Serve(context.Background(), provider.New, providerserver.ServeOpts{
        Address: "registry.terraform.io/supabase/ripe-atlas",
    })
}
```

---

## Step 3: internal/provider/provider.go

Provider struct implementing `provider.Provider`.

### Schema

| Attribute | Type   | Optional | Description                              |
|-----------|--------|----------|------------------------------------------|
| `api_key` | string | yes      | RIPE Atlas API key (UUID). Sensitive.    |

### Configure

1. Read `api_key` from provider config.
2. If absent or empty, fall back to `RIPE_ATLAS_API_KEY` environment variable.
3. If still absent, add a diagnostic error and return.
4. Construct the client:

```go
codec := plan.NewTagCodec(plan.DefaultTagPrefix)
client, err := atlasapi.NewApplyClient(apiKey, false, codec)
```

5. Store `*atlasapi.ApplyClient` as provider data. Resources and data sources
   retrieve it via `req.ProviderData`.

### Methods to implement

- `Metadata` — set provider type name `ripe-atlas`
- `Schema` — declare `api_key`
- `Configure` — construct and store the client
- `Resources` — return `[]func() resource.Resource{ NewMeasurementResource }`
- `DataSources` — return `[]func() datasource.DataSource{ NewProbeSelectionDataSource }`

---

## Step 4: internal/provider/selection.go

Data source `ripe_atlas_probe_selection`.

### Schema

| Attribute   | Type             | Required | Computed | Description                       |
|-------------|------------------|----------|----------|-----------------------------------|
| `snapshot`  | string           | yes      | no       | Path to local `snapshot.json`     |
| `config`    | string           | yes      | no       | Path to `atlasctl.yaml`           |
| `probe_ids` | map(set(int64))  | no       | yes      | Probe IDs keyed by cohort name    |

Framework type for `probe_ids`:

```go
types.MapType{ElemType: types.SetType{ElemType: types.Int64Type}}
```

### Read logic

```
cfg, err  := config.Load(state.Config.ValueString())
snap, err := snapshot.Load(state.Snapshot.ValueString())
cohorts, err := selection.Select(ctx, snap, *cfg)

for each cohort in cohorts:
    ids := []attr.Value{}
    for each probe in cohort.Probes:
        ids = append(ids, types.Int64Value(int64(probe.ID)))
    probeIDs[cohort.Cohort.Name] = types.SetValueMust(types.Int64Type, ids)

write probe_ids map to state
```

---

## Step 5: internal/provider/measurement.go

Resource `ripe_atlas_measurement`.

### Schema

| Attribute          | Type        | Required | Computed | Immutable | Notes                              |
|--------------------|-------------|----------|----------|-----------|------------------------------------|
| `name`             | string      | yes      | no       | yes       | `RequiresReplace()`                |
| `cohort`           | string      | yes      | no       | yes       | `RequiresReplace()`                |
| `target`           | string      | yes      | no       | yes       | `RequiresReplace()`                |
| `msm_type`         | string      | yes      | no       | yes       | `RequiresReplace()`                |
| `af`               | int64       | no       | no       | yes       | `Default(4)` + `RequiresReplace()` |
| `interval_seconds` | int64       | yes      | no       | yes       | `RequiresReplace()`                |
| `probe_ids`        | set(int64)  | yes      | no       | no        | Mutable in-place via participants  |
| `msm_id`           | int64       | no       | yes      | n/a       | Set on create, never changed       |

`msm_type` rather than `type` (reserved word in Go). Maps to `plan.MsmType`.

`af` requires both a `Default` plan modifier and `RequiresReplace()`. Without the
default, Terraform records `null` in state on create, then sees `4` vs `null` on
the next plan and proposes a replacement cycle.

### Create

```
build plan.MsmSpec{
    Key:      plan.MsmKey{Name: name, Cohort: cohort},
    Target:   target,
    Type:     plan.MsmType(msm_type),
    AF:       int(af),
    Interval: int(interval_seconds),
    ProbeIDs: []uint32{ ... from probe_ids ... },
}
msm_id, err := client.CreateMeasurement(ctx, spec)
// write msm_id to state immediately before anything else
// write probe_ids to state
```

Write `msm_id` to state as the first state write. If a subsequent operation fails
the measurement is still in state and Read can reconcile on the next plan.

### Read (drift detection)

```
info, err := client.GetMeasurement(ctx, uint64(msm_id))
if errors.Is(err, plan.ErrMsmNotFound):
    resp.State.RemoveResource(ctx)
    return
reconcile probe_ids from info.ProbeIDs into state
```

`plan.MsmInfo.ProbeIDs []uint32` is populated by `GetMeasurement`, so probe
participant drift is detected without any additional API call.

### Update (probe_ids only)

```
added   = new_probe_ids - state_probe_ids   // set difference
removed = state_probe_ids - new_probe_ids

if len(added) > 0:
    client.AddParticipants(ctx, uint64(msm_id), []uint32{...added...})
if len(removed) > 0:
    client.RemoveParticipants(ctx, uint64(msm_id), []uint32{...removed...})
write updated probe_ids to state
```

All other attributes are immutable and trigger replacement, so Update is only
reachable when `probe_ids` changes.

### Delete

```
client.StopMeasurement(ctx, uint64(msm_id))
```

---

## Step 6: go mod tidy + build verification

```bash
go mod tidy
go build ./...
```

Fix any type errors at the atlasctl boundary (int64 to uint32 casts for probe IDs,
uint64 for msm_id). The provider does not need to compile to a working binary for
this step, just pass `go build`.
