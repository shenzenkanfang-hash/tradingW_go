.PHONY: build test clean

build:
	go build -o bin/collector ./cmd/data_collector
	go build -o bin/calculator ./cmd/indicator_calculator
	go build -o bin/executor ./cmd/strategy_executor

test:
	go test ./internal/...

clean:
	rm -rf bin/*
