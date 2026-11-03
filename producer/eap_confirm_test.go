// Copyright (c) 2026 Intel Corporation
// SPDX-License-Identifier: Apache-2.0

package producer

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"

	ausf_context "github.com/omec-project/ausf/context"
	"github.com/omec-project/openapi/v2/models"
)

const (
	testEapSupi  = "imsi-001010000000099"
	testEapCtxID = "suci-eap-confirm"
	testKaut     = "0123456789abcdef0123456789abcdef" // 32 raw bytes of K_aut as stored by eapAkaPrimePrf
	testKseaf    = "aabbccdd"
)

var testRes = []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08}

// buildAtRes renders an AT_RES attribute with an explicit bit count and zero-padded value.
func buildAtRes(bits uint16, value []byte) []byte {
	words := (len(value)+3)/4 + 1
	b := make([]byte, words*4)
	b[0] = ausf_context.AT_RES_ATTRIBUTE
	b[1] = byte(words)
	b[2], b[3] = byte(bits>>8), byte(bits)
	copy(b[4:], value)
	return b
}

// akaChallengeHeader is the AKA-Challenge Subtype and two reserved bytes that precede the attributes (RFC 4187 section 8.1).
var akaChallengeHeader = []byte{eapAkaSubtypeChallenge, 0, 0}

// buildEapResponse builds an EAP Response whose AT_MAC is computed with kaut and then optionally corrupted.
func buildEapResponse(attrs []byte, corruptMac bool) []byte {
	return buildEapResponseWithMac(akaChallengeHeader, attrs, make([]byte, 16), corruptMac)
}

// buildEapResponseWithMac is buildEapResponse with an explicit method header and AT_MAC value area; a value area
// longer than 16 bytes yields an oversized AT_MAC attribute.
func buildEapResponseWithMac(header, attrs, macValue []byte, corruptMac bool) []byte {
	mac := append([]byte{ausf_context.AT_MAC_ATTRIBUTE, byte(1 + (len(macValue)+3)/4), 0, 0}, macValue...)
	data := append(append(append([]byte{}, header...), attrs...), mac...)
	pkt := EapPacket{Code: EapCodeForResponse, Identifier: 9, Type: EapTypeEapAkaPrime, TypeData: data}
	macOffset := len(header) + len(attrs)
	copy(pkt.TypeData[macOffset+4:], CalculateAtMAC([]byte(testKaut), pkt.Encode()))
	if corruptMac {
		pkt.TypeData[macOffset+4] ^= 0xff
	}
	return pkt.Encode()
}

// buildEapResponseWithType is buildEapResponse with a valid AT_MAC but an arbitrary outer EAP Type.
func buildEapResponseWithType(eapType EapType, header, attrs []byte) []byte {
	mac := append([]byte{ausf_context.AT_MAC_ATTRIBUTE, 5, 0, 0}, make([]byte, 16)...)
	data := append(append(append([]byte{}, header...), attrs...), mac...)
	pkt := EapPacket{Code: EapCodeForResponse, Identifier: 9, Type: eapType, TypeData: data}
	copy(pkt.TypeData[len(header)+len(attrs)+4:], CalculateAtMAC([]byte(testKaut), pkt.Encode()))
	return pkt.Encode()
}

func decodePayload(t *testing.T, payload string) []byte {
	t.Helper()
	wire, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		t.Fatalf("decode EAP payload: %v", err)
	}
	return wire
}

func TestEncodeAtRes_UsesBitLengthAndTrailingPadding(t *testing.T) {
	encoded, err := EapEncodeAttribute("AT_RES", string(testRes))
	if err != nil {
		t.Fatalf("EapEncodeAttribute() error = %v", err)
	}
	want := buildAtRes(64, testRes)
	if encoded != string(want) {
		t.Fatalf("EapEncodeAttribute() = %x, want %x", encoded, want)
	}
	if _, err := EapEncodeAttribute("AT_RES", "abc"); err == nil {
		t.Fatal("EapEncodeAttribute() accepted a 24-bit RES")
	}
	if _, err := EapEncodeAttribute("AT_RES", string(make([]byte, 17))); err == nil {
		t.Fatal("EapEncodeAttribute() accepted a 136-bit RES")
	}
}

