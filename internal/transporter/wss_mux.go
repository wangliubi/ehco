package transporter

import (
	"context"
	"net"
	"net/http"
	"time"

	"github.com/gobwas/ws"
	"github.com/labstack/echo/v4"
	"github.com/xtaci/smux"

	"github.com/Ehco1996/ehco/internal/metrics"
	"github.com/Ehco1996/ehco/pkg/lb"
)

var (
	_ RelayClient = &MwssClient{}
	_ RelayServer = &MwssServer{}
	_ muxServer   = &MwssServer{}
)

type MwssClient struct {
	*WssClient

	muxTP *smuxTransporter
}

func newMwssClient(base *baseTransporter) (*MwssClient, error) {
	wc, err := newWssClient(base)
	if err != nil {
		return nil, err
	}
	c := &MwssClient{WssClient: wc}
	c.muxTP = NewSmuxTransporter(c.l.Named("mwss"), c.initNewSession)
	return c, nil
}

func (c *MwssClient) initNewSession(ctx context.Context, addr string) (*smux.Session, error) {
	rc, _, _, err := c.dialer.Dial(ctx, addr)
	if err != nil {
		return nil, err
	}
	// stream multiplex
	cfg := smux.DefaultConfig()
	cfg.KeepAliveDisabled = true
	session, err := smux.Client(rc, cfg)
	if err != nil {
		return nil, err
	}
	c.l.Infof("init new session to: %s", rc.RemoteAddr())
	return session, nil
}

func (s *MwssClient) TCPHandShake(remote *lb.Node) (net.Conn, error) {
	t1 := time.Now()
	mwssc, err := s.muxTP.Dial(context.TODO(), remote.Address+"/handshake/")
	if err != nil {
		return nil, err
	}
	latency := time.Since(t1)
	metrics.HandShakeDuration.WithLabelValues(remote.Label).Observe(float64(latency.Milliseconds()))
	remote.HandShakeDuration = latency
	return mwssc, nil
}

type MwssServer struct {
	*WssServer
	*muxServerImpl
}

func newMwssServer(base *baseTransporter) (*MwssServer, error) {
	wssServer, err := newWssServer(base)
	if err != nil {
		return nil, err
	}
	s := &MwssServer{
		WssServer:     wssServer,
		muxServerImpl: newMuxServer(base.cfg.Listen, base.l.Named("mwss")),
	}
	s.e.GET("/handshake/", echo.WrapHandler(http.HandlerFunc(s.HandleRequest)))
	return s, nil
}

func (s *MwssServer) ListenAndServe() error {
	go func() {
		s.errChan <- s.e.StartServer(s.httpServer)
	}()

	for {
		conn, e := s.Accept()
		if e != nil {
			return e
		}
		go func(c net.Conn) {
			if err := s.RelayTCPConn(c, s.relayer.TCPHandShake); err != nil {
				s.l.Errorf("RelayTCPConn error: %s", err.Error())
			}
		}(conn)
	}
}

func (s *MwssServer) HandleRequest(w http.ResponseWriter, r *http.Request) {
	conn, _, _, err := ws.UpgradeHTTP(r, w)
	if err != nil {
		s.l.Error(err)
		return
	}
	s.mux(conn)
}

func (s *MwssServer) Close() error {
	return s.e.Close()
}
