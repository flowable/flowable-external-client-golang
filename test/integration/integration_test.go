package integration_test

import (
	"encoding/json"
	"os"
	"reflect"
	"sort"
	"sync/atomic"
	"testing"
	"time"

	"github.com/flowable/flowable-external-client-golang/flowable"
)

func TestMain(m *testing.M) {
	baseURL = getBaseURL()
	flowable.SetAuth(authUser, authPass)
	flowable.SetEnableLogging(true)
	os.Exit(m.Run())
}

// --- REST client tests (similar to Python test_restclient.py) ---

func TestListJobs(t *testing.T) {
	setupVCR(t)
	deploymentID := deployProcess(t, "externalWorkerProcess.bpmn")
	defer deleteDeployment(t, deploymentID)

	defID := getProcessDefinitionID(t, deploymentID)
	processInstanceID := startProcess(t, defID)
	defer terminateProcess(t, processInstanceID)

	status, body, err := flowable.List_jobs(baseURL)
	if err != nil {
		t.Fatalf("List_jobs error: %v", err)
	}
	if status != 200 {
		t.Fatalf("expected status 200, got %d", status)
	}

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(body), &result); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	total, ok := result["total"].(float64)
	if !ok {
		t.Fatalf("expected total field in response")
	}
	if total < 1 {
		t.Fatalf("expected at least 1 job, got %.0f", total)
	}

	data := result["data"].([]interface{})
	if len(data) < 1 {
		t.Fatal("expected at least 1 job in data array")
	}

	// Find our job by processInstanceId
	found := false
	for _, item := range data {
		job := item.(map[string]interface{})
		if pid, ok := job["processInstanceId"].(string); ok && pid == processInstanceID {
			found = true
			if name, ok := job["elementName"].(string); ok {
				if name != "External Worker task" {
					t.Fatalf("expected elementName 'External Worker task', got %q", name)
				}
			}
			if retries, ok := job["retries"].(float64); ok {
				if retries != 3 {
					t.Fatalf("expected 3 retries, got %.0f", retries)
				}
			}
			break
		}
	}
	if !found {
		t.Fatal("could not find job for our process instance in list")
	}
}

func TestAcquireJobs(t *testing.T) {
	setupVCR(t)
	deploymentID := deployProcess(t, "externalWorkerProcess.bpmn")
	defer deleteDeployment(t, deploymentID)

	defID := getProcessDefinitionID(t, deploymentID)
	processInstanceID := startProcess(t, defID)
	defer terminateProcess(t, processInstanceID)

	job := acquireJobForInstance(t, "myTopic", "bpmn", processInstanceID)

	if name, ok := job["elementName"].(string); ok {
		if name != "External Worker task" {
			t.Fatalf("expected elementName 'External Worker task', got %q", name)
		}
	} else {
		t.Fatal("expected elementName field in acquired job")
	}

	// Check that variables are present (at least 'initiator')
	if vars, ok := job["variables"].([]interface{}); ok {
		if len(vars) < 1 {
			t.Fatal("expected at least 1 variable")
		}
		firstVar := vars[0].(map[string]interface{})
		if firstVar["name"] == "initiator" {
			if firstVar["value"] != "admin" {
				t.Fatalf("expected initiator value 'admin', got %v", firstVar["value"])
			}
		}
	}
}

func TestCompleteJob(t *testing.T) {
	setupVCR(t)
	deploymentID := deployProcess(t, "externalWorkerProcess.bpmn")
	defer deleteDeployment(t, deploymentID)

	defID := getProcessDefinitionID(t, deploymentID)
	processInstanceID := startProcess(t, defID)

	job := acquireJobForInstance(t, "myTopic", "bpmn", processInstanceID)
	jobID := job["id"].(string)

	variables := []map[string]interface{}{
		{"name": "testVar", "type": "string", "value": "test content"},
	}
	completeJob(t, jobID, variables)

	// Verify the process variable was set
	variable := getProcessVariable(t, processInstanceID, "testVar")
	if variable == nil {
		t.Fatal("expected testVar to be set")
	}
	if variable["type"] != "string" {
		t.Fatalf("expected type 'string', got %v", variable["type"])
	}
	if variable["value"] != "test content" {
		t.Fatalf("expected value 'test content', got %v", variable["value"])
	}

	// Verify executed activities
	activityIDs := executedActivityIDs(t, processInstanceID)
	expected := []string{"bpmnEndEvent_1", "bpmnSequenceFlow_2", "bpmnSequenceFlow_4", "bpmnTask_3", "startnoneevent1"}
	if !reflect.DeepEqual(activityIDs, expected) {
		t.Fatalf("expected activities %v, got %v", expected, activityIDs)
	}
}

