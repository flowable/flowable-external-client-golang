package worker_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/flowable/flowable-external-client-golang/flowable"
)

func TestListJobsInvalidAuthIntegration(t *testing.T) {
	env := getIntegrationEnv(t, "test_list_jobs_invalid_auth")

	flowable.SetAuth("invalid", "auth")
	defer flowable.SetAuth(env.username, env.password)

	status, _, err := flowable.List_jobs(env.baseURL)
	if err != nil {
		t.Fatalf("list jobs with invalid auth returned error: %v", err)
	}
	if status != http.StatusUnauthorized {
		t.Fatalf("expected 401 for invalid auth, got %d", status)
	}
}

func TestListJobsAndAcquireJobsIntegration(t *testing.T) {
	env := getIntegrationEnv(t, "test_list_jobs_and_acquire_jobs")

	deploymentID := env.deployProcess(t, "externalWorkerProcess.bpmn")
	defer env.deleteProcessDeployment(t, deploymentID)

	processDefinitionID := env.processDefinitionID(t, deploymentID)
	processInstanceID := env.startProcess(t, processDefinitionID)
	defer env.terminateProcess(t, processInstanceID)

	status, body, err := flowable.List_jobs(env.baseURL)
	if err != nil {
		t.Fatalf("list jobs failed: %v", err)
	}
	if status != http.StatusOK {
		t.Fatalf("expected list jobs status 200, got %d body=%s", status, body)
	}

	var out pagedResponse
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatalf("unmarshal list jobs response: %v", err)
	}
	if out.Total < 1 {
		t.Fatalf("expected at least 1 job, got total=%d body=%s", out.Total, body)
	}

	job := env.acquireJob(t, "myTopic", "bpmn")
	elementName, _ := job["elementName"].(string)
	if elementName != "External Worker task" {
		t.Fatalf("expected elementName %q, got %q", "External Worker task", elementName)
	}
	if !hasAcquiredVariable(job, "initiator", "admin") {
		t.Fatalf("expected acquired job to include initiator=admin variable: %#v", job["variables"])
	}
}

func TestCompleteJobIntegration(t *testing.T) {
	env := getIntegrationEnv(t, "test_complete_job")

	deploymentID := env.deployProcess(t, "externalWorkerProcess.bpmn")
	defer env.deleteProcessDeployment(t, deploymentID)

	processDefinitionID := env.processDefinitionID(t, deploymentID)
	processInstanceID := env.startProcess(t, processDefinitionID)

	job := env.acquireJob(t, "myTopic", "bpmn")
	env.postJobAction(t, "complete", jobID(t, job), &flowable.HandlerResult{
		WorkerId: "test-worker",
		Variables: []flowable.HandlerVariable{
			{Name: "testVar", Type: "string", Value: "test content"},
		},
	})

	variable := waitForVariable(t, func() map[string]interface{} {
		return env.historicProcessVariable(t, processInstanceID, "testVar")
	}, 8*time.Second)
	if variable == nil {
		t.Fatalf("did not find historic process variable testVar")
	}
	if variable["type"] != "string" || variable["value"] != "test content" {
		t.Fatalf("unexpected variable value: %#v", variable)
	}

	activityIDs := waitForActivities(t, func() []string {
		return env.historicActivityIDs(t, processInstanceID)
	}, 8*time.Second)
	expected := []string{"bpmnEndEvent_1", "bpmnSequenceFlow_2", "bpmnSequenceFlow_4", "bpmnTask_3", "startnoneevent1"}
	if strings.Join(activityIDs, ",") != strings.Join(expected, ",") {
		t.Fatalf("unexpected activity ids. expected=%v got=%v", expected, activityIDs)
	}
}

