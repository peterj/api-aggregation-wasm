# API Aggregation with Envoy & WASM

I'll try to build a simple API aggregation plugin for Envoy using WebAssembly (Wasm) as the extension mechanism.

- Live stream: https://www.youtube.com/watch?v=Tbdj5Sw6HI0


## What are we building?

If I make a request to `/hello` on the proxy, I want the proxy (or the extension) to make a call to:

- GET host1 `/one`
- GET host2 `/two`

Combine (or aggregate) the responses from those two endpoints and then return that response.

We make request to `/hello` --> Proxy makes requests to `/one` and `/two` --> Proxy aggregates the responses and returns it to the client.

```
GET /hello --> [{ "value": "one" }, { "value": "two"}]
/one --> { "value": "one" }
/two --> { "value": "two" }
```

## Configuration

```json
[{
    "path": "/",
    "upstreams": [
        {
            "clusterName": "httpbin",
            "path": "/ip"
        },
        {
            "clusterName": "httpbin",
            "path": "/headers"
        }
    ],
}]
```


## TODOs

- [x] Scaffold the basic Wasm extension project
- [x] Create (aka re-use) the Makefile for building the Wasm extension
- [x] Implement calling multiple upstream services from the Wasm extension & aggregating the responses in some way
- [x] Make the extension configurable with the upstream services to call and the aggregation strategy
- [ ] Add authority and method to the configuration
- [ ] Properly aggregate the responses