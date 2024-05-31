package main

import (
	"github.com/tetratelabs/proxy-wasm-go-sdk/proxywasm"
	"github.com/tetratelabs/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/tidwall/gjson"
)

func main() {
	proxywasm.SetVMContext(&vmContext{})
}

type UpstreamService struct {
	ID          string
	ClusterName string
	Path        string
	// TODO: We'll assume it's GET, but it should be configurable
	// Method      string

	// TODO: add authority value
}

type AggregationConfig struct {
	Path             string
	UpstreamServices []UpstreamService
}

type PluginConfiguration struct {
	Aggregations []AggregationConfig
}

// vmContext implements types.VMContext.
type vmContext struct {
	// Embed the default VM context here,
	// so that we don't need to reimplement all the methods.
	types.DefaultVMContext
}

// NewPluginContext implements types.VMContext.
func (*vmContext) NewPluginContext(contextID uint32) types.PluginContext {
	return &pluginContext{}
}

// pluginContext implements types.PluginContext.
type pluginContext struct {
	// Embed the default plugin context here,
	// so that we don't need to reimplement all the methods.
	types.DefaultPluginContext

	config PluginConfiguration
}

// OnPluginStart implements types.PluginContext.
func (ctx *pluginContext) OnPluginStart(pluginConfigurationSize int) types.OnPluginStartStatus {
	data, err := proxywasm.GetPluginConfiguration()
	if err != nil && err != types.ErrorStatusNotFound {
		proxywasm.LogCriticalf("error reading plugin configuration: %v", err)
		return types.OnPluginStartStatusFailed
	}

	jsonData := gjson.ParseBytes(data)
	cfg := PluginConfiguration{
		Aggregations: make([]AggregationConfig, 0),
	}

	for _, aggregation := range jsonData.Array() {
		upstreamServices := make([]UpstreamService, 0)
		for _, upstreamService := range aggregation.Get("upstreams").Array() {
			upstreamServices = append(upstreamServices, UpstreamService{
				ID:          upstreamService.Get("id").String(),
				ClusterName: upstreamService.Get("clusterName").String(),
				Path:        upstreamService.Get("path").String(),
			})
		}

		cfg.Aggregations = append(cfg.Aggregations, AggregationConfig{
			Path:             aggregation.Get("path").String(),
			UpstreamServices: upstreamServices,
		})
	}

	ctx.config = cfg
	proxywasm.LogInfof("parsed plugin configuration")

	return types.OnPluginStartStatusOK
}

// NewHttpContext implements types.PluginContext.
func (ctx *pluginContext) NewHttpContext(contextID uint32) types.HttpContext {
	return &httpRouting{config: ctx.config}
}

// httpRouting implements types.HttpContext.
type httpRouting struct {
	// Embed the default http context here,
	// so that we don't need to reimplement all the methods.
	types.DefaultHttpContext

	// An map of []byte to store response bodies by the UpstreamService ID
	responseBodies map[string][]byte

	config PluginConfiguration
}

// OnHttpRequestHeaders implements types.HttpContext.
func (ctx *httpRouting) OnHttpRequestHeaders(numHeaders int, endOfStream bool) types.Action {
	proxywasm.LogInfo("Hello from OnHttpRequestHeaders")

	pathHeader, err := proxywasm.GetHttpRequestHeader(":path")
	if err != nil {
		proxywasm.LogCriticalf("error reading request header: %v", err)
		return types.ActionPause
	}

	// Check if the request path is one of the configured aggregations
	// If not, we'll just let the request pass through
	aggregationConfig := AggregationConfig{}
	for _, aggregation := range ctx.config.Aggregations {
		if aggregation.Path == pathHeader {
			aggregationConfig = aggregation
			break
		}
	}

	// Create the response bodies array, using the UpstreamService ID as the key
	ctx.responseBodies = make(map[string][]byte)

	// Initialize the responseBodies array with nil values
	for _, upstream := range aggregationConfig.UpstreamServices {
		ctx.responseBodies[upstream.ID] = nil
	}

	// Dispatch http calls to the configured upstream services
	for _, upstream := range aggregationConfig.UpstreamServices {
		proxywasm.LogInfof("dispatching http call to %s", upstream.Path)
		_, err := proxywasm.DispatchHttpCall(upstream.ClusterName, [][2]string{
			// TODO: authority should come from the config
			{":authority", "httpbin.org"},
			{":path", upstream.Path},
			// TODO: from config at some point
			{":method", "GET"},
		}, nil, nil, 5000, func(numHeaders, bodySize, numTrailers int) {

			defer func() {
				proxywasm.LogInfo("checking if all responses are received")
				// Check whether the previous body responses are already received
				for id, body := range ctx.responseBodies {
					proxywasm.LogInfof("checking response body for %s", id)
					if id != upstream.ID {
						if body == nil {
							proxywasm.LogInfof("waiting for response from the upstream service: %s", id)
							return
						}
					}
				}
				_ = proxywasm.ResumeHttpRequest()
			}()

			body, err := proxywasm.GetHttpCallResponseBody(0, bodySize)
			if err != nil {
				proxywasm.LogCriticalf("error reading http call response body: %v", err)
				return
			}

			proxywasm.LogInfof("received response body: %s", string(body))
			// Assign the response body to the responseBodies array
			ctx.responseBodies[upstream.ID] = body

		})

		if err != nil {
			proxywasm.LogCriticalf("error dispatching http call: %v", err)
			return types.ActionContinue
		}
	}

	proxywasm.LogInfo("Waiting for responses...")
	return types.ActionPause
}

func (ctx *httpRouting) OnHttpResponseHeaders(numHeaders int, endOfStream bool) types.Action {
	proxywasm.LogInfo("Hello from OnHttpResponseHeaders")

	// Check if any of the response bodies are nil
	// If they are, we'll Pause
	for _, body := range ctx.responseBodies {
		if body == nil {
			proxywasm.LogInfo("waiting for response from the upstream services")
			return types.ActionPause
		}
	}

	if err := proxywasm.RemoveHttpResponseHeader("content-length"); err != nil {
		proxywasm.LogCriticalf("error removing response header: %v", err)
	}
	return types.ActionContinue
}

func (ctx *httpRouting) OnHttpResponseBody(bodySize int, endOfStream bool) types.Action {
	proxywasm.LogInfo("Hello from OnHttpResponseBody")
	if !endOfStream {
		proxywasm.LogInfo("waiting for the end of the response body")
		return types.ActionPause
	}

	newResult := ""
	// Parse the response bodies and aggregate them
	for _, body := range ctx.responseBodies {
		parsed := gjson.ParseBytes(body)
		newResult = newResult + parsed.Raw
	}

	if err := proxywasm.ReplaceHttpResponseBody([]byte(newResult)); err != nil {
		proxywasm.LogCriticalf("error replacing response body: %v", err)
		return types.ActionContinue
	}

	_ = proxywasm.ResumeHttpResponse()
	return types.ActionContinue
}