func TestEncodeAtKdfInput_NetworkNameFirstThenTrailingPadding(t *testing.T) {
	tests := []struct {
		name   string
		snName string
		want   []byte
	}{
		{"NSWO is padded at the end", "5G:NSWO", append([]byte{23, 3, 0, 7}, []byte("5G:NSWO\x00")...)},
		{"aligned name needs no padding", "5G:A", []byte{23, 2, 0, 4, '5', 'G', ':', 'A'}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded, err := EapEncodeAttribute("AT_KDF_INPUT", tt.snName)
			if err != nil {
				t.Fatalf("EapEncodeAttribute() error = %v", err)
			}
			if encoded != string(tt.want) {
				t.Fatalf("EapEncodeAttribute() = %x, want %x", encoded, tt.want)
			}
		})
	}
}

func TestExtractAtRes(t *testing.T) {
	tests := []struct {
		name string
		attr []byte
		want []byte // nil means rejected
	}{
		{"64-bit RES", buildAtRes(64, testRes), testRes},
		{"32-bit RES", buildAtRes(32, testRes[:4]), testRes[:4]},
		{"128-bit RES", buildAtRes(128, bytes.Repeat([]byte{0xaa}, 16)), bytes.Repeat([]byte{0xaa}, 16)},
		{"zero-bit RES with data", buildAtRes(0, testRes), nil},
		{"below 32 bits", buildAtRes(24, testRes[:3]), nil},
		{"above 128 bits", buildAtRes(136, bytes.Repeat([]byte{0xaa}, 17)), nil},
		{"bit count not a whole byte", buildAtRes(36, testRes[:5]), nil},
		{"bit count larger than payload", buildAtRes(128, testRes), nil},
		{"payload larger than bit count", buildAtRes(32, testRes), nil},
		{"header only", []byte{ausf_context.AT_RES_ATTRIBUTE, 1, 0, 0}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := extractAtRes(tt.attr)
			if tt.want == nil {
				if ok {
					t.Fatalf("extractAtRes() = %x, want rejection", got)
				}
				return
			}
			if !ok || !bytes.Equal(got, tt.want) {
				t.Fatalf("extractAtRes() = %x, %v; want %x", got, ok, tt.want)
			}
		})
	}
}

func TestResMatchesXres(t *testing.T) {
	xres := hex.EncodeToString(testRes)
	if !resMatchesXres(xres, testRes) {
		t.Error("matching RES rejected")
	}
	if resMatchesXres(xres, testRes[:4]) {
		t.Error("truncated RES accepted")
	}
	if resMatchesXres(xres, append(bytes.Clone(testRes), 0)) {
		t.Error("RES with trailing padding accepted")
	}
	if resMatchesXres("not-hex", testRes) || resMatchesXres("", nil) {
		t.Error("invalid or empty XRES accepted")
	}
}

