// Copyright (c) 2026 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package producer

import (
	"encoding/binary"
	"testing"
)

func TestEapPacketEncodeDecode_RequestResponseIncludesTypeByte(t *testing.T) {
	original := EapPacket{
		Code:       EapCodeForResponse,
		Identifier: 7,
		Type:       EapTypeEapAkaPrime,
		TypeData:   []byte("payload"),
	}
	wire := original.Encode()
	if len(wire) != eapBaseHeaderSize+1+len(original.TypeData) {
		t.Fatalf("Encode() length = %d, want %d", len(wire), eapBaseHeaderSize+1+len(original.TypeData))
	}

	decoded, err := EapDecode(wire)
	if err != nil {
		t.Fatalf("EapDecode() error = %v", err)
	}
	if decoded.Code != original.Code || decoded.Identifier != original.Identifier || decoded.Type != original.Type {
		t.Fatalf("EapDecode() = %+v, want header matching %+v", decoded, original)
	}
	if string(decoded.TypeData) != string(original.TypeData) {
		t.Fatalf("EapDecode() TypeData = %q, want %q", decoded.TypeData, original.TypeData)
	}
}

func TestEapPacketEncodeDecode_SuccessFailureOmitsTypeByte(t *testing.T) {
	for _, code := range []EapCode{EapCodeForSuccess, EapCodeForFailure} {
		original := EapPacket{Code: code, Identifier: 3}
		wire := original.Encode()
		if len(wire) != eapBaseHeaderSize {
			t.Fatalf("Encode() for code %d length = %d, want %d", code, len(wire), eapBaseHeaderSize)
		}

		decoded, err := EapDecode(wire)
		if err != nil {
			t.Fatalf("EapDecode() for code %d error = %v", code, err)
		}
		if decoded.Code != code || decoded.Identifier != original.Identifier {
			t.Fatalf("EapDecode() for code %d = %+v, want header matching %+v", code, decoded, original)
		}
		if len(decoded.TypeData) != 0 {
			t.Fatalf("EapDecode() for code %d TypeData = %v, want empty", code, decoded.TypeData)
		}
	}
}

func TestEapDecode_RejectsRequestShorterThanFiveBytes(t *testing.T) {
	// A four-byte Request packet is malformed: Request/Response packets always carry a Type byte.
	wire := []byte{byte(EapCodeForRequest), 1, 0, 4}
	if _, err := EapDecode(wire); err == nil {
		t.Fatal("EapDecode() error = nil, want error for undersized Request packet")
	}
}

func TestEapDecode_RejectsTooShortPacket(t *testing.T) {
	wire := []byte{0, 1, 0}
	if _, err := EapDecode(wire); err == nil {
		t.Fatal("EapDecode() error = nil, want error for packet shorter than base header")
	}
}

func TestEapPacketEncode_SuccessFailureDropsTypeData(t *testing.T) {
	// TypeData must be ignored for Success/Failure codes: RFC 3748 mandates an exact 4-byte packet.
	original := EapPacket{Code: EapCodeForSuccess, Identifier: 9, TypeData: []byte("unexpected")}
	wire := original.Encode()
	if len(wire) != eapBaseHeaderSize {
		t.Fatalf("Encode() length = %d, want %d", len(wire), eapBaseHeaderSize)
	}
}

func TestEapDecode_RejectsOversizedSuccessFailurePacket(t *testing.T) {
	// Declaring more than 4 bytes for a Success/Failure packet violates RFC 3748 Section 4.
	wire := make([]byte, eapBaseHeaderSize+4)
	wire[0] = byte(EapCodeForSuccess)
	wire[1] = 9
	binary.BigEndian.PutUint16(wire[2:4], uint16(len(wire)))
	if _, err := EapDecode(wire); err == nil {
		t.Fatal("EapDecode() error = nil, want error for oversized Success packet")
	}
}
