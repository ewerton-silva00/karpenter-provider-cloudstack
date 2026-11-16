# Contributing to Karpenter CloudStack Provider

Thank you for your interest in contributing to the Karpenter CloudStack Provider!

## Development Setup

### Prerequisites

- Go 1.25.3 or later
- Kubernetes cluster (1.28+)
- CloudStack environment (4.22+)
- kubectl
- Helm 3

### Local Development

1. Clone the repository:
```bash
git clone https://github.com/ewerton-silva00/karpenter-provider-cloudstack.git
cd karpenter-provider-cloudstack
```

2. Install dependencies:
```bash
go mod download
```

3. Set up environment variables:
```bash
export CLOUDSTACK_API_URL=https://your-cloudstack-api.com
export CLOUDSTACK_API_KEY=your-api-key
export CLOUDSTACK_SECRET_KEY=your-secret-key
export CLUSTER_NAME=my-cluster
```

4. Run tests:
```bash
make test
```

5. Build the controller:
```bash
make build
```

6. Run locally:
```bash
./bin/controller
```

## Code Structure

```
karpenter-provider-cloudstack/
├── cmd/controller/          # Main entry point
├── pkg/
│   ├── apis/v1/            # CRD definitions
│   ├── cloudprovider/      # CloudProvider implementation
│   ├── cloudstack/         # CloudStack SDK wrapper
│   ├── controllers/        # Kubernetes controllers
│   ├── operators/          # Operator initialization
│   └── providers/          # Resource providers
│       ├── instance/       # VM management
│       ├── instancetype/   # Service offerings
│       ├── network/        # Network management
│       ├── template/       # Template management
│       └── zone/           # Zone management
├── charts/                 # Helm charts
└── examples/              # Example configurations
```

## Testing

### Unit Tests

```bash
make test
```

### Integration Tests

Integration tests require a CloudStack environment:

```bash
make test-integration
```

## Pull Request Process

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Make your changes
4. Add tests for your changes
5. Run tests and linting (`make verify`)
6. Commit your changes (`git commit -m 'Add amazing feature'`)
7. Push to the branch (`git push origin feature/amazing-feature`)
8. Open a Pull Request

### PR Guidelines

- Follow Go best practices and idioms
- Add tests for new functionality
- Update documentation as needed
- Keep PRs focused and atomic
- Write clear commit messages
- Add a description explaining your changes

## Code Style

We follow standard Go conventions:

- Use `gofmt` for formatting
- Follow effective Go guidelines
- Use meaningful variable and function names
- Add comments for exported functions and types
- Keep functions small and focused

Run formatting and linting:

```bash
make fmt
make lint
```

## Documentation

- Update README.md for user-facing changes
- Add godoc comments for exported types and functions
- Update examples/ for new features
- Keep CHANGELOG.md updated

## Reporting Issues

When reporting issues, please include:

- CloudStack version
- Kubernetes version
- Karpenter version
- Steps to reproduce
- Expected vs actual behavior
- Relevant logs

## Community

- Join the discussion in GitHub Issues
- Ask questions in GitHub Discussions
- Follow our Code of Conduct

## License

By contributing, you agree that your contributions will be licensed under the Apache License 2.0.

