# fetch [![godoc](http://img.shields.io/badge/godoc-reference-blue.svg?style=flat)](https://godoc.org/github.com/olebedev/gojax/fetch)

> a window.fetch JavaScript polyfill

### Usage

Install via `go get https://github.com/olebedev/gojax/fetch`.

```go
package main

import (
	"fmt"
	"net/http/httputil"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
	"github.com/olebedev/gojax/fetch"
)

func main() {
	loop := eventloop.NewEventLoop()
	loop.Start()
	defer loop.Stop()

	fetch.Enable(loop, httputil.NewSingleHostReverseProxy(url.Parse("/")))

	wait := make(chan string, 1)
	loop.RunOnLoop(func(vm *goja.Runtime) {
		vm.Set("callback", func(call goja.FunctionCall) goja.Value {
			wait <- call.Argument(0).ToString().String()
			return nil
		})

		vm.RunString(`
			fetch('https://ya.ru').then(function(resp){
				return resp.text();
			}).then(function(resp){
				callback(resp.slice(0, 15));
			});
		`)
	})
	fmt.Println(<-wait)
}
```

This program will prints `<!DOCTYPE html>` into stdout. See `fetch_test.go` for more examples.

### Request Forwarding

[fetch](https://github.com/olebedev/gojax/tree/master/fetch) creates a synthetic http.Request, and will not forward any of the original requests context, headers, or cookies by default. Users who need access to the original requests context, headers, and cookies can set a context to your fetch enabled EventLoop.

```go
package main

import (
	"context"
	"fmt"
	"net/http/httputil"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
	"github.com/olebedev/gojax/fetch"
)

var forwardedHeaders = []string{"X-My-Header"}

func generateRequestContext(request *http.Request) (ctx context.Context) {
	ctx = context.Background()
	ctx = context.WithValue(ctx, fetch.RequestContextKey, request)
	// optional - a nil list will forward no headers, and empty list will forward all headers, specified lists will only forward the headers specified
	ctx = context.WithValue(ctx, fetch.ForwardedHeadersContextKey, forwardedHeaders)
	return
}

func main() {
	loop := eventloop.NewEventLoop()
	loop.Start()
	defer loop.Stop()

	fetch.Enable(loop, httputil.NewSingleHostReverseProxy(url.Parse("/")))

	// create a context that holds the request and any headers you want forwarded from the original request
	ctx := generateRequestContext(request)

	wait := make(chan string, 1)

	// call RunOnLoopWithContext instead of RunOnLoop to set the context for the individual execution run
	loop.RunOnLoopWithContext(ctx, func(vm *goja.Runtime) {
		vm.Set("callback", func(call goja.FunctionCall) goja.Value {
			wait <- call.Argument(0).ToString().String()
			return nil
		})

		vm.RunString(`
			fetch('https://ya.ru').then(function(resp){
				return resp.text();
			}).then(function(resp){
				callback(resp.slice(0, 15));
			});
		`)
	})
	fmt.Println(<-wait)
}
```

