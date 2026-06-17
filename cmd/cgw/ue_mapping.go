package main

import (
	"fmt"
	"log"
	"sync"
)

const (
	directionGNBToAMF = "gNB -> AMF"
	directionAMFToGNB = "AMF -> gNB"
	gatewayRANIDStart = int64(1000000)
)

var globalUEMappingTable = NewUEMappingTable()

type UEAssociationKey struct {
	AssociationID int64
	ID            int64
}

type UEMapping struct {
	AssociationID       int64
	OriginalRANUENGAPID int64
	GatewayRANUENGAPID  int64
	AMFUENGAPID         int64
	HasAMF              bool
}

type UEMappingTable struct {
	mu            sync.RWMutex
	nextGatewayID int64
	byOriginalRAN map[UEAssociationKey]*UEMapping
	byGatewayRAN  map[UEAssociationKey]*UEMapping
	byAMF         map[UEAssociationKey]*UEMapping
}

func NewUEMappingTable() *UEMappingTable {
	return &UEMappingTable{
		nextGatewayID: gatewayRANIDStart,
		byOriginalRAN: make(map[UEAssociationKey]*UEMapping),
		byGatewayRAN:  make(map[UEAssociationKey]*UEMapping),
		byAMF:         make(map[UEAssociationKey]*UEMapping),
	}
}

func (t *UEMappingTable) EnsureForInitialUEMessage(associationID int64, originalRANID int64) *UEMapping {
	t.mu.Lock()
	defer t.mu.Unlock()

	key := UEAssociationKey{AssociationID: associationID, ID: originalRANID}
	if mapping := t.byOriginalRAN[key]; mapping != nil {
		return mapping
	}

	gatewayRANID := t.nextGatewayID
	t.nextGatewayID++

	mapping := &UEMapping{
		AssociationID:       associationID,
		OriginalRANUENGAPID: originalRANID,
		GatewayRANUENGAPID:  gatewayRANID,
	}
	t.byOriginalRAN[key] = mapping
	t.byGatewayRAN[UEAssociationKey{AssociationID: associationID, ID: gatewayRANID}] = mapping

	log.Printf("[CGW][MAP] assoc=%d allocated %s", associationID, mapping.String())
	return mapping
}

func (t *UEMappingTable) GatewayRANID(associationID int64, originalRANID int64) (int64, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	mapping := t.byOriginalRAN[UEAssociationKey{AssociationID: associationID, ID: originalRANID}]
	if mapping == nil {
		return 0, false
	}
	return mapping.GatewayRANUENGAPID, true
}

func (t *UEMappingTable) OriginalRANID(associationID int64, gatewayRANID int64) (int64, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	mapping := t.byGatewayRAN[UEAssociationKey{AssociationID: associationID, ID: gatewayRANID}]
	if mapping == nil {
		return 0, false
	}
	return mapping.OriginalRANUENGAPID, true
}

func (t *UEMappingTable) FindByUEIDs(associationID int64, ids UEIDs) (UEMapping, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	mapping := t.findLocked(associationID, ids)
	if mapping == nil {
		return UEMapping{}, false
	}
	return *mapping, true
}

func (t *UEMappingTable) Observe(associationID int64, direction string, entry *NGAPLogEntry) {
	if entry == nil || (!entry.UEIDs.HasRAN && !entry.UEIDs.HasAMF) {
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	mapping := t.findLocked(associationID, entry.UEIDs)
	if mapping == nil {
		return
	}

	changed := false
	if entry.UEIDs.HasAMF && (!mapping.HasAMF || mapping.AMFUENGAPID != entry.UEIDs.AMF) {
		if mapping.HasAMF {
			delete(t.byAMF, UEAssociationKey{AssociationID: associationID, ID: mapping.AMFUENGAPID})
		}
		mapping.AMFUENGAPID = entry.UEIDs.AMF
		mapping.HasAMF = true
		t.byAMF[UEAssociationKey{AssociationID: associationID, ID: entry.UEIDs.AMF}] = mapping
		changed = true
	}

	if changed {
		log.Printf(
			"[CGW][MAP] assoc=%d direction=%s procedure=%s %s",
			associationID,
			direction,
			entry.Procedure,
			mapping.String(),
		)
	}
}

func (t *UEMappingTable) findLocked(associationID int64, ids UEIDs) *UEMapping {
	if ids.HasRAN {
		if mapping := t.byOriginalRAN[UEAssociationKey{AssociationID: associationID, ID: ids.RAN}]; mapping != nil {
			return mapping
		}
		if mapping := t.byGatewayRAN[UEAssociationKey{AssociationID: associationID, ID: ids.RAN}]; mapping != nil {
			return mapping
		}
	}

	if ids.HasAMF {
		if mapping := t.byAMF[UEAssociationKey{AssociationID: associationID, ID: ids.AMF}]; mapping != nil {
			return mapping
		}
	}

	return nil
}

func (m *UEMapping) String() string {
	base := fmt.Sprintf("originalRanUeNgapId=%d <-> gatewayRanUeNgapId=%d", m.OriginalRANUENGAPID, m.GatewayRANUENGAPID)
	if m.HasAMF {
		return fmt.Sprintf("%s <-> amfUeNgapId=%d", base, m.AMFUENGAPID)
	}
	return base + " <-> amfUeNgapId=<pending>"
}

func (t *UEMappingTable) RemoveByUEIDs(associationID int64, ids UEIDs, reason string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	mapping := t.findLocked(associationID, ids)
	if mapping == nil {
		return false
	}
	t.removeLocked(mapping)
	log.Printf("[CGW][MAP] assoc=%d removed %s reason=%s", associationID, mapping.String(), reason)
	return true
}

func (t *UEMappingTable) RemoveAssociation(associationID int64, reason string) int {
	t.mu.Lock()
	defer t.mu.Unlock()

	removed := 0
	for key, mapping := range t.byOriginalRAN {
		if key.AssociationID != associationID {
			continue
		}
		t.removeLocked(mapping)
		removed++
	}

	if removed > 0 {
		log.Printf("[CGW][MAP] assoc=%d removed %d mapping(s) reason=%s", associationID, removed, reason)
	}
	return removed
}

func (t *UEMappingTable) removeLocked(mapping *UEMapping) {
	delete(t.byOriginalRAN, UEAssociationKey{AssociationID: mapping.AssociationID, ID: mapping.OriginalRANUENGAPID})
	delete(t.byGatewayRAN, UEAssociationKey{AssociationID: mapping.AssociationID, ID: mapping.GatewayRANUENGAPID})
	if mapping.HasAMF {
		delete(t.byAMF, UEAssociationKey{AssociationID: mapping.AssociationID, ID: mapping.AMFUENGAPID})
	}
}
