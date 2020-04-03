.PHONY: run-client run-server proto fmt
run-client:
	go run cmd/client.go
run-server:
	go run cmd/server.go
proto:
	protoc --go_out=plugins=grpc:. proto/*.proto
fmt:
	gofmt -s -w cmd/*.go proto/*.go
