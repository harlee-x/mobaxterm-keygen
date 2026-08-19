# MobaXterm KeyGen - 一键构建（仅 Windows）
# 用法: make           构建 Windows 可执行文件 (MobaXtermKeyGen.exe)
#       make build     同上
#       make run       构建并运行（仅 Windows 平台有效）
#       make clean     清理产物

BINARY_NAME := MobaXtermKeyGen
WIN_BINARY := $(BINARY_NAME).exe
PKG := .

GO := go

.PHONY: all build run clean

all: build

build:
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 $(GO) build -o $(WIN_BINARY) $(PKG)

run: build
	./$(WIN_BINARY)

clean:
	rm -f $(WIN_BINARY)
	rm -f *.zip
