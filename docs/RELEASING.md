# Releasing the Terraform Provider

This document describes the process for releasing the `terraform-provider-claude-enterprise` provider.

## Prerequisites

- The repository must be public
- The repository name must be `terraform-provider-claude-enterprise`
- You must have administrative access to the repository
- You must have a GPG key registered with GitHub and Terraform Registry

## Release Process

### Step 1: Generate and Register GPG Key

First, generate a new GPG key if you don't have one:

```bash
gpg --full-generate-key
```

Choose:
- Key type: RSA
- Key size: 4096
- Email: Must match your GitHub account email

Export the private key and add it to your repository secrets:

```bash
gpg --armor --export-secret-keys <key-id>
```

Add the exported key as a repository secret:
1. Go to Settings → Secrets and variables → Actions
2. Create a new secret named `GPG_PRIVATE_KEY` with the exported private key
3. Create another secret named `PASSPHRASE` with your GPG passphrase

### Step 2: Register Public Key with Terraform Registry

Export your public key:

```bash
gpg --armor --export <key-id>
```

Register it with Terraform Registry:
1. Go to https://registry.terraform.io/settings/gpg-keys
2. Add the public key under the namespace `JeongJaeSoon`

### Step 3: Create and Push a Release Tag

Create a new version tag and push it:

```bash
git tag v0.1.0
git push origin v0.1.0
```

The version number should follow semantic versioning (e.g., v0.1.0, v1.0.0, etc.).

### Step 4: Monitor the Release Workflow

The release workflow will automatically:
1. Build the provider for multiple platforms (Linux, Windows, macOS, FreeBSD)
2. Create signed checksums (SHA256SUMS with GPG signature)
3. Generate the registry manifest
4. Create a GitHub Release with all artifacts

You can monitor the progress in the GitHub Actions tab.

### Step 5: Publish to Terraform Registry

First-time setup:
1. Go to https://registry.terraform.io/publish/provider
2. Connect your GitHub repository
3. Grant permission for Terraform Registry to access your repository

After first-time setup:
- Simply push a new tag (v*) and Terraform Registry will automatically recognize and publish it
- The registry pulls the signed artifacts from the GitHub Release

## Troubleshooting

### GPG Key Issues
- Ensure your GPG key email matches your GitHub email
- Verify the private key is correctly base64-encoded in the repository secret
- Test GPG locally: `gpg --list-secret-keys`

### Release Workflow Failures
- Check GitHub Actions logs for specific error messages
- Verify `go build ./...` passes locally
- Ensure all required secrets are configured

### Registry Publishing Issues
- Verify the provider name follows Terraform conventions: `terraform-provider-<name>`
- Check that the GitHub repository is public
- Confirm the GPG public key is registered in the correct namespace

## Resources

- [Terraform Provider Publishing Guide](https://developer.hashicorp.com/terraform/registry/providers/publishing)
- [GoReleaser Documentation](https://goreleaser.com/)
- [GPG Key Management](https://docs.github.com/en/authentication/managing-commit-signature-verification/generating-a-new-gpg-key)
