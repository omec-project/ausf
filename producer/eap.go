// Copyright (c) 2026 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package producer

import (
	"encoding/binary"

	"github.com/omec-project/openapi/v2"
)

// EapCode enumerates the RFC 3748 Section 4 Code values this service handles.
type EapCode uint8

const (
	EapCodeForRequest  EapCode = 1
	EapCodeForResponse EapCode = 2
	EapCodeForSuccess  EapCode = 3
	EapCodeForFailure  EapCode = 4
)

// EapType identifies the authentication method of a Request/Response packet (RFC 3748 Section 5).
type EapType uint8

// EAP method Type values defined by RFC 3748 Section 5.
const (
	EapTypeIdentity         EapType = 1
	EapTypeNotification     EapType = 2
	EapTypeNak              EapType = 3 // valid on Response packets only
	EapTypeMD5Challenge     EapType = 4
	EapTypeOneTimePassword  EapType = 5
	EapTypeGenericTokenCard EapType = 6
	EapTypeEapAkaPrime      EapType = 50 // according to RFC 5448 Section 6.1
	EapTypeExpandedTypes    EapType = 254
	EapTypeExperimentalUse  EapType = 255
)

// eapBaseHeaderSize is the size in bytes of the Code, Identifier and Length fields present in every EAP packet.
const eapBaseHeaderSize = 4

// EapPacket is the RFC 3748 Section 4 EAP packet layout.
type EapPacket struct {
	Code       EapCode
	Identifier uint8
	Type       EapType
	TypeData   []byte
}

// hasType reports whether the packet carries a Type byte, which RFC 3748 Section 4 defines
// only for Request and Response packets; Success and Failure packets are four bytes long.
func (p *EapPacket) hasType() bool {
	return p.Code == EapCodeForRequest || p.Code == EapCodeForResponse
}

// Encode renders the packet into its wire format, computing the Length field from the payload size.
func (p *EapPacket) Encode() []byte {
	headerSize := eapBaseHeaderSize
	if p.hasType() {
		headerSize++
	}
	wire := make([]byte, headerSize+len(p.TypeData))
	wire[0] = byte(p.Code)
	wire[1] = p.Identifier
	binary.BigEndian.PutUint16(wire[2:4], uint16(len(wire)))
	if p.hasType() {
		wire[4] = byte(p.Type)
	}
	copy(wire[headerSize:], p.TypeData)
	return wire
}

// EapDecode parses an RFC 3748 EAP packet from its wire format, honoring the declared Length field.
func EapDecode(wire []byte) (*EapPacket, error) {
	if len(wire) < eapBaseHeaderSize {
		return nil, openapi.ReportError("eap: packet has %d bytes, need at least %d for the header", len(wire), eapBaseHeaderSize)
	}
	declaredLen := int(binary.BigEndian.Uint16(wire[2:4]))
	if declaredLen < eapBaseHeaderSize {
		return nil, openapi.ReportError("eap: declared length %d is smaller than the %d-byte header", declaredLen, eapBaseHeaderSize)
	}
	if declaredLen > len(wire) {
		return nil, openapi.ReportError("eap: declared length %d exceeds the %d received bytes", declaredLen, len(wire))
	}
	packet := &EapPacket{
		Code:       EapCode(wire[0]),
		Identifier: wire[1],
	}
	headerSize := eapBaseHeaderSize
	if packet.hasType() {
		headerSize++
		if declaredLen < headerSize {
			return nil, openapi.ReportError("eap: declared length %d is smaller than the %d-byte header for Request/Response packets", declaredLen, headerSize)
		}
		packet.Type = EapType(wire[4])
	}
	packet.TypeData = wire[headerSize:declaredLen]
	return packet, nil
}
