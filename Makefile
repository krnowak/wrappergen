.PHONY: all
all: tools/golangci-lint
	go build .
	./tools/golangci-lint run --fix
	go mod tidy

tools/golangci-lint: tools/go.mod tools/go.sum
	cd tools && \
		go build -o golangci-lint github.com/golangci/golangci-lint/cmd/golangci-lint


.PHONY: test
test:
	srcdir="$${PWD}"; \
	cd test && \
		PATH="$${srcdir}:$${PATH}" go generate && \
		go build .
