.PHONY:all build run gotool clean help.

BINARY="main"

all:
	gotool build

build:
	GOOS=windows GOARCH=amd64 go build -o ${BINARY}

run: 
	@go run ./ 

gotool:
	go fmt ./ 

clean:
	go clean

help:
	@echo "make - 格式化Go代码，并编译生成二进制文件"
	@echo "make build - 编译Go代码生成二进制文件"
	@echo "make run - 直接运行Go代码"
	@echo "make clean - 移除二进制文件"
	@echo "make gotool - 运行Go工具'fmt'"
