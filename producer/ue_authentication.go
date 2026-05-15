// Copyright (c) 2026 Intel Corporation
// Copyright 2019 free5GC.org
// SPDX-License-Identifier: Apache-2.0

package producer

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"math/big"
	"net/http"
	"strings"

	"github.com/bronze1man/radius"
	ausf_context "github.com/omec-project/ausf/context"
	"github.com/omec-project/ausf/logger"
	stats "github.com/omec-project/ausf/metrics"
	"github.com/omec-project/openapi/v2"
	"github.com/omec-project/openapi/v2/models"
	"github.com/omec-project/openapi/v2/utils"
	"github.com/omec-project/util/httpwrapper"
	"github.com/omec-project/util/ueauth"
)

const (
	UPSTREAM_SERVER_ERROR                = "UPSTREAM_SERVER_ERROR"
	USER_NOT_FOUND_ERROR                 = "USER_NOT_FOUND"
	SERVING_NETWORK_NOT_AUTHORIZED_ERROR = "SERVING_NETWORK_NOT_AUTHORIZED"
	AV_GENERATION_PROBLEM_ERROR          = "AV_GENERATION_PROBLEM"
)

// EAP constants
const (
	EAPCodeRequest  = 1
	EAPCodeResponse = 2
	EAPCodeSuccess  = 3
	EAPCodeFailure  = 4
)

// Simple EAP packet structure
type EAPPacket struct {
	Code       uint8
	Identifier uint8
	Length     uint16
	Type       uint8
	TypeData   []byte
	Contents   []byte
}

// Generates a random int between 0 and 255
func GenerateRandomNumber() (uint8, error) {
	maximum := big.NewInt(256)
	randomNumber, err := rand.Int(rand.Reader, maximum)
	if err != nil {
		return 0, err
	}
	return uint8(randomNumber.Int64()), nil
}

func HandleEapAuthComfirmRequest(request *httpwrapper.Request) *httpwrapper.Response {
	logger.EapAuthComfirmLog.Infoln("EapAuthConfirmRequest")

	updateEapSession := request.Body.(models.EapSession)
	eapSessionID := request.Params["authCtxId"]

	response, problemDetails := EapAuthComfirmRequestProcedure(updateEapSession, eapSessionID)

	if response != nil {
		return httpwrapper.NewResponse(http.StatusOK, nil, response)
	} else if problemDetails != nil {
		return httpwrapper.NewResponse(int(problemDetails.GetStatus()), nil, problemDetails)
	}
	problemDetails = utils.ProblemDetailsUnspecified()
	return httpwrapper.NewResponse(http.StatusForbidden, nil, problemDetails)
}

func HandleAuth5gAkaComfirmRequest(request *httpwrapper.Request) *httpwrapper.Response {
	logger.Auth5gAkaComfirmLog.Infoln("Auth5gAkaComfirmRequest")
	updateConfirmationData := request.Body.(models.ConfirmationData)
	ConfirmationDataResponseID := request.Params["authCtxId"]

	response, problemDetails := Auth5gAkaComfirmRequestProcedure(updateConfirmationData, ConfirmationDataResponseID)
	if response != nil {
		return httpwrapper.NewResponse(http.StatusOK, nil, response)
	} else if problemDetails != nil {
		return httpwrapper.NewResponse(int(problemDetails.GetStatus()), nil, problemDetails)
	}
	problemDetails = utils.ProblemDetailsUnspecified()
	return httpwrapper.NewResponse(http.StatusForbidden, nil, problemDetails)
}

func HandleUeAuthPostRequest(request *httpwrapper.Request) *httpwrapper.Response {
	logger.UeAuthPostLog.Infoln("HandleUeAuthPostRequest")
	updateAuthenticationInfo := request.Body.(models.AuthenticationInfo)

	response, locationURI, problemDetails := UeAuthPostRequestProcedure(updateAuthenticationInfo)
	respHeader := make(http.Header)
	respHeader.Set("Location", locationURI)

	if response != nil {
		stats.IncrementUeAuthStats(ausf_context.GetSelf().NfId, response.GetServingNetworkName(), string(response.GetAuthType()), "AUTHORIZED")
		return httpwrapper.NewResponse(http.StatusCreated, respHeader, response)
	} else if problemDetails != nil {
		stats.IncrementUeAuthStats(ausf_context.GetSelf().NfId, updateAuthenticationInfo.ServingNetworkName, "", problemDetails.GetCause())
		return httpwrapper.NewResponse(int(problemDetails.GetStatus()), nil, problemDetails)
	}
	problemDetails = utils.ProblemDetailsUnspecified()
	stats.IncrementUeAuthStats(ausf_context.GetSelf().NfId, updateAuthenticationInfo.ServingNetworkName, "", problemDetails.GetCause())
	return httpwrapper.NewResponse(http.StatusForbidden, nil, problemDetails)
}