func TestBPMNErrorWithoutErrorCode(t *testing.T) {
	setupVCR(t)
	deploymentID := deployProcess(t, "withBoundaryEvent.bpmn")
	defer deleteDeployment(t, deploymentID)

	defID := getProcessDefinitionID(t, deploymentID)
	processInstanceID := startProcess(t, defID)

	job := acquireJobForInstance(t, "myTopic", "bpmn", processInstanceID)
	jobID := job["id"].(string)

	variables := []map[string]interface{}{
		{"name": "testVar", "type": "string", "value": "test failure"},
	}
	bpmnErrorJob(t, jobID, variables, "")

	// Verify variable was set
	variable := getProcessVariable(t, processInstanceID, "testVar")
	if variable == nil {
		t.Fatal("expected testVar to be set")
	}
	if variable["value"] != "test failure" {
		t.Fatalf("expected value 'test failure', got %v", variable["value"])
	}

	// Verify boundary event was triggered
	activityIDs := executedActivityIDs(t, processInstanceID)
	expected := sort.StringSlice{"startnoneevent1", "bpmnSequenceFlow_2", "bpmnTask_3",
		"bpmnBoundaryEvent_3", "bpmnSequenceFlow_6", "bpmnEndEvent_5", "bpmnBoundaryEvent_4"}
	sort.Sort(expected)
	if !reflect.DeepEqual(activityIDs, []string(expected)) {
		t.Fatalf("expected activities %v, got %v", expected, activityIDs)
	}
}

func TestBPMNErrorWithErrorCode1(t *testing.T) {
	setupVCR(t)
	deploymentID := deployProcess(t, "withBoundaryEvent.bpmn")
	defer deleteDeployment(t, deploymentID)

	defID := getProcessDefinitionID(t, deploymentID)
	processInstanceID := startProcess(t, defID)

	job := acquireJobForInstance(t, "myTopic", "bpmn", processInstanceID)
	jobID := job["id"].(string)

	variables := []map[string]interface{}{
		{"name": "testVar", "type": "string", "value": "test failure"},
	}
	bpmnErrorJob(t, jobID, variables, "errorCode1")

	// Verify variable
	variable := getProcessVariable(t, processInstanceID, "testVar")
	if variable == nil {
		t.Fatal("expected testVar to be set")
	}
	if variable["value"] != "test failure" {
		t.Fatalf("expected value 'test failure', got %v", variable["value"])
	}

	// Verify errorCode1 boundary event was triggered
	activityIDs := executedActivityIDs(t, processInstanceID)
	expected := sort.StringSlice{"startnoneevent1", "bpmnSequenceFlow_2", "bpmnTask_3",
		"bpmnBoundaryEvent_3", "bpmnSequenceFlow_6", "bpmnEndEvent_5", "bpmnBoundaryEvent_4"}
	sort.Sort(expected)
	if !reflect.DeepEqual(activityIDs, []string(expected)) {
		t.Fatalf("expected activities %v, got %v", expected, activityIDs)
	}
}

func TestBPMNErrorWithErrorCode2(t *testing.T) {
	setupVCR(t)
	deploymentID := deployProcess(t, "withBoundaryEvent.bpmn")
	defer deleteDeployment(t, deploymentID)

	defID := getProcessDefinitionID(t, deploymentID)
	processInstanceID := startProcess(t, defID)

	job := acquireJobForInstance(t, "myTopic", "bpmn", processInstanceID)
	jobID := job["id"].(string)

	variables := []map[string]interface{}{
		{"name": "testVar", "type": "string", "value": "test failure"},
	}
	bpmnErrorJob(t, jobID, variables, "errorCode2")

	// Verify variable
	variable := getProcessVariable(t, processInstanceID, "testVar")
	if variable == nil {
		t.Fatal("expected testVar to be set")
	}

	// Verify errorCode2 goes through the generic boundary event path
	activityIDs := executedActivityIDs(t, processInstanceID)
	expected := sort.StringSlice{"startnoneevent1", "bpmnSequenceFlow_2", "bpmnTask_3",
		"bpmnBoundaryEvent_3", "bpmnSequenceFlow_8", "bpmnEndEvent_7", "bpmnBoundaryEvent_4"}
	sort.Sort(expected)
	if !reflect.DeepEqual(activityIDs, []string(expected)) {
		t.Fatalf("expected activities %v, got %v", expected, activityIDs)
	}
}

