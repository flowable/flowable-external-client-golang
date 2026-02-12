# Flowable External Worker Library for Golang

This project is licensed under the terms of the [Apache License 2.0](LICENSE)

An _External Worker Task_ in BPMN or CMMN is a task where the custom logic of that task is executed externally to Flowable, i.e. on another server.
When the process or case engine arrives at such a task, it will create an **external job**, which is exposed over the REST API.
Through this REST API, the job can be acquired and locked.
Once locked, the custom logic is responsible for signalling over REST that the work is done and the process or case can continue.

This project makes implementing such custom logic in Golang easy by implementing the low-level details of the REST API and focus on the actual custom business logic.
Integrations for other languages are also available.

## Authentication

There are default implementations for basic authentication and bearer tokens..
Basic Auth: `flowable.SetAuth("admin", "test")`
Bearer token: `flowable.SetBearerToken("token")`

## Installation

Installation is not essential as the project can be referenced using standard golang module references from your own project.

A sample main.go and simple worker implementation are provided in the project as examples.

However, the project is licensed with the Apache 2 license and can be readily cloned if you wish to make modificatiosn or customizations.

## Setup

The **main.go** file contains the work job acquisition parameters (`acquireParams`). This is where job acquisitions parameters are declared including base url, poll interval, topic name, retry count, task retrieval batch size.


Example:

- Create the acquire parameters with connection settings:

```
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
```

- Start the subscriber by passing the `AcquireRequest` and your handler:

```
go flowable.Subscribe(acquireParams, worker.ExternalWorker)
```

The sample worker business logic is held in `worker/external_worker.go` and supports access to input parameters from the inbound _body_ variable. If any errors were reported from the REST call or parsing of the job, an http _status_ variable will be available — values >= 400 should be considered errors. Handler results support _success_, _fail_, _bpmnError_ and _cmmnTerminate_ responses.

## Logging

 - **Default:** logging is enabled by default.
 - **Control:** toggle logging at runtime from `main.go` using:

```
flowable.SetEnableLogging(true)  // enable (default)
flowable.SetEnableLogging(false) // disable
```

When logging is disabled the library will suppress internal `log.Printf` messages.

## Integration Tests With Cached HTTP Cassettes

Integration tests in `test/flowable_integration_test.go` use a VCR-style recorder (`go-vcr`) and store HTTP cassettes in `test/fixtures/cassettes`.

### First run (record cassettes)

Requires a running Flowable Work instance.

```bash
FLOWABLE_INTEGRATION=1 \
FLOWABLE_CASSETTE_MODE=record \
FLOWABLE_BASE_URL=http://localhost:8090 \
FLOWABLE_USERNAME=admin \
FLOWABLE_PASSWORD=test \
go test ./test -run Integration -v
```

### Run from cache (no Flowable required)

```bash
FLOWABLE_CASSETTE_MODE=replay go test ./test -run Integration -v
```

### Cassette behavior

- Default behavior:
  - With `FLOWABLE_INTEGRATION=1`: replay existing cassette interactions and record missing ones.
  - Without `FLOWABLE_INTEGRATION=1`: replay only from existing cassettes.
- If a cassette is missing and `FLOWABLE_INTEGRATION` is not set, that test is skipped.
- To re-seed all cassettes, delete `test/fixtures/cassettes/*.yaml` and run in `record` mode again.