build: main.go go.mod
	@echo "Building wasm..."
	@go get
	@tinygo build -o main.wasm -scheduler=none -target=wasi

run: build
	@echo "Running the proxy..."
	docker run -p 10000:10000 -p 8001:8001 -v $(PWD)/envoy.yaml:/etc/envoy/envoy.yaml -v $(PWD)/main.wasm:/main.wasm envoyproxy/envoy:v1.29-latest 
