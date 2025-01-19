package main

import (
	"fmt"
	"os"

	log "github.com/sirupsen/logrus"
)

var (
	// Version of the tool that gets written during build time
	Version = "dev"
	// CommitHash of the code that get written during build time
	CommitHash = ""
)

func main() {
	fmt.Printf("Starting DNS-verifier version:%s - commit hash:%s\n", Version, CommitHash)

	cfg, err := newConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error:%v\n", err)
		os.Exit(1)
	}

	initLogging(cfg.logLevel)

	app := newApp(cfg.appPort, cfg.watchdogRequests)
	app.beforeListen()
	if err := app.run(); err != nil {
		fmt.Fprintf(os.Stderr, "error:%v\n", err)
		os.Exit(1)
	}
}

// initLogging initiliazes our logging behaviour
func initLogging(logLevel string) {
	var l log.Level
	switch logLevel {
	case "DEBUG":
		l = log.DebugLevel
	case "WARNING":
		l = log.WarnLevel
	case "INFO":
		l = log.InfoLevel
	case "ERROR":
		l = log.ErrorLevel
	default:
		l = log.InfoLevel
	}

	log.SetLevel(l)
	log.SetOutput(os.Stdout)
	log.WithFields(log.Fields{})
}
