package main

import (
	"fmt"
	"log"
	"reflect"
	"sort"
	"strings"

	"github.com/free5gc/ngap"
	"github.com/free5gc/ngap/ngapType"
)

type UEIDs struct {
	RAN    int64
	HasRAN bool
	AMF    int64
	HasAMF bool
}

type NGAPLogEntry struct {
	PDUType       string
	Procedure     string
	ProcedureCode int64
	PayloadBytes  int
	UEIDs         UEIDs
}

func logNGAP(direction string, payload []byte) *NGAPLogEntry {
	pdu, err := ngap.Decoder(payload)
	if err != nil {
		log.Printf("[CGW] %s: %d bytes (NGAP decode error: %v)", direction, len(payload), err)
		return nil
	}

	switch pdu.Present {
	case ngapType.NGAPPDUPresentInitiatingMessage:
		return logNGAPMessage(direction, "InitiatingMessage", pdu.InitiatingMessage.ProcedureCode.Value, len(payload), pdu)
	case ngapType.NGAPPDUPresentSuccessfulOutcome:
		return logNGAPMessage(direction, "SuccessfulOutcome", pdu.SuccessfulOutcome.ProcedureCode.Value, len(payload), pdu)
	case ngapType.NGAPPDUPresentUnsuccessfulOutcome:
		return logNGAPMessage(direction, "UnsuccessfulOutcome", pdu.UnsuccessfulOutcome.ProcedureCode.Value, len(payload), pdu)
	default:
		log.Printf("[CGW] %s: Unknown NGAP PDU Present=%d (%d bytes)", direction, pdu.Present, len(payload))
		return nil
	}
}

func logNGAPMessage(direction, pduType string, procedureCode int64, payloadBytes int, pdu *ngapType.NGAPPDU) *NGAPLogEntry {
	entry := &NGAPLogEntry{
		PDUType:       pduType,
		Procedure:     procedureName(procedureCode),
		ProcedureCode: procedureCode,
		PayloadBytes:  payloadBytes,
		UEIDs:         collectUEIDs(reflect.ValueOf(pdu), make(map[uintptr]bool)),
	}

	log.Printf(
		"[CGW] %s: pdu=%s procedure=%s procedureCode=%d size=%d bytes%s",
		direction,
		entry.PDUType,
		entry.Procedure,
		entry.ProcedureCode,
		entry.PayloadBytes,
		entry.UEIDs.LogSuffix(),
	)
	return entry
}

func (ids UEIDs) LogSuffix() string {
	parts := make([]string, 0, 2)
	if ids.HasAMF {
		parts = append(parts, fmt.Sprintf("amfUeNgapId=%d", ids.AMF))
	}
	if ids.HasRAN {
		parts = append(parts, fmt.Sprintf("ranUeNgapId=%d", ids.RAN))
	}
	sort.Strings(parts)
	if len(parts) == 0 {
		return ""
	}
	return " " + strings.Join(parts, " ")
}

func collectUEIDs(value reflect.Value, seen map[uintptr]bool) UEIDs {
	var ids UEIDs
	collectUEIDsInto(value, seen, &ids)
	return ids
}

func collectUEIDsInto(value reflect.Value, seen map[uintptr]bool, ids *UEIDs) {
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
		collectUEIDsInto(value.Elem(), seen, ids)
		return
	}

	switch value.Kind() {
	case reflect.Struct:
		if value.Type().PkgPath() == "github.com/free5gc/ngap/ngapType" {
			switch value.Type().Name() {
			case "AMFUENGAPID":
				if field := value.FieldByName("Value"); field.IsValid() && field.CanInt() {
					ids.AMF = field.Int()
					ids.HasAMF = true
				}
				return
			case "RANUENGAPID":
				if field := value.FieldByName("Value"); field.IsValid() && field.CanInt() {
					ids.RAN = field.Int()
					ids.HasRAN = true
				}
				return
			}
		}

		for i := 0; i < value.NumField(); i++ {
			collectUEIDsInto(value.Field(i), seen, ids)
		}
	case reflect.Slice, reflect.Array:
		for i := 0; i < value.Len(); i++ {
			collectUEIDsInto(value.Index(i), seen, ids)
		}
	}
}

