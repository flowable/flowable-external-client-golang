package worker

import (
	"strconv"

	"github.com/flowable/flowable-external-client-golang/flowable"
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
		vars, err := flowable.ExtractVariablesFromBody(body)
		if err != nil {
			res.ErrorCode = err.Error()
			res.Status = flowable.HandlerFail
		} else {
			// Process variables as needed - here we simply add them to the response
			res.Variables = append(res.Variables, vars...)

			// Example: assign the "inputVar" variable (if present) to a Go string named inputVar
			inputVar := flowable.GetVar(vars, "inputVar")
			_ = inputVar

			// Example: Add a dummy variable for testing
			res.Variables = append(res.Variables, flowable.HandlerVariable{Name: "dummy", Type: "string", Value: "a simple string"})

			// Set response code to success
			res.Status = flowable.HandlerSuccess
		}
	}

	return res.Status, res
}
