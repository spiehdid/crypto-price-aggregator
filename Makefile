.PHONY: build run test test-race test-integration lint vet docker-build docker-up docker-down mocks migrate-new clean

build:
	go build -o bin/cpa ./cmd/server/

run:
	go run ./cmd/server/

test:
	go test ./... -count=1

test-race:
	go test ./... -race -count=1

test-integration:
	go test ./... -tags=integration -count=1

lint:
	golangci-lint run

vet:
	go vet ./...

docker-build:
	docker build -t crypto-price-aggregator .

docker-up:
	docker-compose up -d

docker-down:
	docker-compose down

mocks:
	mockgen -source=internal/domain/port/ports.go -destination=internal/domain/port/mocks/mocks.go -package=mocks

migrate-new:
	@read -p "Migration name: " name; \n	migrate create -ext sql -dir migrations -seq $$name

clean:
	rm -rf bin/ *.exe cpa.exe
