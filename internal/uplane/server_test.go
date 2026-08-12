package uplane

import (
	"context"
	"errors"
	"io"
	"log"
	"net"
	"net/netip"
	"os"
	"testing"
	"time"
)

func TestUDPGatewayForwardsUplinkPacket(t *testing.T) {
	ranListen := reserveUDPAddrPort(t)
	coreListen := reserveUDPAddrPort(t)
	coreReceiver := listenUDPAddrPort(t, "127.0.0.1:0")
	defer coreReceiver.Close()

	router := NewRouter()
	router.Upsert(TunnelRoute{
		SessionID: "pdu-session-1",
		RAN:       Endpoint{Addr: ranListen, TEID: 0x11111111},
		Core:      Endpoint{Addr: udpLocalAddrPort(t, coreReceiver), TEID: 0xaaaa0001},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	gateway := NewUDPGateway(UDPGatewayConfig{RANListen: ranListen, CoreListen: coreListen}, router, log.New(io.Discard, "", 0))
	errCh := make(chan error, 1)
	go func() {
		errCh <- gateway.Serve(ctx)
	}()
	waitForUDPReady(t, ranListen)

	sendUDP(t, ranListen, BuildTPDU(0x11111111, []byte{0xde, 0xad}))
	received := readUDP(t, coreReceiver)
	parsed, err := ParsePacket(received)
	if err != nil {
		t.Fatalf("ParsePacket(received) error = %v", err)
	}
	if parsed.TEID != 0xaaaa0001 {
		t.Fatalf("received TEID = 0x%08x, want 0xaaaa0001", parsed.TEID)
	}

	cancel()
	if err := <-errCh; err != nil {
		t.Fatalf("Serve() error = %v", err)
	}
}

func reserveUDPAddrPort(t *testing.T) netip.AddrPort {
	t.Helper()

	conn := listenUDPAddrPort(t, "127.0.0.1:0")
	defer conn.Close()
	return udpLocalAddrPort(t, conn)
}

func listenUDPAddrPort(t *testing.T, address string) *net.UDPConn {
	t.Helper()

	addrPort := netip.MustParseAddrPort(address)
	conn, err := net.ListenUDP("udp", net.UDPAddrFromAddrPort(addrPort))
	if err != nil {
		if errors.Is(err, os.ErrPermission) {
			t.Skipf("UDP bind is not permitted in this environment: %v", err)
		}
		t.Fatalf("ListenUDP(%s) error = %v", address, err)
	}
	return conn
}

func udpLocalAddrPort(t *testing.T, conn *net.UDPConn) netip.AddrPort {
	t.Helper()

	addrPort, err := netip.ParseAddrPort(conn.LocalAddr().String())
	if err != nil {
		t.Fatalf("ParseAddrPort(%s) error = %v", conn.LocalAddr(), err)
	}
	return addrPort
}

func waitForUDPReady(t *testing.T, target netip.AddrPort) {
	t.Helper()

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		conn, err := net.DialUDP("udp", nil, net.UDPAddrFromAddrPort(target))
		if err == nil {
			conn.Close()
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("UDP listener %s did not become ready", target)
}

func sendUDP(t *testing.T, target netip.AddrPort, payload []byte) {
	t.Helper()

	conn, err := net.DialUDP("udp", nil, net.UDPAddrFromAddrPort(target))
	if err != nil {
		t.Fatalf("DialUDP(%s) error = %v", target, err)
	}
	defer conn.Close()
	if _, err := conn.Write(payload); err != nil {
		t.Fatalf("Write() error = %v", err)
	}
}

func readUDP(t *testing.T, conn *net.UDPConn) []byte {
	t.Helper()

	if err := conn.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatalf("SetReadDeadline() error = %v", err)
	}
	buf := make([]byte, DefaultBufferSize)
	n, _, err := conn.ReadFromUDPAddrPort(buf)
	if err != nil {
		t.Fatalf("ReadFromUDPAddrPort() error = %v", err)
	}
	return append([]byte(nil), buf[:n]...)
}
