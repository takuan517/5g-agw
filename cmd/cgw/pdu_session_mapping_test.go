package main

import "testing"

func TestPDUSessionMappingTableObservesULAndDLTunnels(t *testing.T) {
	ueMappings := NewUEMappingTable()
	ueMapping := ueMappings.EnsureForInitialUEMessage(10, 1)
	ueMappings.Observe(10, directionAMFToGNB, &NGAPLogEntry{
		Procedure: "DownlinkNASTransport",
		UEIDs:     UEIDs{RAN: ueMapping.GatewayRANUENGAPID, HasRAN: true, AMF: 42, HasAMF: true},
	})

	pduMappings := NewPDUSessionMappingTable()
	pduMappings.Observe(10, directionAMFToGNB, ueMappings, &NGAPLogEntry{
		Procedure: "PDUSessionResourceSetup",
		UEIDs:     UEIDs{RAN: 1, HasRAN: true, AMF: 42, HasAMF: true},
		PDUSessions: []PDUSessionObservation{
			{
				SessionID: 5,
				Tunnels: []GTPTunnelObservation{
					{Direction: "DL", Address: "10.100.200.40", TEID: 0x01020304},
				},
			},
		},
	})
	pduMappings.Observe(10, directionGNBToAMF, ueMappings, &NGAPLogEntry{
		Procedure: "PDUSessionResourceSetup",
		UEIDs:     UEIDs{RAN: ueMapping.GatewayRANUENGAPID, HasRAN: true, AMF: 42, HasAMF: true},
		PDUSessions: []PDUSessionObservation{
			{
				SessionID: 5,
				Tunnels: []GTPTunnelObservation{
					{Direction: "UL", Address: "10.100.200.30", TEID: 0x05060708},
				},
			},
		},
	})

	mapping, ok := pduMappings.Find(PDUSessionKey{
		AssociationID:      10,
		GatewayRANUENGAPID: ueMapping.GatewayRANUENGAPID,
		PDUSessionID:       5,
	})
	if !ok {
		t.Fatal("expected PDU session mapping to exist")
	}
	if mapping.OriginalRANUENGAPID != 1 || mapping.GatewayRANUENGAPID != ueMapping.GatewayRANUENGAPID {
		t.Fatalf("unexpected UE IDs in mapping: %+v", mapping)
	}
	if !mapping.HasAMF || mapping.AMFUENGAPID != 42 {
		t.Fatalf("unexpected AMF ID in mapping: %+v", mapping)
	}
	assertEndpoint(t, mapping.DL, "10.100.200.40", 0x01020304)
	assertEndpoint(t, mapping.UL, "10.100.200.30", 0x05060708)
}

func TestPDUSessionMappingTableRemoval(t *testing.T) {
	ueMappings := NewUEMappingTable()
	ueMapping := ueMappings.EnsureForInitialUEMessage(10, 1)
	pduMappings := NewPDUSessionMappingTable()

	pduMappings.Observe(10, directionGNBToAMF, ueMappings, &NGAPLogEntry{
		Procedure: "PDUSessionResourceSetup",
		UEIDs:     UEIDs{RAN: ueMapping.GatewayRANUENGAPID, HasRAN: true},
		PDUSessions: []PDUSessionObservation{
			{SessionID: 5, Tunnels: []GTPTunnelObservation{{Direction: "UL", Address: "10.100.200.30", TEID: 1}}},
			{SessionID: 6, Tunnels: []GTPTunnelObservation{{Direction: "UL", Address: "10.100.200.31", TEID: 2}}},
		},
	})

	removed := pduMappings.RemoveByUEIDs(10, UEIDs{RAN: 1, HasRAN: true}, "test release")
	if removed != 2 {
		t.Fatalf("RemoveByUEIDs removed %d mappings, want 2", removed)
	}
	if _, ok := pduMappings.Find(PDUSessionKey{AssociationID: 10, GatewayRANUENGAPID: ueMapping.GatewayRANUENGAPID, PDUSessionID: 5}); ok {
		t.Fatal("expected PDU session 5 mapping to be removed")
	}

	pduMappings.Observe(10, directionGNBToAMF, ueMappings, &NGAPLogEntry{
		Procedure: "PDUSessionResourceSetup",
		UEIDs:     UEIDs{RAN: ueMapping.GatewayRANUENGAPID, HasRAN: true},
		PDUSessions: []PDUSessionObservation{
			{SessionID: 7, Tunnels: []GTPTunnelObservation{{Direction: "UL", Address: "10.100.200.32", TEID: 3}}},
		},
	})
	if removed := pduMappings.RemoveAssociation(10, "test close"); removed != 1 {
		t.Fatalf("RemoveAssociation removed %d mappings, want 1", removed)
	}
}

func assertEndpoint(t *testing.T, endpoint *GTPTunnelEndpoint, address string, teid uint32) {
	t.Helper()

	if endpoint == nil {
		t.Fatalf("endpoint is nil, want %s/0x%08x", address, teid)
	}
	if endpoint.Address != address || endpoint.TEID != teid {
		t.Fatalf("endpoint = %+v, want address=%s teid=%d", endpoint, address, teid)
	}
}
