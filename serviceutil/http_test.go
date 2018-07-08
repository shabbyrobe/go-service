package serviceutil

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"testing"
	"time"

	service "github.com/shabbyrobe/go-service"
	"github.com/shabbyrobe/golib/assert"
)

func TestHTTP(t *testing.T) {
	tt := assert.WrapTB(t)

	bts := []byte("yep")
	h := NewHTTP(&http.Server{
		Addr: ":0",
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write(bts)
		}),
	})

	ender := service.NewEndListener(1)
	runner := service.NewRunner(service.RunnerOnEnd(ender.OnEnd))
	defer service.MustShutdownTimeout(1*time.Second, runner)
	tt.MustOK(service.StartTimeout(1*time.Second, runner, service.New("http", h)))

	hc, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d", h.Port()))
	tt.MustOK(err)
	b, err := ioutil.ReadAll(hc.Body)
	tt.MustOK(err)
	tt.MustEqual(string(b), string(bts))
	tt.MustOK(service.ShutdownTimeout(1*time.Second, runner))
}
