package context

import (
	"net"
	"sync"
)

// UeContext は1つのUEに関するルーティング状態を保持する
type UeContext struct {
	OriginalRanId int64 // gNBが付与した本来の RAN UE NGAP ID
	GatewayRanId  int64 // GatewayがAMFに向けて付与した独自の RAN UE NGAP ID
	AmfId         int64 // AMFが付与した AMF UE NGAP ID
	GnbConn       net.Conn // 応答を返すためのgNBとのSCTPコネクション
	AmfConn       net.Conn // 転送先のAMFとのSCTPコネクション
}

// NatTable は複数UEのコンテキストをスレッドセーフに管理する
type NatTable struct {
	sync.RWMutex
	GatewayIdToContext map[int64]*UeContext
}

// グローバルなNATテーブル（外部パッケージからアクセス可能にするため頭文字を大文字に）
var MainNatTable = NatTable{
	GatewayIdToContext: make(map[int64]*UeContext),
}

// 次に払い出すGateway側のRAN UE NGAP ID
var NextGatewayRanId int64 = 1