func TestFailJob(t *testing.T) {
	setupVCR(t)
	deploymentID := deployProcess(t, "externalWorkerProcess.bpmn")
	defer deleteDeployment(t, deploymentID)

	defID := getProcessDefinitionID(t, deploymentID)
	processInstanceID := startProcess(t, defID)
	defer terminateProcess(t, processInstanceID)

	job := acquireJobForInstance(t, "myTopic", "bpmn", processInstanceID)
	jobID := job["id"].(string)

	initialRetries := job["retries"].(float64)
	failJob(t, jobID)

	// Verify retries decremented
	jobAfterFail := getJob(t, jobID)
	retriesAfterFail := jobAfterFail["retries"].(float64)
	if retriesAfterFail != initialRetries-1 {
		t.Fatalf("expected retries to be %.0f, got %.0f", initialRetries-1, retriesAfterFail)
	}

	// Verify process is still running (not completed)
	activityIDs := executedActivityIDs(t, processInstanceID)
	expected := sort.StringSlice{"startnoneevent1", "bpmnSequenceFlow_2", "bpmnTask_3"}
	sort.Sort(expected)
	if !reflect.DeepEqual(activityIDs, []string(expected)) {
		t.Fatalf("expected activities %v, got %v", expected, activityIDs)
	}
}

func TestCMMNTerminate(t *testing.T) {
	setupVCR(t)
	deploymentID := deployCase(t, "externalWorkerCase.cmmn")
	defer deleteCaseDeployment(t, deploymentID)

	defID := getCaseDefinitionID(t, deploymentID)
	caseInstanceID := startCase(t, defID)

	job := acquireJobForInstance(t, "cmmnTopic", "cmmn", caseInstanceID)
	jobID := job["id"].(string)

	variables := []map[string]interface{}{
		{"name": "testVar", "type": "string", "value": "test terminate"},
	}
	cmmnTerminateJob(t, jobID, variables)

	// Verify the case variable was set
	variable := getCaseVariable(t, caseInstanceID, "testVar")
	if variable == nil {
		t.Fatal("expected testVar to be set")
	}
	if variable["type"] != "string" {
		t.Fatalf("expected type 'string', got %v", variable["type"])
	}
	if variable["value"] != "test terminate" {
		t.Fatalf("expected value 'test terminate', got %v", variable["value"])
	}
}

// --- Subscribe tests (similar to Python test_init.py) ---

func TestSubscribeComplete(t *testing.T) {
	setupVCR(t)
	deploymentID := deployProcess(t, "externalWorkerProcess.bpmn")
	defer deleteDeployment(t, deploymentID)

	defID := getProcessDefinitionID(t, deploymentID)
	processInstanceID := startProcess(t, defID)

	done := make(chan struct{}, 1)
	var handled int32

	acquireReq := flowable.AcquireRequest{
		Topic:           "myTopic",
		LockDuration:    "PT10S",
		NumberOfTasks:   10,
		NumberOfRetries: 5,
		WorkerId:        workerID,
		ScopeType:       "bpmn",
		URL:             baseURL,
		Interval:        1 * time.Second,
	}

	go flowable.Subscribe(acquireReq, func(status int, body string) (flowable.HandlerStatus, *flowable.HandlerResult) {
		// Check if this job belongs to our process instance
		var jobData map[string]interface{}
		if json.Unmarshal([]byte(body), &jobData) == nil {
			pid, _ := jobData["processInstanceId"].(string)
			if pid != processInstanceID {
				// Not our job, complete it silently to release
				return flowable.HandlerSuccess, nil
			}
		}

		if atomic.CompareAndSwapInt32(&handled, 0, 1) {
			done <- struct{}{}
		}
		return flowable.HandlerSuccess, &flowable.HandlerResult{
			Variables: []flowable.HandlerVariable{
				{Name: "testVar", Type: "string", Value: "test content"},
			},
		}
	})

	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("timeout waiting for subscribe handler")
	}

	// Wait for handle_worker_response to complete the REST call
	time.Sleep(2 * time.Second)

	// Verify process completed
	activityIDs := executedActivityIDs(t, processInstanceID)
	expected := []string{"bpmnEndEvent_1", "bpmnSequenceFlow_2", "bpmnSequenceFlow_4", "bpmnTask_3", "startnoneevent1"}
	if !reflect.DeepEqual(activityIDs, expected) {
		t.Fatalf("expected activities %v, got %v", expected, activityIDs)
	}

	// Verify variable was set
	variable := getProcessVariable(t, processInstanceID, "testVar")
	if variable == nil {
		t.Fatal("expected testVar to be set")
	}
	if variable["value"] != "test content" {
		t.Fatalf("expected value 'test content', got %v", variable["value"])
	}
}

