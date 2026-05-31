.PHONY: help build test clean deploy monitor fmt vet lint install-tools

# Binary name
BINARY_NAME=node-exporter-monitor
BUILD_DIR=bin

# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod
GOFMT=$(GOCMD) fmt
GOVET=$(GOCMD) vet

# Build flags
LDFLAGS=-ldflags "-w -s"

# Default target
.DEFAULT_GOAL := help

help: ## Display this help screen
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-30s\033[0m %s\n", $$1, $$2}'

deps: ## Download dependencies
	$(GOMOD) download
	$(GOMOD) tidy

build: deps ## Build the binary
	@echo "Building $(BINARY_NAME)..."
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) ./cmd/monitor

test: ## Run unit tests
	@echo "Running tests..."
	$(GOTEST) -v -race -coverprofile=coverage.txt -covermode=atomic ./...

test-coverage: test ## Run tests with coverage report
	@echo "Generating coverage report..."
	$(GOCMD) tool cover -html=coverage.txt -o coverage.html
	@echo "Coverage report generated: coverage.html"

fmt: ## Format Go code
	@echo "Formatting code..."
	$(GOFMT) ./...

vet: ## Run go vet
	@echo "Running go vet..."
	$(GOVET) ./...

lint: install-tools ## Run golangci-lint
	@echo "Running golangci-lint..."
	@which golangci-lint > /dev/null || (echo "golangci-lint not found, run 'make install-tools'" && exit 1)
	golangci-lint run ./...

install-tools: ## Install development tools
	@echo "Installing development tools..."
	@which golangci-lint > /dev/null || \
		(echo "Installing golangci-lint..." && \
		go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest)

clean: ## Clean build artifacts
	@echo "Cleaning..."
	$(GOCLEAN)
	rm -rf $(BUILD_DIR)
	rm -f coverage.txt coverage.html
	rm -rf reports/

deploy: ## Deploy node-exporter-zoneinfo using kustomize
	@echo "Deploying node-exporter-zoneinfo..."
	kubectl apply -k .

undeploy: ## Undeploy node-exporter-zoneinfo
	@echo "Undeploying node-exporter-zoneinfo..."
	kubectl delete -k . --ignore-not-found=true

monitor: build ## Build and run the monitoring binary (30 min test)
	@echo "Running resource monitoring for 30 minutes..."
	@mkdir -p reports
	$(BUILD_DIR)/$(BINARY_NAME)

monitor-deploy: build ## Deploy and monitor in one command
	@echo "Deploying and monitoring..."
	@mkdir -p reports
	$(BUILD_DIR)/$(BINARY_NAME) --deploy

quick-test: build ## Quick 2-minute test run
	@echo "Running quick 2-minute test..."
	@mkdir -p reports
	$(BUILD_DIR)/$(BINARY_NAME) --duration=2m

two-phase-monitor: build ## Run two-phase monitoring (Phase 1: with node-exporter, Phase 2: without)
	@echo "Starting two-phase monitoring (60 minutes total)..."
	@mkdir -p reports
	$(BUILD_DIR)/$(BINARY_NAME) --kubeconfig=$(KUBECONFIG) --two-phase --duration=30m

two-phase-quick: build ## Quick two-phase test (2 min per phase)
	@echo "Starting quick two-phase test (4 minutes total)..."
	@mkdir -p reports
	$(BUILD_DIR)/$(BINARY_NAME) --kubeconfig=$(KUBECONFIG) --two-phase --duration=2m

three-phase-monitor: build ## Run three-phase monitoring (Phase 1: all, Phase 2: none, Phase 3: zoneinfo)
	@echo "Starting three-phase monitoring (90 minutes total)..."
	@mkdir -p reports
	$(BUILD_DIR)/$(BINARY_NAME) --kubeconfig=$(KUBECONFIG) --three-phase --duration=30m

three-phase-quick: build ## Quick three-phase test (2 min per phase)
	@echo "Starting quick three-phase test (6 minutes total)..."
	@mkdir -p reports
	$(BUILD_DIR)/$(BINARY_NAME) --kubeconfig=$(KUBECONFIG) --three-phase --duration=2m

six-phase-monitor: build ## Run six-phase monitoring (comprehensive test - 180 minutes)
	@echo "Starting six-phase monitoring (180 minutes total)..."
	@echo "Phases: no-exporter → all → no-exporter → zoneinfo → interrupts → softirqs"
	@mkdir -p reports
	$(BUILD_DIR)/$(BINARY_NAME) --kubeconfig=$(KUBECONFIG) --six-phase --duration=30m

six-phase-quick: build ## Quick six-phase test (2 min per phase = 12 min total)
	@echo "Starting quick six-phase test (12 minutes total)..."
	@mkdir -p reports
	$(BUILD_DIR)/$(BINARY_NAME) --kubeconfig=$(KUBECONFIG) --six-phase --duration=2m

view-charts: ## Open generated charts in default viewer
	@echo "Opening charts..."
	@if [ -d "reports/charts" ]; then \
		xdg-open reports/charts/*.png 2>/dev/null || open reports/charts/*.png 2>/dev/null || echo "Please open reports/charts/ manually"; \
	else \
		echo "No charts found. Run 'make monitor' first."; \
	fi

list-charts: ## List all generated charts
	@echo "Generated charts:"
	@if [ -d "reports/charts" ]; then \
		ls -lh reports/charts/*.png; \
	else \
		echo "No charts found. Run 'make monitor' first."; \
	fi

show-mapping: ## Show pod-to-node mapping from latest report
	@echo "Pod-to-Node Mapping:"
	@if ls reports/monitoring-report-*.txt 1> /dev/null 2>&1; then \
		grep "(Node:" reports/monitoring-report-*.txt | tail -20; \
	else \
		echo "No report found. Run 'make monitor' first."; \
	fi

verify: fmt vet test ## Run all verification steps (fmt, vet, test)
	@echo "All verification steps passed!"

all: clean verify build ## Clean, verify, and build

.PHONY: deps build test test-coverage fmt vet lint clean deploy undeploy monitor monitor-deploy quick-test two-phase-monitor two-phase-quick three-phase-monitor three-phase-quick six-phase-monitor six-phase-quick view-charts list-charts show-mapping verify all
