程序 := huayan

.PHONY: all build test race vet coverage fuzz check examples release clean

all: test build

build:
	go build -o $(程序) ./cmd/huayan

test:
	go test ./...

race:
	go test -race ./...

vet:
	go vet ./...

coverage:
	go test ./... -coverprofile=coverage.out
	go tool cover -func=coverage.out

fuzz:
	go test ./internal/lexer -run=FuzzLexerUnicodeNeverPanics -fuzz=FuzzLexerUnicodeNeverPanics -fuzztime=10s
	go test ./internal/bytecode -run=FuzzValidateNeverPanics -fuzz=FuzzValidateNeverPanics -fuzztime=10s

release:
	bash scripts/release.sh 0.3.0

check:
	go run ./cmd/huayan check examples/核心演示.hua

examples:
	go run ./cmd/huayan examples/你好世界.hua
	go run ./cmd/huayan examples/斐波那契.hua

clean:
	go clean
