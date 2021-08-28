package transporter

import (
	"context"
	"net"

	"github.com/Ehco1996/ehco/internal/logger"
	"github.com/Ehco1996/ehco/internal/web"
	"github.com/gobwas/ws"
)

type Ws struct {
	raw *Raw
}

func (s *Ws) GetOrCreateBufferCh(uaddr *net.UDPAddr) *BufferCh {
	return s.raw.GetOrCreateBufferCh(uaddr)
}

func (s *Ws) HandleUDPConn(uaddr *net.UDPAddr, local *net.UDPConn) {
	s.raw.HandleUDPConn(uaddr, local)
}

func (s *Ws) HandleTCPConn(c *net.TCPConn) error {
	defer c.Close()
	remote := s.raw.TCPRemotes.Next()
	web.CurTCPNum.WithLabelValues(remote.Label).Inc()
	defer web.CurTCPNum.WithLabelValues(remote.Label).Dec()

	wsc, _, _, err := ws.Dial(context.TODO(), remote.Address+"/ws/")
	if err != nil {
		return err
	}
	defer wsc.Close()
	logger.Infof("[ws] HandleTCPConn from %s to %s", c.LocalAddr().String(), remote.Label)
	return transportWithDeadline(c, wsc, remote.Label)
}