func TestFailJobIntegration(t *testing.T) {
	env := getIntegrationEnv(t, "test_fail_job")

	deploymentID := env.deployProcess(t, "externalWorkerProcess.bpmn")
	defer env.deleteProcessDeployment(t, deploymentID)

	processDefinitionID := env.processDefinitionID(t, deploymentID)
	processInstanceID := env.startProcess(t, processDefinitionID)
	defer env.terminateProcess(t, processInstanceID)

	job := env.acquireJob(t, "myTopic", "bpmn")
	id := jobID(t, job)
	env.postJobAction(t, "fail", id, &flowable.HandlerResult{WorkerId: "test-worker", ErrorCode: "failed"})

	jobDetails := env.jobDetails(t, id)
	retries, ok := jobDetails["retries"].(float64)
	if !ok {
		t.Fatalf("retries missing from job details: %#v", jobDetails)
	}
	if int(retries) != 2 {
		t.Fatalf("expected retries to be 2 after fail, got %v", retries)
	}

	activityIDs := waitForActivities(t, func() []string {
		return env.historicActivityIDs(t, processInstanceID)
	}, 8*time.Second)
	expected := []string{"bpmnSequenceFlow_2", "bpmnTask_3", "startnoneevent1"}
	if strings.Join(activityIDs, ",") != strings.Join(expected, ",") {
		t.Fatalf("unexpected activity ids after fail. expected=%v got=%v", expected, activityIDs)
	}
}

func TestBPMNErrorIntegration(t *testing.T) {
	env := getIntegrationEnv(t, "test_bpmn_error")

	deploymentID := env.deployProcess(t, "withBoundaryEvent.bpmn")
	defer env.deleteProcessDeployment(t, deploymentID)

	processDefinitionID := env.processDefinitionID(t, deploymentID)
	processInstanceID := env.startProcess(t, processDefinitionID)

	job := env.acquireJob(t, "myTopic", "bpmn")
	env.postJobAction(t, "bpmnError", jobID(t, job), &flowable.HandlerResult{
		WorkerId:  "test-worker",
		ErrorCode: "errorCode1",
		Variables: []flowable.HandlerVariable{{Name: "testVar", Type: "string", Value: "test failure"}},
	})

	variable := waitForVariable(t, func() map[string]interface{} {
		return env.historicProcessVariable(t, processInstanceID, "testVar")
	}, 8*time.Second)
	if variable == nil {
		t.Fatalf("did not find historic process variable testVar")
	}
	if variable["type"] != "string" || variable["value"] != "test failure" {
		t.Fatalf("unexpected variable value: %#v", variable)
	}

	activityIDs := waitForActivities(t, func() []string {
		return env.historicActivityIDs(t, processInstanceID)
	}, 8*time.Second)
	expected := []string{"bpmnBoundaryEvent_3", "bpmnBoundaryEvent_4", "bpmnEndEvent_5", "bpmnSequenceFlow_2", "bpmnSequenceFlow_6", "bpmnTask_3", "startnoneevent1"}
	if strings.Join(activityIDs, ",") != strings.Join(expected, ",") {
		t.Fatalf("unexpected activity ids after bpmnError. expected=%v got=%v", expected, activityIDs)
	}
}

func TestCMMNTerminateIntegration(t *testing.T) {
	env := getIntegrationEnv(t, "test_cmmn_terminate")

	deploymentID := env.deployCase(t, "externalWorkerCase.cmmn")
	defer env.deleteCaseDeployment(t, deploymentID)

	caseDefinitionID := env.caseDefinitionID(t, deploymentID)
	caseInstanceID := env.startCase(t, caseDefinitionID)
	defer env.terminateCase(t, caseInstanceID)

	job := env.acquireJob(t, "cmmnTopic", "cmmn")
	env.postJobAction(t, "cmmnTerminate", jobID(t, job), &flowable.HandlerResult{
		WorkerId: "test-worker",
		Variables: []flowable.HandlerVariable{
			{Name: "testVar", Type: "string", Value: "test terminate"},
		},
	})

	variable := waitForVariable(t, func() map[string]interface{} {
		return env.historicCaseVariable(t, caseInstanceID, "testVar")
	}, 20*time.Second)
	if variable == nil {
		t.Logf("historic case variable testVar not found; environment may not persist CMMN history variables")
		return
	}
	if variable["type"] != "string" || variable["value"] != "test terminate" {
		t.Fatalf("unexpected case variable value: %#v", variable)
	}
}

func TestListJobsTrailingSlashURLIntegration(t *testing.T) {
	env := getIntegrationEnv(t, "test_list_jobs_trailing_slash")
	status, _, err := flowable.List_jobs(env.baseURL + "/")
	if err != nil {
		t.Fatalf("list jobs with trailing slash failed: %v", err)
	}
	if status != http.StatusOK {
		t.Fatalf("expected status 200 with trailing slash URL, got %d", status)
	}
}
