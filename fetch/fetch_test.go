package fetch

import (
	"context"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
	"testing"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
	"github.com/stretchr/testify/require"
)

type testCtxKey string

const (
	testCookieKey                      = "someCookie"
	testCookieValue                    = "someCookieValue"
	testRequestContextKey   testCtxKey = "someCtxKey"
	testRequestContextValue            = "someCtxValue"
	testForwardedHeader                = "X-User-Header"
	testUser                           = "olebedev"
)

var testForwardedHeaders = []string{testForwardedHeader}

func TestEnable(t *testing.T) {
	loop := eventloop.NewEventLoop()
	loop.Start()
	defer loop.Stop()

	url, _ := url.Parse("/")
	proxy := httputil.NewSingleHostReverseProxy(url)

	Enable(loop, proxy)

	var v goja.Value
	var err error
	var wg sync.WaitGroup
	wg.Add(1)
	loop.RunOnLoop(func(vm *goja.Runtime) {
		v, err = vm.RunString(`typeof fetch`)
		wg.Done()
	})

	wg.Wait()
	require.Nil(t, err)
	require.NotNil(t, v)
	require.Equal(t, "function", v.ToString().String())
}

func TestRequest(t *testing.T) {
	loop := eventloop.NewEventLoop()
	loop.Start()
	defer loop.Stop()

	url, _ := url.Parse("https://ya.ru")
	proxy := httputil.NewSingleHostReverseProxy(url)

	Enable(loop, proxy)

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

	require.Equal(t, "<!DOCTYPE html>", <-wait)
}

func TestRequestWithContext(t *testing.T) {
	loop := eventloop.NewEventLoop()
	loop.Start()
	defer loop.Stop()

	url, _ := url.Parse("https://ya.ru")
	proxy := httputil.NewSingleHostReverseProxy(url)

	Enable(loop, proxy)

	ctx := generateLoopContext(t)

	wait := make(chan string, 1)
	loop.RunOnLoopWithContext(ctx, func(vm *goja.Runtime) {
		vm.Set("callback", func(call goja.FunctionCall) goja.Value {
			wait <- call.Argument(0).ToString().String()
			return nil
		})

		verifyRequest(t, loop)

		vm.RunString(`
			fetch('https://ya.ru').then(function(resp){
				return resp.text();
			}).then(function(resp){
				callback(resp.slice(0, 15));
			});
		`)
	})

	require.Equal(t, "<!DOCTYPE html>", <-wait)
}

func verifyRequest(t *testing.T, eventLoop *eventloop.EventLoop) {
	ctx := eventLoop.GetContext()
	if ctx == nil {
		t.Error("expected EventLoop context but none was found")
	}

	contextRequest := ctx.Value(RequestContextKey)
	if contextRequest == nil {
		t.Error("expected request context but none was found")
	}

	castRequest, ok := contextRequest.(*http.Request)
	if !ok {
		t.Error("could not cast context request to *http.Request")
	}

	verifyHeaders(t, castRequest)
	verifyCookies(t, castRequest)
	verifyContext(t, castRequest)
}

func verifyHeaders(t *testing.T, request *http.Request) {
	result := request.Header.Get(testForwardedHeader)

	require.Equal(t, testUser, result)
}

func verifyCookies(t *testing.T, request *http.Request) {
	result, err := request.Cookie(testCookieKey)
	if err != nil {
		t.Errorf("expected cookie %s but it was not found, error: %v", testCookieKey, err)
	}

	require.Equal(t, testCookieValue, result.Value)
}

func verifyContext(t *testing.T, request *http.Request) {
	ctx := request.Context()
	if ctx == nil {
		t.Error("expected context but none found")
	}

	result := ctx.Value(testRequestContextKey)
	if result == nil {
		t.Errorf("expected context value with key %s but it was not found", testRequestContextKey)
	}

	require.Equal(t, testRequestContextValue, result)
}

func generateLoopContext(t *testing.T) context.Context {
	request := generateRequest(t)

	ctx := context.Background()
	ctx = context.WithValue(ctx, RequestContextKey, request)
	ctx = context.WithValue(ctx, ForwardedHeadersContextKey, testForwardedHeaders)

	return ctx
}

func generateRequest(t *testing.T) *http.Request {
	req, err := http.NewRequest(http.MethodGet, "https://ya.ru", nil)
	if err != nil {
		t.Error("failed to create http.Request")
	}

	req.Header.Set(testForwardedHeader, testUser)

	cookie := generateCookie()
	req.AddCookie(cookie)

	ctx := generateRequestContext()
	req = req.WithContext(ctx)

	return req
}

func generateCookie() *http.Cookie {
	return &http.Cookie{
		Name:  testCookieKey,
		Value: testCookieValue,
	}
}

func generateRequestContext() context.Context {
	ctx := context.Background()
	ctx = context.WithValue(ctx, testRequestContextKey, testRequestContextValue)

	return ctx
}

func TestCustom(t *testing.T) {
	loop := eventloop.NewEventLoop()
	loop.Start()
	defer loop.Stop()

	Enable(loop, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusMethodNotAllowed)
		w.Header().Add("Content-Type", "application/json")
		w.Write([]byte(`{"error": "method not allowed"}`))
	}))

	wait := make(chan string, 3)
	loop.RunOnLoop(func(vm *goja.Runtime) {
		vm.Set("callback", func(call goja.FunctionCall) goja.Value {
			wait <- call.Argument(0).ToString().String()
			return nil
		})

		vm.RunString(`
			fetch('https://ya.ru').then(function(resp){
				callback(resp.ok);
				callback(resp.status);
				callback(resp.url);
				callback(resp.method);
				callback(resp.headers.get('content-type'));
				return resp.json();
			}).then(function(resp){
				callback(resp.error);
			});
		`)
	})

	require.Equal(t, "false", <-wait)
	require.Equal(t, "405", <-wait)
	require.Equal(t, "https://ya.ru", <-wait)
	require.Equal(t, "GET", <-wait)
	require.Equal(t, "application/json", <-wait)
	require.Equal(t, "method not allowed", <-wait)
}
