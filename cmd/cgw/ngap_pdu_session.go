package main

import (
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"reflect"

	"github.com/free5gc/aper"
	"github.com/free5gc/ngap/ngapType"
)

type PDUSessionObservation struct {
	SessionID int64
	Tunnels   []GTPTunnelObservation
	DecodeErr string
}

type GTPTunnelObservation struct {
	Direction string
	Address   string
	TEID      uint32
}

func logPDUSessionObservations(direction string, entry *NGAPLogEntry) {
	for _, session := range entry.PDUSessions {
		if session.DecodeErr != "" {
			log.Printf("[CGW][PDU] %s: procedure=%s pduSessionId=%d decodeError=%s", direction, entry.Procedure, session.SessionID, session.DecodeErr)
			continue
		}
		if len(session.Tunnels) == 0 {
			log.Printf("[CGW][PDU] %s: procedure=%s pduSessionId=%d tunnel=<none>", direction, entry.Procedure, session.SessionID)
			continue
		}
		for _, tunnel := range session.Tunnels {
			log.Printf(
				"[CGW][PDU] %s: procedure=%s pduSessionId=%d tunnel=%s addr=%s teid=%d teidHex=0x%08x",
				direction,
				entry.Procedure,
				session.SessionID,
				tunnel.Direction,
				tunnel.Address,
				tunnel.TEID,
				tunnel.TEID,
			)
		}
	}
}

func collectPDUSessionObservations(value reflect.Value, seen map[uintptr]bool) []PDUSessionObservation {
	var sessions []PDUSessionObservation
	collectPDUSessionObservationsInto(value, seen, &sessions)
	return sessions
}

func collectPDUSessionObservationsInto(value reflect.Value, seen map[uintptr]bool, sessions *[]PDUSessionObservation) {
	if !value.IsValid() {
		return
	}

	if value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return
		}
		ptr := value.Pointer()
		if seen[ptr] {
			return
		}
		seen[ptr] = true
		collectPDUSessionObservationsInto(value.Elem(), seen, sessions)
		return
	}

	switch value.Kind() {
	case reflect.Struct:
		if value.Type().PkgPath() == "github.com/free5gc/ngap/ngapType" {
			switch value.Type().Name() {
			case "PDUSessionResourceSetupItemSUReq":
				if item, ok := value.Interface().(ngapType.PDUSessionResourceSetupItemSUReq); ok {
					*sessions = append(*sessions, observeSetupRequestItem(item))
				}
				return
			case "PDUSessionResourceSetupItemSURes":
				if item, ok := value.Interface().(ngapType.PDUSessionResourceSetupItemSURes); ok {
					*sessions = append(*sessions, observeSetupResponseItem(item))
				}
				return
			}
		}

		for i := 0; i < value.NumField(); i++ {
			collectPDUSessionObservationsInto(value.Field(i), seen, sessions)
		}
	case reflect.Slice, reflect.Array:
		for i := 0; i < value.Len(); i++ {
			collectPDUSessionObservationsInto(value.Index(i), seen, sessions)
		}
	}
}

func observeSetupRequestItem(item ngapType.PDUSessionResourceSetupItemSUReq) PDUSessionObservation {
	observation := PDUSessionObservation{SessionID: item.PDUSessionID.Value}
	var transfer ngapType.PDUSessionResourceSetupRequestTransfer
	if err := aper.UnmarshalWithParams(item.PDUSessionResourceSetupRequestTransfer, &transfer, "valueExt"); err != nil {
		observation.DecodeErr = fmt.Sprintf("setup request transfer: %v", err)
		return observation
	}

	for _, ie := range transfer.ProtocolIEs.List {
		switch ie.Value.Present {
		case ngapType.PDUSessionResourceSetupRequestTransferIEsPresentULNGUUPTNLInformation:
			observation.addTunnel("UL", ie.Value.ULNGUUPTNLInformation)
		case ngapType.PDUSessionResourceSetupRequestTransferIEsPresentAdditionalULNGUUPTNLInformation:
			if ie.Value.AdditionalULNGUUPTNLInformation == nil {
				continue
			}
			for i := range ie.Value.AdditionalULNGUUPTNLInformation.List {
				observation.addTunnel("UL-additional", &ie.Value.AdditionalULNGUUPTNLInformation.List[i].NGUUPTNLInformation)
			}
		}
	}
	return observation
}

func observeSetupResponseItem(item ngapType.PDUSessionResourceSetupItemSURes) PDUSessionObservation {
	observation := PDUSessionObservation{SessionID: item.PDUSessionID.Value}
	var transfer ngapType.PDUSessionResourceSetupResponseTransfer
	if err := aper.UnmarshalWithParams(item.PDUSessionResourceSetupResponseTransfer, &transfer, "valueExt"); err != nil {
		observation.DecodeErr = fmt.Sprintf("setup response transfer: %v", err)
		return observation
	}

	observation.addTunnel("DL", &transfer.DLQosFlowPerTNLInformation.UPTransportLayerInformation)
	if transfer.AdditionalDLQosFlowPerTNLInformation != nil {
		for i := range transfer.AdditionalDLQosFlowPerTNLInformation.List {
			observation.addTunnel("DL-additional", &transfer.AdditionalDLQosFlowPerTNLInformation.List[i].QosFlowPerTNLInformation.UPTransportLayerInformation)
		}
	}
	return observation
}

func (o *PDUSessionObservation) addTunnel(direction string, info *ngapType.UPTransportLayerInformation) {
	tunnel, ok := extractGTPTunnelObservation(direction, info)
	if !ok {
		return
	}
	o.Tunnels = append(o.Tunnels, tunnel)
}

func extractGTPTunnelObservation(direction string, info *ngapType.UPTransportLayerInformation) (GTPTunnelObservation, bool) {
	if info == nil || info.Present != ngapType.UPTransportLayerInformationPresentGTPTunnel || info.GTPTunnel == nil {
		return GTPTunnelObservation{}, false
	}
	if len(info.GTPTunnel.GTPTEID.Value) != 4 {
		return GTPTunnelObservation{}, false
	}

	return GTPTunnelObservation{
		Direction: direction,
		Address:   transportLayerAddressToString(info.GTPTunnel.TransportLayerAddress),
		TEID:      binary.BigEndian.Uint32([]byte(info.GTPTunnel.GTPTEID.Value)),
	}, true
}

func transportLayerAddressToString(address ngapType.TransportLayerAddress) string {
	bits := address.Value
	switch bits.BitLength {
	case 32:
		if len(bits.Bytes) < 4 {
			return ""
		}
		return net.IPv4(bits.Bytes[0], bits.Bytes[1], bits.Bytes[2], bits.Bytes[3]).String()
	case 128:
		if len(bits.Bytes) < 16 {
			return ""
		}
		return net.IP(bits.Bytes[:16]).String()
	case 160:
		if len(bits.Bytes) < 20 {
			return ""
		}
		return fmt.Sprintf("%s,%s", net.IPv4(bits.Bytes[0], bits.Bytes[1], bits.Bytes[2], bits.Bytes[3]).String(), net.IP(bits.Bytes[4:20]).String())
	default:
		return fmt.Sprintf("bitLength=%d bytes=%x", bits.BitLength, bits.Bytes)
	}
}
