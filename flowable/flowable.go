package flowable

import (
	"encoding/json"
	"fmt"
	"log"
	"time"
)

const job_api = "/external-job-api"

// HandlerStatus indicates how the handler processed a job.
type HandlerStatus string

const (
	HandlerSuccess       HandlerStatus = "success"
	HandlerFail          HandlerStatus = "fail"
	HandlerBPMNError     HandlerStatus = "bpmnError"
	HandlerCMMNTerminate HandlerStatus = "cmmnTerminate"
)

// HandlerVariable represents a single variable in the handler result.
type HandlerVariable struct {
	Name  string      `json:"name"`
	Type  string      `json:"type"`
	Value interface{} `json:"value"`
}

// HandlerResult is the structured response returned by the handler.
type HandlerResult struct {
	Status    HandlerStatus     `json:"status"`
	WorkerId  string            `json:"workerId,omitempty"`
	Variables []HandlerVariable `json:"variables"`
	ErrorCode string            `json:"errorCode,omitempty"`
}

// Callback function type. The handler returns a HandlerStatus and an optional structured result.
type ResponseHandler func(status int, body string) (HandlerStatus, *HandlerResult)

// AcquireRequest represents the body sent to the acquire endpoint.
type AcquireRequest struct {
	Topic           string `json:"topic"`
	LockDuration    string `json:"lockDuration"`
	NumberOfTasks   int    `json:"numberOfTasks"`
	NumberOfRetries int    `json:"numberOfRetries"`
	WorkerId        string `json:"workerId"`
	ScopeType       string `json:"scopeType"`
}

// Acquire_jobs performs a POST to the acquire jobs endpoint (/acquire/jobs) with a JSON body.
func Acquire_jobs(url string, reqBody AcquireRequest) (jobs []interface{}, body string, status int, err error) {
	full := url + job_api + "/acquire/jobs"
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return nil, "", -1, err
	}
	status, bodyBytes, err := restPost(full, payload)
	if err != nil {
		return nil, "", status, err
	}

	var parsed []interface{}
	if err := json.Unmarshal(bodyBytes, &parsed); err != nil {
		// If response isn't a JSON array, return an error
		return nil, string(bodyBytes), status, err
	}
	return parsed, string(bodyBytes), status, nil
}

// List_jobs performs a single GET to the jobs endpoint and returns the status and raw response body.
func List_jobs(url string) (status int, body string, err error) {
	full := url + job_api + "/jobs"
	status, bodyBytes, err := restGet(full)
	if err != nil {
		return -1, "", err
	}
	return status, string(bodyBytes), nil
}

// Subscribe polls the given URL at intervals and invokes the handler when jobs are available.
// acquireReq must be provided by the caller with the desired acquire parameters.
func Subscribe(url string, interval time.Duration, handler ResponseHandler, acquireReq AcquireRequest) {
	for {
		jobs, _, status, err := Acquire_jobs(url, acquireReq)
		if err != nil {
			// If acquire failed (including parse errors), treat as status 500 and pass the raw body if available
			resStatus, resObj := handler(500, "")
			handle_worker_response(url, acquireReq.WorkerId, "", resStatus, resObj)
			time.Sleep(interval)
			continue
		}
		if len(jobs) == 0 {
			// No jobs, wait and poll again
			time.Sleep(interval)
			continue
		}
		// Jobs found, invoke handler for each job individually
		for _, job := range jobs {
			jobBytes, err := json.Marshal(job)
			if err != nil {
				// If we can't serialize an individual job, treat as processing/parsing failure => status 500
				resStatus, resObj := handler(500, "")
				handle_worker_response(url, acquireReq.WorkerId, "", resStatus, resObj)
				continue
			}
			// Try to extract a jobId if present in the job object
			jobId := ""
			var jobMap map[string]interface{}
			if err := json.Unmarshal(jobBytes, &jobMap); err == nil {
				if id, ok := jobMap["id"].(string); ok && id != "" {
					jobId = id
				} else if jid, ok := jobMap["jobId"].(string); ok && jid != "" {
					jobId = jid
				} else if idnum, ok := jobMap["id"].(float64); ok {
					jobId = fmt.Sprintf("%.0f", idnum)
				}
			}
			resStatus, resObj := handler(status, string(jobBytes))
			// Delegate result handling to helper
			handle_worker_response(url, acquireReq.WorkerId, jobId, resStatus, resObj)
		}
		time.Sleep(interval)
	}
}

