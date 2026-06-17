package main

import (
	"log"
	"net"

	"5g-agw/internal/context"

	"github.com/free5gc/ngap"
	"github.com/free5gc/ngap/ngapType"
	"github.com/free5gc/sctp"
)

func runServer(cfg config) {
	gwAddr, err := sctp.ResolveSCTPAddr("sctp", cfg.ListenAddr)
	if err != nil {
		log.Fatalf("Gatewayアドレスの解決に失敗: %v", err)
	}

	listener, err := sctp.ListenSCTP("sctp", gwAddr)
	if err != nil {
		log.Fatalf("SCTP Listenに失敗: %v", err)
	}
	defer listener.Close()

	log.Printf("[CGW] Listening for gNB on %s...", gwAddr.String())

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

func handleGnbConnection(gnbConn net.Conn, cfg config) {
	defer gnbConn.Close()

	if cfg.AMFAddr != "" {
		handleTransparentProxy(gnbConn, cfg.AMFAddr)
		return
	}

	handleMockAMF(gnbConn)
}

func handleMockAMF(gnbConn net.Conn) {
	buf := make([]byte, 65535)
	for {
		n, err := gnbConn.Read(buf)
		if err != nil {
			log.Printf("[CGW] gNBからの読み込みエラー/切断: %v", err)
			return
		}

		log.Printf("[CGW] %d バイトのSCTPデータを受信 (アクティブなUE数: %d)", n, len(context.MainNatTable.GatewayIdToContext))
		logNGAP("gNB -> CGW", buf[:n])

		pdu, err := ngap.Decoder(buf[:n])
		if err != nil {
			log.Printf("[CGW] NGAPデコードエラー: %v", err)
			continue
		}

		if pdu.Present != ngapType.NGAPPDUPresentInitiatingMessage {
			continue
		}

		if pdu.InitiatingMessage.ProcedureCode.Value != ngapType.ProcedureCodeNGSetup {
			continue
		}

		log.Printf("[CGW] -> 基地局からの初期登録リクエスト (NGSetupRequest) を検知しました")
		log.Printf("[CGW] -> AMFのフリをして NG Setup Response を返信します...")

		responseBytes, err := buildNGSetupResponse()
		if err != nil {
			log.Printf("[CGW] NG Setup Response の生成に失敗: %v", err)
			continue
		}

		if err := writeFull(gnbConn, responseBytes); err != nil {
			log.Printf("[CGW] NG Setup Response の送信に失敗: %v", err)
			return
		}
		log.Printf("[CGW] NG Setup Response を送信しました。 SCTPリンク確立完了！")
	}
}
