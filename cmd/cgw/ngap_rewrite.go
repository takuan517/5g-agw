package main

import (
	"fmt"
	"log"
	"reflect"

	"github.com/free5gc/ngap"
	"github.com/free5gc/ngap/ngapType"
)

func rewriteNGAPForProxy(session *proxySession, direction string, payload []byte) ([]byte, error) {
	if direction != directionGNBToAMF && direction != directionAMFToGNB {
		return payload, nil
	}

	pdu, err := ngap.Decoder(payload)
	if err != nil {
		return payload, nil
	}

	ids := collectUEIDs(reflect.ValueOf(pdu), make(map[uintptr]bool))
	if !ids.HasRAN {
		return payload, nil
	}

	var targetRANID int64
	var ok bool

	switch direction {
	case directionGNBToAMF:
		if isInitialUEMessage(pdu) {
			mapping := session.ueMappings.EnsureForInitialUEMessage(session.associationID, ids.RAN)
			targetRANID = mapping.GatewayRANUENGAPID
			ok = true
		} else {
			targetRANID, ok = session.ueMappings.GatewayRANID(session.associationID, ids.RAN)
		}
	case directionAMFToGNB:
		targetRANID, ok = session.ueMappings.OriginalRANID(session.associationID, ids.RAN)
	}

	if !ok || targetRANID == ids.RAN {
		return payload, nil
	}

	if !replaceRANUENGAPID(reflect.ValueOf(pdu), ids.RAN, targetRANID, make(map[uintptr]bool)) {
		return payload, fmt.Errorf("RANUENGAPID %d not found for rewrite", ids.RAN)
	}

	rewritten, err := ngap.Encoder(*pdu)
	if err != nil {
		return payload, fmt.Errorf("encode rewritten NGAP: %w", err)
	}

	log.Printf(
		"[CGW][REWRITE] assoc=%d direction=%s procedure=%s ranUeNgapId %d -> %d",
		session.associationID,
		direction,
		procedureName(procedureCodeOf(pdu)),
		ids.RAN,
		targetRANID,
	)
	return rewritten, nil
}

func isInitialUEMessage(pdu *ngapType.NGAPPDU) bool {
	return pdu != nil &&
		pdu.Present == ngapType.NGAPPDUPresentInitiatingMessage &&
		pdu.InitiatingMessage != nil &&
		pdu.InitiatingMessage.ProcedureCode.Value == ngapType.ProcedureCodeInitialUEMessage
}

func procedureCodeOf(pdu *ngapType.NGAPPDU) int64 {
	if pdu == nil {
		return -1
	}
	switch pdu.Present {
	case ngapType.NGAPPDUPresentInitiatingMessage:
		if pdu.InitiatingMessage != nil {
			return pdu.InitiatingMessage.ProcedureCode.Value
		}
	case ngapType.NGAPPDUPresentSuccessfulOutcome:
		if pdu.SuccessfulOutcome != nil {
			return pdu.SuccessfulOutcome.ProcedureCode.Value
		}
	case ngapType.NGAPPDUPresentUnsuccessfulOutcome:
		if pdu.UnsuccessfulOutcome != nil {
			return pdu.UnsuccessfulOutcome.ProcedureCode.Value
		}
	}
	return -1
}

func replaceRANUENGAPID(value reflect.Value, oldID int64, newID int64, seen map[uintptr]bool) bool {
	if !value.IsValid() {
		return false
	}

	if value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return false
		}
		ptr := value.Pointer()
		if seen[ptr] {
			return false
		}
		seen[ptr] = true
		return replaceRANUENGAPID(value.Elem(), oldID, newID, seen)
	}

	switch value.Kind() {
	case reflect.Struct:
		if value.Type().PkgPath() == "github.com/free5gc/ngap/ngapType" && value.Type().Name() == "RANUENGAPID" {
			field := value.FieldByName("Value")
			if field.IsValid() && field.CanSet() && field.CanInt() && field.Int() == oldID {
				field.SetInt(newID)
				return true
			}
			return false
		}

		rewritten := false
		for i := 0; i < value.NumField(); i++ {
			if replaceRANUENGAPID(value.Field(i), oldID, newID, seen) {
				rewritten = true
			}
		}
		return rewritten
	case reflect.Slice, reflect.Array:
		rewritten := false
		for i := 0; i < value.Len(); i++ {
			if replaceRANUENGAPID(value.Index(i), oldID, newID, seen) {
				rewritten = true
			}
		}
		return rewritten
	}

	return false
}
