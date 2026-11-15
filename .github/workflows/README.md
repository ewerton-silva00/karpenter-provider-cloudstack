# GitHub Actions Workflows

This directory contains the CI/CD workflows for the project.

## Available Workflows

### 1. CI (ci.yaml)

Continuous integration workflow that runs on every push and pull request.

**Triggers:**
- Push to `main`
- Pull requests to `main`

**Jobs:**
- **test**: Unit tests and linting
- **build**: Binary compilation
- **docker**: Docker image build (push only)

### 2. Release (release.yaml)

Automatic publication workflow that runs when creating tags.

**Triggers:**
- Push tags: `v*` (e.g., `v0.1.0`, `v0.1.0-alpha.1`)

**Jobs:**
- **release**:
  - Multi-arch container image build (amd64, arm64)
  - Push to GitHub Container Registry
  - Package Helm chart
  - Push Helm chart to OCI registry
  - Create GitHub Release with assets

**Generated artifacts** (`{OWNER}` is the owner of the repository the workflow runs in):
- Container Image: `ghcr.io/{OWNER}/cloudstack/karpenter/controller:v{VERSION}`, plus `latest` for stable versions
- Helm Chart (OCI): `oci://ghcr.io/{OWNER}/charts/karpenter-cloudstack`, version `{VERSION}`
- GitHub Release with the chart package and the CRDs shipped in the chart

The chart version and `appVersion` are taken from the tag; the values in
`Chart.yaml` are overwritten at release time.

## Creating a Release

```bash
# 1. Ensure you're on updated main
git checkout main
git pull origin main

# 2. Create and push the tag
git tag -a v0.1.0 -m "Release v0.1.0"
git push origin v0.1.0

# 3. The workflow runs automatically
# Monitor in the Actions tab of the repository
```

## Environment Variables

### CI Workflow
- `go-version`: Go version (1.25.4)

### Release Workflow
- `REGISTRY`: GitHub Container Registry (`ghcr.io`)
- `IMAGE_NAME`: Image name (`cloudstack/karpenter/controller`)
- `CHART_NAME`: Chart name (`karpenter-cloudstack`)
- `CHART_REPOSITORY`: Path of the chart under the owner's namespace (`charts`)

## Required Permissions

Workflows require the following permissions on the `GITHUB_TOKEN`:

- `contents: write` - To create GitHub Releases
- `packages: write` - To publish to GHCR

These permissions are automatically configured in the workflow.

## Troubleshooting

### Error: "Resource not accessible by integration"
**Cause:** Missing `packages: write` permission
**Solution:** Verify that the workflow has `permissions: packages: write`

### Error: "Image not found" when installing
**Cause:** Image was not published correctly
**Solution:** Verify that the `release` workflow completed successfully

### Error: "denied: permission_denied: write_package" when pushing
**Cause:** The package already exists and was not created by this repository's workflow, so the `GITHUB_TOKEN` cannot write to it
**Solution:** In the package settings, under "Manage Actions access", add this repository with the `Write` role. Do it for both the image and the chart packages.