func procedureName(procedureCode int64) string {
	switch procedureCode {
	case ngapType.ProcedureCodeAMFConfigurationUpdate:
		return "AMFConfigurationUpdate"
	case ngapType.ProcedureCodeAMFStatusIndication:
		return "AMFStatusIndication"
	case ngapType.ProcedureCodeCellTrafficTrace:
		return "CellTrafficTrace"
	case ngapType.ProcedureCodeDeactivateTrace:
		return "DeactivateTrace"
	case ngapType.ProcedureCodeDownlinkNASTransport:
		return "DownlinkNASTransport"
	case ngapType.ProcedureCodeDownlinkNonUEAssociatedNRPPaTransport:
		return "DownlinkNonUEAssociatedNRPPaTransport"
	case ngapType.ProcedureCodeDownlinkRANConfigurationTransfer:
		return "DownlinkRANConfigurationTransfer"
	case ngapType.ProcedureCodeDownlinkRANStatusTransfer:
		return "DownlinkRANStatusTransfer"
	case ngapType.ProcedureCodeDownlinkUEAssociatedNRPPaTransport:
		return "DownlinkUEAssociatedNRPPaTransport"
	case ngapType.ProcedureCodeErrorIndication:
		return "ErrorIndication"
	case ngapType.ProcedureCodeHandoverCancel:
		return "HandoverCancel"
	case ngapType.ProcedureCodeHandoverNotification:
		return "HandoverNotification"
	case ngapType.ProcedureCodeHandoverPreparation:
		return "HandoverPreparation"
	case ngapType.ProcedureCodeHandoverResourceAllocation:
		return "HandoverResourceAllocation"
	case ngapType.ProcedureCodeInitialContextSetup:
		return "InitialContextSetup"
	case ngapType.ProcedureCodeInitialUEMessage:
		return "InitialUEMessage"
	case ngapType.ProcedureCodeLocationReportingControl:
		return "LocationReportingControl"
	case ngapType.ProcedureCodeLocationReportingFailureIndication:
		return "LocationReportingFailureIndication"
	case ngapType.ProcedureCodeLocationReport:
		return "LocationReport"
	case ngapType.ProcedureCodeNASNonDeliveryIndication:
		return "NASNonDeliveryIndication"
	case ngapType.ProcedureCodeNGReset:
		return "NGReset"
	case ngapType.ProcedureCodeNGSetup:
		return "NGSetup"
	case ngapType.ProcedureCodeOverloadStart:
		return "OverloadStart"
	case ngapType.ProcedureCodeOverloadStop:
		return "OverloadStop"
	case ngapType.ProcedureCodePaging:
		return "Paging"
	case ngapType.ProcedureCodePathSwitchRequest:
		return "PathSwitchRequest"
	case ngapType.ProcedureCodePDUSessionResourceModify:
		return "PDUSessionResourceModify"
	case ngapType.ProcedureCodePDUSessionResourceModifyIndication:
		return "PDUSessionResourceModifyIndication"
	case ngapType.ProcedureCodePDUSessionResourceRelease:
		return "PDUSessionResourceRelease"
	case ngapType.ProcedureCodePDUSessionResourceSetup:
		return "PDUSessionResourceSetup"
	case ngapType.ProcedureCodePDUSessionResourceNotify:
		return "PDUSessionResourceNotify"
	case ngapType.ProcedureCodePrivateMessage:
		return "PrivateMessage"
	case ngapType.ProcedureCodePWSCancel:
		return "PWSCancel"
	case ngapType.ProcedureCodePWSFailureIndication:
		return "PWSFailureIndication"
	case ngapType.ProcedureCodePWSRestartIndication:
		return "PWSRestartIndication"
	case ngapType.ProcedureCodeRANConfigurationUpdate:
		return "RANConfigurationUpdate"
	case ngapType.ProcedureCodeRerouteNASRequest:
		return "RerouteNASRequest"
	case ngapType.ProcedureCodeRRCInactiveTransitionReport:
		return "RRCInactiveTransitionReport"
	case ngapType.ProcedureCodeTraceFailureIndication:
		return "TraceFailureIndication"
	case ngapType.ProcedureCodeTraceStart:
		return "TraceStart"
	case ngapType.ProcedureCodeUEContextModification:
		return "UEContextModification"
	case ngapType.ProcedureCodeUEContextRelease:
		return "UEContextRelease"
	case ngapType.ProcedureCodeUEContextReleaseRequest:
		return "UEContextReleaseRequest"
	case ngapType.ProcedureCodeUERadioCapabilityCheck:
		return "UERadioCapabilityCheck"
	case ngapType.ProcedureCodeUERadioCapabilityInfoIndication:
		return "UERadioCapabilityInfoIndication"
	case ngapType.ProcedureCodeUETNLABindingRelease:
		return "UETNLABindingRelease"
	case ngapType.ProcedureCodeUplinkNASTransport:
		return "UplinkNASTransport"
	case ngapType.ProcedureCodeUplinkNonUEAssociatedNRPPaTransport:
		return "UplinkNonUEAssociatedNRPPaTransport"
	case ngapType.ProcedureCodeUplinkRANConfigurationTransfer:
		return "UplinkRANConfigurationTransfer"
	case ngapType.ProcedureCodeUplinkRANStatusTransfer:
		return "UplinkRANStatusTransfer"
	case ngapType.ProcedureCodeUplinkUEAssociatedNRPPaTransport:
		return "UplinkUEAssociatedNRPPaTransport"
	case ngapType.ProcedureCodeWriteReplaceWarning:
		return "WriteReplaceWarning"
	case ngapType.ProcedureCodeSecondaryRATDataUsageReport:
		return "SecondaryRATDataUsageReport"
	default:
		return "Unknown"
	}
}
