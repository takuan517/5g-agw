package uplane

import (
	"errors"
	"fmt"
	"net/netip"
	"sync"
)

type Direction string

const (
	DirectionUplink   Direction = "uplink"
	DirectionDownlink Direction = "downlink"
)

var ErrRouteNotFound = errors.New("gtp-u route not found")

type Endpoint struct {
	Addr netip.AddrPort
	TEID uint32
}

type TunnelRoute struct {
	SessionID string
	RAN       Endpoint
	Core      Endpoint
}

type ForwardResult struct {
	SessionID       string
	Target          netip.AddrPort
	Original        Packet
	Rewritten       []byte
	RewrittenPacket Packet
}

type Router struct {
	mu         sync.RWMutex
	byRANTEID  map[uint32]TunnelRoute
	byCoreTEID map[uint32]TunnelRoute
}

func NewRouter() *Router {
	return &Router{
		byRANTEID:  make(map[uint32]TunnelRoute),
		byCoreTEID: make(map[uint32]TunnelRoute),
	}
}

func (r *Router) Upsert(route TunnelRoute) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.byRANTEID[route.RAN.TEID] = route
	r.byCoreTEID[route.Core.TEID] = route
}

func (r *Router) Remove(route TunnelRoute) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.byRANTEID, route.RAN.TEID)
	delete(r.byCoreTEID, route.Core.TEID)
}

func (r *Router) Forward(direction Direction, packet []byte) (ForwardResult, error) {
	parsed, err := ParsePacket(packet)
	if err != nil {
		return ForwardResult{}, err
	}

	route, target, nextTEID, ok := r.lookup(direction, parsed.TEID)
	if !ok {
		return ForwardResult{}, fmt.Errorf("%w: direction=%s teid=0x%08x", ErrRouteNotFound, direction, parsed.TEID)
	}

	rewritten, err := RewriteTEID(packet, nextTEID)
	if err != nil {
		return ForwardResult{}, err
	}

	rewrittenPacket, err := ParsePacket(rewritten)
	if err != nil {
		return ForwardResult{}, err
	}

	return ForwardResult{
		SessionID:       route.SessionID,
		Target:          target,
		Original:        parsed,
		Rewritten:       rewritten,
		RewrittenPacket: rewrittenPacket,
	}, nil
}

func (r *Router) lookup(direction Direction, teid uint32) (TunnelRoute, netip.AddrPort, uint32, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	switch direction {
	case DirectionUplink:
		route, ok := r.byRANTEID[teid]
		return route, route.Core.Addr, route.Core.TEID, ok
	case DirectionDownlink:
		route, ok := r.byCoreTEID[teid]
		return route, route.RAN.Addr, route.RAN.TEID, ok
	default:
		return TunnelRoute{}, netip.AddrPort{}, 0, false
	}
}
