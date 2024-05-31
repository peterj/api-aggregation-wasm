package main

import (
	"github.com/tetratelabs/proxy-wasm-go-sdk/proxywasm"
	"github.com/tetratelabs/proxy-wasm-go-sdk/proxywasm/types"
	"github.com/tidwall/gjson"
)

func main() {
	proxywasm.SetVMContext(&vmContext{})
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
}

// OnPluginStart implements types.PluginContext.
func (ctx *pluginContext) OnPluginStart(pluginConfigurationSize int) types.OnPluginStartStatus {
	// data, err := proxywasm.GetPluginConfiguration()
	// if err != nil && err != types.ErrorStatusNotFound {
	// 	proxywasm.LogCriticalf("error reading plugin configuration: %v", err)
	// 	return types.OnPluginStartStatusFailed
	// }

	return types.OnPluginStartStatusOK
}

// NewHttpContext implements types.PluginContext.
func (ctx *pluginContext) NewHttpContext(contextID uint32) types.HttpContext {
	return &httpRouting{}
}

// httpRouting implements types.HttpContext.
type httpRouting struct {
	// Embed the default http context here,
	// so that we don't need to reimplement all the methods.
	types.DefaultHttpContext

	headersResponseBody []byte
	ipResponseBody      []byte
}

// OnHttpRequestHeaders implements types.HttpContext.
func (ctx *httpRouting) OnHttpRequestHeaders(numHeaders int, endOfStream bool) types.Action {
	proxywasm.LogInfo("Hello from OnHttpRequestHeaders")

	proxywasm.LogInfo("dispatching http call to httpbin/headers")
	_, err := proxywasm.DispatchHttpCall("httpbin", [][2]string{
		{":authority", "httpbin.org"},
		{":path", "/headers"},
		{":method", "GET"},
	}, nil, nil, 5000, ctx.httpbinHeadersCallback)

	if err != nil {
		proxywasm.LogCriticalf("error dispatching http call: %v", err)
		return types.ActionContinue
	}

	proxywasm.LogInfo("dispatching http call to httpbin/ip")
	_, err = proxywasm.DispatchHttpCall("httpbin", [][2]string{
		{":authority", "httpbin.org"},
		{":path", "/ip"},
		{":method", "GET"},
	}, nil, nil, 5000, ctx.httpbinIpCallback)

	if err != nil {
		proxywasm.LogCriticalf("error dispatching http call: %v", err)
		return types.ActionContinue
	}

	proxywasm.LogInfo("Waiting for responses in OnHttpRequestHeaders...")
	return types.ActionPause
}

func (ctx *httpRouting) httpbinHeadersCallback(numHeaders, bodySize, numTrailers int) {
	defer func() {
		if ctx.ipResponseBody == nil {
			return
		}
		proxywasm.ResumeHttpRequest()
	}()

	proxywasm.LogInfo("Hello from httpbin/headers")

	body, err := proxywasm.GetHttpCallResponseBody(0, bodySize)
	if err != nil {
		proxywasm.LogCriticalf("error reading http call response body: %v", err)
		return
	}

	proxywasm.LogInfof("httpbin/headers response body: %s", string(body))

	// Set the response body to be sent aggregated and sent to the client later
	ctx.headersResponseBody = body
}

func (ctx *httpRouting) httpbinIpCallback(numHeaders, bodySize, numTrailers int) {
	defer func() {
		if ctx.headersResponseBody == nil {
			return
		}
		proxywasm.ResumeHttpRequest()
	}()

	proxywasm.LogInfo("Hello from httpbin/ip")

	body, err := proxywasm.GetHttpCallResponseBody(0, bodySize)
	if err != nil {
		proxywasm.LogCriticalf("error reading http call response body: %v", err)
		return
	}

	proxywasm.LogInfof("httpbin/ip response body: %s", string(body))
	ctx.ipResponseBody = body
}

func (ctx *httpRouting) OnHttpResponseHeaders(numHeaders int, endOfStream bool) types.Action {
	proxywasm.LogInfo("Hello from OnHttpResponseHeaders")
	if ctx.headersResponseBody == nil || ctx.ipResponseBody == nil {
		proxywasm.LogInfo("waiting for response from the /headers or /ip call")
		return types.ActionPause
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

	// Get the "X-Amzn-Trace-Id" from the ctx.headersResponseBody
	headersJsonResponse := gjson.ParseBytes(ctx.headersResponseBody)
	amznTraceId := headersJsonResponse.Get("headers.X-Amzn-Trace-Id").String()

	// Get the "origin" from the ctx.ipResponseBody
	ipJsonResponse := gjson.ParseBytes(ctx.ipResponseBody)
	origin := ipJsonResponse.Get("origin").String()

	body := []byte(`{"amazon-trace-id" : "` + amznTraceId + `", "origin": "` + origin + `"}`)
	if err := proxywasm.ReplaceHttpResponseBody(body); err != nil {
		proxywasm.LogCriticalf("error replacing response body: %v", err)
	}
	_ = proxywasm.ResumeHttpResponse()
	return types.ActionContinue
}
