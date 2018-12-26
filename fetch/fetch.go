package fetch

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/textproto"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
	"github.com/olebedev/gojax/fetch/internal/data"
	"github.com/pkg/errors"
)

// ContextKey specifies a custom type for the fetch event loop context
type ContextKey string

// RequestContextKey defines a request scope context value
const RequestContextKey ContextKey = "request"

// ForwardedHeadersContextKey defines a list of headers that will be forwarded from the context request
const ForwardedHeadersContextKey ContextKey = "forwardedHeaders"

// Enable enables fetch for the instance. Loop instance is required instead of
// flat goja's. B/c fetch polyfill uses timeouts for promises.
//
// The second parameter could be any http handler. Even you local instance,
// to handle http requests locally programmatically.
func Enable(loop *eventloop.EventLoop, proxy http.Handler) error {
	if proxy == nil {
		return errors.New("proxy handler cannot be nil")
	}

	script := string(data.MustAsset("dist/bundle.js"))
	prg, err := goja.Compile("fetch.js", script, false)
	if err != nil {
		return errors.Wrap(err, "compile script")
	}
	loop.RunOnLoop(func(vm *goja.Runtime) {
		vm.Set("__fetch__", request(loop, proxy))
		_, err := vm.RunProgram(prg)
		if err != nil {
			panic(err)
		}
	})

	return nil
}

func request(loop *eventloop.EventLoop, proxy http.Handler) func(call goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		if fn, ok := goja.AssertFunction(call.Argument(2)); ok {
			u := call.Argument(0).String()
			o := call.Argument(1).Export().(map[string]interface{})
			ctx := loop.GetContext()

			go func() {
				var body io.Reader
				method := http.MethodGet
				header := make(http.Header)

				if headers, ex := o["headers"]; ex {
					if hmap, okh := headers.(map[string]interface{}); okh {
						for key, value := range hmap {
							v := value.([]interface{})
							for _, item := range v {
								var i []string
								i = append(i, item.(string))
								header[textproto.CanonicalMIMEHeaderKey(key)] = i
							}
						}
					}
				}

				if b, ex := o["body"]; ex {
					if bo, ok := b.(string); ok {
						body = bytes.NewBufferString(bo)
					}
				}

				if m, ex := o["method"]; ex {
					if me, ok := m.(string); ok {
						method = me
					}
				}

				var toRet map[string]interface{}

				res := httptest.NewRecorder()
				req, err := http.NewRequest(method, u, body)
				if err != nil {
					toRet = map[string]interface{}{
						"body":    fmt.Sprintf("Internal Server Error: %s", err.Error()),
						"headers": make(map[string][]string),
						"status":  http.StatusInternalServerError,
						"method":  method,
						"url":     u,
					}
				} else {
					compositeRequest(ctx, &header, req)

					req.Header = header
					proxy.ServeHTTP(res, req)
					toRet = map[string]interface{}{
						"body":    res.Body.String(),
						"headers": map[string][]string(res.Header()),
						"status":  res.Code,
						"method":  method,
						"url":     u,
					}
				}
				loop.RunOnLoop(func(vm *goja.Runtime) { fn(nil, vm.ToValue(toRet)) })
			}()
		}
		return nil
	}
}

// composite loop scoped request headers, context, and cookies into the synthetic
// request if a request exists in the EventLoop context
func compositeRequest(ctx context.Context, header *http.Header, req *http.Request) {
	if ctx == nil {
		return
	}

	// composite loop scoped request headers, context, and cookies if exists in the EventLoop context
	if contextRequest := ctx.Value(RequestContextKey); contextRequest != nil {
		castRequest, ok := contextRequest.(*http.Request)
		if ok {
			if len(castRequest.Cookies()) > 0 {
				for _, cookie := range castRequest.Cookies() {
					req.AddCookie(cookie)
				}
			}

			if castRequest.Context() != nil {
				req = req.WithContext(castRequest.Context())
			}

			forwardedHeaders := ctx.Value(ForwardedHeadersContextKey)
			if forwardedHeaders != nil {
				castForwardedHeaders, ok := forwardedHeaders.([]string)
				if ok {
					if len(castForwardedHeaders) > 0 {
						for _, headerName := range castForwardedHeaders {
							headerValues := castRequest.Header[headerName]
							addToHeader(header, headerName, headerValues)
						}
					} else {
						for headerName, headerValues := range castRequest.Header {
							addToHeader(header, headerName, headerValues)
						}
					}
				}
			}
		}
	}
}

func addToHeader(header *http.Header, headerName string, headerValues []string) {
	for _, headerValue := range headerValues {
		header.Add(headerName, headerValue)
	}
}
