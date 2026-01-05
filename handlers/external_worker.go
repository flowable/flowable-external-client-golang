package handlers

import (
	"encoding/json"
	"strconv"

	"github.com/gdharley/flowable-external-client-golang/flowable"
)

// ExternalWorker is the worker handler function used by the Flowable subscriber.
func ExternalWorker(status int, body string) (flowable.HandlerStatus, *flowable.HandlerResult) {
	// Initialize the response object
	res := &flowable.HandlerResult{
		Status:    flowable.HandlerSuccess,
		WorkerId:  "",
		Variables: []flowable.HandlerVariable{},
		ErrorCode: "",
	}

	// Either Acquire_jobs or the job itself could not be parsed
	if status >= 400 {
		res.ErrorCode = strconv.Itoa(status)
		res.Status = flowable.HandlerFail
	}

	if body != "" {
		var data interface{}
		if err := json.Unmarshal([]byte(body), &data); err != nil {
			// Unmarshal failed — mark handler as failed and set error code
			res.ErrorCode = err.Error()
			res.Status = flowable.HandlerFail
		} else {
			// Add a dummy variable for testing/visibility
			res.Variables = append(res.Variables, flowable.HandlerVariable{Name: "dummy", Type: "string", Value: "a simple string"})
			res.Status = flowable.HandlerSuccess
		}
	}

	return res.Status, res
}
