package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
)

// app encapsulates information needed for our
// application to run.
type app struct {
	port     int
	watchdog *watchdog
}

// newApp creates a new application struct.
func newApp(port int, requests []*dnsStream) *app {
	w := newWatchdog(requests)
	return &app{
		port:     port,
		watchdog: w,
	}
}

// beforeListen implements the logic to be able to start
// any kind of process before we start our web browser. Currently
// we start the watchdog process in a new goroutine to avoid blocking
// the starting of the webserver.
func (a *app) beforeListen() {
	go a.watchdog.watch()
}

// run is responsible for running our webserver and also shut it down along
// with the watchdog processes.
func (a *app) run() error {
	// Initialize HTTP server
	hostPort := fmt.Sprintf(":%d", a.port)
	server := &http.Server{
		Addr: hostPort,
	}
	http.Handle("/metrics", promhttp.Handler())
	http.HandleFunc("/live", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	})
	http.HandleFunc("/ready", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	})

	// Start listening asynchronously
	go func() {
		log.Info("Starting server on port:", a.port)
		if err := server.ListenAndServe(); err != nil {
			log.Error("Failed to start listening server")
			panic(err)
		}
	}()

	// Setting up signal capturing
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, syscall.SIGINT, syscall.SIGTERM)

	// Waiting for SIGINT/SIGTERM
	<-shutdown

	// Kill watchdog internal loop
	a.watchdog.stop()

	// Shut down server, waiting 5secs for all requests before kill them.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	log.Info("Shutting down...")
	if err := server.Shutdown(ctx); err != nil {
		log.Error("Failed to stop listening server")
	}

	return nil
}
