unit-test:
	go test -timeout 9000s -a -v -coverprofile=coverage.out -coverpkg=./... ./... 2>&1 | tee report.out
unit-test-ci:
	go test -timeout 9000s -a -v -coverprofile=coverage.out -coverpkg=./... ./... 2>&1 | go-junit-report > report.xml
mock:
	go generate ./...
dev:
	air -c .air.toml

vet:
	go vet ./...
	gosec -quiet ./...
	govulncheck -show verbose ./...
	staticcheck ./...

generate-proto:
	protoc --proto_path=pkg/proto --go_out=pkg/proto/gen --go_opt=paths=source_relative \
    	 --go-grpc_out=pkg/grpc --go-grpc_opt=paths=source_relative \
    	pkg/proto/**/*.proto


integration-test:
	go test -timeout 9000s -a -v -coverprofile=coverage.out -coverpkg=./... ./... -tags=integration 2>&1 | tee report.out


install-tools:
	# Godoc
	go get -u golang.org/x/tools/cmd/godoc@latest
	go install golang.org/x/tools/cmd/godoc@latest

	# Profiling
	go get -u github.com/google/pprof@latest
	go install github.com/google/pprof@latest

	# WebUI for Code Coverage
	go get -u github.com/smartystreets/goconvey@latest
	go install github.com/smartystreets/goconvey@latest

	# Security scanning tools
	go get -u github.com/securego/gosec/v2/cmd/gosec@latest
	go install github.com/securego/gosec/v2/cmd/gosec@latest

	go get -u golang.org/x/vuln/cmd/govulncheck@latest
	go install golang.org/x/vuln/cmd/govulncheck@latest

	# Linting and Formatting
	go get -u golang.org/x/tools/cmd/goimports@latest
	go install golang.org/x/tools/cmd/goimports@latest

	go get -u honnef.co/go/tools/cmd/staticcheck@latest
	go install honnef.co/go/tools/cmd/staticcheck@latest

	go get -u mvdan.cc/gofumpt@latest
	go install mvdan.cc/gofumpt@latest

	# migrations
	go get -u -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest
	go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest

	# test fixtures
	go get -u github.com/go-testfixtures/testfixtures/v3/cmd/testfixtures@latest
	go install github.com/go-testfixtures/testfixtures/v3/cmd/testfixtures@latest