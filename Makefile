.PHONY: run-client run-client-local run-server proto fmt
run-client:
	go run cmd/client.go
run-client-local:
	go run cmd/client_local.go
run-server:
	go run cmd/server.go
proto:
	protoc --go_out=plugins=grpc:. proto/*.proto
fmt:
	gofmt -s -w cmd/*.go proto/*.go pkg/*/*.go
build:
	mkdir -p bin
	go build -ldflags "-s -w" -o bin/client cmd/client.go
	go build -ldflags "-s -w" -o bin/server cmd/server.go
	go build -ldflags "-s -w" -o bin/client_local cmd/client_local.go
