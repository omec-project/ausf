// SPDX-FileCopyrightText: 2021 Open Networking Foundation <info@opennetworking.org>
//
// SPDX-License-Identifier: Apache-2.0
// SPDX-License-Identifier: LicenseRef-ONF-Member-Only-1.0

//+build debug

package util

import (
	"github.com/free5gc/path_util"
)

var (
	AusfLogPath           = path_util.Free5gcPath("free5gc/ausfsslkey.log")
	AusfPemPath           = path_util.Free5gcPath("free5gc/support/TLS/ausf.pem")
	AusfKeyPath           = path_util.Free5gcPath("free5gc/support/TLS/ausf.key")
	DefaultAusfConfigPath = path_util.Free5gcPath("free5gc/config/ausfcfg.yaml")
)
