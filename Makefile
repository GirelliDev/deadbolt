build:
	go build -ldflags "-X github.com/GirelliDev/deadbolt/internal/cli.Version=$$(git describe --tags --always --dirty)" -o bin/dbolt ./cmd/dbolt
test:
	go test ./...
