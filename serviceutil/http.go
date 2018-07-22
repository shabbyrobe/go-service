package serviceutil

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"sync/atomic"
	"time"

	service "github.com/shabbyrobe/go-service"
)

const DefaultShutdownTimeout = 5 * time.Second

// HTTP wraps a http.Server in a service.Service.
type HTTP struct {
	Name   service.Name
	Server *http.Server

	// When the service is instructed to stop, wait this long for
	// existing clients to finish before terminating.
	ShutdownTimeout time.Duration

	TLS      bool   // Starts with ListenAndServeTLS if true.
	CertFile string // If TLS == true, use this file for the certificate.
	KeyFile  string // If TLS == true, use this file for the key.

	port int32
}

var _ service.Runnable = &HTTP{}

func NewHTTP(server *http.Server) *HTTP {
	return &HTTP{Server: server}
}

func (h *HTTP) Port() int { return int(atomic.LoadInt32(&h.port)) }

// SetTLSCertFile configures the HTTP server to use TLS with a cert file and a
// key file
//
// Any certificates set in Server.TLSConfig are cleared.
func (h *HTTP) SetTLSCertFile(certFile, keyFile string) {
	h.TLS = true
	h.CertFile = certFile
	h.KeyFile = keyFile
	h.Server.TLSConfig.Certificates = nil
	if h.Server.TLSConfig == nil {
		h.Server.TLSConfig = &tls.Config{}
	}
}

// SetTLSCertFile configures the HTTP server to use TLS with a cert and key
// supplied as byte arrays.
//
// Any certificates set in Server.TLSConfig, or set in CertFile/KeyFile are
// cleared.
func (h *HTTP) SetTLSCert(certPem, keyPem []byte) error {
	h.TLS = true
	h.CertFile, h.KeyFile = "", ""

	key, err := tls.X509KeyPair([]byte(certPem), []byte(keyPem))
	if err != nil {
		return err
	}
	if h.Server.TLSConfig == nil {
		h.Server.TLSConfig = &tls.Config{}
	}
	h.Server.TLSConfig.Certificates = []tls.Certificate{key}
	return nil
}

func (h *HTTP) addr() string {
	addr := h.Server.Addr
	if addr == "" && !h.TLS {
		addr = ":http"
	} else if addr == "" && h.TLS {
		addr = ":https"
	}
	return addr
}

// Run the HTTP server as a service.Service.
func (h *HTTP) Run(ctx service.Context) (rerr error) {
	done := make(chan struct{})
	failer := service.NewFailureListener(1)

	go func() {
		defer close(done)
		defer atomic.StoreInt32(&h.port, 0)

		ln, err := net.Listen("tcp", h.addr())
		if err != nil {
			failer.Send(err)
			return
		}

		atomic.StoreInt32(&h.port, int32(ln.Addr().(*net.TCPAddr).Port))
		if err := ctx.Ready(); err != nil {
			failer.Send(err)
			return
		}

		failer.SendNonNil(h.Server.Serve(tcpKeepAliveListener{TCPListener: ln.(*net.TCPListener)}))
	}()

	defer func() {
		to := h.ShutdownTimeout
		if to <= 0 {
			to = DefaultShutdownTimeout
		}
		hctx, cancelFunc := context.WithTimeout(context.Background(), to)
		defer cancelFunc()
		if err := h.Server.Shutdown(hctx); err != nil && rerr == nil {
			rerr = err
		}
		<-done
	}()

	select {
	case err := <-failer.Failures():
		fmt.Println(err)
		return err
	case <-ctx.Done():
		return nil
	}
}

type tcpKeepAliveListener struct {
	*net.TCPListener
}

func (ln tcpKeepAliveListener) Accept() (c net.Conn, err error) {
	tc, err := ln.AcceptTCP()
	if err != nil {
		return
	}
	if err = tc.SetKeepAlive(true); err != nil {
		return
	}
	if err = tc.SetKeepAlivePeriod(3 * time.Minute); err != nil {
		return
	}
	return tc, nil
}
