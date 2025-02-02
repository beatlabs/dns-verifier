package main

import (
	"fmt"
	"time"

	log "github.com/sirupsen/logrus"
)

type watchdogWorker struct {
	dnsStream *dnsStream
	exit      chan bool
	stopped   bool
	ticker    *time.Ticker
}

func newWatchdogWorker(d *dnsStream) *watchdogWorker {
	return &watchdogWorker{
		dnsStream: d,
		exit:      make(chan bool, 1),
		ticker:    time.NewTicker(time.Duration(d.interval) * time.Second),
		stopped:   true,
	}
}

func (ww *watchdogWorker) watch() {
	ww.stopped = false
	dnsClient := newDNSClient()

	log.Infof("Entering watchdog's worker(%s) internal loop", ww)
	for {
		select {
		case <-ww.exit:
			log.Infof("Got message in watchdog's worker(%s) exit channel, exiting watchdog's loop", ww)
			return
		case <-ww.ticker.C:
			log.Debugf("Starting new watchdog's worker(%s) interval check", ww)
			log.Debugf("Start query for domain:<%s> and DNS query type:<%s>", ww.dnsStream.request.domain, ww.dnsStream.request.queryType)
			err := ww.dnsStream.query(dnsClient)
			if err != nil {
				log.Error(err)
			}
			log.Debugf("Finished query for domain:<%s> and DNS query type:<%s> with verification status:<%.f>", ww.dnsStream.request.domain, ww.dnsStream.request.queryType, ww.dnsStream.verificationStatus)

			ww.dnsStream.updateStats()

			log.Debugf("Finished watchdog's worker(%s) interval check", ww)
		}
	}
}

func (ww *watchdogWorker) stop() {
	if ww.stopped {
		log.Infof("Watchdog's worker(%s) already stopped", ww)
		return
	}

	ww.exit <- true
	ww.stopped = true
	log.Debugf("Sent message to watchdog's worker(%s) exit channel", ww)
}

func (ww *watchdogWorker) String() string {
	return fmt.Sprintf("Domain:<%s> - Query Type:<%s> - interval:<%d>", ww.dnsStream.request.domain, ww.dnsStream.request.queryType, ww.dnsStream.interval)
}

type watchdog struct {
	exit    chan bool
	workers []*watchdogWorker
}

func newWatchdog(requests []*dnsStream) *watchdog {
	workers := []*watchdogWorker{}
	for _, r := range requests {
		w := newWatchdogWorker(r)
		workers = append(workers, w)
	}

	return &watchdog{
		exit:    make(chan bool, 1),
		workers: workers,
	}
}

func (w *watchdog) watch() {
	for _, worker := range w.workers {
		log.Info(w.workers)
		go worker.watch()
	}

	log.Debug("Blocking on the watchdog's exit channel")
	<-w.exit
	log.Debug("Exiting watchdog watch function.")
}

func (w *watchdog) stop() {
	log.Debug("Sending message to main watchdog's exit channel")
	w.exit <- true

	log.Info("Sending message to all watchdog's workers exit channels")
	for _, worker := range w.workers {
		worker.stop()
	}

	log.Debug("Waiting couple of seconds for all watchdog workers to exit")
	// Wait couple of seconds workers to finish
	time.Sleep(2 * time.Second)
	log.Debug("Exiting watchdog stop now.")
}
