package main

import (
	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"
)

var (
	dnsVerificationStatus = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "dns_verifier_verification_status",
			Help: "Verification Status of a DNS request.",
		},
		[]string{"domain", "qtype"},
	)

	dnsRequestsCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "dns_verifier_stats_total",
			Help: "Statistics of requests made from DNS verifier",
		},
		[]string{"domain", "qtype"},
	)

	dnsRTTHistogram = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "dns_verifier_rtt_s",
			Help:    "Histogram of response times for DNS requests made from DNS verifier",
			Buckets: []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5},
		},
		[]string{"domain", "qtype"},
	)
)

// init initializes all our metrics, if we need to add
// a new one we have to add it here as well.
func init() {
	prometheus.MustRegister(dnsVerificationStatus)
	prometheus.MustRegister(dnsRequestsCounter)
	prometheus.MustRegister(dnsRTTHistogram)
	log.Info("Metrics setup - scrape /metrics")
}

// increaseRequestsCounter increases the counter metric for
// the specified domain and qtype
func increaseRequestsCounter(domain string, qtype string) {
	dnsRequestsCounter.WithLabelValues(domain, qtype).Inc()
}

// updateRTTHistogram updates rtt histogram metric for the
// given domain and qtype
func updateRTTHistogram(domain, qtype string, rtt float64) {
	dnsRTTHistogram.WithLabelValues(domain, qtype).Observe(rtt)
}

// updateGaugeVerificationStatus updates verification status metric
// for the given domain, qtype
func updateGaugeVerificationStatus(domain, qtype string, status float64) {
	dnsVerificationStatus.WithLabelValues(domain, qtype).Set(status)
}
