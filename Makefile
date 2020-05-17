.PHONY: build run run-client run-client-local run-server proto fmt release
build:
	# Linux
	for command in client_local client server; do \
		GOOS=linux GOARCH=amd64 go build -ldflags "-s -w" -o "bin/tshooter_linux_$${command}" "cmd/$${command}.go"; \
		GOOS=linux GOARCH=amd64 go build -ldflags "-s -w -X main.Command=$${command}" -o "bin/tshooter_linux_launcher_$${command}" cmd/launcher.go; \
	done
	# Mac
	for command in client_local client server; do \
		GOOS=darwin GOARCH=amd64 go build -ldflags "-s -w" -o "bin/tshooter_darwin_$${command}" "cmd/$${command}.go"; \
		GOOS=darwin GOARCH=amd64 go build -ldflags "-s -w -X main.Command=$${command}" -o "bin/tshooter_darwin_launcher_$${command}" cmd/launcher.go; \
	done
	# @todo package .app and .dmg
	# Windows
	for command in client_local client server; do \
		GOOS=windows GOARCH=amd64 go build -ldflags "-s -w" -o "bin/tshooter_windows_$${command}.exe" "cmd/$${command}.go"; \
		GOOS=windows GOARCH=amd64 go build -ldflags "-s -w -X main.Command=$${command}.exe" -o "bin/tshooter_windows_launcher_$${command}.exe" cmd/launcher.go; \
	done
release:
	cp assets/README.txt bin/
	cd bin && \
	zip tshooter_windows README.txt *_windows_* && \
	zip tshooter_linux README.txt *_linux_* && \
	zip tshooter_darwin README.txt *_darwin_*
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
