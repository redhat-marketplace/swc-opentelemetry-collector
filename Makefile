IMAGE_REGISTRY ?= localhost
IMAGE_NAME ?= swc-opentelemetry-collector

build:
	mkdir -p output
	docker run --user 0:0 -v "$$(pwd)/builder-config.yaml:/build/builder-config.yaml" -v "$$(pwd)/output:/tmp/dist" --entrypoint /usr/local/bin/ocb docker.io/otel/opentelemetry-collector-builder:latest --config=/build/builder-config.yaml

docker-build:
	docker build -f builder.Dockerfile -t $(IMAGE_REGISTRY)/$(IMAGE_NAME) .

run:
	mkdir -p otc
	./output/otelcol-custom --config=./exporter/ibmsoftwarecentralexporter/examples/config_ils.yaml

add-licenses: addlicense
	find . -type f -name "*.go" | xargs addlicense -c "IBM Corp."

addlicense:
	go install github.com/google/addlicense@v1.1.1
