# TODO

## GPG signing for Terraform Registry publication

GPG signing is currently disabled in `.goreleaser.yml` and the release workflow.
It is required before publishing to the Terraform Registry.

Steps to complete:

1. Generate a GPG key (RSA 4096):
   `gpg --full-generate-key`

2. Export the private key:
   `gpg --armor --export-secret-keys YOUR_KEY_ID`

3. Add two secrets to the GitHub repo (Settings > Secrets > Actions):
   - `GPG_PRIVATE_KEY` — the armored private key output from step 2
   - `GPG_PASSPHRASE` — the passphrase used when generating the key

4. Re-enable the `signs` block in `.goreleaser.yml`.

5. Re-add the GPG import step and `GPG_FINGERPRINT` env var to `.github/workflows/release.yml`.

6. Register the corresponding public key with the Terraform Registry under the `supabase` namespace at registry.terraform.io.
