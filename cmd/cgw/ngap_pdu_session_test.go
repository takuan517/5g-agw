package main

import (
	"testing"

	"github.com/free5gc/aper"
	"github.com/free5gc/ngap/ngapType"
)

func TestObserveSetupRequestItemExtractsULGTPTunnel(t *testing.T) {
	transfer := ngapType.PDUSessionResourceSetupRequestTransfer{
		ProtocolIEs: ngapType.ProtocolIEContainerPDUSessionResourceSetupRequestTransferIEs{
			List: []ngapType.PDUSessionResourceSetupRequestTransferIEs{
				{
					Id:          ngapType.ProtocolIEID{Value: ngapType.ProtocolIEIDULNGUUPTNLInformation},
					Criticality: ngapType.Criticality{Value: ngapType.CriticalityPresentReject},
					Value: ngapType.PDUSessionResourceSetupRequestTransferIEsValue{
						Present: ngapType.PDUSessionResourceSetupRequestTransferIEsPresentULNGUUPTNLInformation,
						ULNGUUPTNLInformation: &ngapType.UPTransportLayerInformation{
							Present: ngapType.UPTransportLayerInformationPresentGTPTunnel,
							GTPTunnel: &ngapType.GTPTunnel{
								TransportLayerAddress: testIPv4TransportLayerAddress(10, 100, 200, 30),
								GTPTEID:               ngapType.GTPTEID{Value: aper.OctetString{0x01, 0x02, 0x03, 0x04}},
							},
						},
					},
				},
			},
		},
	}
	encoded, err := aper.MarshalWithParams(transfer, "valueExt")
	if err != nil {
		t.Fatalf("marshal setup request transfer: %v", err)
	}

	observation := observeSetupRequestItem(ngapType.PDUSessionResourceSetupItemSUReq{
		PDUSessionID:                           ngapType.PDUSessionID{Value: 7},
		PDUSessionResourceSetupRequestTransfer: encoded,
	})

	assertSingleTunnel(t, observation, 7, "UL", "10.100.200.30", 0x01020304)
}

func TestObserveSetupResponseItemExtractsDLGTPTunnel(t *testing.T) {
	transfer := ngapType.PDUSessionResourceSetupResponseTransfer{
		DLQosFlowPerTNLInformation: ngapType.QosFlowPerTNLInformation{
			UPTransportLayerInformation: ngapType.UPTransportLayerInformation{
				Present: ngapType.UPTransportLayerInformationPresentGTPTunnel,
				GTPTunnel: &ngapType.GTPTunnel{
					TransportLayerAddress: testIPv4TransportLayerAddress(10, 100, 200, 40),
					GTPTEID:               ngapType.GTPTEID{Value: aper.OctetString{0x05, 0x06, 0x07, 0x08}},
				},
			},
			AssociatedQosFlowList: ngapType.AssociatedQosFlowList{
				List: []ngapType.AssociatedQosFlowItem{
					{QosFlowIdentifier: ngapType.QosFlowIdentifier{Value: 1}},
				},
			},
		},
	}
	encoded, err := aper.MarshalWithParams(transfer, "valueExt")
	if err != nil {
		t.Fatalf("marshal setup response transfer: %v", err)
	}

	observation := observeSetupResponseItem(ngapType.PDUSessionResourceSetupItemSURes{
		PDUSessionID:                            ngapType.PDUSessionID{Value: 8},
		PDUSessionResourceSetupResponseTransfer: encoded,
	})

	assertSingleTunnel(t, observation, 8, "DL", "10.100.200.40", 0x05060708)
}

func assertSingleTunnel(t *testing.T, observation PDUSessionObservation, sessionID int64, direction string, address string, teid uint32) {
	t.Helper()

	if observation.DecodeErr != "" {
		t.Fatalf("unexpected decode error: %s", observation.DecodeErr)
	}
	if observation.SessionID != sessionID {
		t.Fatalf("SessionID = %d, want %d", observation.SessionID, sessionID)
	}
	if len(observation.Tunnels) != 1 {
		t.Fatalf("len(Tunnels) = %d, want 1", len(observation.Tunnels))
	}
	tunnel := observation.Tunnels[0]
	if tunnel.Direction != direction || tunnel.Address != address || tunnel.TEID != teid {
		t.Fatalf("tunnel = %+v, want direction=%s address=%s teid=%d", tunnel, direction, address, teid)
	}
}

func testIPv4TransportLayerAddress(a, b, c, d byte) ngapType.TransportLayerAddress {
	return ngapType.TransportLayerAddress{
		Value: aper.BitString{
			Bytes:     []byte{a, b, c, d},
			BitLength: 32,
		},
	}
}
