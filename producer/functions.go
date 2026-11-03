// Copyright 2019 free5GC.org
//
// SPDX-License-Identifier: Apache-2.0
//

package producer

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/omec-project/ausf/consumer"
	ausf_context "github.com/omec-project/ausf/context"
	"github.com/omec-project/ausf/logger"
	"github.com/omec-project/openapi/v2"
	"github.com/omec-project/openapi/v2/Nnrf_NFDiscovery"
	"github.com/omec-project/openapi/v2/Nudm_UEAU"
	"github.com/omec-project/openapi/v2/models"
	"github.com/omec-project/openapi/v2/utils"
)

// atMacAttributeSize is the size in bytes of AT_MAC: type, length, 2 reserved bytes and a 16-byte MAC.
const atMacAttributeSize = 20

// EAP-AKA' packets carry a 1-byte Subtype and 2 reserved bytes between the EAP Type and the attributes
// (RFC 4187 section 8.1, RFC 9048). Subtype values are listed in RFC 4187 section 11.
const (
	eapAkaHeaderSize          = 3
	eapAkaSubtypeChallenge    = 1
	eapAkaSubtypeNotification = 12
)

// AT_RES carries RES as a 16-bit bit count followed by the value padded with zeros to a 4-byte boundary
// (RFC 4187 section 10.8). RES is 32 to 128 bits long (RFC 4187 section 10.8, TS 33.102 clause 6.3.2).
const (
	atResHeaderSize = 4
	minResBits      = 32
	maxResBits      = 128
)

var (
	udmUrlMu    sync.RWMutex // protects AUSFContext.UdmUeauUrl
	udmClientMu sync.Mutex   // protects cachedUdmClient and cachedUdmClientURL

	cachedUdmClient    *Nudm_UEAU.APIClient
	cachedUdmClientURL string
)

var (
	resolveUdmURL           = GetUdmUrl
	executeGenerateAuthData = func(client *Nudm_UEAU.APIClient, supiOrSuci string,
		authInfoReq models.AuthenticationInfoRequest,
	) (*models.AuthenticationInfoResult, *http.Response, error) {
		apiGenerateAuthDataRequest := client.GenerateAuthDataAPI.GenerateAuthData(context.Background(), supiOrSuci)
		apiGenerateAuthDataRequest = apiGenerateAuthDataRequest.AuthenticationInfoRequest(authInfoReq)
		return client.GenerateAuthDataAPI.GenerateAuthDataExecute(apiGenerateAuthDataRequest)
	}
	executeDeleteAuth = func(client *Nudm_UEAU.APIClient, supi, authEventID string,
		authEvent models.AuthEvent,
	) (*http.Response, error) {
		apiDeleteAuthRequest := client.DeleteAuthAPI.DeleteAuth(context.Background(), supi, authEventID)
		apiDeleteAuthRequest = apiDeleteAuthRequest.AuthEvent(authEvent)
		return client.DeleteAuthAPI.DeleteAuthExecute(apiDeleteAuthRequest)
	}
)

// problemDetailsFromUdmAuthError maps a failed Nudm_UEAuthentication_Get call to the ProblemDetails the AUSF returns
// (TS 29.509 table 6.1.3.2.3.1-3). 400/403/404 from the UDM are propagated with the UDM's cause when present;
// any other UDM failure means no AV could be generated; no UDM response at all is an upstream failure reported as
// 504 UPSTREAM_SERVER_ERROR (TS 29.509 table 6.1.7.3-1).
func problemDetailsFromUdmAuthError(rsp *http.Response, err error) *models.ProblemDetails {
	if rsp == nil {
		return utils.ProblemDetailsWithCause("Upstream server error", http.StatusGatewayTimeout, "", UPSTREAM_SERVER_ERROR)
	}
	var defaultCause string
	switch rsp.StatusCode {
	case http.StatusBadRequest:
		defaultCause = utils.CauseInvalidRequest
	case http.StatusForbidden:
		defaultCause = utils.CauseAuthenticationRejected
	case http.StatusNotFound:
		defaultCause = utils.CauseUserNotFound
	default:
		return utils.ProblemDetailsWithCause("AV generation problem", http.StatusInternalServerError, "", AV_GENERATION_PROBLEM_ERROR)
	}
	cause := defaultCause
	if upstream, ok := openapi.ErrorModel[models.ProblemDetails](err); ok && upstream.GetCause() != "" {
		cause = upstream.GetCause()
	}
	return utils.ProblemDetailsWithCause(http.StatusText(rsp.StatusCode), rsp.StatusCode, "", cause)
}

