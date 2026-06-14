package main

import (
	"fmt"
	"log"
	"net"

	"5g-agw/internal/context"

	"github.com/free5gc/ngap"
	"github.com/free5gc/ngap/ngapType"
	"github.com/free5gc/sctp"
)

func main() {
	fmt.Println("=========================================")
	fmt.Println("Starting 5G-AGW: C-Plane Gateway (CGW)...")
	fmt.Println("=========================================")

	// [Northbound] AMFへの接続準備（アドレス解決のみ）
	amfAddr, err := sctp.ResolveSCTPAddr("sctp", "127.0.0.1:38412")
	if err != nil {
		log.Fatalf("AMFアドレスの解決に失敗: %v", err)
	}
	_ = amfAddr

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

		// ==========================================
		// NGAP メッセージのデコード (パース)
		// ==========================================
		pdu, err := ngap.Decoder(buf[:n])
		if err != nil {
			log.Printf("[CGW] NGAPデコードエラー: %v", err)
			continue
		}

		// メッセージの種類を判定
		switch pdu.Present {
		case ngapType.NGAPPDUPresentInitiatingMessage:
			procedureCode := pdu.InitiatingMessage.ProcedureCode.Value
			log.Printf("[CGW] 受信: InitiatingMessage (ProcedureCode: %d)", procedureCode)
			
			// ProcedureCode == 21 は NGSetupRequest を意味する
			if procedureCode == 21 {
				log.Printf("[CGW] -> 基地局からの初期登録リクエスト (NGSetupRequest) を検知")
			}

		case ngapType.NGAPPDUPresentSuccessfulOutcome:
			log.Printf("[CGW] 受信: SuccessfulOutcome")
			
		case ngapType.NGAPPDUPresentUnsuccessfulOutcome:
			log.Printf("[CGW] 受信: UnsuccessfulOutcome")
			
		default:
			log.Printf("[CGW] 受信: 未知のメッセージタイプ")
		}
	}
}
