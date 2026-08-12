package uplane

import (
	"bytes"
	"errors"
	"testing"
)

func TestParsePacketReadsTPDUTEID(t *testing.T) {
	payload := []byte{0xde, 0xad, 0xbe, 0xef}
	packet := BuildTPDU(0x11111111, payload)

	parsed, err := ParsePacket(packet)
	if err != nil {
		t.Fatalf("ParsePacket() error = %v", err)
	}

	if parsed.MessageType != MessageTypeTPDU {
		t.Fatalf("MessageType = 0x%02x, want 0x%02x", parsed.MessageType, MessageTypeTPDU)
	}
	if parsed.TEID != 0x11111111 {
		t.Fatalf("TEID = 0x%08x, want 0x11111111", parsed.TEID)
	}
	if !bytes.Equal(parsed.Payload, payload) {
		t.Fatalf("Payload = %x, want %x", parsed.Payload, payload)
	}
}

func TestRewriteTEIDOnlyChangesTEID(t *testing.T) {
	original := BuildTPDU(0x11111111, []byte{0xca, 0xfe, 0xba, 0xbe})

	rewritten, err := RewriteTEID(original, 0xaaaa0001)
	if err != nil {
		t.Fatalf("RewriteTEID() error = %v", err)
	}

	if bytes.Equal(original, rewritten) {
		t.Fatal("RewriteTEID() returned an unchanged packet")
	}
	if got := originalTEID(t, original); got != 0x11111111 {
		t.Fatalf("original TEID was mutated to 0x%08x", got)
	}
	if got := originalTEID(t, rewritten); got != 0xaaaa0001 {
		t.Fatalf("rewritten TEID = 0x%08x, want 0xaaaa0001", got)
	}
	if !bytes.Equal(original[:4], rewritten[:4]) || !bytes.Equal(original[8:], rewritten[8:]) {
		t.Fatalf("RewriteTEID() changed bytes outside TEID: original=%x rewritten=%x", original, rewritten)
	}
}

func TestParsePacketRejectsInvalidPackets(t *testing.T) {
	tests := []struct {
		name    string
		packet  []byte
		wantErr error
	}{
		{
			name:    "too short",
			packet:  []byte{0x30, MessageTypeTPDU},
			wantErr: ErrPacketTooShort,
		},
		{
			name:    "unsupported version",
			packet:  []byte{0x10, MessageTypeTPDU, 0, 0, 0, 0, 0, 1},
			wantErr: ErrUnsupportedVersion,
		},
		{
			name:    "unsupported protocol type",
			packet:  []byte{0x20, MessageTypeTPDU, 0, 0, 0, 0, 0, 1},
			wantErr: ErrUnsupportedProtocol,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParsePacket(tt.packet)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("ParsePacket() error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func originalTEID(t *testing.T, packet []byte) uint32 {
	t.Helper()

	parsed, err := ParsePacket(packet)
	if err != nil {
		t.Fatalf("ParsePacket() error = %v", err)
	}
	return parsed.TEID
}
