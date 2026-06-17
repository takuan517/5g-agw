package main

import "testing"

func TestUEMappingTableAllocatesAndResolvesGatewayRANID(t *testing.T) {
	table := NewUEMappingTable()

	first := table.EnsureForInitialUEMessage(10, 1)
	again := table.EnsureForInitialUEMessage(10, 1)

	if first != again {
		t.Fatal("expected repeated InitialUEMessage for the same association/RAN ID to reuse mapping")
	}
	if first.GatewayRANUENGAPID != gatewayRANIDStart {
		t.Fatalf("expected first gateway RAN UE NGAP ID %d, got %d", gatewayRANIDStart, first.GatewayRANUENGAPID)
	}

	gatewayID, ok := table.GatewayRANID(10, 1)
	if !ok || gatewayID != first.GatewayRANUENGAPID {
		t.Fatalf("GatewayRANID() = (%d, %t), want (%d, true)", gatewayID, ok, first.GatewayRANUENGAPID)
	}

	originalID, ok := table.OriginalRANID(10, first.GatewayRANUENGAPID)
	if !ok || originalID != 1 {
		t.Fatalf("OriginalRANID() = (%d, %t), want (1, true)", originalID, ok)
	}
}

func TestUEMappingTableLearnsAMFIDAndRemovesByAnyUEID(t *testing.T) {
	table := NewUEMappingTable()
	mapping := table.EnsureForInitialUEMessage(10, 1)

	table.Observe(10, directionAMFToGNB, &NGAPLogEntry{
		Procedure: "DownlinkNASTransport",
		UEIDs: UEIDs{
			RAN:    mapping.GatewayRANUENGAPID,
			HasRAN: true,
			AMF:    42,
			HasAMF: true,
		},
	})

	if !mapping.HasAMF || mapping.AMFUENGAPID != 42 {
		t.Fatalf("expected AMF UE NGAP ID 42 to be learned, got hasAMF=%t amf=%d", mapping.HasAMF, mapping.AMFUENGAPID)
	}

	removed := table.RemoveByUEIDs(10, UEIDs{AMF: 42, HasAMF: true}, "test release")
	if !removed {
		t.Fatal("expected RemoveByUEIDs to remove mapping by AMF UE NGAP ID")
	}

	if _, ok := table.GatewayRANID(10, 1); ok {
		t.Fatal("expected original RAN index to be removed")
	}
	if _, ok := table.OriginalRANID(10, mapping.GatewayRANUENGAPID); ok {
		t.Fatal("expected gateway RAN index to be removed")
	}
	if table.RemoveByUEIDs(10, UEIDs{AMF: 42, HasAMF: true}, "test release again") {
		t.Fatal("expected AMF index to be removed")
	}
}

func TestUEMappingTableRemoveAssociationOnlyRemovesTargetAssociation(t *testing.T) {
	table := NewUEMappingTable()

	assoc10 := table.EnsureForInitialUEMessage(10, 1)
	assoc20 := table.EnsureForInitialUEMessage(20, 1)

	removed := table.RemoveAssociation(10, "test association close")
	if removed != 1 {
		t.Fatalf("RemoveAssociation() removed %d mappings, want 1", removed)
	}

	if _, ok := table.GatewayRANID(10, 1); ok {
		t.Fatal("expected association 10 mapping to be removed")
	}
	if originalID, ok := table.OriginalRANID(20, assoc20.GatewayRANUENGAPID); !ok || originalID != 1 {
		t.Fatalf("expected association 20 mapping to remain, got original=%d ok=%t", originalID, ok)
	}
	if assoc10.GatewayRANUENGAPID == assoc20.GatewayRANUENGAPID {
		t.Fatal("expected gateway RAN IDs to be globally unique")
	}
}
