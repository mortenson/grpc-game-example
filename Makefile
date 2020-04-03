run:
	go run cmd/client.go
proto:
	protoc --go_out=plugins=grpc:proto proto/*.proto
fmt:
	gofmt -s -w cmd/*.go proto/*.go