func TestEapAuthComfirmRequestProcedure_ReleasesKeysOnlyOnSuccess(t *testing.T) {
	initProducerTestContext(t)

	udm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		if _, err := w.Write([]byte(`{}`)); err != nil {
			t.Errorf("write UDM response: %v", err)
		}
	}))
	defer udm.Close()

	goodRes := buildAtRes(64, testRes)
	wrongRes := buildAtRes(64, []byte{9, 9, 9, 9, 9, 9, 9, 9})
	unknownAttr := []byte{0xf0, 0x00, 0, 0} // zero length attribute

	tests := []struct {
		name       string
		payload    []byte
		success    bool
		wantNotify bool // failure notification (ongoing) rather than a bare Failure
		wantStatus models.AuthResult
	}{
		{name: "valid RES and MAC", payload: buildEapResponse(goodRes, false), success: true, wantStatus: models.AUTHRESULT_AUTHENTICATION_SUCCESS},
		{name: "invalid MAC", payload: buildEapResponse(goodRes, true), wantNotify: true, wantStatus: models.AUTHRESULT_AUTHENTICATION_ONGOING},
		{name: "mismatched RES", payload: buildEapResponse(wrongRes, false), wantNotify: true, wantStatus: models.AUTHRESULT_AUTHENTICATION_ONGOING},
		{name: "zero-bit AT_RES", payload: buildEapResponse(buildAtRes(0, testRes), false), wantNotify: true, wantStatus: models.AUTHRESULT_AUTHENTICATION_ONGOING},
		{name: "malformed attribute", payload: buildEapResponse(unknownAttr, false), wantNotify: true, wantStatus: models.AUTHRESULT_AUTHENTICATION_ONGOING},
		{name: "missing AKA header", payload: buildEapResponseWithMac(nil, goodRes, make([]byte, 16), false), wantNotify: true, wantStatus: models.AUTHRESULT_AUTHENTICATION_ONGOING},
		{name: "wrong subtype", payload: buildEapResponseWithMac([]byte{eapAkaSubtypeNotification, 0, 0}, goodRes, make([]byte, 16), false), wantNotify: true, wantStatus: models.AUTHRESULT_AUTHENTICATION_ONGOING},
		{name: "oversized AT_MAC", payload: buildEapResponseWithMac(akaChallengeHeader, goodRes, make([]byte, 20), false), wantNotify: true, wantStatus: models.AUTHRESULT_AUTHENTICATION_ONGOING},
		{name: "missing AT_RES", payload: buildEapResponse(nil, false), wantNotify: true, wantStatus: models.AUTHRESULT_AUTHENTICATION_ONGOING},
		{name: "wrong EAP Type", payload: buildEapResponseWithType(EapTypeIdentity, akaChallengeHeader, goodRes), wantNotify: true, wantStatus: models.AUTHRESULT_AUTHENTICATION_ONGOING},
		{name: "non-response code", payload: (&EapPacket{Code: EapCodeForRequest, Identifier: 9, Type: EapTypeEapAkaPrime}).Encode(), wantNotify: true, wantStatus: models.AUTHRESULT_AUTHENTICATION_ONGOING},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ausf_context.AddSuciSupiPairToMap(testEapCtxID, testEapSupi)
			defer ausf_context.RemoveSuciSupiPairFromMap(testEapCtxID)
			ausf_context.AddAusfUeContextToPool(&ausf_context.AusfUeContext{
				Supi:               testEapSupi,
				ServingNetworkName: testServingNetwork,
				UdmUeauUrl:         udm.URL,
				AuthStatus:         models.AUTHRESULT_AUTHENTICATION_ONGOING,
				XRES:               hex.EncodeToString(testRes),
				K_aut:              testKaut,
				Kseaf:              testKseaf,
			})
			defer ausf_context.RemoveAusfUeContextFromPool(testEapSupi)

			session := models.NewEapSessionWithDefaults()
			session.SetEapPayload(base64.StdEncoding.EncodeToString(tt.payload))
			rsp, problem := EapAuthComfirmRequestProcedure(*session, testEapCtxID)
			if problem != nil || rsp == nil {
				t.Fatalf("unexpected result: rsp=%v problem=%+v", rsp, problem)
			}
			if rsp.GetAuthResult() != tt.wantStatus {
				t.Fatalf("auth result = %s, want %s", rsp.GetAuthResult(), tt.wantStatus)
			}
			if tt.success {
				if rsp.GetKSeaf() != testKseaf || rsp.GetSupi() != testEapSupi {
					t.Fatalf("success must carry KSeaf and SUPI, got %q / %q", rsp.GetKSeaf(), rsp.GetSupi())
				}
				wire := decodePayload(t, rsp.GetEapPayload())
				if pkt, err := EapDecode(wire); err != nil || pkt.Code != EapCodeForSuccess {
					t.Fatalf("expected EAP Success, got %+v (err %v)", pkt, err)
				}
				return
			}
			if rsp.GetKSeaf() != "" || rsp.GetSupi() != "" {
				t.Fatalf("KSeaf/SUPI leaked on non-success: %q / %q", rsp.GetKSeaf(), rsp.GetSupi())
			}
			wire := decodePayload(t, rsp.GetEapPayload())
			pkt, err := EapDecode(wire)
			if err != nil || pkt.Code != EapCodeForRequest {
				t.Fatalf("expected failure notification Request, got %+v (err %v)", pkt, err)
			}

			// Any Response after a failure yields EAP Failure, still without keys or SUPI.
			session.SetEapPayload(base64.StdEncoding.EncodeToString(buildEapResponse(goodRes, false)))
			rsp, problem = EapAuthComfirmRequestProcedure(*session, testEapCtxID)
			if problem != nil || rsp == nil {
				t.Fatalf("unexpected follow-up result: rsp=%v problem=%+v", rsp, problem)
			}
			if rsp.GetAuthResult() != models.AUTHRESULT_AUTHENTICATION_FAILURE || rsp.GetKSeaf() != "" || rsp.GetSupi() != "" {
				t.Fatalf("follow-up = %s KSeaf=%q SUPI=%q", rsp.GetAuthResult(), rsp.GetKSeaf(), rsp.GetSupi())
			}
			wire = decodePayload(t, rsp.GetEapPayload())
			if pkt, err := EapDecode(wire); err != nil || pkt.Code != EapCodeForFailure {
				t.Fatalf("expected EAP Failure, got %+v (err %v)", pkt, err)
			}
		})
	}
}

