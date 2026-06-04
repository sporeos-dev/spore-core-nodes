.DEFAULT_GOAL := check

.PHONY: check analyze setup

check:
	@echo "==> Cleaning build and test cache..."
	@go clean -cache -testcache
	@echo "==> Building..."
	@go build ./...
	@echo "==> Vetting..."
	@go vet ./...
	@echo "==> Testing..."
	@go test ./...
	@echo "==> All checks passed."

analyze:
	@echo "==> Running tests with race detector..."
	@go test -race ./...

	@echo "==> Generating coverage report..."
	@go test -coverprofile=coverage.out ./...
	@go tool cover -func=coverage.out

	@echo "==> Running staticcheck..."
	@staticcheck ./...

	@echo "==> Running golangci-lint..."
	@golangci-lint run

	@echo "==> Running govulncheck..."
	@govulncheck ./...

	@echo "==> Analysis complete."

setup:
	@echo "==> Installing staticcheck..."
	@go install honnef.co/go/tools/cmd/staticcheck@latest
	@echo "==> Installing golangci-lint..."
	@go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	@echo "==> Installing govulncheck..."
	@go install golang.org/x/vuln/cmd/govulncheck@latest
	@echo "==> Done. Ensure Go bin directory is on your PATH."
