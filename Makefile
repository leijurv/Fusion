VERSION=0.1
BUILD_TIME=$(shell date +%s)

.PHONY: protos build

build:
	go build

protos:
	rm -rf protos/*.go
	protoc -I=./protos/ --go_out=./protos ./protos/*.proto
