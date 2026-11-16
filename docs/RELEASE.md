# Release Process

This document describes the process for publishing new versions of the Karpenter Provider for CloudStack.

## Published Artifacts

Each release generates the following artifacts:

### 1. Container Image
```
ghcr.io/ewerton-silva00/cloudstack/karpenter/controller:v{VERSION}
```
- Published to GitHub Container Registry (GHCR)
- Architectures: `amd64`, `arm64`
- Generated tags:
  - `v{VERSION}` - Specific version (e.g., `v0.1.0`)
  - `latest` - Latest stable version (only for stable releases, not pre-releases)

### 2. Helm Chart
```
oci://ghcr.io/ewerton-silva00/charts/karpenter-cloudstack:{VERSION}
```
- Published to GHCR as OCI artifact; the chart version has no `v` prefix

### 3. CRDs (Custom Resource Definitions)
- Included in the Helm Chart
- Also available as YAML files in GitHub Releases

### 4. GitHub Release
- Automatically generated release notes
- Attached assets:
  - Helm Chart (`.tgz`)
  - CRDs (`.yaml`)
  - Installation documentation

---

## Release Types

### Development (main branch)
```bash
git push origin main
```
**Generated artifacts:** none. The CI workflow tests, lints and builds, but
publishes nothing.

**Usage:** Continuous testing, active development

---

### Pre-Release (alpha/beta/rc)
```bash
git tag v0.1.0-alpha.1
git push origin v0.1.0-alpha.1
```

**Generated artifacts:**
- Container Image: `:v0.1.0-alpha.1`
- Helm Chart: `0.1.0-alpha.1`
- GitHub Pre-Release (marked as pre-release)
- CRDs in assets

**Usage:** Staging testing, validation before stable release

**Installation:**
```bash
helm install karpenter-cloudstack \
  oci://ghcr.io/ewerton-silva00/charts/karpenter-cloudstack \
  --version 0.1.0-alpha.1
```

---

### Stable Release (vX.Y.Z)
```bash
git tag v0.1.0
git push origin v0.1.0
```

**Generated artifacts:**
- Container Image: `:v0.1.0` + `:latest`
- Helm Chart: `0.1.0`
- GitHub Release (stable, complete release notes)
- CRDs in release assets
- Documentation update

**Usage:** Production

**Installation:**
```bash
helm install karpenter-cloudstack \
  oci://ghcr.io/ewerton-silva00/charts/karpenter-cloudstack \
  --version 0.1.0
```

---

## Semantic Versioning

We follow **[Semantic Versioning 2.0.0](https://semver.org/spec/v2.0.0.html)**:

```
v{MAJOR}.{MINOR}.{PATCH}[-{PRERELEASE}][+{BUILD}]

Examples:
- v1.0.0         → First stable version
- v0.1.1         → Bugfix
- v0.2.0         → New feature
- v1.0.0         → Production-ready version
- v0.1.0-alpha.1 → Alpha pre-release
- v0.1.0-beta.1  → Beta pre-release
- v0.1.0-rc.1    → Release candidate
```

**Rules:**
- **MAJOR (1.x.x)**: Incompatible changes (breaking changes)
- **MINOR (x.1.x)**: New backwards-compatible functionality
- **PATCH (x.x.1)**: Backwards-compatible bug fixes
- **Pre-release**: `-alpha`, `-beta`, `-rc` for unstable versions

---

## Manual Release Process

### 1. Preparation

```bash
# Update main branch
git checkout main
git pull origin main

# Create release branch (optional, for last-minute changes)
git checkout -b release/v0.1.0

# Update CHANGELOG (if exists)
# Update versions in Chart.yaml if needed
```

### 2. Pre-Release Verification

```bash
# Run all tests
make test

# Verify linting
make lint

# Local build
make build

# Verify everything compiles
make docker-build
```

### 3. Create Tag

```bash
# For stable release
git tag -a v0.1.0 -m "Release v0.1.0"

# For pre-release
git tag -a v0.1.0-alpha.1 -m "Pre-release v0.1.0-alpha.1"

# Verify tag
git tag -l -n9 v0.1.0
```

### 4. Push Tag

```bash
# Push tag (triggers release workflow)
git push origin v0.1.0
```

### 5. Monitor Pipeline

```bash
# GitHub Actions workflow runs automatically
# Check: https://github.com/ewerton-silva00/karpenter-provider-cloudstack/actions
```

### 6. Verify Artifacts

```bash
# Verify Docker image
docker pull ghcr.io/ewerton-silva00/cloudstack/karpenter/controller:v0.1.0

# Verify Helm Chart (OCI)
helm pull oci://ghcr.io/ewerton-silva00/charts/karpenter-cloudstack --version 0.1.0

# Verify GitHub Release
# https://github.com/ewerton-silva00/karpenter-provider-cloudstack/releases
```

### 7. Update Documentation

```bash
# Update README with new version
# Update INSTALLATION.md if there are changes
# Announce in relevant channels (Slack, Twitter, etc.)
```

---

## Release Rollback

If you need to revert a release:

```bash
# Delete tag locally
git tag -d v0.1.0

# Delete tag from remote
git push origin :refs/tags/v0.1.0

# Delete GitHub Release (from UI or with gh CLI)
gh release delete v0.1.0

# Delete GHCR images (from UI)
# https://github.com/users/ewerton-silva00/packages/container/cloudstack%2Fkarpenter%2Fcontroller
```

---

## Release Checklist

### Pre-Release
- [ ] All tests pass (`make test`)
- [ ] Clean linting (`make lint`)
- [ ] Documentation updated
- [ ] CHANGELOG updated (if applicable)
- [ ] Version updated in Chart.yaml (if manual)
- [ ] Main branch updated

### Release
- [ ] Tag created with correct name (vX.Y.Z or vX.Y.Z-alpha.N)
- [ ] Tag pushed to GitHub
- [ ] CI/CD pipeline completed successfully
- [ ] Artifacts verified (image, chart, GitHub release)

### Post-Release
- [ ] Installation tested from Helm
- [ ] Documentation verified
- [ ] Release announced (if relevant)
- [ ] Issues/PRs updated with new version

---

## References

- [Semantic Versioning 2.0.0](https://semver.org/)
- [Karpenter Documentation](https://karpenter.sh)
- [Helm Best Practices](https://helm.sh/docs/chart_best_practices/)
- [GitHub Container Registry](https://docs.github.com/en/packages/working-with-a-github-packages-registry/working-with-the-container-registry)