func TestSubscribeBPMNError(t *testing.T) {
	setupVCR(t)
	deploymentID := deployProcess(t, "withBoundaryEvent.bpmn")
	defer deleteDeployment(t, deploymentID)

	defID := getProcessDefinitionID(t, deploymentID)
	processInstanceID := startProcess(t, defID)

	done := make(chan struct{}, 1)
	var handled int32

	acquireReq := flowable.AcquireRequest{
		Topic:           "myTopic",
		LockDuration:    "PT10S",
		NumberOfTasks:   10,
		NumberOfRetries: 5,
		WorkerId:        workerID,
		ScopeType:       "bpmn",
		URL:             baseURL,
		Interval:        1 * time.Second,
	}

	go flowable.Subscribe(acquireReq, func(status int, body string) (flowable.HandlerStatus, *flowable.HandlerResult) {
		var jobData map[string]interface{}
		if json.Unmarshal([]byte(body), &jobData) == nil {
			pid, _ := jobData["processInstanceId"].(string)
			if pid != processInstanceID {
				return flowable.HandlerSuccess, nil
			}
		}

		if atomic.CompareAndSwapInt32(&handled, 0, 1) {
			done <- struct{}{}
		}
		return flowable.HandlerBPMNError, &flowable.HandlerResult{
			ErrorCode: "errorCode1",
			Variables: []flowable.HandlerVariable{
				{Name: "testVar", Type: "string", Value: "test failure"},
			},
		}
	})

	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("timeout waiting for subscribe handler")
	}

	time.Sleep(2 * time.Second)

	// Verify variable
	variable := getProcessVariable(t, processInstanceID, "testVar")
	if variable == nil {
		t.Fatal("expected testVar to be set")
	}
	if variable["value"] != "test failure" {
		t.Fatalf("expected value 'test failure', got %v", variable["value"])
	}

	// Verify boundary event was triggered
	activityIDs := executedActivityIDs(t, processInstanceID)
	expected := sort.StringSlice{"startnoneevent1", "bpmnSequenceFlow_2", "bpmnTask_3",
		"bpmnBoundaryEvent_3", "bpmnSequenceFlow_6", "bpmnEndEvent_5", "bpmnBoundaryEvent_4"}
	sort.Sort(expected)
	if !reflect.DeepEqual(activityIDs, []string(expected)) {
		t.Fatalf("expected activities %v, got %v", expected, activityIDs)
	}
}

func TestSubscribeCMMNTerminate(t *testing.T) {
	setupVCR(t)
	deploymentID := deployCase(t, "externalWorkerCase.cmmn")
	defer deleteCaseDeployment(t, deploymentID)

	defID := getCaseDefinitionID(t, deploymentID)
	caseInstanceID := startCase(t, defID)

	done := make(chan struct{}, 1)
	var handled int32

	acquireReq := flowable.AcquireRequest{
		Topic:           "cmmnTopic",
		LockDuration:    "PT10S",
		NumberOfTasks:   10,
		NumberOfRetries: 5,
		WorkerId:        workerID,
		ScopeType:       "cmmn",
		URL:             baseURL,
		Interval:        1 * time.Second,
	}

	go flowable.Subscribe(acquireReq, func(status int, body string) (flowable.HandlerStatus, *flowable.HandlerResult) {
		var jobData map[string]interface{}
		if json.Unmarshal([]byte(body), &jobData) == nil {
			sid, _ := jobData["scopeId"].(string)
			if sid != caseInstanceID {
				return flowable.HandlerSuccess, nil
			}
		}

		if atomic.CompareAndSwapInt32(&handled, 0, 1) {
			done <- struct{}{}
		}
		return flowable.HandlerCMMNTerminate, &flowable.HandlerResult{
			Variables: []flowable.HandlerVariable{
				{Name: "testVar", Type: "string", Value: "test terminate"},
			},
		}
	})

	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("timeout waiting for subscribe handler")
	}

	time.Sleep(2 * time.Second)

	// Verify variable
	variable := getCaseVariable(t, caseInstanceID, "testVar")
	if variable == nil {
		t.Fatal("expected testVar to be set")
	}
	if variable["type"] != "string" {
		t.Fatalf("expected type 'string', got %v", variable["type"])
	}
	if variable["value"] != "test terminate" {
		t.Fatalf("expected value 'test terminate', got %v", variable["value"])
	}
}