func CalculateAtMAC(key []byte, input []byte) []byte {
	// keyed with K_aut
	h := hmac.New(sha256.New, key)
	if _, err := h.Write(input); err != nil {
		logger.EapAuthComfirmLog.Errorln(err.Error())
	}
	sha := string(h.Sum(nil))
	return []byte(sha[:16])
}

// func EapEncodeAttribute(attributeType string, data string) (returnStr string, err error) {
func EapEncodeAttribute(attributeType string, data string) (string, error) {
	var attribute string
	var length int

	switch attributeType {
	case "AT_RAND":
		length = len(data)/8 + 1
		if length != 5 {
			return "", openapi.ReportError("[eapEncodeAttribute] AT_RAND Length Error")
		}
		attrNum := fmt.Sprintf("%02x", ausf_context.AT_RAND_ATTRIBUTE)
		attribute = attrNum + "05" + "0000" + data

	case "AT_AUTN":
		length = len(data)/8 + 1
		if length != 5 {
			return "", openapi.ReportError("[eapEncodeAttribute] AT_AUTN Length Error")
		}
		attrNum := fmt.Sprintf("%02x", ausf_context.AT_AUTN_ATTRIBUTE)
		attribute = attrNum + "05" + "0000" + data

	case "AT_KDF_INPUT":
		// the attribute carries the actual network name length in bytes; the name is followed by zero padding
		// up to a 4-byte boundary (RFC 5448 section 3.1)
		length := (len(data)+3)/4 + 1
		b := make([]byte, length*4)
		b[0] = ausf_context.AT_KDF_INPUT_ATTRIBUTE
		b[1] = byte(length)
		binary.BigEndian.PutUint16(b[2:4], uint16(len(data)))
		copy(b[4:], data)
		return string(b), nil

	case "AT_KDF":
		// Value 1 default key derivation function for EAP-AKA'
		attrNum := fmt.Sprintf("%02x", ausf_context.AT_KDF_ATTRIBUTE)
		attribute = attrNum + "01" + "0001"

	case "AT_MAC":
		// Pad MAC value with 16 bytes of 0 since this is just for the calculation of MAC
		attrNum := fmt.Sprintf("%02x", ausf_context.AT_MAC_ATTRIBUTE)
		attribute = attrNum + "05" + "0000" + "00000000000000000000000000000000"

	case "AT_RES":
		// data is the raw RES; the attribute carries its length in bits and pads the value at the end
		resBits := len(data) * 8
		if resBits < minResBits || resBits > maxResBits {
			return "", openapi.ReportError("[eapEncodeAttribute] AT_RES Length Error")
		}
		length := (len(data)+3)/4 + 1
		b := make([]byte, length*4)
		b[0] = ausf_context.AT_RES_ATTRIBUTE
		b[1] = byte(length)
		binary.BigEndian.PutUint16(b[2:4], uint16(resBits))
		copy(b[atResHeaderSize:], data)
		return string(b), nil

	default:
		logger.EapAuthComfirmLog.Errorf("UNKNOWN attributeType %s\n", attributeType)
		return "", nil
	}

	if r, err := hex.DecodeString(attribute); err != nil {
		return "", err
	} else {
		return string(r), nil
	}
}

