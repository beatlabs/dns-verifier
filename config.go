// Config contains logic that is related with our application's configutation.
// Configuration can come from environmental variables or yaml config file.
package main

import (
	"strconv"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

// YamlRequests encapsulates yaml objects that represent the
// array that holds the requests with the monitoring domains.
type YamlRequests struct {
	Requests []YamlRequest `yaml:"requests"`
}

// getCleanRequests holds the logic that gets the requests from the
// yaml config, and verify for each one if they are valid.
// At the end it returns a list of dnsStream structures that can be used
// further.
func (r *YamlRequests) getCleanRequests() ([]*dnsStream, error) {
	if len(r.Requests) == 0 {
		return []*dnsStream{}, errors.Errorf("Yaml configuration seems empty or malformed, cannot proceed with no valid requests")
	}
	cleanRequests := []*dnsStream{}
	for _, req := range r.Requests {
		c, err := req.getCleanRequest()
		if err != nil {
			log.Error(err.Error())
			continue
		}
		cleanRequests = append(cleanRequests, c)
	}
	if len(cleanRequests) == 0 {
		return []*dnsStream{}, errors.Errorf("No valid requests found inside the request sections coming from yaml config")
	}

	return cleanRequests, nil
}

// YamlRequest encapsulates yaml objects that represent single
// requests for a domain that we want to monitor.
type YamlRequest struct {
	Domain               string   `yaml:"domain"`
	QueryType            string   `yaml:"queryType"`
	Resolver             *string  `yaml:"resolver"`
	ExpectedResponse     []string `yaml:"expectedRespone"`
	ExpectedResponseCode *string  `yaml:"expectedResponseCode"`
	Interval             *int     `yaml:"interval"`
}

// getCleanRequest holds the logic of cleaning a request for a domain
// coming from the yaml config and returns a dnsStream structure that
// can be used further in our code.
func (r *YamlRequest) getCleanRequest() (*dnsStream, error) {
	dr := &dnsRequest{r.Domain, r.QueryType, r.Resolver, r.ExpectedResponse, nil}

	if dr.queryType == "" {
		dr.queryType = "A"
	}

	if dr.domain == "" {
		return nil, errors.New("domain needs to be a valid domain and not empty string")
	}

	if r.ExpectedResponseCode != nil {
		rCode, err := newRCode(*r.ExpectedResponseCode)
		if err != nil {
			return nil, err
		}
		dr.expectedResponseCode = &rCode
	}
	interval := 360 // Default interval loop at 5min
	if r.Interval != nil {
		interval = *r.Interval
	}

	return newDNSStream(dr, interval), nil
}

type config struct {
	appPort          int
	logLevel         string
	watchdogRequests []*dnsStream
}

func newConfig() (*config, error) {
	initViper()
	r, err := getYamlConfig()
	if err != nil {
		return nil, errors.Wrap(err, "Couldn't get a yaml config")
	}
	cleanDNSRequests, err := r.getCleanRequests()
	if err != nil {
		return nil, errors.Wrap(err, "Couldn't get a valid yaml config")
	}

	port := viper.GetString("app_port")
	intPort, ok := strconv.Atoi(port)
	if ok != nil {
		return nil, errors.New("Couldn't get a valid integer for the DNS_VERIFIER_PORT configuration variable")
	}

	return &config{
		appPort:          intPort,
		logLevel:         viper.GetString("log_level"),
		watchdogRequests: cleanDNSRequests,
	}, nil
}

// initViper initializes all viper configuration that we need.
func initViper() {
	// Set global options
	viper.AddConfigPath(".")
	viper.AddConfigPath("/etc/dns-verifier")
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.SetEnvPrefix("dns_verifier")

	// Set default for our existing env variables
	viper.SetDefault("APP_PORT", "3333")
	viper.SetDefault("LOG_LEVEL", "DEBUG")
	viper.SetDefault("INTERVAL", 30)

	// Enable VIPER to read Environment Variables
	viper.AutomaticEnv()
}

// getYamlConfig reads the config yaml file that contains the user's
// requests for monitoring domains. After successfully reading the file
// the funciton return a YamlRequests struct that contains all info from
// the file.
func getYamlConfig() (*YamlRequests, error) {
	if err := viper.ReadInConfig(); err != nil {
		return nil, errors.Wrap(err, "Error reading config file")
	}

	var yr YamlRequests

	err := viper.Unmarshal(&yr)
	if err != nil {
		return nil, errors.Wrap(err, "Unable to decode config yaml into struct")
	}

	return &yr, nil
}
