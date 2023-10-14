build-linux:
	go build -o .
build-windows:
	GOOS=windows GOARCH=amd64 go build -o .
