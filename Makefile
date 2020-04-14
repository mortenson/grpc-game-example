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
	GOOS=windows GOARCH=amd64 go build -ldflags "-s -w" -o bin/win_client.exe cmd/client.go
	GOOS=windows GOARCH=amd64 go build -ldflags "-s -w" -o bin/win_server.exe cmd/server.go
	GOOS=windows GOARCH=amd64 go build -ldflags "-s -w" -o bin/win_client_local.exe cmd/client_local.go
	GOOS=linux GOARCH=amd64 go build -ldflags "-s -w" -o bin/lin_client cmd/client.go
	GOOS=linux GOARCH=amd64 go build -ldflags "-s -w" -o bin/lin_server cmd/server.go
	GOOS=linux GOARCH=amd64 go build -ldflags "-s -w" -o bin/lin_client_local cmd/client_local.go
	GOOS=darwin GOARCH=amd64 go build -ldflags "-s -w" -o bin/dar_client cmd/client.go
	GOOS=darwin GOARCH=amd64 go build -ldflags "-s -w" -o bin/dar_server cmd/server.go
	GOOS=darwin GOARCH=amd64 go build -ldflags "-s -w" -o bin/dar_client_local cmd/client_local.go