// eapAkaIdentityFromSupi returns the Identity used in the EAP-AKA' key derivation (TS 33.501 Annex F.3): the IMSI
// digits or the NAI of the SUPI, without the TS 29.571 type prefix.
func eapAkaIdentityFromSupi(supi string) string {
	if rest, ok := strings.CutPrefix(supi, "imsi-"); ok {
		return rest
	}
	if rest, ok := strings.CutPrefix(supi, "nai-"); ok {
		return rest
	}
	return supi
}

// func eapAkaPrimePrf(ikPrime string, ckPrime string, identity string) (K_encr string, K_aut string, K_re string,
//
//	MSK string, EMSK string, err error) {
func eapAkaPrimePrf(ikPrime string, ckPrime string, identity string) (string, string, string, string, string, error) {
	// IK' and CK' are each 128 bits (TS 33.501 Annex A.3); reject anything else rather than deriving keys from a bad HMAC key
	ik, err := decodeAkaPrimeKey("IK'", ikPrime)
	if err != nil {
		return "", "", "", "", "", err
	}
	ck, err := decodeAkaPrimeKey("CK'", ckPrime)
	if err != nil {
		return "", "", "", "", "", err
	}
	key := append(ik, ck...)
	sBase := []byte("EAP-AKA'" + identity)

	// MK = PRF'(IK'|CK', "EAP-AKA'"|Identity) with PRF' = T1 | T2 | ..., Tn = HMAC-SHA-256(K, Tn-1 | S | n) and
	// n a single octet counter starting at 0x01 (RFC 5448 section 3.4.1)
	MK := ""
	prev := []byte{}
	prfRounds := 208/32 + 1
	for i := 0; i < prfRounds; i++ {
		// Create a new HMAC by defining the hash type and the key (as byte array)
		h := hmac.New(sha256.New, key)

		s := append(append(append([]byte{}, prev...), sBase...), byte(i+1))

		// Write Data to it
		if _, err := h.Write(s); err != nil {
			logger.EapAuthComfirmLog.Errorln(err.Error())
		}

		sha := string(h.Sum(nil))
		MK += sha
		prev = []byte(sha)
	}

	K_encr := MK[0:16]  // 0..127
	K_aut := MK[16:48]  // 128..383
	K_re := MK[48:80]   // 384..639
	MSK := MK[80:144]   // 640..1151
	EMSK := MK[144:208] // 1152..1663
	return K_encr, K_aut, K_re, MSK, EMSK, nil
}

// decodeAkaPrimeKey hex decodes a 128 bit IK' or CK' value.
func decodeAkaPrimeKey(name, value string) ([]byte, error) {
	key, err := hex.DecodeString(value)
	if err != nil {
		return nil, fmt.Errorf("decode %s failed: %w", name, err)
	}
	if len(key) != 16 {
		return nil, fmt.Errorf("%s must be 16 bytes, got %d", name, len(key))
	}
	return key, nil
}

func checkMACintegrity(offset int, expectedMacValue []byte, packet []byte, Kautn string) bool {
	eapDecode, decodeErr := EapDecode(packet)
	if decodeErr != nil {
		logger.EapAuthComfirmLog.Infoln(decodeErr.Error())
		return false
	}
	if offset < 0 || offset+atMacAttributeSize > len(eapDecode.TypeData) {
		logger.EapAuthComfirmLog.Infoln("AT_MAC attribute exceeds EAP packet")
		return false
	}
	// zero the MAC value (bytes 4..20 of the attribute) before recomputing it
	clear(eapDecode.TypeData[offset+4 : offset+atMacAttributeSize])
	encodeAfter := eapDecode.Encode()
	MACvalue := CalculateAtMAC([]byte(Kautn), encodeAfter)

	return hmac.Equal(MACvalue, expectedMacValue)
}

