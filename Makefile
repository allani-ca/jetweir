.PHONY: build build-consumer run run-docker test clean help

help: ## Show this help message
	@echo "Jetweir - Bluesky Jetstream Multiplexer"
	@echo ""
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

build: ## Build Jetweir service
	go build -o jetweir ./

build-consumer: ## Build example consumer
	cd examples/consumer && go build -o consumer ./

build-all: build build-consumer ## Build all binaries

run: build ## Build and run Jetweir
	./jetweir

run-docker: ## Run Jetweir in Docker
	docker build -t jetweir:latest .
	docker run -it --rm \
		-p 8080:8080 \
		-p 9090:9090 \
		-e JETWEIR_LISTEN_ADDR=0.0.0.0 \
		jetweir:latest

compose-up: ## Start Jetweir + Prometheus with Docker Compose
	docker-compose up -d
	@echo "Jetweir:   http://localhost:8080"
	@echo "Prometheus: http://localhost:9091"

compose-down: ## Stop Docker Compose services
	docker-compose down

compose-logs: ## Show Docker Compose logs
	docker-compose logs -f

test: ## Run tests
	go test -v -cover ./...

clean: ## Clean up binaries and build artifacts
	rm -f jetweir jetweir.exe
	rm -f examples/consumer/consumer examples/consumer/consumer.exe
	go clean

fmt: ## Format code
	go fmt ./...

vet: ## Run go vet
	go vet ./...

lint: fmt vet ## Run formatters and linters

check: lint test ## Run all checks

deps: ## Download dependencies
	go mod download
	go mod tidy

docs: ## Generate documentation
	@echo "Documentation:"
	@echo "  - README.md: Full documentation"
	@echo "  - QUICKSTART.md: Quick start guide"
	@echo "  - Dockerfile: Docker build configuration"
	@echo "  - docker-compose.yml: Local development stack"