func HandleDelete5gAkaAuthenticationResultRequest(request *httpwrapper.Request) *httpwrapper.Response {
	problemDetails := DeleteAuthenticationResultProcedure(request.Params["authCtxId"], models.AUTHTYPE__5_G_AKA)
	if problemDetails != nil {
		return httpwrapper.NewResponse(int(problemDetails.GetStatus()), nil, problemDetails)
	}
	return httpwrapper.NewResponse(http.StatusNoContent, nil, nil)
}

func HandleDeleteEapAuthenticationResultRequest(request *httpwrapper.Request) *httpwrapper.Response {
	problemDetails := DeleteAuthenticationResultProcedure(request.Params["authCtxId"], models.AUTHTYPE_EAP_AKA_PRIME)
	if problemDetails != nil {
		return httpwrapper.NewResponse(int(problemDetails.GetStatus()), nil, problemDetails)
	}
	return httpwrapper.NewResponse(http.StatusNoContent, nil, nil)
}

func HandleUeAuthenticationsDeregisterRequest(request *httpwrapper.Request) *httpwrapper.Response {
	deregistrationInfo := request.Body.(models.DeregistrationInfo)
	problemDetails := DeregisterAuthContextProcedure(deregistrationInfo)
	if problemDetails != nil {
		return httpwrapper.NewResponse(int(problemDetails.GetStatus()), nil, problemDetails)
	}
	return httpwrapper.NewResponse(http.StatusNoContent, nil, nil)
}

func deleteAuthContextLocally(authCtxID, supi string) {
	ausf_context.RemoveSuciSupiPairFromMap(authCtxID)
	if !ausf_context.HasSuciSupiPairForSupi(supi) {
		ausf_context.RemoveAusfUeContextFromPool(supi)
	}
}

func authTypeFromContext(ausfCurrentContext *ausf_context.AusfUeContext) models.AuthType {
	if ausfCurrentContext == nil {
		return models.AUTHTYPE__5_G_AKA
	}
	if ausfCurrentContext.XRES != "" || ausfCurrentContext.K_aut != "" {
		return models.AUTHTYPE_EAP_AKA_PRIME
	}
	return models.AUTHTYPE__5_G_AKA
}

func DeleteAuthenticationResultProcedure(authCtxID string, authType models.AuthType) *models.ProblemDetails {
	if !ausf_context.CheckIfSuciSupiPairExists(authCtxID) {
		problemDetails := models.NewProblemDetails()
		problemDetails.SetCause(USER_NOT_FOUND_ERROR)
		problemDetails.SetStatus(http.StatusNotFound)
		return problemDetails
	}

	currentSupi := ausf_context.GetSupiFromSuciSupiMap(authCtxID)
	if !ausf_context.CheckIfAusfUeContextExists(currentSupi) {
		problemDetails := models.NewProblemDetails()
		problemDetails.SetCause(USER_NOT_FOUND_ERROR)
		problemDetails.SetStatus(http.StatusNotFound)
		return problemDetails
	}

	ausfCurrentContext := ausf_context.GetAusfUeContext(currentSupi)
	if err := deleteAuthResultFromUDM(currentSupi, authCtxID, authType, ausfCurrentContext.ServingNetworkName,
		ausfCurrentContext.UdmUeauUrl); err != nil {
		problemDetails := models.NewProblemDetails()
		problemDetails.SetStatus(http.StatusInternalServerError)
		problemDetails.SetCause(UPSTREAM_SERVER_ERROR)
		return problemDetails
	}

	deleteAuthContextLocally(authCtxID, currentSupi)
	return nil
}

