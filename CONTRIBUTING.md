# Contributing

Thanks for your interest in contributing to willow!

## Getting Started

Prerequisites: Go 1.26+

```sh
go build -o bin/willow ./cmd/willow
go test ./... -count=1 -v
GO_COVERAGE_MIN=70.0 scripts/check-go-coverage.sh
```

## Pull Requests

1. Fork the repo and create a branch from `main`.
2. If you've added code that should be tested, add tests.
3. Make sure tests and the Go coverage gate pass. CI enforces at least 70% total Go coverage.
4. Open a pull request.

For new features, please open an issue first to discuss the change.

## Bug Reports

Open a [bug report](https://github.com/iamrajjoshi/willow/issues/new?template=bug_report.md) with steps to reproduce.

## Code Style

Follow standard Go conventions. Commit messages use [conventional commits](https://www.conventionalcommits.org/en/v1.0.0/) with emoji prefixes.

## License

By submitting a contribution, you agree that your work will be licensed under the project's [MIT License](./LICENSE).
