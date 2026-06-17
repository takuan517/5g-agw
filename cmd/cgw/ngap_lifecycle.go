package main

import "github.com/free5gc/ngap/ngapType"

func shouldReleaseMapping(entry *NGAPLogEntry) bool {
	return entry != nil &&
		entry.PDUType == "SuccessfulOutcome" &&
		entry.ProcedureCode == ngapType.ProcedureCodeUEContextRelease
}
