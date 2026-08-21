# Binário
BINARY=louvores

CMD=./cmd/louvores

.PHONY: help
help:
	@echo "Comandos disponíveis:"
	@echo "  make run        - Executa o servidor HTTP"
	@echo "  make build      - Compila o binário"
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
	go build -o $(BINARY) $(CMD)

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
