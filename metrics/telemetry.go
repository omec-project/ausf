// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2024 Canonical Ltd.

/*
 *  Metrics package is used to expose the metrics of the AUSF service.
 */

package metrics

import (
	"net/http"

	"github.com/omec-project/ausf/logger"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// AusfStats captures AUSF stats
type AusfStats struct {
	ueAuths *prometheus.CounterVec
}

var ausfStats *AusfStats

func initAusfStats() *AusfStats {
	return &AusfStats{
		ueAuths: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "ausf_ue_authentications_total",
			Help: "Counter of total UE Authentications",
		}, []string{"ausf_id", "serving_network_name", "auth_type", "result"}),
	}
}

func (ps *AusfStats) register() error {
	if err := prometheus.Register(ps.ueAuths); err != nil {
		return err
	}
	return nil
}

func init() {
	ausfStats = initAusfStats()

	if err := ausfStats.register(); err != nil {
		logger.InitLog.Panicln("AUSF Stats register failed")
	}
}

// InitMetrics initialises AUSF metrics
func InitMetrics() {
	http.Handle("/metrics", promhttp.Handler())
	if err := http.ListenAndServe(":8080", nil); err != nil {
		logger.InitLog.Errorf("Could not open metrics port: %v", err)
	}
}

// IncrementUeAuthStats increments number of total UE authentications
func IncrementUeAuthStats(ausfID, servingNetworkName, authType, result string) {
	ausfStats.ueAuths.WithLabelValues(ausfID, servingNetworkName, authType, result).Inc()
}
