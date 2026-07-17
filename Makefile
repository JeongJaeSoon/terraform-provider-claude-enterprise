default: build

build:
	go build ./...

install:
	go install .

fmt:
	gofmt -w .

test:
	go test ./... -count=1

testacc:
	TF_ACC=1 go test ./internal/provider/ -v -count=1 -timeout 10m
