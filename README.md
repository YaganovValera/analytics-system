go tool cover -html=coverage.out -o coverage.html

go test ./pkg/... -coverprofile=coverage.out