VERSION=0.1
BUILD_TIME=$(shell date +%s)

.PHONY: protos build

build:
	go build

protos:
	rm -rf protos/*
	protoc -I=./Protobuf/ --go_out=./protos ./Protobuf/*.proto
