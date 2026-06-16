package main

import (
	"encoding/hex"
	"fmt"
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

func main() {
	fmt.Println("=========================================")
	fmt.Println("Starting 5G-AGW: C-Plane Gateway (CGW)...")
	fmt.Println("=========================================")

	// [Southbound] gNBからの接続待ち受け
	gwAddr, err := sctp.ResolveSCTPAddr("sctp", "0.0.0.0:38412")
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

		go handleGnbConnection(conn)
	}
}

func handleGnbConnection(conn net.Conn) {
	defer conn.Close()

	buf := make([]byte, 65535)
	for {
		n, err := conn.Read(buf)
		if err != nil {
			log.Printf("[CGW] gNBからの読み込みエラー/切断: %v", err)
			return
		}

		log.Printf("[CGW] %d バイトのSCTPデータを受信 (アクティブなUE数: %d)", n, len(context.MainNatTable.GatewayIdToContext))

		// NGAP メッセージのデコード
		pdu, err := ngap.Decoder(buf[:n])
		if err != nil {
			log.Printf("[CGW] NGAPデコードエラー: %v", err)
			continue
		}

		if pdu.Present == ngapType.NGAPPDUPresentInitiatingMessage {
			procedureCode := pdu.InitiatingMessage.ProcedureCode.Value

			// 21: NGSetupRequest
			if procedureCode == 21 {
				log.Printf("[CGW] -> 基地局からの初期登録リクエスト (NGSetupRequest) を検知しました！")
				log.Printf("[CGW] -> AMFのフリをして NG Setup Response を返信します...")

				// 返信メッセージの生成。環境変数があればHexリプレイを優先します。
				responseBytes, err := buildNGSetupResponse()
				if err != nil {
					log.Printf("[CGW] NG Setup Response の生成に失敗: %v", err)
					continue
				}

				// gNBへ送信
				_, err = conn.Write(responseBytes)
				if err != nil {
					log.Printf("[CGW] NG Setup Response の送信に失敗: %v", err)
					return
				}
				log.Printf("[CGW] NG Setup Response を送信しました！ SCTPリンク確立完了！")
			}
		}
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