func DeregisterAuthContextProcedure(deregistrationInfo models.DeregistrationInfo) *models.ProblemDetails {
	supi := deregistrationInfo.GetSupi()
	authCtxIDs := ausf_context.ListSuciSupiPairsForSupi(supi)

	if len(authCtxIDs) == 0 {
		if ausf_context.CheckIfAusfUeContextExists(supi) {
			ausf_context.RemoveAusfUeContextFromPool(supi)
		}
		return nil
	}

	var ausfCurrentContext *ausf_context.AusfUeContext
	if ausf_context.CheckIfAusfUeContextExists(supi) {
		ausfCurrentContext = ausf_context.GetAusfUeContext(supi)
	}

	servingNetworkName := ""
	udmURL := resolveUdmURL(ausf_context.GetSelf().NrfUri)
	authType := authTypeFromContext(ausfCurrentContext)
	if ausfCurrentContext != nil {
		servingNetworkName = ausfCurrentContext.ServingNetworkName
		if ausfCurrentContext.UdmUeauUrl != "" {
			udmURL = ausfCurrentContext.UdmUeauUrl
		}
	}

	for _, authCtxID := range authCtxIDs {
		if err := deleteAuthResultFromUDM(supi, authCtxID, authType, servingNetworkName, udmURL); err != nil {
			problemDetails := models.NewProblemDetails()
			problemDetails.SetStatus(http.StatusInternalServerError)
			problemDetails.SetCause(UPSTREAM_SERVER_ERROR)
			return problemDetails
		}
	}

	for _, authCtxID := range authCtxIDs {
		ausf_context.RemoveSuciSupiPairFromMap(authCtxID)
	}
	ausf_context.RemoveAusfUeContextFromPool(supi)
	return nil
}

