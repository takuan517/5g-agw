package main

import "testing"

func TestParseStaticRoute(t *testing.T) {
	route, err := parseStaticRoute("pdu-session-1,10.100.200.20:2152,0x11111111,10.100.200.40:2152,0xaaaa0001")
	if err != nil {
		t.Fatalf("parseStaticRoute() error = %v", err)
	}

	if route.SessionID != "pdu-session-1" {
		t.Fatalf("SessionID = %q, want pdu-session-1", route.SessionID)
	}
	if route.RAN.Addr.String() != "10.100.200.20:2152" || route.RAN.TEID != 0x11111111 {
		t.Fatalf("unexpected RAN endpoint: %+v", route.RAN)
	}
	if route.Core.Addr.String() != "10.100.200.40:2152" || route.Core.TEID != 0xaaaa0001 {
		t.Fatalf("unexpected core endpoint: %+v", route.Core)
	}
}

func TestParseStaticRouteRejectsInvalidRoute(t *testing.T) {
	if _, err := parseStaticRoute("pdu-session-1,10.100.200.20:2152,0x11111111"); err == nil {
		t.Fatal("parseStaticRoute() error = nil, want error")
	}
}
