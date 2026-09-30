# Contributing to agy-swap

Thank you for your interest in contributing to `agy-swap`! We welcome contributions from the community to help make managing Google Antigravity accounts, quotas, and sessions faster, safer, and more reliable.

---

## Code of Conduct

All contributors and participants are expected to adhere to our [Code of Conduct](CODE_OF_CONDUCT.md). Please read it before participating in discussions or opening pull requests.

---

## Development Environment Setup

### Prerequisites
- **Go**: Version 1.26 or higher.
- **CGO**: Required for macOS native Keychain integration (`Security.framework`). Linux and Windows builds run without CGO.
- **Make**: For running developer workflows and quality checks.
- **Bun** or **Node.js**: Required only if modifying the web documentation/simulator in `site/`.
- **expect**: Optional, used by `./scripts/tui-smoke.sh` to smoke-test terminal dimensions.

### Clone & Build
```bash
git clone https://github.com/aklkbqx/agy-swap.git
cd agy-swap

# Build local binary to ./agy-swap
make build

# Install to ~/.local/bin/agy-swap
make install
```

---

## Verification & Quality Assurance (QA)

Before submitting any code changes, ensure all tests and quality checks pass locally:

```bash
# Run all unit tests
make test

# Run tests with the Go race detector
make race

# Run go vet
make vet

# Run the complete QA pipeline (formatting, tests, race, vet, TUI smoke)
make qa
```

If you modify the web documentation or 3D terminal simulator:
```bash
cd site
bun install
bun run build
bun test
```

---

## Code Style & Architecture Guidelines

1. **Idiomatic Go**:
   - Follow the [Go Code Review Comments](https://go.dev/wiki/CodeReviewComments).
   - Format all Go files with `gofmt` before committing (`make qa` enforces this).
   - Wrap errors with context using `%w` (`fmt.Errorf("fetching quota for %s: %w", email, err)`).
   - Avoid `panic()` in production logic; return explicit errors.
   - Propagate `context.Context` as the first parameter for all blocking, I/O, or asynchronous functions.

2. **Zero Extra Dependencies**:
   - Keep binary overhead minimal and secure. `agy-swap` relies exclusively on the Go standard library, `golang.org/x/sys`, and `golang.org/x/term`. Do not introduce heavy third-party dependencies without prior discussion.

3. **Cross-Platform Compatibility**:
   - `agy-swap` runs on macOS, Linux, and Windows.
   - Platform-specific logic (keychains, file locks, atomic replacements) must use explicit Go build tags:
     - `//go:build darwin`
     - `//go:build windows`
     - `//go:build !windows`
   - Provide a safe fallback implementation when hardware keystores or CGO are unavailable.

4. **Self-Hosted Infrastructure Policy**:
   - Note: GitHub Actions workflows are strictly prohibited in this repository. All build, test, and release verifications run through local scripts and our private runner infrastructure.

---

## Commit & Pull Request Guidelines

### Conventional Commits
We follow the [Conventional Commits](https://www.conventionalcommits.org/) specification:

- `feat:` A new user-facing feature or CLI command.
- `fix:` A bug fix.
- `docs:` Documentation changes only.
- `refactor:` Code refactoring that neither fixes a bug nor adds a feature.
- `perf:` Performance improvements.
- `test:` Adding or updating tests.
- `chore:` Maintenance tasks, dependency updates, or internal scripts.

### Pull Request Checklist
- [ ] Code is formatted with `gofmt`.
- [ ] `make qa` passes with zero warnings or test failures.
- [ ] New features or bug fixes include corresponding unit tests.
- [ ] Documentation, CLI help strings, and README references are kept up to date.
- [ ] No secrets, tokens, or personal configuration files are committed.