// extractAtRes returns the RES carried by a whole AT_RES attribute. The attribute's length field counts 4-byte
// words and its RES Length field counts bits; the value is taken without the trailing alignment padding.
func extractAtRes(attribute []byte) ([]byte, bool) {
	if len(attribute) < atResHeaderSize {
		logger.EapAuthComfirmLog.Infoln("AT_RES attribute too short")
		return nil, false
	}
	resBits := int(binary.BigEndian.Uint16(attribute[2:4]))
	if resBits < minResBits || resBits > maxResBits || resBits%8 != 0 {
		logger.EapAuthComfirmLog.Infof("invalid AT_RES length of %d bits", resBits)
		return nil, false
	}
	resLen := resBits / 8
	if len(attribute) != atResHeaderSize+(resLen+3)/4*4 {
		logger.EapAuthComfirmLog.Infof("AT_RES attribute size %d does not match %d-bit RES", len(attribute), resBits)
		return nil, false
	}
	return bytes.Clone(attribute[atResHeaderSize : atResHeaderSize+resLen]), true
}

// resMatchesXres compares, in constant time, the RES received from the UE with the hex-encoded XRES from the UDM.
func resMatchesXres(xresHex string, res []byte) bool {
	xres, err := hex.DecodeString(xresHex)
	if err != nil || len(xres) == 0 {
		return false
	}
	return subtle.ConstantTimeCompare(xres, res) == 1
}

// func decodeResMac(packetData []byte, wholePacket []byte, Kautn string) (RES []byte, success bool) {
func decodeResMac(packetData []byte, wholePacket []byte, Kautn string) ([]byte, bool) {
	detectRes := false
	detectMac := false
	macCorrect := false
	if len(packetData) < eapAkaHeaderSize || packetData[0] != eapAkaSubtypeChallenge {
		logger.EapAuthComfirmLog.Infoln("EAP-AKA' packet is not an AKA-Challenge response")
		return nil, false
	}
	dataArray := packetData[eapAkaHeaderSize:]
	var attributeLength int
	var attributeType int
	var RES []byte

	for i := 0; i < len(dataArray); i += attributeLength {
		// every attribute has at least a type and a length byte; the length is in units of 4 bytes
		// and must be non-zero and within the packet (RFC 4187 section 8.1)
		if i+2 > len(dataArray) {
			logger.EapAuthComfirmLog.Infoln("truncated EAP attribute header")
			return nil, false
		}
		attributeLength = int(dataArray[1+i]) * 4
		attributeType = int(dataArray[0+i])
		if attributeLength == 0 || i+attributeLength > len(dataArray) {
			logger.EapAuthComfirmLog.Infof("invalid EAP attribute length %d for type %d", attributeLength, attributeType)
			return nil, false
		}

		switch attributeType {
		case ausf_context.AT_RES_ATTRIBUTE:
			logger.EapAuthComfirmLog.Infoln("Detect AT_RES attribute")
			if detectRes {
				logger.EapAuthComfirmLog.Infoln("duplicate AT_RES attribute")
				return nil, false
			}
			res, ok := extractAtRes(dataArray[i : i+attributeLength])
			if !ok {
				return nil, false
			}
			detectRes = true
			RES = res
		case ausf_context.AT_MAC_ATTRIBUTE:
			logger.EapAuthComfirmLog.Infoln("Detect AT_MAC attribute")
			if attributeLength != atMacAttributeSize {
				logger.EapAuthComfirmLog.Infof("invalid AT_MAC attribute length %d", attributeLength)
				return nil, false
			}
			if detectMac {
				logger.EapAuthComfirmLog.Infoln("duplicate AT_MAC attribute")
				return nil, false
			}
			detectMac = true
			macStr := string(dataArray[4+i : atMacAttributeSize+i])
			if checkMACintegrity(eapAkaHeaderSize+i, []byte(macStr), wholePacket, Kautn) {
				logger.EapAuthComfirmLog.Infoln("check MAC integrity succeed")
				macCorrect = true
			} else {
				logger.EapAuthComfirmLog.Infoln("check MAC integrity failed")
			}
		default:
			logger.EapAuthComfirmLog.Infof("Detect unknown attribute with type %d", attributeType)
		}
	}
	if detectRes && detectMac && macCorrect {
		return RES, true
	}
	return nil, false
}

