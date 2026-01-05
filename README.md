# Flowable External Worker Library for Golang

This project is licensed under the terms of the [Apache License 2.0](LICENSE)

An _External Worker Task_ in BPMN or CMMN is a task where the custom logic of that task is executed externally to Flowable, i.e. on another server.
When the process or case engine arrives at such a task, it will create an **external job**, which is exposed over the REST API.
Through this REST API, the job can be acquired and locked.
Once locked, the custom logic is responsible for signalling over REST that the work is done and the process or case can continue.

This project makes implementing such custom logic in Golang easy by not having the worry about the low-level details of the REST API and focus on the actual custom business logic.
Integrations for other languages are also available.

## Authentication

There are default implementations for example for basic authentication e.g. `flowable.SetAuth("admin", "test")`.
The module offers a bearer token implementation `flowable.SetBearerToken("token")` which allows to specify an access token to the Flowable Cloud offering.

## Installation

To install, clone the Github project

## Setup

The **main.go** file contains the base server address (url) as well as the work job acquisition parameters (acquireParams).
The custom worker business logic is held in **handlers/external_worker.go** And supports access to input parameters from the inbound _body_ variable.
If any errors were reported from the rest call or parsing of the job, an http _status_ variable will be available, values over 400 should be considered errors. This allows the handler to determine the best course of action.
Handler results support _success_, _fail_, _bpmnError_ and _cmmnTerminate_ responses.