func UeAuthPostRequestProcedure(updateAuthenticationInfo models.AuthenticationInfo) (*models.UEAuthenticationCtx,
	string, *models.ProblemDetails,
) {
	responseBody := models.NewUEAuthenticationCtxWithDefaults()
	var authInfoReq models.AuthenticationInfoRequest

	supiOrSuci := updateAuthenticationInfo.SupiOrSuci

	snName := updateAuthenticationInfo.ServingNetworkName
	servingNetworkAuthorized := ausf_context.IsServingNetworkAuthorized(snName)
	if !servingNetworkAuthorized {
		problemDetails := models.NewProblemDetails()
		problemDetails.SetCause(SERVING_NETWORK_NOT_AUTHORIZED_ERROR)
		problemDetails.SetStatus(http.StatusForbidden)
		logger.UeAuthPostLog.Infoln("403 forbidden: serving network NOT AUTHORIZED")
		return nil, "", problemDetails
	}
	logger.UeAuthPostLog.Infoln("serving network authorized")

	responseBody.SetServingNetworkName(snName)
	authInfoReq.ServingNetworkName = snName
	self := ausf_context.GetSelf()
	authInfoReq.AusfInstanceId = self.GetSelfID()

	if updateAuthenticationInfo.ResynchronizationInfo != nil {
		logger.UeAuthPostLog.Warnln("Auts:", updateAuthenticationInfo.ResynchronizationInfo.Auts)
		ausfCurrentSupi := ausf_context.GetSupiFromSuciSupiMap(supiOrSuci)
		logger.UeAuthPostLog.Warnln(ausfCurrentSupi)
		ausfCurrentContext := ausf_context.GetAusfUeContext(ausfCurrentSupi)
		logger.UeAuthPostLog.Warnln(ausfCurrentContext.Rand)
		updateAuthenticationInfo.ResynchronizationInfo.Rand = ausfCurrentContext.Rand
		logger.UeAuthPostLog.Warnln("Rand:", updateAuthenticationInfo.ResynchronizationInfo.Rand)
		authInfoReq.ResynchronizationInfo = updateAuthenticationInfo.ResynchronizationInfo
	}

	udmUrl := resolveUdmURL(self.NrfUri)
	client := createClientToUdmUeau(udmUrl)
	authInfoResult, rsp, err := executeGenerateAuthData(client, supiOrSuci, authInfoReq)
	defer func() {
		if rsp == nil || rsp.Body == nil {
			return
		}
		if rspCloseErr := rsp.Body.Close(); rspCloseErr != nil {
			logger.UeAuthPostLog.Errorf("GenerateAuthDataApi response body cannot close: %+v", rspCloseErr)
		}
	}()
	if err != nil {
		logger.UeAuthPostLog.Infoln(err.Error())
		problemDetails := models.NewProblemDetails()
		if authInfoResult == nil || authInfoResult.AuthenticationVector == nil {
			problemDetails.SetCause(AV_GENERATION_PROBLEM_ERROR)
		} else {
			problemDetails.SetCause(UPSTREAM_SERVER_ERROR)
		}
		problemDetails.SetStatus(http.StatusInternalServerError)
		return nil, "", problemDetails
	}

	ueid := authInfoResult.GetSupi()
	ausfUeContext := ausf_context.NewAusfUeContext(ueid)
	ausfUeContext.ServingNetworkName = snName
	ausfUeContext.AuthStatus = models.AUTHRESULT_AUTHENTICATION_ONGOING
	ausfUeContext.UdmUeauUrl = udmUrl

	locationURI := self.Url + "/nausf-auth/v1/ue-authentications/" + supiOrSuci
	putLink := locationURI
	switch authInfoResult.AuthType {
	case models.AUTHTYPE__5_G_AKA:
		logger.UeAuthPostLog.Infoln("use 5G AKA auth method")
		putLink += "/5g-aka-confirmation"

		// Derive HXRES* from XRES*
		concat := authInfoResult.AuthenticationVector.Av5GHeAka.GetRand() + authInfoResult.AuthenticationVector.Av5GHeAka.GetXresStar()
		hxresStarBytes, err := hex.DecodeString(concat)
		if err != nil {
			logger.Auth5gAkaComfirmLog.Warnf("decode error: %+v", err)
			problemDetails := models.NewProblemDetails()
			problemDetails.SetStatus(http.StatusInternalServerError)
			problemDetails.SetCause(AV_GENERATION_PROBLEM_ERROR)
			problemDetails.SetDetail("Failed to derive HXRES*")
			return nil, "", problemDetails
		}
		hxresStarAll := sha256.Sum256(hxresStarBytes)
		hxresStar := hex.EncodeToString(hxresStarAll[16:]) // last 128 bits
		logger.Auth5gAkaComfirmLog.Infof("XresStar = %s", authInfoResult.AuthenticationVector.Av5GHeAka.GetXresStar())

		// Derive Kseaf from Kausf
		Kausf := authInfoResult.AuthenticationVector.Av5GHeAka.GetKausf()
		ausfDecode, err := hex.DecodeString(Kausf)
		if err != nil {
			logger.Auth5gAkaComfirmLog.Warnf("AUSF decode failed: %+v", err)
			problemDetails := models.NewProblemDetails()
			problemDetails.SetStatus(http.StatusInternalServerError)
			problemDetails.SetCause(AV_GENERATION_PROBLEM_ERROR)
			problemDetails.SetDetail("Failed to decode Kausf")
			return nil, "", problemDetails
		}
		P0 := []byte(snName)
		Kseaf, err := ueauth.GetKDFValue(ausfDecode, ueauth.FC_FOR_KSEAF_DERIVATION, P0, ueauth.KDFLen(P0))
		if err != nil {
			logger.Auth5gAkaComfirmLog.Error(err)
			problemDetails := models.NewProblemDetails()
			problemDetails.SetStatus(http.StatusInternalServerError)
			problemDetails.SetCause(AV_GENERATION_PROBLEM_ERROR)
			problemDetails.SetDetail("Failed to derive Kseaf")
			return nil, "", problemDetails
		}
		ausfUeContext.XresStar = authInfoResult.AuthenticationVector.Av5GHeAka.GetXresStar()
		ausfUeContext.Kausf = Kausf
		ausfUeContext.Kseaf = hex.EncodeToString(Kseaf)
		ausfUeContext.Rand = authInfoResult.AuthenticationVector.Av5GHeAka.GetRand()

		var av5gAka models.Av5gAka
		av5gAka.Rand = authInfoResult.AuthenticationVector.Av5GHeAka.GetRand()
		av5gAka.Autn = authInfoResult.AuthenticationVector.Av5GHeAka.GetAutn()
		av5gAka.HxresStar = hxresStar
		av5gAkaCtx := models.UEAuthenticationCtx5gAuthData{
			Av5gAka: &av5gAka,
		}

		responseBody.SetVar5gAuthData(av5gAkaCtx)
	case models.AUTHTYPE_EAP_AKA_PRIME:
		logger.UeAuthPostLog.Infoln("use EAP-AKA' auth method")
		putLink += "/eap-session"

		identity := ueid
		ikPrime := authInfoResult.AuthenticationVector.AvEapAkaPrime.GetIkPrime()
		ckPrime := authInfoResult.AuthenticationVector.AvEapAkaPrime.GetCkPrime()
		RAND := authInfoResult.AuthenticationVector.AvEapAkaPrime.GetRand()
		AUTN := authInfoResult.AuthenticationVector.AvEapAkaPrime.GetAutn()
		XRES := authInfoResult.AuthenticationVector.AvEapAkaPrime.GetXres()
		ausfUeContext.XRES = XRES

		ausfUeContext.Rand = authInfoResult.AuthenticationVector.AvEapAkaPrime.GetRand()

		K_encr, K_aut, K_re, MSK, EMSK := eapAkaPrimePrf(ikPrime, ckPrime, identity)
		_, _, _, _, _ = K_encr, K_aut, K_re, MSK, EMSK
		ausfUeContext.K_aut = K_aut
		Kausf := EMSK[0:32]
		ausfUeContext.Kausf = Kausf
		if ausfDecode, err := hex.DecodeString(Kausf); err != nil {
			logger.Auth5gAkaComfirmLog.Warnf("AUSF decode failed: %+v", err)
			problemDetails := models.NewProblemDetails()
			problemDetails.SetStatus(http.StatusInternalServerError)
			problemDetails.SetCause(AV_GENERATION_PROBLEM_ERROR)
			problemDetails.SetDetail("Failed to decode Kausf")
			return nil, "", problemDetails
		} else {
			P0 := []byte(snName)
			Kseaf, err := ueauth.GetKDFValue(ausfDecode, ueauth.FC_FOR_KSEAF_DERIVATION, P0, ueauth.KDFLen(P0))
			if err != nil {
				logger.Auth5gAkaComfirmLog.Error(err)
				problemDetails := models.NewProblemDetails()
				problemDetails.SetStatus(http.StatusInternalServerError)
				problemDetails.SetCause(AV_GENERATION_PROBLEM_ERROR)
				problemDetails.SetDetail("Failed to derive Kseaf")
				return nil, "", problemDetails
			}
			ausfUeContext.Kseaf = hex.EncodeToString(Kseaf)
		}

		var eapPkt radius.EapPacket
		randIdentifier, err := GenerateRandomNumber()
		if err != nil {
			logger.Auth5gAkaComfirmLog.Warnf("generate random number failed: %+v", err)
		}
		eapPkt.Identifier = randIdentifier
		eapPkt.Code = radius.EapCode(1)
		eapPkt.Type = radius.EapType(50) // according to RFC5448 6.1
		var atRand, atAutn, atKdf, atKdfInput, atMAC string
		if atRandTmp, err := EapEncodeAttribute("AT_RAND", RAND); err != nil {
			logger.Auth5gAkaComfirmLog.Warnf("EAP encode RAND failed: %+v", err)
		} else {
			atRand = atRandTmp
		}
		if atAutnTmp, err := EapEncodeAttribute("AT_AUTN", AUTN); err != nil {
			logger.Auth5gAkaComfirmLog.Warnf("EAP encode AUTN failed: %+v", err)
		} else {
			atAutn = atAutnTmp
		}
		if atKdfTmp, err := EapEncodeAttribute("AT_KDF", snName); err != nil {
			logger.Auth5gAkaComfirmLog.Warnf("EAP encode KDF failed: %+v", err)
		} else {
			atKdf = atKdfTmp
		}
		if atKdfInputTmp, err := EapEncodeAttribute("AT_KDF_INPUT", snName); err != nil {
			logger.Auth5gAkaComfirmLog.Warnf("EAP encode KDF failed: %+v", err)
		} else {
			atKdfInput = atKdfInputTmp
		}
		if atMACTmp, err := EapEncodeAttribute("AT_MAC", ""); err != nil {
			logger.Auth5gAkaComfirmLog.Warnf("EAP encode MAC failed: %+v", err)
		} else {
			atMAC = atMACTmp
		}

		dataArrayBeforeMAC := atRand + atAutn + atMAC + atKdf + atKdfInput
		eapPkt.Data = []byte(dataArrayBeforeMAC)
		encodedPktBeforeMAC := eapPkt.Encode()

		MACvalue := CalculateAtMAC([]byte(K_aut), encodedPktBeforeMAC)
		atMacNum := fmt.Sprintf("%02x", ausf_context.AT_MAC_ATTRIBUTE)
		var atMACfirstRow []byte
		if atMACfirstRowTmp, err := hex.DecodeString(atMacNum + "05" + "0000"); err != nil {
			logger.Auth5gAkaComfirmLog.Warnf("MAC decode failed: %+v", err)
		} else {
			atMACfirstRow = atMACfirstRowTmp
		}
		wholeAtMAC := append(atMACfirstRow, MACvalue...)

		atMAC = string(wholeAtMAC)
		dataArrayAfterMAC := atRand + atAutn + atMAC + atKdf + atKdfInput

		eapPkt.Data = []byte(dataArrayAfterMAC)
		encodedPktAfterMAC := eapPkt.Encode()
		uEAuthenticationCtx5gAuthData := models.UEAuthenticationCtx5gAuthData{
			String: openapi.PtrString(base64.StdEncoding.EncodeToString(encodedPktAfterMAC)),
		}
		responseBody.SetVar5gAuthData(uEAuthenticationCtx5gAuthData)
	default:
		problemDetails := models.NewProblemDetails()
		problemDetails.SetStatus(http.StatusInternalServerError)
		problemDetails.SetCause(UPSTREAM_SERVER_ERROR)
		problemDetails.SetDetail(fmt.Sprintf("unsupported auth type: %s", authInfoResult.AuthType))
		return nil, "", problemDetails
	}

	ausf_context.AddAusfUeContextToPool(ausfUeContext)
	logger.UeAuthPostLog.Infof("add SuciSupiPair (%s, %s) to map", supiOrSuci, ueid)
	ausf_context.AddSuciSupiPairToMap(supiOrSuci, ueid)

	putLinkPtr := models.NewLink()
	putLinkPtr.SetHref(putLink)
	linksValue := models.LinksValueSchema{Link: putLinkPtr}
	responseBody.Links = make(map[string]models.LinksValueSchema)
	responseBody.Links["link"] = linksValue
	responseBody.SetAuthType(authInfoResult.AuthType)

	return responseBody, locationURI, nil
}

