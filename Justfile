# ctrlplane-cli task runner.
# Run `just` to list recipes.

# List available recipes.
default:
    @just --list

# Pin both BSR gen modules to the newest pushed version and tidy.
buf-update:
    go get buf.build/gen/go/ctrlplane/ctrlplane/protocolbuffers/go@latest
    go get buf.build/gen/go/ctrlplane/ctrlplane/connectrpc/go@latest
    go mod tidy


# Build, vet, and test.
verify:
    go build ./...
    go vet ./...
    go test ./...

