package main

import (
	"errors"
	"net"
	"testing"
	"time"

	"github.com/miekg/dns"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRcodeString(t *testing.T) {
	r, _ := newRCode("NOERROR")
	assert.Equal(t, "NOERROR", r.String())
}

func TestNewRcode(t *testing.T) {
	// Valid case
	r, err := newRCode("NOERROR")
	require.NoError(t, err)
	assert.Equal(t, NOERROR, r)
	// Invalid case
	r, err = newRCode("TestValue")
	require.Error(t, err)
	assert.Equal(t, OTHER, r)
}

type dnsClientTest struct {
	rcode  int
	doFail bool
}

func (d *dnsClientTest) query(_ *dns.Msg, _ string) (*dns.Msg, time.Duration, error) {
	var td time.Duration = 1000000000

	if d.doFail {
		return nil, td, errors.New("dummy error message")
	}

	rawResponse := new(dns.Msg)
	rawResponse.Rcode = d.rcode
	rawResponse.Answer = append(rawResponse.Answer, &dns.A{A: net.ParseIP("127.0.0.1"), Hdr: dns.RR_Header{Name: "thebeat.co"}})

	return rawResponse, td, nil
}

func TestQuery(t *testing.T) {
	t.Parallel() // marks TLog as capable of running in parallel with other tests
	rc := NOERROR
	dr := &dnsRequest{"thebeat.co", "A", nil, []string{}, &rc}
	s := newDNSStream(dr, 100)
	c := dnsClientTest{dns.RcodeSuccess, false}
	var expectedRTT time.Duration = 1000000000

	err := s.query(&c)

	require.NoError(t, err)
	assert.Equal(t, expectedRTT, s.rtt)
	assert.InDelta(t, 1, s.verificationStatus, 0.0001)
}

func TestQueryNoResponse(t *testing.T) {
	t.Parallel() // marks TLog as capable of running in parallel with other tests
	rc := NOERROR
	dr := &dnsRequest{"thebeat.co", "A", nil, []string{}, &rc}
	s := newDNSStream(dr, 100)
	c := dnsClientTest{dns.RcodeSuccess, true}

	err := s.query(&c)

	require.Error(t, err)
	// Make sure we exited function and we didn't update rtt
	assert.Equal(t, time.Duration(0), s.rtt)
}

func TestQueryValidationFails(t *testing.T) {
	t.Parallel() // marks TLog as capable of running in parallel with other tests
	rc := NOERROR
	dr := &dnsRequest{"thebeat.co", "A", nil, []string{}, &rc}
	s := newDNSStream(dr, 100)
	c := dnsClientTest{dns.RcodeNameError, false}
	var expectedRTT time.Duration = 1000000000

	err := s.query(&c)

	require.NoError(t, err)
	assert.Equal(t, expectedRTT, s.rtt)
	// We expect validationStatus to be 0 since rcode is not as expected
	assert.InDelta(t, 0, s.verificationStatus, 0.0001)
}

func TestConstructResolver(t *testing.T) {
	// Test case where user specifies custom resolver
	resolver := "1.2.3.4"
	dr := &dnsRequest{"thebeat.co", "A", &resolver, []string{}, nil}
	d := newDNSStream(dr, 100)
	res, err := d.constructResolver()
	require.NoError(t, err)
	expectedResolver := "1.2.3.4:53"
	assert.Equal(t, expectedResolver, res)

	// Test case where user doesn't specify resolver
	// We could do a bit more testing here and mock '/etc/resolv.conf' file
	// but for start this seemed okay
	dr = &dnsRequest{"thebeat.co", "A", nil, []string{}, nil}
	d = newDNSStream(dr, 100)
	_, err = d.constructResolver()
	require.NoError(t, err)
}

func TestConstructQuery(t *testing.T) {
	t.Parallel() // marks TLog as capable of running in parallel with other tests
	tests := []struct {
		testName          string
		testDomain        string
		testQtype         string
		testReturnedQtype uint16
	}{
		{"test A type", "thebeat.co", "A", dns.TypeA},
		{"test CNAME type", "thebeat.co", "CNAME", dns.TypeCNAME},
		{"test MX type", "thebeat.co", "MX", dns.TypeMX},
		{"test NS type", "thebeat.co", "NS", dns.TypeNS},
	}
	for _, tt := range tests {
		tt := tt // NOTE: https://github.com/golang/go/wiki/CommonMistakes#using-goroutines-on-loop-iterator-variables
		t.Run(tt.testName, func(t *testing.T) {
			t.Parallel() // marks each test case as capable of running in parallel with each other
			dr := &dnsRequest{tt.testDomain, tt.testQtype, nil, []string{}, nil}
			s := newDNSStream(dr, 100)
			dm := s.constructQuery()
			// we need recursion
			assert.True(t, dm.MsgHdr.RecursionDesired) //nolint:staticcheck
			// domain should fqdn
			assert.Equal(t, tt.testDomain+".", dm.Question[0].Name)
			// type should be A
			assert.Equal(t, tt.testReturnedQtype, dm.Question[0].Qtype)
		})
	}
}

func newTestDNSStream(domain, qtype, ip string, rcode int, expectedAnswers []string, expectedRcode *rCode) *dnsStream {
	dr := &dnsRequest{domain, qtype, nil, expectedAnswers, expectedRcode}
	s := newDNSStream(dr, 100)

	rawResponse := new(dns.Msg)
	rawResponse.Rcode = rcode
	switch qtype {
	case "A":
		rawResponse.Answer = append(rawResponse.Answer, &dns.A{A: net.ParseIP(ip), Hdr: dns.RR_Header{Name: domain}})
	case "AAAA":
		rawResponse.Answer = append(rawResponse.Answer, &dns.AAAA{AAAA: net.ParseIP(ip), Hdr: dns.RR_Header{Name: domain}})
	case "NS":
		rawResponse.Answer = append(rawResponse.Answer, &dns.NS{Ns: ip, Hdr: dns.RR_Header{Name: domain}})
	case "MX":
		rawResponse.Answer = append(rawResponse.Answer, &dns.MX{Mx: ip, Hdr: dns.RR_Header{Name: domain}})
	}
	s.response.rawResponse = rawResponse
	return s
}

func TestParseResponseSuccessful(t *testing.T) {
	t.Parallel() // marks TLog as capable of running in parallel with other tests
	tests := []struct {
		testName   string
		testDomain string
		testQtype  string
		testIP     string
	}{
		{"test A answer", "thebeat.co", "A", "127.0.0.1"},
		{"test AAAA answer", "thebeat.co", "AAAA", "::1"},
		{"test NS answer", "thebeat.co", "NS", "127.0.0.1"},
		{"test MX answer", "thebeat.co", "MX", "127.0.0.1"},
	}
	for _, tt := range tests {
		tt := tt // NOTE: https://github.com/golang/go/wiki/CommonMistakes#using-goroutines-on-loop-iterator-variables
		t.Run(tt.testName, func(t *testing.T) {
			t.Parallel() // marks each test case as capable of running in parallel with each other
			s := newTestDNSStream(tt.testDomain, tt.testQtype, tt.testIP, dns.RcodeSuccess, []string{}, nil)
			s.parseResponse()
			assert.Len(t, s.response.answers, 1)
			assert.Equal(t, tt.testIP, s.response.answers[0])
		})
	}
}

func TestParseResponseUnSuccessful(t *testing.T) {
	t.Parallel() // marks TLog as capable of running in parallel with other tests
	tests := []struct {
		testName          string
		testRCode         int
		testExpectedRCode rCode
	}{
		{"test NXDOMAIN answer", dns.RcodeNameError, NXDOMAIN},
		{"test SERVFAIL answer", dns.RcodeServerFailure, SERVFAIL},
		{"test Other type answer", dns.RcodeFormatError, OTHER},
	}
	for _, tt := range tests {
		tt := tt // NOTE: https://github.com/golang/go/wiki/CommonMistakes#using-goroutines-on-loop-iterator-variables
		t.Run(tt.testName, func(t *testing.T) {
			t.Parallel() // marks each test case as capable of running in parallel with each other
			s := newTestDNSStream("thebeat.co", "A", "127.0.0.1", tt.testRCode, []string{}, nil)
			s.parseResponse()
			assert.Empty(t, s.response.answers)
			assert.Equal(t, tt.testExpectedRCode, s.response.code)
		})
	}
}

func TestIsResponseLegit(t *testing.T) {
	t.Parallel() // marks TLog as capable of running in parallel with other tests
	tests := []struct {
		name            string
		domain          string
		qtype           string
		mockedAnswer    string
		rcode           int
		expectedRcode   rCode
		expectedAnswers []string
		testResponse    bool
	}{
		{"test no expected rcode & no expected answers", "thebeat.co", "A", "127.0.0.1", dns.RcodeSuccess, -1, []string{}, true},
		{"test expected rcode and different rcode & no expected answers", "thebeat.co", "A", "127.0.0.1", dns.RcodeSuccess, NXDOMAIN, []string{}, false},
		{"test expected rcode and different rcode & matching expected answers", "thebeat.co", "A", "127.0.0.1", dns.RcodeSuccess, NXDOMAIN, []string{"127.0.0.1"}, false},
		{"test expected rcode and same rcode & no expected answers", "thebeat.co", "A", "127.0.0.1", dns.RcodeSuccess, NOERROR, []string{}, true},
		{"test expected rcode and same rcode & matching expected answers", "thebeat.co", "A", "127.0.0.1", dns.RcodeSuccess, NOERROR, []string{"127.0.0.1"}, true},
		{"test no expected rcode & matching expected answers", "thebeat.co", "A", "127.0.0.1", dns.RcodeSuccess, -1, []string{"127.0.0.1"}, true},
		{"test no expected rcode & no matching expected answers", "thebeat.co", "A", "127.0.0.1", dns.RcodeSuccess, -1, []string{"128.0.0.1"}, false},
		{"test no expected rcode & no matching expected answers", "thebeat.co", "A", "127.0.0.1", dns.RcodeSuccess, -1, []string{"128.0.0.1", "127.0.0.1"}, false},
	}
	for _, tt := range tests {
		tt := tt // NOTE: https://github.com/golang/go/wiki/CommonMistakes#using-goroutines-on-loop-iterator-variables
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel() // marks each test case as capable of running in parallel with each other
			expRcode := &tt.expectedRcode

			if tt.expectedRcode == -1 {
				expRcode = nil
			}
			s := newTestDNSStream(tt.domain, tt.qtype, tt.mockedAnswer, tt.rcode, tt.expectedAnswers, expRcode)

			s.parseResponse()
			res := s.isResponseLegit()
			assert.Equal(t, tt.testResponse, res)
		})
	}
}

func TestAreEqual(t *testing.T) {
	t.Parallel() // marks TLog as capable of running in parallel with other tests
	tests := []struct {
		name         string
		a            []string
		b            []string
		testResponse bool
	}{
		{"test empty lists", []string{}, []string{}, true},
		{"test same elements", []string{"a", "b"}, []string{"a", "b"}, true},
		{"test same elements but our of order", []string{"b", "a"}, []string{"a", "b"}, true},
		{"test subset elements first", []string{"a", "b", "c"}, []string{"a", "b"}, false},
		{"test subset elements second", []string{"a", "b"}, []string{"a", "b", "c"}, false},
	}
	for _, tt := range tests {
		tt := tt // NOTE: https://github.com/golang/go/wiki/CommonMistakes#using-goroutines-on-loop-iterator-variables
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel() // marks each test case as capable of running in parallel with each other
			res := areEqual(tt.a, tt.b)
			assert.Equal(t, tt.testResponse, res)
		})
	}
}