func ConstructFailEapAkaNotification(oldPktId uint8) string {
	var eapPkt EapPacket
	eapPkt.Code = EapCodeForRequest
	eapPkt.Identifier = oldPktId + 1
	eapPkt.Type = EapTypeEapAkaPrime
	attrNum := fmt.Sprintf("%02x", ausf_context.AT_NOTIFICATION_ATTRIBUTE)
	attribute := fmt.Sprintf("%02x", eapAkaSubtypeNotification) + "0000" + attrNum + "01" + "4000"
	var attrHex []byte
	if attrHexTmp, err := hex.DecodeString(attribute); err != nil {
		logger.EapAuthComfirmLog.Warnf("Decode attribute failed: %+v", err)
	} else {
		attrHex = attrHexTmp
	}
	eapPkt.TypeData = attrHex
	eapPktEncode := eapPkt.Encode()
	return base64.StdEncoding.EncodeToString(eapPktEncode)
}

// ConstructEapNoTypePkt builds a Success/Failure EAP packet, which per RFC 3748 Section 4 has no Type byte.
func ConstructEapNoTypePkt(code EapCode, pktID uint8) string {
	eapPkt := EapPacket{Code: code, Identifier: pktID}
	return base64.StdEncoding.EncodeToString(eapPkt.Encode())
}

func GetUdmUrl(nrfUri string) string {
	self := ausf_context.GetSelf()
	udmUrlMu.RLock()
	cached := self.UdmUeauUrl
	udmUrlMu.RUnlock()
	if cached != "" {
		return cached
	}

	udmUrl := "https://localhost:29503" // default
	configureSearchUDMRequest := func(request Nnrf_NFDiscovery.ApiSearchNFInstancesRequest) Nnrf_NFDiscovery.ApiSearchNFInstancesRequest {
		return request.ServiceNames([]models.ServiceName{models.SERVICENAME_NUDM_UEAU})
	}
	res, err := consumer.SendSearchNFInstances(nrfUri, models.NFTYPE_UDM, models.NFTYPE_AUSF, configureSearchUDMRequest)
	if err != nil {
		logger.UeAuthPostLog.Errorln("[Search UDM UEAU] ", err.Error())
	}
	if res == nil || len(res.NfInstances) == 0 {
		directRes, directErr := consumer.SendNfDiscoveryToNrf(context.Background(), nrfUri, models.NFTYPE_UDM, models.NFTYPE_AUSF, configureSearchUDMRequest)
		if directErr != nil {
			logger.UeAuthPostLog.Errorln("[Direct Search UDM UEAU] ", directErr.Error())
		}
		if directRes != nil {
			res = directRes
		}
	}
	if res != nil && len(res.NfInstances) > 0 {
		for _, udmInstance := range res.NfInstances {
			for _, ueauService := range udmInstance.NfServices {
				if ueauService.GetServiceName() != models.SERVICENAME_NUDM_UEAU {
					continue
				}
				if apiPrefix, ok := ueauService.GetApiPrefixOk(); ok && apiPrefix != nil && *apiPrefix != "" {
					udmUrlMu.Lock()
					self.UdmUeauUrl = *apiPrefix
					udmUrlMu.Unlock()
					return *apiPrefix
				}
				for _, ueauEndPoint := range ueauService.IpEndPoints {
					if ueauEndPoint.GetIpv4Address() == "" || ueauEndPoint.GetPort() == 0 {
						continue
					}
					url := string(ueauService.GetScheme()) + "://" + ueauEndPoint.GetIpv4Address() + ":" + strconv.Itoa(int(ueauEndPoint.GetPort()))
					udmUrlMu.Lock()
					self.UdmUeauUrl = url
					udmUrlMu.Unlock()
					return url
				}
			}
		}
		logger.UeAuthPostLog.Errorln("[search UDM UEAU] no usable UDM service endpoints found")
	} else {
		logger.UeAuthPostLog.Errorln("[search UDM UEAU] len(NfInstances) = 0")
	}
	return udmUrl
}

