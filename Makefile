
GO := go


all: build

release: format build

build:
	$(GO) build redis_exporter.go

format:
	$(GO) fmt redis_exporter.go

clean:
	@rm -rf redis_exporter
	@echo Done.
