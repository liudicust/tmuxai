CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o cnp-ai main.go
mv cnp-ai /usr/local/bin
