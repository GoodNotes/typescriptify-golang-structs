.PHONY: build
build:
	go build -v -o /dev/null ./...

.PHONY: install
install:
	go install ./...

.PHONY: test
test: node_modules lint
	go test ./...
	go run example/example.go
	npx tsc browser_test/example_output.ts
	# Make sure dommandline tool works:
	go run tscriptify/main.go -package github.com/GoodNotes/typescriptify-golang-structs/example/example-models -verbose -target tmp_classes.ts example/example-models/example_models.go
	go run tscriptify/main.go -package github.com/GoodNotes/typescriptify-golang-structs/example/example-models -verbose -target tmp_interfaces.ts -interface example/example-models/example_models.go
	go run tscriptify/main.go -package=github.com/aws/secrets-store-csi-driver-provider-aws/provider -verbose -target=tmp_jsiiIntefaces.ts -interface -readonly -all-optional SecretDescriptor

.PHONY: lint
lint:
	go vet ./...
	-golangci-lint run

node_modules:
	npm install