// func Auth5gAkaComfirmRequestProcedure(updateConfirmationData models.ConfirmationData,
//	ConfirmationDataResponseID string) (response *models.ConfirmationDataResponse,
//  problemDetails *models.ProblemDetails) {

func Auth5gAkaComfirmRequestProcedure(updateConfirmationData models.ConfirmationData,
	ConfirmationDataResponseID string,
) (*models.ConfirmationDataResponse, *models.ProblemDetails) {
	responseBody := models.NewConfirmationDataResponse(models.AUTHRESULT_AUTHENTICATION_FAILURE)
	success := false
	responseBody.AuthResult = models.AUTHRESULT_AUTHENTICATION_FAILURE

	if !ausf_context.CheckIfSuciSupiPairExists(ConfirmationDataResponseID) {
		logger.Auth5gAkaComfirmLog.Infof("supiSuciPair does not exist, confirmation failed (queried by %s)",
			ConfirmationDataResponseID)
		problemDetails := models.NewProblemDetails()
		problemDetails.SetCause(USER_NOT_FOUND_ERROR)
		problemDetails.SetStatus(http.StatusNotFound)
		return nil, problemDetails
	}

	currentSupi := ausf_context.GetSupiFromSuciSupiMap(ConfirmationDataResponseID)
	if !ausf_context.CheckIfAusfUeContextExists(currentSupi) {
		logger.Auth5gAkaComfirmLog.Infof("SUPI does not exist, confirmation failed (queried by %s)", currentSupi)
		problemDetails := models.NewProblemDetails()
		problemDetails.SetCause(USER_NOT_FOUND_ERROR)
		problemDetails.SetStatus(http.StatusNotFound)
		return nil, problemDetails
	}

	ausfCurrentContext := ausf_context.GetAusfUeContext(currentSupi)
	servingNetworkName := ausfCurrentContext.ServingNetworkName

	// Compare the received RES* with the stored XRES*
	logger.Auth5gAkaComfirmLog.Infof("res*: %s, Xres*: %s", updateConfirmationData.GetResStar(), ausfCurrentContext.XresStar)
	if strings.Compare(updateConfirmationData.GetResStar(), ausfCurrentContext.XresStar) == 0 {
		ausfCurrentContext.AuthStatus = models.AUTHRESULT_AUTHENTICATION_SUCCESS
		responseBody.AuthResult = models.AUTHRESULT_AUTHENTICATION_SUCCESS
		success = true
		logger.Auth5gAkaComfirmLog.Infoln("5G AKA confirmation succeeded")
		responseBody.SetKseaf(ausfCurrentContext.Kseaf)
	} else {
		ausfCurrentContext.AuthStatus = models.AUTHRESULT_AUTHENTICATION_FAILURE
		responseBody.AuthResult = models.AUTHRESULT_AUTHENTICATION_FAILURE
		logConfirmFailureAndInformUDM(ConfirmationDataResponseID, models.AUTHTYPE__5_G_AKA, servingNetworkName,
			"5G AKA confirmation failed", ausfCurrentContext.UdmUeauUrl)
	}

	if sendErr := sendAuthResultToUDM(currentSupi, models.AUTHTYPE__5_G_AKA, success, servingNetworkName,
		ausfCurrentContext.UdmUeauUrl); sendErr != nil {
		logger.Auth5gAkaComfirmLog.Infoln(sendErr.Error())
		problemDetails := models.NewProblemDetails()
		problemDetails.SetStatus(http.StatusInternalServerError)
		problemDetails.SetCause(UPSTREAM_SERVER_ERROR)

		return nil, problemDetails
	}

	responseBody.SetSupi(currentSupi)
	return responseBody, nil
}

