# Contributing to OpenTelemetry Collector OPC UA Receiver

Thank you for your interest in contributing! This document provides guidelines and instructions for contributing to this project.

## Code of Conduct

This project follows the OpenTelemetry Community [Code of Conduct](https://github.com/open-telemetry/community/blob/main/code-of-conduct.md).

## Getting Started

### Prerequisites

- Go 1.25.1 or later
- Git
- OpenTelemetry Collector Builder (OCB) v0.145.0+
- Basic understanding of OpenTelemetry Collector architecture
- Familiarity with OPC UA (helpful but not required)

### Development Setup

1. Fork the repository on GitHub
2. Clone your fork locally:
   ```bash
   git clone https://github.com/YOUR_USERNAME/opentelemetry-collector-opcua-receiver.git
   cd opentelemetry-collector-opcua-receiver
   ```

3. Add the upstream repository:
   ```bash
   git remote add upstream https://github.com/bruegth/opentelemetry-collector-opcua-receiver.git
   ```

4. Install dependencies:
   ```bash
   cd receiver/opcua
   go mod download
   go mod tidy
   ```

## Making Changes

### Branching Strategy

- `main` - Production-ready code
- `initial` - Development branch
- Feature branches - `feature/your-feature-name`
- Bug fix branches - `fix/bug-description`

### Workflow

1. **Create a branch** from `main` or `initial`:
   ```bash
   git checkout -b feature/my-new-feature
   ```

2. **Make your changes**:
   - Write clear, concise code
   - Follow Go conventions and idioms
   - Add tests for new functionality
   - Update documentation as needed

3. **Test your changes**:
   ```bash
   cd receiver/opcua
   go test ./...
   go test -race ./...
   go test -cover ./...
   ```

4. **Format your code**:
   ```bash
   go fmt ./...
   go vet ./...
   ```

5. **Run linter** (optional but recommended):
   ```bash
   golangci-lint run
   ```

6. **Commit your changes**:
   ```bash
   git add .
   git commit -m "feat: add new feature"
   ```

   Follow [Conventional Commits](https://www.conventionalcommits.org/):
   - `feat:` - New feature
   - `fix:` - Bug fix
   - `docs:` - Documentation changes
   - `test:` - Adding or updating tests
   - `refactor:` - Code refactoring
   - `chore:` - Maintenance tasks

7. **Push to your fork**:
   ```bash
   git push origin feature/my-new-feature
   ```

8. **Create a Pull Request** on GitHub

## Pull Request Guidelines

### Before Submitting

- [ ] All tests pass
- [ ] Code is formatted (`go fmt`)
- [ ] No linter warnings (`go vet`, `golangci-lint`)
- [ ] Documentation is updated
- [ ] Commit messages follow conventional commits
- [ ] PR description clearly explains the changes

### PR Description Template

```markdown
## Description
Brief description of what this PR does

## Type of Change
- [ ] Bug fix
- [ ] New feature
- [ ] Breaking change
- [ ] Documentation update

## Testing
Describe how you tested your changes

## Checklist
- [ ] Tests pass locally
- [ ] Code is formatted
- [ ] Documentation updated
- [ ] No breaking changes (or documented if unavoidable)
```

## Code Style

### Go Guidelines

- Follow the [Effective Go](https://golang.org/doc/effective_go.html) guide
- Use meaningful variable and function names
- Keep functions small and focused
- Avoid global variables
- Use interfaces for testability
- Handle errors explicitly

### Testing

- Write unit tests for all new code
- Use table-driven tests when appropriate
- Mock external dependencies
- Aim for >80% code coverage
- Test edge cases and error conditions

Example:
```go
func TestConfigValidate(t *testing.T) {
    tests := []struct {
        name    string
        config  *Config
        wantErr bool
        errMsg  string
    }{
        {
            name: "valid config",
            config: &Config{
                Endpoint: "opc.tcp://localhost:4840",
            },
            wantErr: false,
        },
        // More test cases...
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := tt.config.Validate()
            if tt.wantErr {
                require.Error(t, err)
                assert.Contains(t, err.Error(), tt.errMsg)
            } else {
                require.NoError(t, err)
            }
        })
    }
}
```

### Documentation

- Document all public functions, types, and constants
- Use godoc format
- Include examples where helpful
- Update README.md for new features
- Keep documentation in sync with code

Example:
```go
// Config defines configuration for the OPC UA receiver.
// It includes connection settings, security options, and collection parameters.
type Config struct {
    // Endpoint is the OPC UA server endpoint URL (e.g., opc.tcp://localhost:4840)
    Endpoint string `mapstructure:"endpoint"`
}
```

## Testing

### Running Tests

```bash
# All tests
go test ./...

# With coverage
go test -cover ./...

# With race detector
go test -race ./...

# Verbose output
go test -v ./...

# Specific package
go test ./receiver/opcua

# Specific test
go test -run TestConfigValidate
```

### Writing Tests

- Place tests in `*_test.go` files
- Use `testify` for assertions
- Mock external dependencies
- Test both success and failure cases
- Use descriptive test names

## Building

### Build Receiver Module

```bash
cd receiver/opcua
go build
```

### Build Complete Collector

```bash
# Download OCB if not present
# Then build:
./ocb.exe --config builder-config.yaml
```

## Debugging

### Enable Debug Logging

In `config.yaml`:
```yaml
service:
  telemetry:
    logs:
      level: debug
```

### Use Delve Debugger

```bash
# Install delve
go install github.com/go-delve/delve/cmd/dlv@latest

# Debug tests
cd receiver/opcua
dlv test
```

## Common Issues

### Import Cycle

If you encounter import cycles, refactor to use interfaces or move shared code to a common package.

### Module Version Issues

```bash
go mod tidy
go mod download
go clean -modcache
```

### Build Failures

1. Check Go version: `go version`
2. Update dependencies: `go mod tidy`
3. Clean build cache: `go clean -cache`

## Getting Help

- **GitHub Issues**: Report bugs or request features
- **GitHub Discussions**: Ask questions or discuss ideas
- **Pull Requests**: Get code review and feedback

## Release Process

Releases are managed by project maintainers:

1. Version is tagged (e.g., `v0.1.0`)
2. GitHub Actions builds and publishes artifacts
3. Release notes are generated automatically
4. Docker images are published

## Attribution

Contributors will be acknowledged in:
- GitHub contributors list
- Release notes
- Project documentation (where significant)

## License

By contributing, you agree that your contributions will be licensed under the Apache License 2.0.

## Questions?

Feel free to open a GitHub Discussion or Issue if you have questions!

---

Thank you for contributing! 🎉
