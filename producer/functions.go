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
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/bronze1man/radius"
	"github.com/omec-project/ausf/consumer"
	ausf_context "github.com/omec-project/ausf/context"
	"github.com/omec-project/ausf/logger"
	"github.com/omec-project/openapi/v2/Nnrf_NFDiscovery"
	"github.com/omec-project/openapi/v2/Nudm_UEAU"
	"github.com/omec-project/openapi/v2/models"
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

func intToByteArray(i int) []byte {
	r := make([]byte, 2)
	binary.BigEndian.PutUint16(r, uint16(i))
	return r
}

func padZeros(byteArray []byte, size int) []byte {
	l := len(byteArray)
	if l == size {
		return byteArray
	}
	r := make([]byte, size)
	copy(r[size-l:], byteArray)
	return r
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
			return "", fmt.Errorf("[eapEncodeAttribute] AT_RAND Length Error")
		}
		attrNum := fmt.Sprintf("%02x", ausf_context.AT_RAND_ATTRIBUTE)
		attribute = attrNum + "05" + "0000" + data

	case "AT_AUTN":
		length = len(data)/8 + 1
		if length != 5 {
			return "", fmt.Errorf("[eapEncodeAttribute] AT_AUTN Length Error")
		}
		attrNum := fmt.Sprintf("%02x", ausf_context.AT_AUTN_ATTRIBUTE)
		attribute = attrNum + "05" + "0000" + data

	case "AT_KDF_INPUT":
		var byteName []byte
		nLength := len(data)
		length := (nLength+3)/4 + 1
		b := make([]byte, length*4)
		byteNameLength := intToByteArray(nLength)
		byteName = []byte(data)
		pad := padZeros(byteName, (length-1)*4)
		b[0] = 23
		b[1] = byte(length)
		copy(b[2:4], byteNameLength)
		copy(b[4:], pad)
		return string(b[:]), nil

	case "AT_KDF":
		// Value 1 default key derivation function for EAP-AKA'
		attrNum := fmt.Sprintf("%02x", ausf_context.AT_KDF_ATTRIBUTE)
		attribute = attrNum + "01" + "0001"

	case "AT_MAC":
		// Pad MAC value with 16 bytes of 0 since this is just for the calculation of MAC
		attrNum := fmt.Sprintf("%02x", ausf_context.AT_MAC_ATTRIBUTE)
		attribute = attrNum + "05" + "0000" + "00000000000000000000000000000000"

	case "AT_RES":
		var byteName []byte
		nLength := len(data)
		length := (nLength+3)/4 + 1
		b := make([]byte, length*4)
		byteNameLength := intToByteArray(nLength)
		byteName = []byte(data)
		pad := padZeros(byteName, (length-1)*4)
		b[0] = 3
		b[1] = byte(length)
		copy(b[2:4], byteNameLength)
		copy(b[4:], pad)
		return string(b[:]), nil

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

// func eapAkaPrimePrf(ikPrime string, ckPrime string, identity string) (K_encr string, K_aut string, K_re string,
//
//	MSK string, EMSK string) {
func eapAkaPrimePrf(ikPrime string, ckPrime string, identity string) (string, string, string, string, string) {
	keyAp := ikPrime + ckPrime

	var key []byte
	if keyTmp, err := hex.DecodeString(keyAp); err != nil {
		logger.EapAuthComfirmLog.Warnf("Decode key AP failed: %+v", err)
	} else {
		key = keyTmp
	}
	sBase := []byte("EAP-AKA'" + identity)

	MK := ""
	prev := []byte("")
	//_ = prev
	prfRounds := 208/32 + 1
	for i := 0; i < prfRounds; i++ {
		// Create a new HMAC by defining the hash type and the key (as byte array)
		h := hmac.New(sha256.New, key)

		hexNum := fmt.Sprint(i + 1)
		ap := append(sBase, hexNum...)
		s := append(prev, ap...)

		// Write Data to it
		if _, err := h.Write(s); err != nil {
			logger.EapAuthComfirmLog.Errorln(err.Error())
		}

		// Get result and encode as hexadecimal string
		sha := string(h.Sum(nil))
		MK += sha
		prev = []byte(sha)
	}

	K_encr := MK[0:16]  // 0..127
	K_aut := MK[16:48]  // 128..383
	K_re := MK[48:80]   // 384..639
	MSK := MK[80:144]   // 640..1151
	EMSK := MK[144:208] // 1152..1663
	return K_encr, K_aut, K_re, MSK, EMSK
}

