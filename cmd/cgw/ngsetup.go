package main

import (
	"encoding/hex"
	"os"
	"strings"

	"github.com/free5gc/aper"
	"github.com/free5gc/ngap"
	"github.com/free5gc/ngap/ngapType"
)

// buildNGSetupResponse はAMFとして振る舞うための応答メッセージを構築
// CGW_NGSETUP_RESPONSE_HEX が設定されている場合は、そのバイナリをリプレイする
func buildNGSetupResponse() ([]byte, error) {
	if replayHex := strings.TrimSpace(os.Getenv("CGW_NGSETUP_RESPONSE_HEX")); replayHex != "" {
		replayHex = strings.NewReplacer(" ", "", "\n", "", "\t", "", ":", "").Replace(replayHex)
		return hex.DecodeString(replayHex)
	}

	var pdu ngapType.NGAPPDU
	pdu.Present = ngapType.NGAPPDUPresentSuccessfulOutcome
	pdu.SuccessfulOutcome = new(ngapType.SuccessfulOutcome)

	successfulOutcome := pdu.SuccessfulOutcome
	successfulOutcome.ProcedureCode.Value = ngapType.ProcedureCodeNGSetup
	successfulOutcome.Criticality.Value = ngapType.CriticalityPresentReject

	successfulOutcome.Value.Present = ngapType.SuccessfulOutcomePresentNGSetupResponse
	successfulOutcome.Value.NGSetupResponse = new(ngapType.NGSetupResponse)

	ieList := &successfulOutcome.Value.NGSetupResponse.ProtocolIEs.List

	ie1 := ngapType.NGSetupResponseIEs{}
	ie1.Id.Value = ngapType.ProtocolIEIDAMFName
	ie1.Criticality.Value = ngapType.CriticalityPresentReject
	ie1.Value.Present = ngapType.NGSetupResponseIEsPresentAMFName
	ie1.Value.AMFName = new(ngapType.AMFName)
	ie1.Value.AMFName.Value = "5G-AGW"
	*ieList = append(*ieList, ie1)

	ie2 := ngapType.NGSetupResponseIEs{}
	ie2.Id.Value = ngapType.ProtocolIEIDServedGUAMIList
	ie2.Criticality.Value = ngapType.CriticalityPresentReject
	ie2.Value.Present = ngapType.NGSetupResponseIEsPresentServedGUAMIList
	ie2.Value.ServedGUAMIList = new(ngapType.ServedGUAMIList)

	guamiItem := ngapType.ServedGUAMIItem{}
	guamiItem.GUAMI.PLMNIdentity.Value = []byte{0x99, 0xf9, 0x20}
	guamiItem.GUAMI.AMFRegionID.Value = aper.BitString{Bytes: []byte{0x01}, BitLength: 8}
	guamiItem.GUAMI.AMFSetID.Value = aper.BitString{Bytes: []byte{0x00, 0x40}, BitLength: 10}
	guamiItem.GUAMI.AMFPointer.Value = aper.BitString{Bytes: []byte{0x00}, BitLength: 6}
	ie2.Value.ServedGUAMIList.List = append(ie2.Value.ServedGUAMIList.List, guamiItem)
	*ieList = append(*ieList, ie2)

	ie3 := ngapType.NGSetupResponseIEs{}
	ie3.Id.Value = ngapType.ProtocolIEIDRelativeAMFCapacity
	ie3.Criticality.Value = ngapType.CriticalityPresentIgnore
	ie3.Value.Present = ngapType.NGSetupResponseIEsPresentRelativeAMFCapacity
	ie3.Value.RelativeAMFCapacity = new(ngapType.RelativeAMFCapacity)
	ie3.Value.RelativeAMFCapacity.Value = 100
	*ieList = append(*ieList, ie3)

	ie4 := ngapType.NGSetupResponseIEs{}
	ie4.Id.Value = ngapType.ProtocolIEIDPLMNSupportList
	ie4.Criticality.Value = ngapType.CriticalityPresentReject
	ie4.Value.Present = ngapType.NGSetupResponseIEsPresentPLMNSupportList
	ie4.Value.PLMNSupportList = new(ngapType.PLMNSupportList)
	ie4.Value.PLMNSupportList.List = append(ie4.Value.PLMNSupportList.List, ngapType.PLMNSupportItem{
		PLMNIdentity: ngapType.PLMNIdentity{
			Value: []byte{0x99, 0xf9, 0x20},
		},
		SliceSupportList: ngapType.SliceSupportList{
			List: []ngapType.SliceSupportItem{
				{
					SNSSAI: ngapType.SNSSAI{
						SST: ngapType.SST{Value: []byte{0x01}},
						SD:  &ngapType.SD{Value: []byte{0x00, 0x00, 0x01}},
					},
				},
			},
		},
	})
	*ieList = append(*ieList, ie4)

	return ngap.Encoder(pdu)
}
