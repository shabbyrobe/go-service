package serviceutil

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"testing"
	"time"

	service "github.com/shabbyrobe/go-service"
	"github.com/shabbyrobe/go-service/servicemgr"
	"github.com/shabbyrobe/golib/assert"
)

func TestHTTP(t *testing.T) {
	tt := assert.WrapTB(t)

	bts := []byte("yep")
	h := NewHTTP("http", &http.Server{
		Addr: ":0",
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write(bts)
		}),
	})

	ender := servicemgr.NewEndListener(1)
	runner := service.NewRunner(ender)
	defer runner.HaltAll(1*time.Second, 0)
	tt.MustOK(runner.StartWait(1*time.Second, h))

	hc, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d", h.Port()))
	tt.MustOK(err)
	b, err := ioutil.ReadAll(hc.Body)
	tt.MustOK(err)
	tt.MustEqual(string(b), string(bts))
	tt.MustOK(runner.HaltAll(1*time.Second, 0))
}
