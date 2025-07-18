// Copyright 2019 free5GC.org
// SPDX-FileCopyrightText: 2024 Canonical Ltd.
// SPDX-License-Identifier: Apache-2.0
//

package context

import (
	"fmt"
	"regexp"
	"sync"
	"time"

	"github.com/omec-project/ausf/logger"
	"github.com/omec-project/openapi/models"
)

type AUSFContext struct {
	suciSupiMap              sync.Map
	UePool                   sync.Map
	NfStatusSubscriptions    sync.Map // map[NfInstanceID]models.NrfSubscriptionData.SubscriptionId
	snRegex                  *regexp.Regexp
	NfId                     string
	GroupID                  string
	RegisterIPv4             string
	BindingIPv4              string
	Url                      string
	NrfUri                   string
	UdmUeauUrl               string
	UriScheme                models.UriScheme
	Key                      string
	PEM                      string
	NfService                map[models.ServiceName]models.NfService
	SBIPort                  int
	EnableNrfCaching         bool
	NrfCacheEvictionInterval time.Duration
}

type AusfUeContext struct {
	Supi               string
	Kausf              string
	Kseaf              string
	ServingNetworkName string
	AuthStatus         models.AuthResult
	UdmUeauUrl         string

	// for 5G AKA
	XresStar string

	// for EAP-AKA'
	K_aut string
	XRES  string
	Rand  string
}

type SuciSupiMap struct {
	SupiOrSuci string
	Supi       string
}

const (
	EAP_AKA_PRIME_TYPENUM = 50
)

// Attribute Types for EAP-AKA'
const (
	AT_RAND_ATTRIBUTE         = 1
	AT_AUTN_ATTRIBUTE         = 2
	AT_RES_ATTRIBUTE          = 3
	AT_MAC_ATTRIBUTE          = 11
	AT_NOTIFICATION_ATTRIBUTE = 12
	AT_IDENTITY_ATTRIBUTE     = 14
	AT_KDF_INPUT_ATTRIBUTE    = 23
	AT_KDF_ATTRIBUTE          = 24
)

var ausfContext AUSFContext

func Init() {
	if snRegex, err := regexp.Compile("5G:mnc[0-9]{3}[.]mcc[0-9]{3}[.]3gppnetwork[.]org"); err != nil {
		logger.ContextLog.Warnf("SN compile error: %+v", err)
	} else {
		ausfContext.snRegex = snRegex
	}
	InitAusfContext(&ausfContext)
}

func NewAusfUeContext(identifier string) (ausfUeContext *AusfUeContext) {
	ausfUeContext = new(AusfUeContext)
	ausfUeContext.Supi = identifier // supi
	return ausfUeContext
}

func AddAusfUeContextToPool(ausfUeContext *AusfUeContext) {
	ausfContext.UePool.Store(ausfUeContext.Supi, ausfUeContext)
}

func CheckIfAusfUeContextExists(ref string) bool {
	_, ok := ausfContext.UePool.Load(ref)
	return ok
}

func GetAusfUeContext(ref string) *AusfUeContext {
	context, _ := ausfContext.UePool.Load(ref)
	ausfUeContext := context.(*AusfUeContext)
	return ausfUeContext
}

func (context *AUSFContext) GetIPv4Uri() string {
	return fmt.Sprintf("%s://%s:%d", context.UriScheme, context.RegisterIPv4, context.SBIPort)
}

func AddSuciSupiPairToMap(supiOrSuci string, supi string) {
	newPair := new(SuciSupiMap)
	newPair.SupiOrSuci = supiOrSuci
	newPair.Supi = supi
	ausfContext.suciSupiMap.Store(supiOrSuci, newPair)
}

func CheckIfSuciSupiPairExists(ref string) bool {
	_, ok := ausfContext.suciSupiMap.Load(ref)
	return ok
}

func GetSupiFromSuciSupiMap(ref string) (supi string) {
	val, _ := ausfContext.suciSupiMap.Load(ref)
	suciSupiMap := val.(*SuciSupiMap)
	supi = suciSupiMap.Supi
	return supi
}

func IsServingNetworkAuthorized(lookup string) bool {
	if ausfContext.snRegex.MatchString(lookup) {
		return true
	} else {
		return false
	}
}

func GetSelf() *AUSFContext {
	return &ausfContext
}

func (context *AUSFContext) GetSelfID() string {
	return context.NfId
}
