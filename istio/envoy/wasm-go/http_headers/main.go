package main

import (
	"strings"

	"github.com/tidwall/gjson"

	"github.com/tetratelabs/proxy-wasm-go-sdk/proxywasm"
	"github.com/tetratelabs/proxy-wasm-go-sdk/proxywasm/types"
)

func main() {
	proxywasm.SetVMContext(&vmContext{})
}

type vmContext struct {
	// Embed the default VM context here,
	// so that we don't need to reimplement all the methods.
	types.DefaultVMContext
}

// Override types.DefaultVMContext.
func (*vmContext) NewPluginContext(contextID uint32) types.PluginContext {
	return &pluginContext{}
}

type pluginContext struct {
	// Embed the default plugin context here,
	// so that we don't need to reimplement all the methods.
	types.DefaultPluginContext

	// headerName and headerValue are the header to be added to response. They are configured via
	// plugin configuration during OnPluginStart.
	headerName  string
	headerValue string
}

// Override types.DefaultPluginContext.
func (p *pluginContext) NewHttpContext(contextID uint32) types.HttpContext {
	proxywasm.LogDebug("NewHttpContext()=================================")
	return &httpHeaders{
		contextID:   contextID,
		headerName:  p.headerName,
		headerValue: p.headerValue,
	}
}

func (p *pluginContext) OnPluginStart(pluginConfigurationSize int) types.OnPluginStartStatus {
	proxywasm.LogDebug("OnPluginStart()=================================")
	data, err := proxywasm.GetPluginConfiguration()
	if data == nil {
		return types.OnPluginStartStatusOK
	}

	if err != nil {
		proxywasm.LogCriticalf("error reading plugin configuration: %v", err)
		return types.OnPluginStartStatusFailed
	}

	if !gjson.Valid(string(data)) {
		proxywasm.LogCritical(`invalid configuration format; expected {"header": "<header name>", "value": "<header value>"}`)
		return types.OnPluginStartStatusFailed
	}

	p.headerName = strings.TrimSpace(gjson.Get(string(data), "header").Str)
	p.headerValue = strings.TrimSpace(gjson.Get(string(data), "value").Str)

	if p.headerName == "" || p.headerValue == "" {
		proxywasm.LogCritical(`invalid configuration format; expected {"header": "<header name>", "value": "<header value>"}`)
		return types.OnPluginStartStatusFailed
	}

	proxywasm.LogInfof("header from config: %s = %s", p.headerName, p.headerValue)

	return types.OnPluginStartStatusOK
}

type httpHeaders struct {
	// Embed the default http context here,
	// so that we don't need to reimplement all the methods.
	types.DefaultHttpContext
	contextID   uint32
	headerName  string
	headerValue string
}

// Override types.DefaultHttpContext.
func (ctx *httpHeaders) OnHttpRequestHeaders(numHeaders int, endOfStream bool) types.Action {
	proxywasm.LogDebug("OnHttpRequestHeaders()=================================")
	err := proxywasm.ReplaceHttpRequestHeader("test", "best")
	if err != nil {
		proxywasm.LogCritical("failed to set request header: test")
	}

	hs, err := proxywasm.GetHttpRequestHeaders()
	if err != nil {
		proxywasm.LogCriticalf("failed to get request headers: %v", err)
	}

	for _, h := range hs {
		proxywasm.LogInfof("request header --> %s: %s", h[0], h[1])
	}
	return types.ActionContinue
}

// Override types.DefaultHttpContext.
func (ctx *httpHeaders) OnHttpResponseHeaders(_ int, _ bool) types.Action {
	proxywasm.LogDebug("OnHttpResponseHeaders()=================================")
	proxywasm.LogInfof("adding header: %s=%s", ctx.headerName, ctx.headerValue)

	// Add a hardcoded header
	if err := proxywasm.AddHttpResponseHeader("x-proxy-wasm-go-sdk-example", "http_headers"); err != nil {
		proxywasm.LogCriticalf("failed to set response constant header: %v", err)
	}

	// Add the header passed by arguments
	if ctx.headerName != "" {
		if err := proxywasm.AddHttpResponseHeader(ctx.headerName, ctx.headerValue); err != nil {
			proxywasm.LogCriticalf("failed to set response headers: %v", err)
		}
	}

	// Get and log the headers
	hs, err := proxywasm.GetHttpResponseHeaders()
	if err != nil {
		proxywasm.LogCriticalf("failed to get response headers: %v", err)
	}

	for _, h := range hs {
		proxywasm.LogInfof("response header <-- %s: %s", h[0], h[1])
	}
	return types.ActionContinue
}

// Override types.DefaultHttpContext.
func (ctx *httpHeaders) OnHttpStreamDone() {
	proxywasm.LogDebug("OnHttpStreamDone()=================================")
	proxywasm.LogInfof("%d finished", ctx.contextID)
}

func (ctx *httpHeaders) OnHttpRequestBody(_ int, _ bool) types.Action {
	proxywasm.LogDebug("OnHttpRequestBody()=================================")
	return types.ActionContinue
}
func (ctx *httpHeaders) OnHttpRequestTrailers(_ int) types.Action {
	proxywasm.LogDebug("OnHttpRequestTrailers()=================================")
	return types.ActionContinue
}
func (ctx *httpHeaders) OnHttpResponseBody(_ int, _ bool) types.Action {
	proxywasm.LogDebug(" OnHttpResponseBody()=================================")
	return types.ActionContinue
}
func (ctx *httpHeaders) OnHttpResponseTrailers(_ int) types.Action {
	proxywasm.LogDebug("OnHttpResponseTrailers()=================================")
	return types.ActionContinue
}