// return response, problemDetails
func EapAuthComfirmRequestProcedure(updateEapSession models.EapSession, eapSessionID string) (*models.EapSession,
	*models.ProblemDetails,
) {
	responseBody := models.NewEapSessionWithDefaults()

	if !ausf_context.CheckIfSuciSupiPairExists(eapSessionID) {
		logger.EapAuthComfirmLog.Infoln("supiSuciPair does not exist, confirmation failed")
		problemDetails := models.NewProblemDetails()
		problemDetails.SetStatus(http.StatusNotFound)
		problemDetails.SetCause(USER_NOT_FOUND_ERROR)
		return nil, problemDetails
	}

	currentSupi := ausf_context.GetSupiFromSuciSupiMap(eapSessionID)
	if !ausf_context.CheckIfAusfUeContextExists(currentSupi) {
		logger.EapAuthComfirmLog.Infoln("SUPI does not exist, confirmation failed")
		problemDetails := models.NewProblemDetails()
		problemDetails.SetStatus(http.StatusNotFound)
		problemDetails.SetCause(USER_NOT_FOUND_ERROR)
		return nil, problemDetails
	}

	ausfCurrentContext := ausf_context.GetAusfUeContext(currentSupi)
	servingNetworkName := ausfCurrentContext.ServingNetworkName
	var eapPayload []byte
	if eapPayloadTmp, err := base64.StdEncoding.DecodeString(updateEapSession.GetEapPayload()); err != nil {
		logger.EapAuthComfirmLog.Warnf("EAP payload decode failed: %+v", err)
	} else {
		eapPayload = eapPayloadTmp
	}

	eapContent, err := parseEAPPacket(eapPayload)
	if err != nil {
		logger.EapAuthComfirmLog.Warnf("EAP packet parsing failed: %+v", err)
		problemDetails := models.NewProblemDetails()
		problemDetails.SetStatus(http.StatusBadRequest)
		problemDetails.SetCause("EAP_PACKET_PARSE_ERROR")
		return nil, problemDetails
	}

	if eapContent.Code != EAPCodeResponse {
		logConfirmFailureAndInformUDM(eapSessionID, models.AUTHTYPE_EAP_AKA_PRIME, servingNetworkName,
			"eap packet code error", ausfCurrentContext.UdmUeauUrl)
		ausfCurrentContext.AuthStatus = models.AUTHRESULT_AUTHENTICATION_FAILURE
		responseBody.SetAuthResult(models.AUTHRESULT_AUTHENTICATION_ONGOING)
		failEapAkaNoti := ConstructFailEapAkaNotification(eapContent.Identifier)
		responseBody.SetEapPayload(failEapAkaNoti)
		return responseBody, nil
	}
	switch ausfCurrentContext.AuthStatus {
	case models.AUTHRESULT_AUTHENTICATION_ONGOING:
		responseBody.SetKSeaf(ausfCurrentContext.Kseaf)
		responseBody.SetSupi(currentSupi)
		Kautn := ausfCurrentContext.K_aut
		XRES := ausfCurrentContext.XRES
		RES, decodeOK := decodeResMac(eapContent.TypeData, eapContent.Contents, Kautn)
		if !decodeOK {
			ausfCurrentContext.AuthStatus = models.AUTHRESULT_AUTHENTICATION_FAILURE
			responseBody.SetAuthResult(models.AUTHRESULT_AUTHENTICATION_ONGOING)
			logConfirmFailureAndInformUDM(eapSessionID, models.AUTHTYPE_EAP_AKA_PRIME, servingNetworkName,
				"eap packet decode error", ausfCurrentContext.UdmUeauUrl)
			failEapAkaNoti := ConstructFailEapAkaNotification(eapContent.Identifier)
			responseBody.SetEapPayload(failEapAkaNoti)
		} else if XRES == string(RES) { // decodeOK && XRES == res, auth success
			logger.EapAuthComfirmLog.Infoln("correct RES value, EAP-AKA' auth succeed")
			responseBody.SetAuthResult(models.AUTHRESULT_AUTHENTICATION_SUCCESS)
			eapSuccPkt := ConstructEapNoTypePkt(radius.EapCodeSuccess, eapContent.Identifier)
			responseBody.SetEapPayload(eapSuccPkt)
			udmUrl := ausfCurrentContext.UdmUeauUrl
			if sendErr := sendAuthResultToUDM(eapSessionID, models.AUTHTYPE_EAP_AKA_PRIME, true, servingNetworkName,
				udmUrl); sendErr != nil {
				logger.EapAuthComfirmLog.Infoln(sendErr.Error())
				problemDetails := models.NewProblemDetails()
				problemDetails.SetStatus(http.StatusInternalServerError)
				problemDetails.SetCause(UPSTREAM_SERVER_ERROR)
				return nil, problemDetails
			}
			ausfCurrentContext.AuthStatus = models.AUTHRESULT_AUTHENTICATION_SUCCESS
		} else {
			ausfCurrentContext.AuthStatus = models.AUTHRESULT_AUTHENTICATION_FAILURE
			responseBody.SetAuthResult(models.AUTHRESULT_AUTHENTICATION_ONGOING)
			logConfirmFailureAndInformUDM(eapSessionID, models.AUTHTYPE_EAP_AKA_PRIME, servingNetworkName,
				"Wrong RES value, EAP-AKA' auth failed", ausfCurrentContext.UdmUeauUrl)
			failEapAkaNoti := ConstructFailEapAkaNotification(eapContent.Identifier)
			responseBody.SetEapPayload(failEapAkaNoti)
		}

	case models.AUTHRESULT_AUTHENTICATION_FAILURE:
		eapFailPkt := ConstructEapNoTypePkt(radius.EapCodeFailure, eapPayload[1])
		responseBody.SetEapPayload(eapFailPkt)
		responseBody.SetAuthResult(models.AUTHRESULT_AUTHENTICATION_FAILURE)
	}

	return responseBody, nil
}

// parseEAPPacket parses raw EAP payload into EAPPacket struct
func parseEAPPacket(payload []byte) (*EAPPacket, error) {
	if len(payload) < 4 {
		return nil, fmt.Errorf("EAP packet too short: %d bytes", len(payload))
	}

	packet := &EAPPacket{
		Code:       payload[0],
		Identifier: payload[1],
		Length:     uint16(payload[2])<<8 | uint16(payload[3]),
		Contents:   payload,
	}

	// Validate that the Length field matches the actual payload length
	if int(packet.Length) > len(payload) {
		return nil, fmt.Errorf("EAP packet Length field (%d) exceeds actual payload length (%d)",
			packet.Length, len(payload))
	}

	// Additional validation: Length should be at least 4 (header size)
	if packet.Length < 4 {
		return nil, fmt.Errorf("EAP packet Length field (%d) is less than minimum header size (4)",
			packet.Length)
	}

	// For Request and Response packets, extract Type and TypeData
	if (packet.Code == EAPCodeRequest || packet.Code == EAPCodeResponse) && len(payload) > 4 {
		packet.Type = payload[4]
		if len(payload) > 5 {
			packet.TypeData = payload[5:]
		}
	}

	return packet, nil
}
