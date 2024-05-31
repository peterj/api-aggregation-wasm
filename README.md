# API Aggregation with Envoy & WASM

I'll try to build a simple API aggregation plugin for Envoy using WebAssembly (Wasm) as the extension mechanism.

- Live stream: https://www.youtube.com/watch?v=Tbdj5Sw6HI0 


## TODOs:

- [ ] Scaffold the basic Wasm extension project
- [ ] Create (aka re-use) the Makefile & Dockerfile for building the Wasm extension
- [ ] Implement calling multiple upstream services from the Wasm extension & aggregating the responses in some way
- [ ] Make the extension configurable with the upstream services to call and the aggregation strategy