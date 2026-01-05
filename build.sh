CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags "-s -w" -o cnp-ai main.go
mv cnp-ai /usr/local/bin
