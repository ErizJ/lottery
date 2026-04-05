.PHONY: all build run gotool clean mock help

BINARY="main"
MOCK_PORT ?= 3000

all:
	gotool build

build:
	GOOS=linux GOARCH=amd64 go build -o ${BINARY}

run:
	@go run ./

gotool:
	go fmt ./

clean:
	go clean

# 启动纯前端 mock 服务（无需 Go 后端）
# 访问: http://localhost:3000/lottery.html?mock=1
mock:
	@echo ">>> Mock 服务启动: http://localhost:$(MOCK_PORT)/lottery.html?mock=1"
	@cd view && python3 -m http.server $(MOCK_PORT)

help:
	@echo "make run        - 启动完整后端服务"
	@echo "make mock       - 启动纯前端 mock（无需后端）"
	@echo "make build      - 编译 Linux 二进制"
	@echo "make gotool     - 格式化 Go 代码"
	@echo "make clean      - 清理编译产物"
