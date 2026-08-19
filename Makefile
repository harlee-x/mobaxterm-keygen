# MobaXterm KeyGen - 一键构建
# 用法: make           构建当前平台二进制
#       make build     同上
#       make run       构建并运行
#       make clean     清理产物

BINARY_NAME := MobaXtermKeyGen
PKG := .

GO := go

.PHONY: all build run clean

all: build

build:
	$(GO) build -o $(BINARY_NAME) $(PKG)

run: build
	./$(BINARY_NAME)

clean:
	rm -f $(BINARY_NAME)
	rm -f *.zip
