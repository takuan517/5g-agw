package uplane

import (
	"encoding/binary"
	"errors"
	"fmt"
)

const (
	gtpuMinHeaderLen = 8
	gtpuVersionMask  = 0xe0
	gtpuVersion1     = 0x20
	gtpuProtocolType = 0x10

	MessageTypeTPDU = 0xff
)

var (
	ErrPacketTooShort      = errors.New("gtp-u packet too short")
	ErrUnsupportedVersion  = errors.New("unsupported gtp-u version")
	ErrUnsupportedProtocol = errors.New("unsupported gtp-u protocol type")
)

type Packet struct {
	Flags       byte
	MessageType byte
	Length      uint16
	TEID        uint32
	Payload     []byte
}

func ParsePacket(packet []byte) (Packet, error) {
	if len(packet) < gtpuMinHeaderLen {
		return Packet{}, ErrPacketTooShort
	}
	if packet[0]&gtpuVersionMask != gtpuVersion1 {
		return Packet{}, fmt.Errorf("%w: flags=0x%02x", ErrUnsupportedVersion, packet[0])
	}
	if packet[0]&gtpuProtocolType == 0 {
		return Packet{}, fmt.Errorf("%w: flags=0x%02x", ErrUnsupportedProtocol, packet[0])
	}

	return Packet{
		Flags:       packet[0],
		MessageType: packet[1],
		Length:      binary.BigEndian.Uint16(packet[2:4]),
		TEID:        binary.BigEndian.Uint32(packet[4:8]),
		Payload:     packet[gtpuMinHeaderLen:],
	}, nil
}

func RewriteTEID(packet []byte, teid uint32) ([]byte, error) {
	parsed, err := ParsePacket(packet)
	if err != nil {
		return nil, err
	}

	rewritten := append([]byte(nil), packet...)
	binary.BigEndian.PutUint32(rewritten[4:8], teid)

	// Keep the original length and payload intact; TEID translation only changes
	// the tunnel identifier in the fixed GTP-U header.
	rewritten[2] = byte(parsed.Length >> 8)
	rewritten[3] = byte(parsed.Length)
	return rewritten, nil
}

func BuildTPDU(teid uint32, payload []byte) []byte {
	packet := make([]byte, gtpuMinHeaderLen+len(payload))
	packet[0] = gtpuVersion1 | gtpuProtocolType
	packet[1] = MessageTypeTPDU
	binary.BigEndian.PutUint16(packet[2:4], uint16(len(payload)))
	binary.BigEndian.PutUint32(packet[4:8], teid)
	copy(packet[gtpuMinHeaderLen:], payload)
	return packet
}
