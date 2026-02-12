package main

import (
	"time"

	"github.com/flowable/flowable-external-client-golang/flowable"
	"github.com/flowable/flowable-external-client-golang/worker"
)

// Sample main function to start the Flowable external worker subscription

func main() {

	// Configure package-level defaults for auth/headers
	flowable.SetAuth("admin", "test")
	// flowable.SetBearerToken("token")
	// override headers if needed
	// flowable.SetDefaultHeader("X-My-Header", "value")
	// Enable logging (default is true)
	flowable.SetEnableLogging(true)
	// if using a bearer token

	// Provide subscription parameters
	acquireParams := flowable.AcquireRequest{
		Topic:           "testing",
		LockDuration:    "PT10M",
		NumberOfTasks:   1,
		NumberOfRetries: 5,
		WorkerId:        "worker1",
		ScopeType:       "bpmn",
		URL:             "http://localhost:8090",
		Interval:        10 * time.Second,
	}
	// Start the subscription to Flowable
	go flowable.Subscribe(acquireParams, worker.ExternalWorker)

	// Keep the main function running indefinitely
	select {}
}
