# TODO

## Publish to Terraform Registry

Prerequisites are complete (GPG key registered, `supabase` namespace connected).
Remaining step: push a release tag to trigger the signed release workflow.

```bash
git tag v0.1.0
git push origin v0.1.0
```

After the GitHub Actions release workflow completes, the provider should appear at
registry.terraform.io/providers/supabase/ripe-atlas within a few minutes.
