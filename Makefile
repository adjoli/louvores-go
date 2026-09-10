# Binário
BINARY=louvores

CMD=./cmd/louvores

# Versão: VERSION pode ser sobrescrita (ex.: make build VERSION=1.2.3).
# COMMIT/DATE são derivados do git; injetados no binário via -ldflags -X.
VERSION ?= 0.3.0
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "")
DATE   ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || echo "")

LDFLAGS = -X github.com/adjoli/louvores-go/internal/version.Version=$(VERSION) \
          -X github.com/adjoli/louvores-go/internal/version.Commit=$(COMMIT) \
          -X github.com/adjoli/louvores-go/internal/version.Date=$(DATE) \
		  -s -w

.PHONY: help
help:
	@echo "Comandos disponíveis:"
	@echo "  make run        - Executa o servidor HTTP"
	@echo "  make build      - Compila o binário (com versão via -ldflags)"
	@echo "  make version    - Imprime a versão do binário"
	@echo "  make clean      - Remove o binário"
	@echo "  make fmt        - Formata o código"
	@echo "  make vet        - Executa go vet"
	@echo "  make test       - Executa testes"
	@echo "  make test-cover - Executa testes com cobertura"
	@echo "  make tidy       - Atualiza go.mod"
	@echo "  make deps       - Baixa dependências"
	@echo "  make dev        - fmt + vet + run"

.PHONY: run
run:
	go run $(CMD)

.PHONY: build
build:
	go build -tags netgo -ldflags "$(LDFLAGS)" -o $(BINARY) $(CMD)

.PHONY: version
version:
	go build -tags netgo -ldflags "$(LDFLAGS)" -o $(BINARY) $(CMD)
	./$(BINARY) -version

.PHONY: clean
clean:
	rm -f $(BINARY)

.PHONY: fmt
fmt:
	go fmt ./...

.PHONY: vet
vet:
	go vet ./...

.PHONY: test
test:
	go test ./...

.PHONY: test-cover
test-cover:
	go test -cover ./...

.PHONY: tidy
tidy:
	go mod tidy

.PHONY: deps
deps:
	go mod download

.PHONY: dev
dev: fmt vet run
