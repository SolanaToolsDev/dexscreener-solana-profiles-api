.PHONY: api poller tidy fmt
api: ; go run ./cmd/app -mode=api
poller: ; go run ./cmd/app -mode=poller
tidy: ; go mod tidy
fmt: ; go fmt ./...
