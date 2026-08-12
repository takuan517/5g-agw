package uplane

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/netip"
	"sync"
)

const DefaultBufferSize = 65535

type UDPGatewayConfig struct {
	RANListen  netip.AddrPort
	CoreListen netip.AddrPort
	BufferSize int
}

type UDPGateway struct {
	config UDPGatewayConfig
	router *Router
	logger *log.Logger
}

func NewUDPGateway(config UDPGatewayConfig, router *Router, logger *log.Logger) *UDPGateway {
	if config.BufferSize == 0 {
		config.BufferSize = DefaultBufferSize
	}
	if router == nil {
		router = NewRouter()
	}
	if logger == nil {
		logger = log.Default()
	}
	return &UDPGateway{
		config: config,
		router: router,
		logger: logger,
	}
}

func (g *UDPGateway) Serve(ctx context.Context) error {
	ranConn, err := net.ListenUDP("udp", net.UDPAddrFromAddrPort(g.config.RANListen))
	if err != nil {
		return fmt.Errorf("listen RAN side UDP: %w", err)
	}
	defer ranConn.Close()

	coreConn, err := net.ListenUDP("udp", net.UDPAddrFromAddrPort(g.config.CoreListen))
	if err != nil {
		return fmt.Errorf("listen core side UDP: %w", err)
	}
	defer coreConn.Close()

	g.logger.Printf("[UGW] listening RAN side on %s", ranConn.LocalAddr())
	g.logger.Printf("[UGW] listening core side on %s", coreConn.LocalAddr())

	errCh := make(chan error, 2)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		errCh <- g.proxyLoop(ctx, DirectionUplink, ranConn, coreConn)
	}()
	go func() {
		defer wg.Done()
		errCh <- g.proxyLoop(ctx, DirectionDownlink, coreConn, ranConn)
	}()

	var serveErr error
	select {
	case <-ctx.Done():
		serveErr = ctx.Err()
	case err := <-errCh:
		serveErr = err
	}

	ranConn.Close()
	coreConn.Close()
	wg.Wait()

	if errors.Is(serveErr, context.Canceled) {
		return nil
	}
	return serveErr
}

func (g *UDPGateway) proxyLoop(ctx context.Context, direction Direction, src *net.UDPConn, dst *net.UDPConn) error {
	buf := make([]byte, g.config.BufferSize)
	for {
		n, remote, err := src.ReadFromUDPAddrPort(buf)
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, net.ErrClosed) {
				return nil
			}
			return fmt.Errorf("%s read UDP: %w", direction, err)
		}

		result, err := g.router.Forward(direction, buf[:n])
		if err != nil {
			g.logger.Printf("[UGW] direction=%s from=%s drop: %v", direction, remote, err)
			continue
		}

		if _, err := dst.WriteToUDPAddrPort(result.Rewritten, result.Target); err != nil {
			return fmt.Errorf("%s write UDP target=%s: %w", direction, result.Target, err)
		}
		g.logger.Printf(
			"[UGW] direction=%s session=%s from=%s target=%s teid=0x%08x->0x%08x size=%d",
			direction,
			result.SessionID,
			remote,
			result.Target,
			result.Original.TEID,
			result.RewrittenPacket.TEID,
			len(result.Rewritten),
		)
	}
}
