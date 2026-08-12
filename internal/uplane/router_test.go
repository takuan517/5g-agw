package uplane

import (
	"bytes"
	"errors"
	"net/netip"
	"testing"
)

func TestRouterForwardRewritesUplinkAndSelectsCoreEndpoint(t *testing.T) {
	router := NewRouter()
	route := testRoute()
	router.Upsert(route)

	result, err := router.Forward(DirectionUplink, BuildTPDU(0x11111111, []byte{0x01, 0x02}))
	if err != nil {
		t.Fatalf("Forward(uplink) error = %v", err)
	}

	assertForwardResult(t, result, "pdu-session-1", route.Core.Addr, 0x11111111, 0xaaaa0001, []byte{0x01, 0x02})
}

func TestRouterForwardRewritesDownlinkAndSelectsRANEndpoint(t *testing.T) {
	router := NewRouter()
	route := testRoute()
	router.Upsert(route)

	result, err := router.Forward(DirectionDownlink, BuildTPDU(0xaaaa0001, []byte{0x03, 0x04}))
	if err != nil {
		t.Fatalf("Forward(downlink) error = %v", err)
	}

	assertForwardResult(t, result, "pdu-session-1", route.RAN.Addr, 0xaaaa0001, 0x11111111, []byte{0x03, 0x04})
}

func TestRouterRemoveDeletesBothDirections(t *testing.T) {
	router := NewRouter()
	route := testRoute()
	router.Upsert(route)
	router.Remove(route)

	if _, err := router.Forward(DirectionUplink, BuildTPDU(route.RAN.TEID, []byte{0x01})); !errors.Is(err, ErrRouteNotFound) {
		t.Fatalf("Forward(uplink) error = %v, want ErrRouteNotFound", err)
	}
	if _, err := router.Forward(DirectionDownlink, BuildTPDU(route.Core.TEID, []byte{0x01})); !errors.Is(err, ErrRouteNotFound) {
		t.Fatalf("Forward(downlink) error = %v, want ErrRouteNotFound", err)
	}
}

func TestRouterForwardReturnsRouteNotFound(t *testing.T) {
	router := NewRouter()

	_, err := router.Forward(DirectionUplink, BuildTPDU(0x99999999, []byte{0x01}))
	if !errors.Is(err, ErrRouteNotFound) {
		t.Fatalf("Forward() error = %v, want ErrRouteNotFound", err)
	}
}

func testRoute() TunnelRoute {
	return TunnelRoute{
		SessionID: "pdu-session-1",
		RAN: Endpoint{
			Addr: netip.MustParseAddrPort("10.100.200.20:2152"),
			TEID: 0x11111111,
		},
		Core: Endpoint{
			Addr: netip.MustParseAddrPort("10.100.200.40:2152"),
			TEID: 0xaaaa0001,
		},
	}
}

func assertForwardResult(t *testing.T, result ForwardResult, sessionID string, target netip.AddrPort, originalTEID uint32, rewrittenTEID uint32, payload []byte) {
	t.Helper()

	if result.SessionID != sessionID {
		t.Fatalf("SessionID = %q, want %q", result.SessionID, sessionID)
	}
	if result.Target != target {
		t.Fatalf("Target = %s, want %s", result.Target, target)
	}
	if result.Original.TEID != originalTEID {
		t.Fatalf("Original.TEID = 0x%08x, want 0x%08x", result.Original.TEID, originalTEID)
	}

	rewritten, err := ParsePacket(result.Rewritten)
	if err != nil {
		t.Fatalf("ParsePacket(rewritten) error = %v", err)
	}
	if rewritten.TEID != rewrittenTEID {
		t.Fatalf("rewritten TEID = 0x%08x, want 0x%08x", rewritten.TEID, rewrittenTEID)
	}
	if !bytes.Equal(rewritten.Payload, payload) {
		t.Fatalf("rewritten payload = %x, want %x", rewritten.Payload, payload)
	}
}
