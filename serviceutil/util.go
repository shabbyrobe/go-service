package serviceutil

import (
	"net"
	"net/http"
	"sync"
	"time"
)

// Ready allows these utilities to signal when they are ready to callers.
// service.Context should implement Ready.
type Ready interface {
	Ready() error
}

// ReadyGroup wraps a sync.WaitGroup so it satisfies the Ready interface.
type ReadyGroup struct {
	sync.WaitGroup
}

func NewReadyGroup(cnt int) *ReadyGroup {
	rg := &ReadyGroup{}
	if cnt > 0 {
		rg.Add(cnt)
	}
	return rg
}

func (r *ReadyGroup) Ready() error {
	r.Done()
	return nil
}

// ListenAndServeHTTP allows you to use this very convenient method
// but, unlike the stdlib version, also get a signal when it's ready.
//
// Several bowls of copy-pasta were harmed in the making of this
// function.
//
func ListenAndServeHTTP(srv *http.Server, ready Ready) error {
	addr := srv.Addr
	if addr == "" {
		addr = ":http"
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	if err := ready.Ready(); err != nil {
		return err
	}
	return srv.Serve(tcpKeepAliveListener{TCPListener: ln.(*net.TCPListener)})
}

// ListenAndServeTLS allows you to use this very convenient method
// but, unlike the stdlib version, also get a signal when it's ready.
//
// Several bowls of copy-pasta were harmed in the making of this
// function.
//
// If your server contains TLSConfig with populated certificates, you can pass
// an empty string for both certFile and keyFile.
//
func ListenAndServeTLS(srv *http.Server, certFile, keyFile string, ready Ready) error {
	addr := srv.Addr
	if addr == "" {
		addr = ":https"
	}

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	if err := ready.Ready(); err != nil {
		return err
	}
	return srv.ServeTLS(tcpKeepAliveListener{TCPListener: ln.(*net.TCPListener)}, certFile, keyFile)
}

// tcpKeepAliveListener is copy-pasta from the go source. There appears to be
// no way to integrate ready signalling without copying the whole darn thing.
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
