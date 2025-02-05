IMAGE_REGISTRY ?= localhost
IMAGE_NAME ?= swc-opentelemetry-collector

build:
	docker run --user 0:0 -v "$$(pwd)/builder-config.yaml:/build/builder-config.yaml" -v "$$(pwd)/output:/tmp/dist" --entrypoint /usr/local/bin/ocb docker.io/otel/opentelemetry-collector-builder:latest --config=/build/builder-config.yaml

docker-build:
	docker build -f builder.Dockerfile -t $(IMAGE_REGISTRY)/$(IMAGE_NAME) .

run:
	./output/otelcol-custom --config=./exporter/ibmsoftwarecentralexporter/examples/config_full.yaml

add-licenses: addlicense
	find . -type f -name "*.go" | xargs addlicense -c "IBM Corp."

addlicense:
	go install github.com/google/addlicense@v1.1.1