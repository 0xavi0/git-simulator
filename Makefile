.PHONY: build test race lint tidy validate

build:
	go build ./...

test:
	go test ./...

race:
	go test -race ./...

lint:
	golangci-lint run --timeout=5m

tidy:
	go mod tidy

validate: race
	@echo "Checking for rancher/fleet imports..."
	@if grep -rn 'github\.com/rancher/fleet' --include='*.go' . \
	       | grep -v 'nofleet_test\.go'; then \
		echo "ERROR: found import of github.com/rancher/fleet"; exit 1; \
	fi
	@echo "All validation checks passed."
