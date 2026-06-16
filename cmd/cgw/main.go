package main

import (
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strings"

	"5g-agw/internal/context"

	"github.com/free5gc/aper"
	"github.com/free5gc/ngap"
	"github.com/free5gc/ngap/ngapType"
	"github.com/free5gc/sctp"
)

type config struct {
	ListenAddr string
	AMFAddr    string
}

func main() {
	fmt.Println("=========================================")
	fmt.Println("Starting 5G-AGW: C-Plane Gateway (CGW)...")
	fmt.Println("=========================================")

	cfg := loadConfig()
	if cfg.AMFAddr == "" {
		log.Printf("[CGW] Running in mock AMF mode. Set CGW_AMF_ADDR to enable transparent proxy mode.")
	} else {
		log.Printf("[CGW] Running in transparent proxy mode. Upstream AMF: %s", cfg.AMFAddr)
	}

	// [Southbound] gNBからの接続待ち受け
	gwAddr, err := sctp.ResolveSCTPAddr("sctp", cfg.ListenAddr)
	if err != nil {
		log.Fatalf("Gatewayアドレスの解決に失敗: %v", err)
	}

	listener, err := sctp.ListenSCTP("sctp", gwAddr)
	if err != nil {
		log.Fatalf("SCTP Listenに失敗: %v", err)
	}
	defer listener.Close()

	fmt.Printf("[CGW] Listening for gNB on %s...\n", gwAddr.String())

	for {
		conn, err := listener.Accept(0)
		if err != nil {
			log.Printf("[CGW] SCTP Acceptエラー: %v", err)
			continue
		}
		log.Printf("[CGW] gNBからの新規接続を受信: %s", conn.RemoteAddr().String())

		go handleGnbConnection(conn, cfg)
	}
}

func loadConfig() config {
	return config{
		ListenAddr: envOrDefault("CGW_LISTEN_ADDR", "0.0.0.0:38412"),
		AMFAddr:    strings.TrimSpace(os.Getenv("CGW_AMF_ADDR")),
	}
}

func envOrDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func handleGnbConnection(gnbConn net.Conn, cfg config) {
	defer gnbConn.Close()

	if cfg.AMFAddr != "" {
		handleTransparentProxy(gnbConn, cfg.AMFAddr)
		return
	}

	buf := make([]byte, 65535)
	for {
		n, err := gnbConn.Read(buf)
		if err != nil {
			log.Printf("[CGW] gNBからの読み込みエラー/切断: %v", err)
			return
		}

		log.Printf("[CGW] %d バイトのSCTPデータを受信 (アクティブなUE数: %d)", n, len(context.MainNatTable.GatewayIdToContext))
		logNGAP("gNB -> CGW", buf[:n])

		// NGAP メッセージのデコード
		pdu, err := ngap.Decoder(buf[:n])
		if err != nil {
			log.Printf("[CGW] NGAPデコードエラー: %v", err)
			continue
		}

		if pdu.Present == ngapType.NGAPPDUPresentInitiatingMessage {
			procedureCode := pdu.InitiatingMessage.ProcedureCode.Value

			// 21: NGSetupRequest
			if procedureCode == ngapType.ProcedureCodeNGSetup {
				log.Printf("[CGW] -> 基地局からの初期登録リクエスト (NGSetupRequest) を検知しました！")
				log.Printf("[CGW] -> AMFのフリをして NG Setup Response を返信します...")

				// 返信メッセージの生成。環境変数があればHexリプレイを優先します。
				responseBytes, err := buildNGSetupResponse()
				if err != nil {
					log.Printf("[CGW] NG Setup Response の生成に失敗: %v", err)
					continue
				}

				// gNBへ送信
				_, err = gnbConn.Write(responseBytes)
				if err != nil {
					log.Printf("[CGW] NG Setup Response の送信に失敗: %v", err)
					return
				}
				log.Printf("[CGW] NG Setup Response を送信しました！ SCTPリンク確立完了！")
			}
		}
	}
}

func handleTransparentProxy(gnbConn net.Conn, amfAddr string) {
	remoteAddr, err := sctp.ResolveSCTPAddr("sctp", amfAddr)
	if err != nil {
		log.Printf("[CGW] AMFアドレスの解決に失敗: %v", err)
		return
	}

	amfConn, err := sctp.DialSCTP("sctp", nil, remoteAddr)
	if err != nil {
		log.Printf("[CGW] AMFへのSCTP接続に失敗: %v", err)
		log.Printf("[CGW] ヒント: AMFが %s で起動しているか、またはCGW_AMF_ADDRを未設定にしてmock AMF modeで起動してください。", amfAddr)
		return
	}
	defer amfConn.Close()

	log.Printf("[CGW] AMFへのSCTP接続を確立: %s", amfConn.RemoteAddr().String())

	errCh := make(chan error, 2)
	go proxyNGAP("AMF -> gNB", gnbConn, amfConn, errCh)
	go proxyNGAP("gNB -> AMF", amfConn, gnbConn, errCh)

	if err := <-errCh; err != nil {
		log.Printf("[CGW] 透過プロキシを終了: %v", err)
	}
}

