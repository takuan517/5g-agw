package main

import (
	"fmt"
	"log"
	"strings"
	"sync"
)

var globalPDUSessionMappingTable = NewPDUSessionMappingTable()

type PDUSessionKey struct {
	AssociationID      int64
	GatewayRANUENGAPID int64
	PDUSessionID       int64
}

type GTPTunnelEndpoint struct {
	Address string
	TEID    uint32
}

type PDUSessionMapping struct {
	AssociationID       int64
	OriginalRANUENGAPID int64
	GatewayRANUENGAPID  int64
	AMFUENGAPID         int64
	HasAMF              bool
	PDUSessionID        int64
	UL                  *GTPTunnelEndpoint
	DL                  *GTPTunnelEndpoint
}

type PDUSessionMappingTable struct {
	mu        sync.RWMutex
	bySession map[PDUSessionKey]*PDUSessionMapping
}

func NewPDUSessionMappingTable() *PDUSessionMappingTable {
	return &PDUSessionMappingTable{
		bySession: make(map[PDUSessionKey]*PDUSessionMapping),
	}
}

func (t *PDUSessionMappingTable) Observe(associationID int64, direction string, ueMappings *UEMappingTable, entry *NGAPLogEntry) {
	if entry == nil || len(entry.PDUSessions) == 0 {
		return
	}

	ueMapping, ok := ueMappings.FindByUEIDs(associationID, entry.UEIDs)
	if !ok {
		log.Printf("[CGW][PDU-MAP] assoc=%d direction=%s procedure=%s skipped: UE mapping not found%s", associationID, direction, entry.Procedure, entry.UEIDs.LogSuffix())
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	for _, observation := range entry.PDUSessions {
		if observation.DecodeErr != "" {
			continue
		}

		key := PDUSessionKey{
			AssociationID:      associationID,
			GatewayRANUENGAPID: ueMapping.GatewayRANUENGAPID,
			PDUSessionID:       observation.SessionID,
		}
		mapping := t.bySession[key]
		if mapping == nil {
			mapping = &PDUSessionMapping{
				AssociationID:       associationID,
				OriginalRANUENGAPID: ueMapping.OriginalRANUENGAPID,
				GatewayRANUENGAPID:  ueMapping.GatewayRANUENGAPID,
				AMFUENGAPID:         ueMapping.AMFUENGAPID,
				HasAMF:              ueMapping.HasAMF,
				PDUSessionID:        observation.SessionID,
			}
			t.bySession[key] = mapping
		} else {
			mapping.AMFUENGAPID = ueMapping.AMFUENGAPID
			mapping.HasAMF = ueMapping.HasAMF
		}

		changed := false
		for _, tunnel := range observation.Tunnels {
			changed = mapping.applyTunnel(tunnel) || changed
		}
		if changed {
			log.Printf("[CGW][PDU-MAP] assoc=%d direction=%s procedure=%s %s", associationID, direction, entry.Procedure, mapping.String())
		}
	}
}

func (t *PDUSessionMappingTable) Find(key PDUSessionKey) (PDUSessionMapping, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	mapping := t.bySession[key]
	if mapping == nil {
		return PDUSessionMapping{}, false
	}
	return *mapping, true
}

func (t *PDUSessionMappingTable) RemoveByUEIDs(associationID int64, ids UEIDs, reason string) int {
	t.mu.Lock()
	defer t.mu.Unlock()

	removed := 0
	for key, mapping := range t.bySession {
		if mapping.AssociationID != associationID || !mapping.matchesUEIDs(ids) {
			continue
		}
		delete(t.bySession, key)
		removed++
	}

	if removed > 0 {
		log.Printf("[CGW][PDU-MAP] assoc=%d removed %d PDU session mapping(s) reason=%s", associationID, removed, reason)
	}
	return removed
}

func (t *PDUSessionMappingTable) RemoveAssociation(associationID int64, reason string) int {
	t.mu.Lock()
	defer t.mu.Unlock()

	removed := 0
	for key, mapping := range t.bySession {
		if mapping.AssociationID != associationID {
			continue
		}
		delete(t.bySession, key)
		removed++
	}

	if removed > 0 {
		log.Printf("[CGW][PDU-MAP] assoc=%d removed %d PDU session mapping(s) reason=%s", associationID, removed, reason)
	}
	return removed
}

func (m *PDUSessionMapping) applyTunnel(tunnel GTPTunnelObservation) bool {
	endpoint := &GTPTunnelEndpoint{Address: tunnel.Address, TEID: tunnel.TEID}
	switch tunnel.Direction {
	case "UL", "UL-additional":
		return setEndpoint(&m.UL, endpoint)
	case "DL", "DL-additional":
		return setEndpoint(&m.DL, endpoint)
	default:
		return false
	}
}

func setEndpoint(current **GTPTunnelEndpoint, next *GTPTunnelEndpoint) bool {
	if *current != nil && (*current).Address == next.Address && (*current).TEID == next.TEID {
		return false
	}
	*current = next
	return true
}

func (m *PDUSessionMapping) matchesUEIDs(ids UEIDs) bool {
	if ids.HasRAN && ids.RAN != m.OriginalRANUENGAPID && ids.RAN != m.GatewayRANUENGAPID {
		return false
	}
	if ids.HasAMF && (!m.HasAMF || ids.AMF != m.AMFUENGAPID) {
		return false
	}
	return ids.HasRAN || ids.HasAMF
}

func (m *PDUSessionMapping) String() string {
	parts := []string{
		fmt.Sprintf("pduSessionId=%d", m.PDUSessionID),
		fmt.Sprintf("originalRanUeNgapId=%d", m.OriginalRANUENGAPID),
		fmt.Sprintf("gatewayRanUeNgapId=%d", m.GatewayRANUENGAPID),
	}
	if m.HasAMF {
		parts = append(parts, fmt.Sprintf("amfUeNgapId=%d", m.AMFUENGAPID))
	}
	parts = append(parts, "ul="+endpointString(m.UL), "dl="+endpointString(m.DL))
	return strings.Join(parts, " ")
}

func endpointString(endpoint *GTPTunnelEndpoint) string {
	if endpoint == nil {
		return "<pending>"
	}
	return fmt.Sprintf("%s/0x%08x", endpoint.Address, endpoint.TEID)
}
