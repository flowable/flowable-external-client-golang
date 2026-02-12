package integration_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"testing"

	"github.com/flowable/flowable-external-client-golang/flowable"
)

var (
	baseURL        string
	authUser       = "admin"
	authPass       = "test"
	workerID       = "test-worker"
	testHTTPClient *http.Client = http.DefaultClient
)

func getBaseURL() string {
	if url := os.Getenv("FLOWABLE_URL"); url != "" {
		return url
	}
	return "http://localhost:8090/flowable-work"
}

func fixturesDir() string {
	_, filename, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(filename), "fixtures")
}

// --- HTTP helpers ---

func doRequest(t *testing.T, method, url string, body io.Reader, contentType string) (int, []byte) {
	t.Helper()
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	req.SetBasicAuth(authUser, authPass)
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := testHTTPClient.Do(req)
	if err != nil {
		t.Fatalf("request to %s failed: %v", url, err)
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response: %v", err)
	}
	return resp.StatusCode, respBody
}

func httpGet(t *testing.T, url string) (int, []byte) {
	t.Helper()
	return doRequest(t, "GET", url, nil, "")
}

func httpPostJSON(t *testing.T, url string, payload interface{}) (int, []byte) {
	t.Helper()
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("failed to marshal payload: %v", err)
	}
	return doRequest(t, "POST", url, bytes.NewReader(data), "application/json")
}

func httpDelete(t *testing.T, url string) int {
	t.Helper()
	status, _ := doRequest(t, "DELETE", url, nil, "")
	return status
}

func httpPostMultipart(t *testing.T, url, fieldName, filePath string) (int, []byte) {
	t.Helper()
	file, err := os.Open(filePath)
	if err != nil {
		t.Fatalf("failed to open file %s: %v", filePath, err)
	}
	defer file.Close()

	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, err := writer.CreateFormFile(fieldName, filepath.Base(filePath))
	if err != nil {
		t.Fatalf("failed to create form file: %v", err)
	}
	if _, err := io.Copy(part, file); err != nil {
		t.Fatalf("failed to copy file: %v", err)
	}
	writer.Close()

	return doRequest(t, "POST", url, &buf, writer.FormDataContentType())
}

// --- BPMN helpers ---

func deployProcess(t *testing.T, processFile string) string {
	t.Helper()
	filePath := filepath.Join(fixturesDir(), "processes", processFile)
	status, body := httpPostMultipart(t, baseURL+"/process-api/repository/deployments", "file", filePath)
	if status != 201 {
		t.Fatalf("deploy process: expected 201, got %d: %s", status, string(body))
	}
	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("deploy process: failed to parse response: %v", err)
	}
	return result["id"].(string)
}

func deleteDeployment(t *testing.T, deploymentID string) {
	t.Helper()
	status := httpDelete(t, baseURL+"/process-api/repository/deployments/"+deploymentID+"?cascade=true")
	if status != 204 {
		t.Logf("delete deployment %s: expected 204, got %d", deploymentID, status)
	}
}

func getProcessDefinitionID(t *testing.T, deploymentID string) string {
	t.Helper()
	status, body := httpGet(t, baseURL+"/process-api/repository/process-definitions?deploymentId="+deploymentID)
	if status != 200 {
		t.Fatalf("get process definition: expected 200, got %d: %s", status, string(body))
	}
	var result map[string]interface{}
	json.Unmarshal(body, &result)
	data := result["data"].([]interface{})
	if len(data) != 1 {
		t.Fatalf("expected 1 process definition, got %d", len(data))
	}
	return data[0].(map[string]interface{})["id"].(string)
}

func startProcess(t *testing.T, processDefinitionID string) string {
	t.Helper()
	payload := map[string]string{"processDefinitionId": processDefinitionID}
	status, body := httpPostJSON(t, baseURL+"/process-api/runtime/process-instances", payload)
	if status != 201 {
		t.Fatalf("start process: expected 201, got %d: %s", status, string(body))
	}
	var result map[string]interface{}
	json.Unmarshal(body, &result)
	return result["id"].(string)
}

