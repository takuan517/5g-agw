package main

import (
	"fmt"
	"io"
	"log"
	"net"
	"sync/atomic"

	"github.com/free5gc/ngap"
	"github.com/free5gc/sctp"
)

var nextAssociationID atomic.Int64

type proxySession struct {
	associationID int64
	ueMappings    *UEMappingTable
	pduSessions   *PDUSessionMappingTable
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
		log.Printf("[CGW] AMFが %s で起動しているか、またはCGW_AMF_ADDRを未設定にしてmock AMF modeで起動してください。", amfAddr)
		return
	}
	defer amfConn.Close()

	session := &proxySession{
		associationID: nextAssociationID.Add(1),
		ueMappings:    globalUEMappingTable,
		pduSessions:   globalPDUSessionMappingTable,
	}

	log.Printf("[CGW] AMFへのSCTP接続を確立: %s (assoc=%d)", amfConn.RemoteAddr().String(), session.associationID)

	errCh := make(chan error, 2)
	go proxyNGAP(session, directionAMFToGNB, gnbConn, amfConn, errCh)
	go proxyNGAP(session, directionGNBToAMF, amfConn, gnbConn, errCh)

	if err := <-errCh; err != nil {
		log.Printf("[CGW] 透過プロキシを終了: %v", err)
	}
	session.pduSessions.RemoveAssociation(session.associationID, "association closed")
	session.ueMappings.RemoveAssociation(session.associationID, "association closed")
}

func proxyNGAP(session *proxySession, direction string, dst net.Conn, src net.Conn, errCh chan<- error) {
	buf := make([]byte, 65535)
	for {
		n, err := src.Read(buf)
		if err != nil {
			errCh <- fmt.Errorf("%s read error: %w", direction, err)
			return
		}

		payload := buf[:n]
		rewrittenPayload, err := rewriteNGAPForProxy(session, direction, payload)
		if err != nil {
			errCh <- fmt.Errorf("%s rewrite error: %w", direction, err)
			return
		}

		entry := logNGAP(direction, rewrittenPayload)
		session.ueMappings.Observe(session.associationID, direction, entry)
		session.pduSessions.Observe(session.associationID, direction, session.ueMappings, entry)

		if err := writeFull(dst, rewrittenPayload); err != nil {
			errCh <- fmt.Errorf("%s write error: %w", direction, err)
			return
		}

		if shouldReleaseMapping(entry) {
			session.pduSessions.RemoveByUEIDs(session.associationID, entry.UEIDs, entry.Procedure)
			session.ueMappings.RemoveByUEIDs(session.associationID, entry.UEIDs, entry.Procedure)
		}
	}
}

func writeFull(conn net.Conn, payload []byte) error {
	if sctpConn, ok := conn.(*sctp.SCTPConn); ok {
		return writeFullSCTP(sctpConn, payload)
	}

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

func writeFullSCTP(conn *sctp.SCTPConn, payload []byte) error {
	info := &sctp.SndRcvInfo{
		Stream: 0,
		PPID:   ngap.PPID,
	}

	for len(payload) > 0 {
		n, err := conn.SCTPWrite(payload, info)
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