func TestConstructFailEapAkaNotification_HasNotificationHeader(t *testing.T) {
	pkt, err := EapDecode(decodePayload(t, ConstructFailEapAkaNotification(9)))
	if err != nil {
		t.Fatalf("EapDecode() error = %v", err)
	}
	want := []byte{eapAkaSubtypeNotification, 0, 0, ausf_context.AT_NOTIFICATION_ATTRIBUTE, 1, 0x40, 0x00}
	if !bytes.Equal(pkt.TypeData, want) {
		t.Fatalf("TypeData = %x, want %x", pkt.TypeData, want)
	}
}

// TestEapAkaPrimePrf_Rfc5448Vector checks the key hierarchy against RFC 5448 Appendix C Case 1.
func TestEapAkaPrimePrf_Rfc5448Vector(t *testing.T) {
	const (
		identity = "0555444333222111"
		ckPrime  = "0093962d0dd84aa5684b045c9edffa04"
		ikPrime  = "ccfc230ca74fcc96c0a5d61164f5a76c"
	)
	want := []string{
		"766fa0a6c317174b812d52fbcd11a179",
		"0842ea722ff6835bfa2032499fc3ec23c2f0e388b4f07543ffc677f1696d71ea",
		"cf83aa8bc7e0aced892acc98e76a9b2095b558c7795c7094715cb3393aa7d17a",
		"67c42d9aa56c1b79e295e3459fc3d187d42be0bf818d3070e362c5e967a4d544e8ecfe19358ab3039aff03b7c930588c055babee58a02650b067ec4e9347c75a",
		"f861703cd775590e16c7679ea3874ada866311de290764d760cf76df647ea01c313f69924bdd7650ca9bac141ea075c4ef9e8029c0e290cdbad5638b63bc23fb",
	}
	kEncr, kAut, kRe, msk, emsk, err := eapAkaPrimePrf(ikPrime, ckPrime, identity)
	if err != nil {
		t.Fatalf("eapAkaPrimePrf: %v", err)
	}
	for i, got := range []string{kEncr, kAut, kRe, msk, emsk} {
		if hex.EncodeToString([]byte(got)) != want[i] {
			t.Errorf("key %d = %x, want %s", i, got, want[i])
		}
	}
}

func TestEapAkaIdentityFromSupi(t *testing.T) {
	tests := map[string]string{
		"imsi-001010000000001": "001010000000001",
		"nai-user@example.com": "user@example.com",
		"other":                "other",
	}
	for supi, want := range tests {
		if got := eapAkaIdentityFromSupi(supi); got != want {
			t.Errorf("eapAkaIdentityFromSupi(%q) = %q, want %q", supi, got, want)
		}
	}
}