func terminateProcess(t *testing.T, processInstanceID string) {
	t.Helper()
	status := httpDelete(t, baseURL+"/process-api/runtime/process-instances/"+processInstanceID)
	if status != 204 && status != 404 {
		t.Logf("terminate process %s: got status %d", processInstanceID, status)
	}
}

func getProcessVariable(t *testing.T, processInstanceID, variableName string) map[string]interface{} {
	t.Helper()
	url := fmt.Sprintf("%s/process-api/history/historic-variable-instances?processInstanceId=%s&variableName=%s",
		baseURL, processInstanceID, variableName)
	status, body := httpGet(t, url)
	if status != 200 {
		t.Fatalf("get process variable: expected 200, got %d: %s", status, string(body))
	}
	var result map[string]interface{}
	json.Unmarshal(body, &result)
	data := result["data"].([]interface{})
	if len(data) >= 1 {
		return data[0].(map[string]interface{})["variable"].(map[string]interface{})
	}
	return nil
}

func executedActivityIDs(t *testing.T, processInstanceID string) []string {
	t.Helper()
	url := fmt.Sprintf("%s/process-api/history/historic-activity-instances?processInstanceId=%s&size=100",
		baseURL, processInstanceID)
	status, body := httpGet(t, url)
	if status != 200 {
		t.Fatalf("get activity instances: expected 200, got %d: %s", status, string(body))
	}
	var result map[string]interface{}
	json.Unmarshal(body, &result)
	data := result["data"].([]interface{})
	var ids []string
	for _, item := range data {
		m := item.(map[string]interface{})
		if id, ok := m["activityId"].(string); ok {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids
}

// --- CMMN helpers ---

func deployCase(t *testing.T, caseFile string) string {
	t.Helper()
	filePath := filepath.Join(fixturesDir(), "cases", caseFile)
	status, body := httpPostMultipart(t, baseURL+"/cmmn-api/cmmn-repository/deployments", "file", filePath)
	if status != 201 {
		t.Fatalf("deploy case: expected 201, got %d: %s", status, string(body))
	}
	var result map[string]interface{}
	json.Unmarshal(body, &result)
	return result["id"].(string)
}

func deleteCaseDeployment(t *testing.T, deploymentID string) {
	t.Helper()
	status := httpDelete(t, baseURL+"/cmmn-api/cmmn-repository/deployments/"+deploymentID+"?cascade=true")
	if status != 204 {
		t.Logf("delete case deployment %s: expected 204, got %d", deploymentID, status)
	}
}

func getCaseDefinitionID(t *testing.T, deploymentID string) string {
	t.Helper()
	status, body := httpGet(t, baseURL+"/cmmn-api/cmmn-repository/case-definitions?deploymentId="+deploymentID)
	if status != 200 {
		t.Fatalf("get case definition: expected 200, got %d: %s", status, string(body))
	}
	var result map[string]interface{}
	json.Unmarshal(body, &result)
	data := result["data"].([]interface{})
	if len(data) != 1 {
		t.Fatalf("expected 1 case definition, got %d", len(data))
	}
	return data[0].(map[string]interface{})["id"].(string)
}

func startCase(t *testing.T, caseDefinitionID string) string {
	t.Helper()
	payload := map[string]string{"caseDefinitionId": caseDefinitionID}
	status, body := httpPostJSON(t, baseURL+"/cmmn-api/cmmn-runtime/case-instances", payload)
	if status != 201 {
		t.Fatalf("start case: expected 201, got %d: %s", status, string(body))
	}
	var result map[string]interface{}
	json.Unmarshal(body, &result)
	return result["id"].(string)
}

func terminateCase(t *testing.T, caseInstanceID string) {
	t.Helper()
	status := httpDelete(t, baseURL+"/cmmn-api/cmmn-runtime/case-instances/"+caseInstanceID)
	if status != 204 && status != 404 {
		t.Logf("terminate case %s: got status %d", caseInstanceID, status)
	}
}

func getCaseVariable(t *testing.T, caseInstanceID, variableName string) map[string]interface{} {
	t.Helper()
	url := fmt.Sprintf("%s/cmmn-api/cmmn-history/historic-variable-instances?caseInstanceId=%s&variableName=%s",
		baseURL, caseInstanceID, variableName)
	status, body := httpGet(t, url)
	if status != 200 {
		t.Fatalf("get case variable: expected 200, got %d: %s", status, string(body))
	}
	var result map[string]interface{}
	json.Unmarshal(body, &result)
	data := result["data"].([]interface{})
	if len(data) >= 1 {
		return data[0].(map[string]interface{})["variable"].(map[string]interface{})
	}
	return nil
}

// --- Job helpers ---

// acquireJobForInstance acquires jobs and returns the one matching the given instance ID.
// This handles the case where stale jobs from previous runs exist on the server.
func acquireJobForInstance(t *testing.T, topic, scopeType, instanceID string) map[string]interface{} {
	t.Helper()
	req := flowable.AcquireRequest{
		Topic:           topic,
		LockDuration:    "PT1M",
		NumberOfTasks:   10,
		NumberOfRetries: 5,
		WorkerId:        workerID,
		ScopeType:       scopeType,
		URL:             baseURL,
	}
	jobs, body, status, err := flowable.Acquire_jobs(req)
	if err != nil {
		t.Fatalf("Acquire_jobs error: %v", err)
	}
	if status != 200 {
		t.Fatalf("Acquire_jobs: expected 200, got %d: %s", status, body)
	}
	if len(jobs) == 0 {
		t.Fatal("no jobs acquired")
	}

	for _, j := range jobs {
		job := j.(map[string]interface{})
		pid, _ := job["processInstanceId"].(string)
		sid, _ := job["scopeId"].(string)
		if pid == instanceID || sid == instanceID {
			return job
		}
	}

	t.Fatalf("no job found for instance %s among %d acquired jobs", instanceID, len(jobs))
	return nil
}

func completeJob(t *testing.T, jobID string, variables []map[string]interface{}) {
	t.Helper()
	payload := map[string]interface{}{
		"workerId": workerID,
	}
	if variables != nil {
		payload["variables"] = variables
	}
	status, body := httpPostJSON(t, baseURL+"/external-job-api/acquire/jobs/"+jobID+"/complete", payload)
	if status != 200 && status != 204 {
		t.Fatalf("complete job: expected 200/204, got %d: %s", status, string(body))
	}
}

func failJob(t *testing.T, jobID string) {
	t.Helper()
	payload := map[string]interface{}{
		"workerId": workerID,
	}
	status, body := httpPostJSON(t, baseURL+"/external-job-api/acquire/jobs/"+jobID+"/fail", payload)
	if status != 200 && status != 204 {
		t.Fatalf("fail job: expected 200/204, got %d: %s", status, string(body))
	}
}

func bpmnErrorJob(t *testing.T, jobID string, variables []map[string]interface{}, errorCode string) {
	t.Helper()
	payload := map[string]interface{}{
		"workerId": workerID,
	}
	if variables != nil {
		payload["variables"] = variables
	}
	if errorCode != "" {
		payload["errorCode"] = errorCode
	}
	status, body := httpPostJSON(t, baseURL+"/external-job-api/acquire/jobs/"+jobID+"/bpmnError", payload)
	if status != 200 && status != 204 {
		t.Fatalf("bpmn error job: expected 200/204, got %d: %s", status, string(body))
	}
}

func cmmnTerminateJob(t *testing.T, jobID string, variables []map[string]interface{}) {
	t.Helper()
	payload := map[string]interface{}{
		"workerId": workerID,
	}
	if variables != nil {
		payload["variables"] = variables
	}
	status, body := httpPostJSON(t, baseURL+"/external-job-api/acquire/jobs/"+jobID+"/cmmnTerminate", payload)
	if status != 200 && status != 204 {
		t.Fatalf("cmmn terminate job: expected 200/204, got %d: %s", status, string(body))
	}
}

func getJob(t *testing.T, jobID string) map[string]interface{} {
	t.Helper()
	status, body := httpGet(t, baseURL+"/external-job-api/jobs/"+jobID)
	if status != 200 {
		t.Fatalf("get job: expected 200, got %d: %s", status, string(body))
	}
	var result map[string]interface{}
	json.Unmarshal(body, &result)
	return result
}
