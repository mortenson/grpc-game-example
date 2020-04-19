.PHONY: run run-client run-client-local run-server proto fmt
build:
	mkdir -p bin
	GOOS=windows GOARCH=amd64 go build -ldflags "-s -w" -o "bin/client_win${BUILD_SUFFIX}.exe" cmd/client.go
	GOOS=windows GOARCH=amd64 go build -ldflags "-s -w" -o "bin/win_server${BUILD_SUFFIX}.exe" cmd/server.go
	GOOS=windows GOARCH=amd64 go build -ldflags "-s -w" -o "bin/win_client_local${BUILD_SUFFIX}.exe" cmd/client_local.go
	GOOS=linux GOARCH=amd64 go build -ldflags "-s -w" -o "bin/lin_client${BUILD_SUFFIX}" cmd/client.go
	GOOS=linux GOARCH=amd64 go build -ldflags "-s -w" -o "bin/lin_server${BUILD_SUFFIX}" cmd/server.go
	GOOS=linux GOARCH=amd64 go build -ldflags "-s -w" -o "bin/lin_client_local${BUILD_SUFFIX}" cmd/client_local.go
	GOOS=darwin GOARCH=amd64 go build -ldflags "-s -w" -o "bin/dar_client${BUILD_SUFFIX}" cmd/client.go
	GOOS=darwin GOARCH=amd64 go build -ldflags "-s -w" -o "bin/dar_server${BUILD_SUFFIX}" cmd/server.go
	GOOS=darwin GOARCH=amd64 go build -ldflags "-s -w" -o "bin/dar_client_local${BUILD_SUFFIX}" cmd/client_local.go
run-client-local:
run:
	go run cmd/client_local.go
run-client:
	go run cmd/client.go
run-bot-client:
	go run cmd/bot_client.go
run-server:
	go run cmd/server.go
proto:
	protoc --go_out=plugins=grpc:. proto/*.proto
fmt:
	gofmt -s -w cmd/*.go proto/*.go pkg/*/*.go
