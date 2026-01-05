package main

import (
	"time"

	"github.com/gdharley/flowable-external-client-golang/flowable"
	"github.com/gdharley/flowable-external-client-golang/handlers"
)

// external_worker moved to `handlers.ExternalWorker`

func main() {
	url := "http://localhost:8090"
	interval := 10 * time.Second

	// Configure package-level defaults for auth/headers
	flowable.SetAuth("admin", "test")
	// if using a bearer token
	// flowable.SetBearerToken("token")
	// override headers if needed
	// flowable.SetDefaultHeader("X-My-Header", "value")

	// Start polling in a goroutine to avoid blocking, provide acquire params
	acquireParams := flowable.AcquireRequest{
		Topic:           "testing",
		LockDuration:    "PT10M",
		NumberOfTasks:   1,
		NumberOfRetries: 5,
		WorkerId:        "worker1",
		ScopeType:       "bpmn",
	}
	// Start the subscription to Flowable - external worker is in `handlers` package
	go flowable.Subscribe(url, interval, handlers.ExternalWorker, acquireParams)

	// Keep the main function running indefinitely
	select {}
}
