default: install

generate:
	go generate ./...

install:
	go install ./cmd/terraform-provider-crusoe.go

test:
	go test -count=1 -parallel=4 ./...

lint:
	golangci-lint run --fix

testacc:
	TF_ACC=1 go test -count=1 -parallel=4 -timeout 10m -v ./...

# build cross platform binaries
# TODO: this is a temporary solution until goreleaser is configured
build:
	GOOS=linux GOARCH=amd64 go build -o bin/linux/terraform-provider-crusoe ./cmd/terraform-provider-crusoe.go
	GOOS=darwin GOARCH=amd64 go build -o bin/macos/intel/terraform-provider-crusoe ./cmd/terraform-provider-crusoe.go
	GOOS=darwin GOARCH=arm64 go build -o bin/macos/arm/terraform-provider-crusoe ./cmd/terraform-provider-crusoe.go