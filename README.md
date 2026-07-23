# terraform-provider-ripe-atlas

A Terraform provider for [RIPE Atlas](https://atlas.ripe.net) measurements. It wraps the [atlasctl](https://github.com/supabase/atlasctl) domain library to bring RIPE Atlas measurement management into the Terraform resource model: declare your measurements and probe selections in HCL, preview changes before applying, and let Terraform track state.

## Background

[RIPE Atlas](https://atlas.ripe.net) is a global network of roughly 12,000 hardware probes distributed across ISPs worldwide. Each probe runs active measurements (DNS, ping, TLS, traceroute) and reports results in real time. Supabase uses RIPE Atlas to detect failures invisible from its own infrastructure: DNS resolution problems at specific ISPs, TCP/TLS reachability from particular networks, and regional outages that internal monitoring cannot see.

This provider is an adapter over the [atlasctl](https://github.com/supabase/atlasctl) domain library, which owns all probe selection, lifecycle, and state logic. Full background on RIPE Atlas, the probe pool, and credit accounting is in the [atlasctl README](https://github.com/supabase/atlasctl/blob/main/README.md).

## Resources

### `ripeatlas_measurement`

One resource holds one or more cohorts. Each cohort creates exactly one RIPE Atlas measurement ID. Cohort ordering in the list determines probe selection priority: probes selected for cohort N are excluded from cohort N+1, so successive cohorts draw from a diminishing pool. This drawdown is what enables geographically diverse, non-overlapping probe sets across measurement tiers.

```hcl
provider "ripeatlas" {
  # api_key    = "..."  # or set RIPE_ATLAS_API_KEY
  # snapshot   = "..."  # or set RIPE_ATLAS_SNAPSHOT
  # namespace = "..."  # optional
}

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
```

**Inputs**

| Attribute | Type | Immutable | Description |
|-----------|------|-----------|-------------|
| `name` | string | yes | Logical measurement name. |
| `target` | string | yes | DNS name or IP address. |
| `msm_type` | string | yes | `dns`, `ping`, `tls`, or `traceroute`. |
| `af` | number | yes | Address family: `4` or `6`. Default `4`. |
| `exclude_tags` | list(string) | no | Probe tags that hard-exclude a probe from selection. |
| `cohorts[*].name` | string | yes | Cohort tier name. |
| `cohorts[*].probe_count` | number | yes | Number of probes to select. |
| `cohorts[*].max_probes_per_cell` | number | yes | Maximum probes per H3 geographic cell. |
| `cohorts[*].interval_seconds` | number | yes | Measurement interval in seconds (minimum 60). |
| `cohorts[*].include_probe_ids` | set(number) | no | Probes always included regardless of scoring. |
| `cohorts[*].exclude_probe_ids` | set(number) | no | Probes never selected in this cohort. |
| `cohorts[*].cfg.asn` | map(number) | no | Score bonuses by ASN (string key). |
| `cohorts[*].cfg.tags` | map(number) | no | Score bonuses by probe tag string. |
| `cohorts[*].cfg.countries` | map(number) | no | Score bonuses by ISO 3166-1 alpha-2 country code. |
| `cohorts[*].cfg.stability` | map(number) | no | Score bonuses by RIPE Atlas stability tag. |

Changing any immutable attribute stops the old measurement and creates a new one. Adding or removing a cohort creates or stops only the affected measurement. Changes to `exclude_tags` or cohort scoring fields re-run probe selection on the next plan; the resulting `probe_ids` diff drives `AddParticipants` or `RemoveParticipants` without recreating the measurement.

**Computed outputs (per cohort)**

| Attribute | Type | Description |
|-----------|------|-------------|
| `cohorts[*].msm_id` | number | RIPE Atlas measurement ID assigned at creation. |
| `cohorts[*].probe_ids` | set(number) | Probe IDs selected and participating in the measurement. |

## Probe selection

The provider runs probe selection locally during `terraform plan`. Update the snapshot before planning to pick up probe pool changes:

```bash
atlasctl refresh   # fetch connected probes from the RIPE Atlas API
terraform plan     # selection runs here, probe_ids is computed
terraform apply
```

Selection uses scoring bands, continental interleaving, and H3-based geographic diversity. The [atlasctl probe selection docs](https://github.com/supabase/atlasctl/blob/main/README.md#probe-selection) cover the algorithm in full.

## Provider configuration

```hcl
terraform {
  required_providers {
    ripeatlas = {
      source  = "supabase/ripe-atlas"
      version = "~> 0.1"
    }
  }
}

provider "ripeatlas" {
  # api_key    = "..."  # or set RIPE_ATLAS_API_KEY
  # snapshot   = "..."  # or set RIPE_ATLAS_SNAPSHOT
  # namespace = "..."  # optional, default is the atlasctl tag prefix
}
```

| Attribute | Description |
|-----------|-------------|
| `api_key` | RIPE Atlas API key. Sensitive; never appears in plain text in state. Falls back to `RIPE_ATLAS_API_KEY`. |
| `snapshot` | Path to `snapshot.json` produced by `atlasctl refresh`. Falls back to `RIPE_ATLAS_SNAPSHOT`. |
| `namespace` | Identifier embedded in each measurement description on the RIPE Atlas API. Used to distinguish measurements created by different Terraform states or workspaces. Optional; defaults to `terraform-provider-ripe-atlas`. |

### Required API key permissions

Generate an API key at [atlas.ripe.net/keys/](https://atlas.ripe.net/keys/). The required permissions are listed in the [atlasctl documentation](https://github.com/supabase/atlasctl#required-api-key-permissions).

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
