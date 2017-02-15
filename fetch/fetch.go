package fetch

import (
	"net/http"

	"github.com/dop251/goja"
	"github.com/dop251/goja_nodejs/eventloop"
	"github.com/pkg/errors"
)

func Enable(loop *eventloop.EventLoop, handler http.Handler) error {
	script := string(MustAsset("dist/bundle.js"))
	prg, err := goja.Compile("fetch.js", script, true)
	if err != nil {
		return errors.Wrap(err, "compile")
	}

	return nil
}