// handle_worker_response centralizes logging/processing of handler responses.
// It also calls the appropriate task action (complete/fail/bpmnError/cmmnTerminate) via REST.
func handle_worker_response(baseURL string, workerId string, jobId string, resStatus HandlerStatus, resObj *HandlerResult) {
	// Ensure resObj has workerId populated
	if resObj != nil && resObj.WorkerId == "" {
		resObj.WorkerId = workerId
	}

	// Ensure we have a non-nil result object to send (create a minimal one if needed)
	if resObj == nil {
		resObj = &HandlerResult{WorkerId: workerId}
	}

	switch resStatus {
	case HandlerSuccess:
		task_complete(baseURL, jobId, resObj)
	case HandlerFail:
		if resObj.ErrorCode == "" {
			resObj.ErrorCode = "failed"
		}
		task_fail(baseURL, jobId, resObj)
	case HandlerBPMNError:
		if resObj.ErrorCode == "" {
			resObj.ErrorCode = "bpmnError"
		}
		task_bpmnError(baseURL, jobId, resObj)
	case HandlerCMMNTerminate:
		if resObj.ErrorCode == "" {
			resObj.ErrorCode = "cmmnTerminate"
		}
		task_cmmnTerminate(baseURL, jobId, resObj)
	default:
		log.Printf("Unhandled handler status: %s", resStatus)
	}
}

// task_complete posts completion with workerId and result to the job-specific URL
func task_complete(baseURL string, jobId string, res *HandlerResult) {
	if jobId == "" {
		log.Printf("task_complete: missing jobId, skipping")
		return
	}
	path := baseURL + job_api + "/acquire/jobs/" + jobId + "/complete"
	b, err := json.Marshal(res)
	if err != nil {
		log.Printf("task_complete: marshal error: %v", err)
		return
	}
	status, body, err := restPost(path, b)
	if err != nil {
		log.Printf("task_complete: post error: %v", err)
		return
	}
	log.Printf("task_complete: status=%d, body=%s", status, string(body))
}

// task_fail posts failure with workerId and result to the job-specific URL
func task_fail(baseURL string, jobId string, res *HandlerResult) {
	if jobId == "" {
		log.Printf("task_fail: missing jobId, skipping")
		return
	}
	path := baseURL + job_api + "/acquire/jobs/" + jobId + "/fail"
	b, err := json.Marshal(res)
	if err != nil {
		log.Printf("task_fail: marshal error: %v", err)
		return
	}
	status, body, err := restPost(path, b)
	if err != nil {
		log.Printf("task_fail: post error: %v", err)
		return
	}
	log.Printf("task_fail: status=%d, body=%s", status, string(body))
}

// task_bpmnError posts a BPMN error with workerId and result to the job-specific URL
func task_bpmnError(baseURL string, jobId string, res *HandlerResult) {
	if jobId == "" {
		log.Printf("task_bpmnError: missing jobId, skipping")
		return
	}
	path := baseURL + job_api + "/acquire/jobs/" + jobId + "/bpmnError"
	b, err := json.Marshal(res)
	if err != nil {
		log.Printf("task_bpmnError: marshal error: %v", err)
		return
	}
	status, body, err := restPost(path, b)
	if err != nil {
		log.Printf("task_bpmnError: post error: %v", err)
		return
	}
	log.Printf("task_bpmnError: status=%d, body=%s", status, string(body))
}

// task_cmmnTerminate posts a CMMN terminate with workerId and result to the job-specific URL
func task_cmmnTerminate(baseURL string, jobId string, res *HandlerResult) {
	if jobId == "" {
		log.Printf("task_cmmnTerminate: missing jobId, skipping")
		return
	}
	path := baseURL + job_api + "/acquire/jobs/" + jobId + "/cmmnTerminate"
	b, err := json.Marshal(res)
	if err != nil {
		log.Printf("task_cmmnTerminate: marshal error: %v", err)
		return
	}
	status, body, err := restPost(path, b)
	if err != nil {
		log.Printf("task_cmmnTerminate: post error: %v", err)
		return
	}
	log.Printf("task_cmmnTerminate: status=%d, body=%s", status, string(body))
}
