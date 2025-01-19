package main

import (
	"fmt"
	"time"

	"github.com/miekg/dns"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

const (
	// DefaultTimeout is default timeout for the DNS requests
	DefaultTimeout time.Duration = 5 * time.Second
)

// rCode is a new int type to encapsulate the different response code
// in a more UX friendly way.
type rCode int

// rCode constants to represent response code in more
// UX friendly way
const (
	NOERROR rCode = iota
	NXDOMAIN
	SERVFAIL
	OTHER
)

// String pretty formats the rCode type when we
// want to print it.
func (r rCode) String() string {
	rCodes := [...]string{"NOERROR", "NXDOMAIN", "SERVFAIL", "OTHER"}
	if int(r) < 0 || int(r) > len(rCodes) {
		return "OTHER"
	}
	return rCodes[r]
}

// newRCode will return a new rCode instance based on the
// given string representation.
func newRCode(rc string) (rCode, error) {
	switch rc {
	case "NOERROR":
		return NOERROR, nil
	case "SERVFAIL":
		return SERVFAIL, nil
	case "NXDOMAIN":
		return NXDOMAIN, nil
	case "OTHER":
		return OTHER, nil
	}

	return OTHER, fmt.Errorf("%s is not a supported response code", rc)
}

// dnsResponse holds all the information that
// have to do with a DNS response
type dnsResponse struct {
	rawResponse *dns.Msg
	code        rCode
	answers     []string
}

// dnsRequest holds all infromation that have to do
// with a DNS request and its expected answers
type dnsRequest struct {
	domain               string
	queryType            string
	resolver             *string
	expectedResponse     []string
	expectedResponseCode *rCode
}

// dnsStream encapsulates the info related with monitoring DNS
// request and its response. Hence the stream in the name although
// UDP is stream-less.
type dnsStream struct {
	request            dnsRequest
	response           dnsResponse
	interval           int
	rtt                time.Duration
	verificationStatus float64
}

// newDNSStream constructs a new dnsStream struct
func newDNSStream(r *dnsRequest, interval int) *dnsStream {
	return &dnsStream{request: *r, rtt: 0, verificationStatus: 0, interval: interval}
}

type dnsClientInterface interface {
	query(*dns.Msg, string) (*dns.Msg, time.Duration, error)
}

type dnsClient struct {
	client *dns.Client
}

func newDNSClient() *dnsClient {
	c := &dns.Client{Net: "udp", ReadTimeout: DefaultTimeout}
	return &dnsClient{client: c}
}

func (d *dnsClient) query(query *dns.Msg, resolver string) (*dns.Msg, time.Duration, error) {
	return d.client.Exchange(query, resolver)
}

// query holds the high level logic of constructing requery, executing it
// and parsing and verifying its results. This is the fuction that
// watchdog worker will call to monitor a specific domain.
func (d *dnsStream) query(dnsClient dnsClientInterface) error {
	server, err := d.constructResolver()
	if err != nil {
		return errors.Wrapf(err, "Cannot proceed with query to: %s", d.request.domain)
	}

	query := d.constructQuery()
	response, rtt, err := dnsClient.query(query, server)
	if err != nil {
		return errors.Wrapf(err, "DNS request for: %s failed", d.request.domain)
	}

	d.rtt = rtt
	d.response.rawResponse = response
	d.parseResponse()

	verification := d.isResponseLegit()
	if verification {
		d.verificationStatus = 1
	} else {
		d.verificationStatus = 0
	}

	return nil
}

// constructResolver returns the resolver our DNS query will contact
// to make the request. If user hasn't specified a custom one we fall to the
// first one that is in the resolv.conf of the system.
func (d *dnsStream) constructResolver() (string, error) {
	if d.request.resolver != nil {
		return fmt.Sprintf("%s:53", *d.request.resolver), nil
	}

	conf, err := dns.ClientConfigFromFile("/etc/resolv.conf")
	if err != nil || conf == nil {
		return "", errors.Wrap(err, "Cannot initialize the local resolver")
	}

	return fmt.Sprintf("%s:%s", conf.Servers[0], conf.Port), nil
}

// constructQuery creates and fills dns.Msg struture with our
// domain, query type and resolver. After we fill that info we return the structure
// that can be used to send the actual packet with our query.
func (d *dnsStream) constructQuery() *dns.Msg {
	query := &dns.Msg{
		MsgHdr: dns.MsgHdr{
			RecursionDesired: true,
		},
		Question: make([]dns.Question, 1),
	}
	var qtype uint16
	switch d.request.queryType {
	case "A":
		qtype = dns.TypeA
	case "CNAME":
		qtype = dns.TypeCNAME
	case "MX":
		qtype = dns.TypeMX
	case "NS":
		qtype = dns.TypeNS
	}
	query.SetQuestion(dns.Fqdn(d.request.domain), qtype)
	return query
}

// parseResponse holds the logic of parsing a DNS response and
// storing different answers based on type and also the response
// code.
func (d *dnsStream) parseResponse() {
	switch d.response.rawResponse.Rcode {
	case dns.RcodeSuccess:
		d.response.code = NOERROR
	case dns.RcodeNameError:
		d.response.code = NXDOMAIN
	case dns.RcodeServerFailure:
		d.response.code = SERVFAIL
	default:
		d.response.code = OTHER
	}

	// If we have an error then there will be no answers, so exit.
	if d.response.code != NOERROR {
		return
	}

	var answers []string

	for _, answer := range d.response.rawResponse.Answer {
		switch t := answer.(type) {
		case *dns.A:
			answers = append(answers, t.A.String())
		case *dns.AAAA:
			answers = append(answers, t.AAAA.String())
		case *dns.NS:
			answers = append(answers, t.Ns)
		case *dns.MX:
			answers = append(answers, t.Mx)
		}
	}

	d.response.answers = answers
}

// isResponseLegit implements the logic of checking if DNS response
// is what user has set to be expected in terms of answers and response
// code.
func (d *dnsStream) isResponseLegit() bool {
	// If we have expectations for RC check it against the expected one
	if d.request.expectedResponseCode != nil {
		if *d.request.expectedResponseCode != d.response.code {
			log.Infof("Expected respond code:<%s> for quering domain:<%s> and DNS query type:<%s> is not the same as the response code:<%s>",
				d.request.expectedResponseCode, d.request.domain, d.request.queryType, d.response.code)
			return false
		}
	}

	// If there are expectations for answers as well check list the two lists (expected/responded)
	if len(d.request.expectedResponse) > 0 {
		if !areEqual(d.request.expectedResponse, d.response.answers) {
			log.Infof("Expected answers:%v for quering domain:<%s> and DNS query type:<%s> are not the same as the response answers:<%v>",
				d.request.expectedResponse, d.request.domain, d.request.queryType, d.response.answers)
			return false
		}
	}

	return true
}

// areEqual is a helper function that provides us a way to
// compare if two lists have the same elements regardless their order
func areEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}

	for _, i := range a {
		findEqual := false
		for _, j := range b {
			if i == j {
				findEqual = true
				break
			}
		}
		if !findEqual {
			return false
		}
	}
	return true
}

// updateStats updates the prometheus stats for specific dnsStream struct
// after we have gotten the DNS response
func (d *dnsStream) updateStats() {
	increaseRequestsCounter(d.request.domain, d.request.queryType)
	updateRTTHistogram(d.request.domain, d.request.queryType, d.rtt.Seconds())
	updateGaugeVerificationStatus(d.request.domain, d.request.queryType, d.verificationStatus)
	log.Debugf("Updated prometheus stats for domain:<%s> and querytype:<%s>", d.request.domain, d.request.queryType)
}
