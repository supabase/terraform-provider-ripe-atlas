# terraform-provider-ripe-atlas

A Terraform provider for [RIPE Atlas](https://atlas.ripe.net) measurements. It wraps the [atlasctl](https://github.com/supabase/atlasctl) domain library to bring RIPE Atlas measurement management into the Terraform resource model: declare your measurements and probe selections in configuration, preview changes before applying, and let Terraform track state.

## Background

[RIPE Atlas](https://atlas.ripe.net) is a global network of roughly 12,000 hardware probes distributed across ISPs worldwide. Each probe runs active measurements (DNS, ping, TLS, traceroute) and reports results in real time. Supabase uses RIPE Atlas to detect failures invisible from its own infrastructure: DNS resolution problems at specific ISPs, TCP/TLS reachability from particular networks, and regional outages that internal monitoring cannot see.

This provider is an adapter over the [atlasctl](https://github.com/supabase/atlasctl) domain library, which owns all probe selection, lifecycle, and state logic. Full background on RIPE Atlas, the probe pool, and credit accounting is in the [atlasctl README](https://github.com/supabase/atlasctl/blob/main/README.md).

## Resources

### `ripeatlas_measurement`

One resource per RIPE Atlas measurement. Each maps to a single `(name, cohort)` pair and one measurement ID on the RIPE Atlas platform.

```hcl
resource "ripeatlas_measurement" "dns_canary_high" {
  name             = "dns-canary"
  cohort           = "high-freq"
  target           = "canary.example.com"
  msm_type         = "dns"
  af               = 4
  interval_seconds = 60
  probe_ids        = [1001, 2002, 3003]
}

output "msm_id" {
  value = ripeatlas_measurement.dns_canary_high.msm_id
}
```

**Inputs**

| Attribute | Type | Description |
|-----------|------|-------------|
| `name` | string | Logical measurement name. Immutable. |
| `cohort` | string | Probe group name. Immutable. |
| `target` | string | DNS name or IP address. Immutable. |
| `msm_type` | string | `dns`, `ping`, `tls`, or `traceroute`. Immutable. |
| `af` | number | Address family: `4` or `6`. Default `4`. Immutable. |
| `interval_seconds` | number | Measurement interval in seconds (minimum 60). Immutable. |
| `probe_ids` | set(number) | RIPE Atlas probe IDs. Mutable in place. |

Changing any immutable attribute stops the old measurement and creates a new one. Changing `probe_ids` adds or removes participants on the running measurement without recreating it.

**Outputs**

| Attribute | Type | Description |
|-----------|------|-------------|
| `msm_id` | number | RIPE Atlas measurement ID assigned at creation. |

---

### `ripeatlas_probe_selection` (data source)

Runs the atlasctl probe selection algorithm during `terraform plan` and makes the results available as data. Cohort definitions, scoring weights, exclude tags, and geographic diversity are all read from an `atlasctl.yaml` config file.

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

The data source reads only the `cohorts`, `scoring`, and `exclude_tags` stanzas from `atlasctl.yaml`. Any `measurements` stanzas in the file are ignored. Measurements are declared as `ripeatlas_measurement` resources in HCL instead.

**Inputs**

| Attribute | Type | Description |
|-----------|------|-------------|
| `snapshot` | string | Path to a local `snapshot.json` produced by `atlasctl refresh`. |
| `config` | string | Path to `atlasctl.yaml`. |

**Outputs**

| Attribute | Type | Description |
|-----------|------|-------------|
| `probe_ids` | map(set(number)) | Probe IDs per cohort name. |

## Probe selection

The `ripeatlas_probe_selection` data source runs selection locally using the snapshot on disk. Update the snapshot before planning to pick up probe pool changes:

```bash
atlasctl refresh   # fetch connected probes from the RIPE Atlas API
terraform plan     # selection runs here, probe IDs propagate to measurements
```

Selection uses scoring bands, continental interleaving, and H3-based geographic diversity. The [atlasctl probe selection docs](https://github.com/supabase/atlasctl/blob/main/README.md#probe-selection) cover the algorithm in full.

## Provider configuration

```hcl
terraform {
  required_providers {
    ripe-atlas = {
      source  = "supabase/ripe-atlas"
      version = "~> 0.1"
    }
  }
}

provider "ripe-atlas" {
  # api_key = "..."  # or set RIPE_ATLAS_API_KEY
}
```

`api_key` is marked sensitive and never appears in plain text in Terraform state.

## Credit costs

| Type | Credits per result |
|------|--------------------|
| dns | 10 |
| tls | 10 |
| ping | 3 |
| traceroute | 30 |

Minimum measurement interval is 60 seconds. All measurements created by this provider are periodic.

## Building

```bash
make build    # build the provider binary
make install  # install into ~/.terraform.d/plugins for local development
make test     # run tests
```

Requires Go 1.26 or later.

## Local development

`make install` places the binary in the mirror directory layout Terraform expects. Add a `dev_overrides` block to `~/.terraformrc` to use the local build without a version constraint:

```hcl
provider_installation {
  dev_overrides {
    "supabase/ripe-atlas" = "/path/to/terraform-provider-ripe-atlas"
  }
  direct {}
}
```

## Release

Tag a commit to trigger GoReleaser. The release workflow signs the checksums file with GPG before uploading to GitHub Releases. The Terraform Registry picks up new versions automatically once the provider namespace is configured.

```bash
git tag v0.1.0 && git push origin v0.1.0
```

Two repository secrets are required: `GPG_PRIVATE_KEY` and `GPG_PASSPHRASE`. The corresponding public key must be registered in the Terraform Registry under the `supabase` namespace.

## Requirements

- Go 1.26+
- `RIPE_ATLAS_API_KEY` set to a valid RIPE Atlas API key