func checkMACintegrity(offset int, expectedMacValue []byte, packet []byte, Kautn string) bool {
	eapDecode, decodeErr := radius.EapDecode(packet)
	if decodeErr != nil {
		logger.EapAuthComfirmLog.Infoln(decodeErr.Error())
	}
	if zeroBytes, err := hex.DecodeString("00000000000000000000000000000000"); err != nil {
		logger.EapAuthComfirmLog.Warnf("Decode error: %+v", err)
	} else {
		copy(eapDecode.Data[offset+4:offset+20], zeroBytes)
	}
	encodeAfter := eapDecode.Encode()
	MACvalue := CalculateAtMAC([]byte(Kautn), encodeAfter)

	if bytes.Equal(MACvalue, expectedMacValue) {
		return true
	} else {
		return false
	}
}

// func decodeResMac(packetData []byte, wholePacket []byte, Kautn string) (RES []byte, success bool) {
func decodeResMac(packetData []byte, wholePacket []byte, Kautn string) ([]byte, bool) {
	detectRes := false
	detectMac := false
	macCorrect := false
	dataArray := packetData
	var attributeLength int
	var attributeType int
	var RES []byte

	for i := 0; i < len(dataArray); i += attributeLength {
		attributeLength = int(uint(dataArray[1+i])) * 4
		attributeType = int(uint(dataArray[0+i]))

		switch attributeType {
		case ausf_context.AT_RES_ATTRIBUTE:
			logger.EapAuthComfirmLog.Infoln("Detect AT_RES attribute")
			detectRes = true
			resLength := int(uint(dataArray[3+i]) | uint(dataArray[2+i])<<8)
			RES = dataArray[4+i : 4+i+attributeLength-4]
			byteRes := padZeros(RES, resLength)
			RES = byteRes
		case ausf_context.AT_MAC_ATTRIBUTE:
			logger.EapAuthComfirmLog.Infoln("Detect AT_MAC attribute")
			detectMac = true
			macStr := string(dataArray[4+i : 20+i])
			if checkMACintegrity(i, []byte(macStr), wholePacket, Kautn) {
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
	var eapPkt radius.EapPacket
	eapPkt.Code = radius.EapCodeRequest
	eapPkt.Identifier = oldPktId + 1
	eapPkt.Type = ausf_context.EAP_AKA_PRIME_TYPENUM
	attrNum := fmt.Sprintf("%02x", ausf_context.AT_NOTIFICATION_ATTRIBUTE)
	attribute := attrNum + "01" + "4000"
	var attrHex []byte
	if attrHexTmp, err := hex.DecodeString(attribute); err != nil {
		logger.EapAuthComfirmLog.Warnf("Decode attribute failed: %+v", err)
	} else {
		attrHex = attrHexTmp
	}
	eapPkt.Data = attrHex
	eapPktEncode := eapPkt.Encode()
	return base64.StdEncoding.EncodeToString(eapPktEncode)
}

func ConstructEapNoTypePkt(code radius.EapCode, pktID uint8) string {
	b := make([]byte, 4)
	b[0] = byte(code)
	b[1] = pktID
	binary.BigEndian.PutUint16(b[2:4], uint16(4))
	return base64.StdEncoding.EncodeToString(b)
}

func GetUdmUrl(nrfUri string) string {
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
					return *apiPrefix
				}
				for _, ueauEndPoint := range ueauService.IpEndPoints {
					if ueauEndPoint.GetIpv4Address() == "" || ueauEndPoint.GetPort() == 0 {
						continue
					}
					return string(ueauService.GetScheme()) + "://" + ueauEndPoint.GetIpv4Address() + ":" + strconv.Itoa(int(ueauEndPoint.GetPort()))
				}
			}
		}
		logger.UeAuthPostLog.Errorln("[search UDM UEAU] no usable UDM service endpoints found")
	} else {
		logger.UeAuthPostLog.Errorln("[search UDM UEAU] len(NfInstances) = 0")
	}
	return udmUrl
}

func createClientToUdmUeau(udmUrl string) *Nudm_UEAU.APIClient {
	configuration := Nudm_UEAU.NewConfiguration()
	serverConfig := &configuration.Servers[0]
	if apiRootVar, exists := serverConfig.Variables["apiRoot"]; exists {
		apiRootVar.DefaultValue = udmUrl
		serverConfig.Variables["apiRoot"] = apiRootVar
	}
	clientAPI := Nudm_UEAU.NewAPIClient(configuration)
	return clientAPI
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