func proxyNGAP(direction string, dst net.Conn, src net.Conn, errCh chan<- error) {
	buf := make([]byte, 65535)
	for {
		n, err := src.Read(buf)
		if err != nil {
			errCh <- fmt.Errorf("%s read error: %w", direction, err)
			return
		}

		logNGAP(direction, buf[:n])

		if err := writeFull(dst, buf[:n]); err != nil {
			errCh <- fmt.Errorf("%s write error: %w", direction, err)
			return
		}
	}
}

func writeFull(conn net.Conn, payload []byte) error {
	for len(payload) > 0 {
		n, err := conn.Write(payload)
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
		payload = payload[n:]
	}
	return nil
}

func logNGAP(direction string, payload []byte) {
	pdu, err := ngap.Decoder(payload)
	if err != nil {
		log.Printf("[CGW] %s: %d bytes (NGAP decode error: %v)", direction, len(payload), err)
		return
	}

	switch pdu.Present {
	case ngapType.NGAPPDUPresentInitiatingMessage:
		logNGAPMessage(direction, "InitiatingMessage", pdu.InitiatingMessage.ProcedureCode.Value, len(payload))
	case ngapType.NGAPPDUPresentSuccessfulOutcome:
		logNGAPMessage(direction, "SuccessfulOutcome", pdu.SuccessfulOutcome.ProcedureCode.Value, len(payload))
	case ngapType.NGAPPDUPresentUnsuccessfulOutcome:
		logNGAPMessage(direction, "UnsuccessfulOutcome", pdu.UnsuccessfulOutcome.ProcedureCode.Value, len(payload))
	default:
		log.Printf("[CGW] %s: Unknown NGAP PDU Present=%d (%d bytes)", direction, pdu.Present, len(payload))
	}
}

func logNGAPMessage(direction, pduType string, procedureCode int64, payloadBytes int) {
	log.Printf(
		"[CGW] %s: pdu=%s procedure=%s procedureCode=%d size=%d bytes",
		direction,
		pduType,
		procedureName(procedureCode),
		procedureCode,
		payloadBytes,
	)
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

// buildNGSetupResponse はAMFとして振る舞うための応答メッセージを構築します。
// CGW_NGSETUP_RESPONSE_HEX が設定されている場合は、そのバイナリをリプレイします。
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

	// 1. AMF Name (Gatewayの名前を名乗ります)
	ie1 := ngapType.NGSetupResponseIEs{}
	ie1.Id.Value = ngapType.ProtocolIEIDAMFName
	ie1.Criticality.Value = ngapType.CriticalityPresentReject
	ie1.Value.Present = ngapType.NGSetupResponseIEsPresentAMFName
	ie1.Value.AMFName = new(ngapType.AMFName)
	ie1.Value.AMFName.Value = "5G-AGW"
	*ieList = append(*ieList, ie1)

	// 2. Served GUAMI List (対応しているネットワーク情報)
	ie2 := ngapType.NGSetupResponseIEs{}
	ie2.Id.Value = ngapType.ProtocolIEIDServedGUAMIList
	ie2.Criticality.Value = ngapType.CriticalityPresentReject
	ie2.Value.Present = ngapType.NGSetupResponseIEsPresentServedGUAMIList
	ie2.Value.ServedGUAMIList = new(ngapType.ServedGUAMIList)

	guamiItem := ngapType.ServedGUAMIItem{}
	// PLMN: 999-02 は 16進数で 0x99, 0xF9, 0x20 と表現されます
	guamiItem.GUAMI.PLMNIdentity.Value = []byte{0x99, 0xf9, 0x20}
	guamiItem.GUAMI.AMFRegionID.Value = aper.BitString{Bytes: []byte{0x01}, BitLength: 8}
	guamiItem.GUAMI.AMFSetID.Value = aper.BitString{Bytes: []byte{0x00, 0x40}, BitLength: 10}
	guamiItem.GUAMI.AMFPointer.Value = aper.BitString{Bytes: []byte{0x00}, BitLength: 6}
	ie2.Value.ServedGUAMIList.List = append(ie2.Value.ServedGUAMIList.List, guamiItem)
	*ieList = append(*ieList, ie2)

	// 3. Relative AMF Capacity (処理能力の余裕度)
	ie3 := ngapType.NGSetupResponseIEs{}
	ie3.Id.Value = ngapType.ProtocolIEIDRelativeAMFCapacity
	ie3.Criticality.Value = ngapType.CriticalityPresentIgnore
	ie3.Value.Present = ngapType.NGSetupResponseIEsPresentRelativeAMFCapacity
	ie3.Value.RelativeAMFCapacity = new(ngapType.RelativeAMFCapacity)
	ie3.Value.RelativeAMFCapacity.Value = 100
	*ieList = append(*ieList, ie3)

	// 4. PLMN Support List (PacketRusherのPLMN/S-NSSAIと合わせる)
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
