# Publishing to the Terraform Registry

This document describes how to publish this provider to the [Terraform Registry](https://registry.terraform.io) so users can install it with:

```hcl
terraform {
  required_providers {
    ipam = {
      source  = "jakeneyer/ipam"
      version = "~> 0.1"
    }
  }
}
```

## Prerequisites

- **Repository**: Public GitHub repo named `terraform-provider-ipam` (lowercase).
- **Registry**: Sign in at [registry.terraform.io](https://registry.terraform.io) with your GitHub account and publish the provider (one-time setup).
- **Releases**: Semantic version tags (e.g. `v0.1.0`) and GitHub Releases with the correct assets.

## Registry requirements

The Terraform Registry expects:

1. **Naming**: Repo name `terraform-provider-ipam`; registry address `registry.terraform.io/<namespace>/ipam` (e.g. `jakeneyer/ipam`).
2. **Manifest**: `terraform-registry-manifest.json` at repo root (already included) with `protocol_versions` (e.g. `["6.0"]`).
3. **Releases**: Each version is a GitHub Release with:
   - Binaries: `terraform-provider-ipam_<version>_<os>_<arch>.zip` (Linux, Windows, macOS; amd64, arm64).
   - Checksums: `terraform-provider-ipam_<version>_SHA256SUMS`.
   - Signature: `terraform-provider-ipam_<version>_SHA256SUMS.sig` (GPG signature of the checksum file).

## Release flow

1. **Tag a version** (semver only; do not remove or change published versions):

   ```bash
   git tag v0.1.0
   git push origin v0.1.0
   ```

2. **CI**: The GitHub Action runs GoReleaser and creates a release with binaries and `SHA256SUMS`.

3. **Signing (for Registry)**: The Registry requires `SHA256SUMS.sig`. To add signing:

   - Generate a GPG key (if needed) and add the **fingerprint** as a repo secret `GPG_FINGERPRINT`.
   - Add the **private key** as a repo secret (e.g. `GPG_PRIVATE_KEY`) and import it in the workflow before running GoReleaser.
   - In `.goreleaser.yaml`, uncomment or add a `signs` block that signs the checksum file (see [GoReleaser signing](https://goreleaser.com/customization/sign/)).
   - In `.github/workflows/release.yml`, set `GPG_FINGERPRINT` (and key import) in the GoReleaser step env.

4. **Publish on the Registry**: In [registry.terraform.io](https://registry.terraform.io) → **Publish** → **Provider**, connect this repo. The Registry will ingest each new release automatically.

## Documentation

- **Docs**: The `docs/` directory (e.g. `docs/index.md`, `docs/resources/*.md`, `docs/data-sources/*.md`) is used by the Registry for the provider documentation. Regenerate with:

  ```bash
  cd tools && go generate .
  ```

- **Doc preview**: Use the [Terraform Registry Doc Preview](https://registry.terraform.io/tools/doc-preview) before releasing.

## References

- [Publish providers (Terraform)](https://developer.hashicorp.com/terraform/registry/providers/publishing)
- [Provider documentation](https://developer.hashicorp.com/terraform/registry/providers/docs)
- [GoReleaser](https://goreleaser.com)