// invalidateUdmCache clears URL + client caches so the next request triggers fresh NRF discovery.
func invalidateUdmCache() {
	udmUrlMu.Lock()
	ausf_context.GetSelf().UdmUeauUrl = ""
	udmUrlMu.Unlock()

	udmClientMu.Lock()
	cachedUdmClient = nil
	cachedUdmClientURL = ""
	udmClientMu.Unlock()
}

func createClientToUdmUeau(udmUrl string) *Nudm_UEAU.APIClient {
	udmClientMu.Lock()
	defer udmClientMu.Unlock()
	if cachedUdmClient != nil && cachedUdmClientURL == udmUrl {
		return cachedUdmClient
	}
	configuration := Nudm_UEAU.NewConfiguration()
	serverConfig := &configuration.Servers[0]
	if apiRootVar, exists := serverConfig.Variables["apiRoot"]; exists {
		apiRootVar.DefaultValue = udmUrl
		serverConfig.Variables["apiRoot"] = apiRootVar
	}
	cachedUdmClient = Nudm_UEAU.NewAPIClient(configuration)
	cachedUdmClientURL = udmUrl
	return cachedUdmClient
}

func sendAuthResultToUDM(id string, authType models.AuthType, success bool, servingNetworkName, udmUrl string) error {
	if servingNetworkName == "" {
		servingNetworkName = "5G:NSWO"
	}
	authEvent := models.NewAuthEvent(ausf_context.GetSelf().NfId, success, time.Now(), authType, servingNetworkName)

	client := createClientToUdmUeau(udmUrl)
	apiConfirmAuthRequest := client.ConfirmAuthAPI.ConfirmAuth(context.Background(), id)
	apiConfirmAuthRequest = apiConfirmAuthRequest.AuthEvent(*authEvent)
	_, resp, confirmAuthErr := client.ConfirmAuthAPI.ConfirmAuthExecute(apiConfirmAuthRequest)
	if resp != nil && resp.Body != nil {
		defer func() {
			if rspCloseErr := resp.Body.Close(); rspCloseErr != nil {
				logger.UeAuthPostLog.Errorf("ConfirmAuthAPI response body cannot close: %+v", rspCloseErr)
			}
		}()
	}
	return confirmAuthErr
}

func deleteAuthResultFromUDM(supi, authEventID string, authType models.AuthType, servingNetworkName, udmUrl string) error {
	if servingNetworkName == "" {
		servingNetworkName = "5G:NSWO"
	}
	authEvent := models.NewAuthEvent(ausf_context.GetSelf().NfId, false, time.Now(), authType, servingNetworkName)
	authEvent.SetAuthRemovalInd(true)

	client := createClientToUdmUeau(udmUrl)
	resp, deleteAuthErr := executeDeleteAuth(client, supi, authEventID, *authEvent)
	if resp != nil && resp.Body != nil {
		defer func() {
			if rspCloseErr := resp.Body.Close(); rspCloseErr != nil {
				logger.UeAuthPostLog.Errorf("DeleteAuthAPI response body cannot close: %+v", rspCloseErr)
			}
		}()
	}
	return deleteAuthErr
}

func logConfirmFailureAndInformUDM(id string, authType models.AuthType, servingNetworkName, errStr, udmUrl string) {
	switch authType {
	case models.AUTHTYPE__5_G_AKA:
		logger.Auth5gAkaComfirmLog.Infoln(errStr)
		if sendErr := sendAuthResultToUDM(id, authType, false, servingNetworkName, udmUrl); sendErr != nil {
			logger.Auth5gAkaComfirmLog.Infoln(sendErr.Error())
		}
	case models.AUTHTYPE_EAP_AKA_PRIME:
		logger.EapAuthComfirmLog.Infoln(errStr)
		if sendErr := sendAuthResultToUDM(id, authType, false, servingNetworkName, udmUrl); sendErr != nil {
			logger.EapAuthComfirmLog.Infoln(sendErr.Error())
		}
	}
}
