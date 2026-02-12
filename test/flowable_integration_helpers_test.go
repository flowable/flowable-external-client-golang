package worker_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/flowable/flowable-external-client-golang/flowable"
	"gopkg.in/dnaeon/go-vcr.v2/recorder"
)

type integrationEnv struct {
	baseURL  string
	username string
	password string
	client   *http.Client
}

type deploymentResponse struct {
	ID string `json:"id"`
}

type pagedResponse struct {
	Total int                      `json:"total"`
	Data  []map[string]interface{} `json:"data"`
}

func getIntegrationEnv(t *testing.T, cassetteName string) *integrationEnv {
	t.Helper()

	baseURL := os.Getenv("FLOWABLE_BASE_URL")
	if baseURL == "" {
		baseURL = "http://localhost:8090"
	}
	baseURL = strings.TrimRight(baseURL, "/")

	username := os.Getenv("FLOWABLE_USERNAME")
	if username == "" {
		username = "admin"
	}
	password := os.Getenv("FLOWABLE_PASSWORD")
	if password == "" {
		password = "test"
	}

	env := &integrationEnv{
		baseURL:  baseURL,
		username: username,
		password: password,
	}

	cassettePath := filepath.Join("fixtures", "cassettes", cassetteName)
	cassetteFile := cassettePath + ".yaml"
	flowableIntegration := os.Getenv("FLOWABLE_INTEGRATION") == "1"

	mode := recorder.ModeReplayingOrRecording
	if !flowableIntegration {
		if _, err := os.Stat(cassetteFile); err == nil {
			mode = recorder.ModeReplaying
		} else {
			t.Skipf("set FLOWABLE_INTEGRATION=1 to record cassette %s", cassetteFile)
		}
	}

	switch strings.ToLower(os.Getenv("FLOWABLE_CASSETTE_MODE")) {
	case "replay":
		mode = recorder.ModeReplaying
	case "record":
		mode = recorder.ModeRecording
	}

	if err := os.MkdirAll(filepath.Dir(cassettePath), 0o755); err != nil {
		t.Fatalf("create cassette directory: %v", err)
	}
	rec, err := recorder.NewAsMode(cassettePath, mode, nil)
	if err != nil {
		t.Fatalf("create cassette recorder: %v", err)
	}
	t.Cleanup(func() {
		_ = rec.Stop()
	})

	env.client = &http.Client{Timeout: 20 * time.Second, Transport: rec}

	flowable.SetAuth(username, password)
	flowable.SetHTTPClient(env.client)
	t.Cleanup(func() {
		flowable.SetHTTPClient(nil)
	})

	return env
}

func (e *integrationEnv) newRequest(method, path string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequest(method, e.baseURL+path, body)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(e.username, e.password)
	return req, nil
}

func (e *integrationEnv) jsonRequest(method, path string, payload interface{}) (int, []byte, error) {
	var body io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return -1, nil, err
		}
		body = bytes.NewReader(b)
	}

	req, err := e.newRequest(method, path, body)
	if err != nil {
		return -1, nil, err
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")

	resp, err := e.client.Do(req)
	if err != nil {
		return -1, nil, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, err
	}
	return resp.StatusCode, respBody, nil
}

func (e *integrationEnv) deployProcess(t *testing.T, processFile string) string {
	t.Helper()
	return e.deployFile(t, "/process-api/repository/deployments", filepath.Join("fixtures", "processes", processFile))
}

func (e *integrationEnv) deployCase(t *testing.T, caseFile string) string {
	t.Helper()
	return e.deployFile(t, "/cmmn-api/cmmn-repository/deployments", filepath.Join("fixtures", "cases", caseFile))
}

func (e *integrationEnv) deployFile(t *testing.T, endpoint, sourcePath string) string {
	t.Helper()

	f, err := os.Open(sourcePath)
	if err != nil {
		t.Fatalf("open fixture %s: %v", sourcePath, err)
	}
	defer f.Close()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", filepath.Base(sourcePath))
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := io.Copy(part, f); err != nil {
		t.Fatalf("copy fixture into multipart: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart: %v", err)
	}

	req, err := e.newRequest(http.MethodPost, endpoint, &body)
	if err != nil {
		t.Fatalf("create deploy request: %v", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := e.client.Do(req)
	if err != nil {
		t.Fatalf("deploy request failed: %v", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("deploy failed: status=%d body=%s", resp.StatusCode, string(respBody))
	}

	var out deploymentResponse
	if err := json.Unmarshal(respBody, &out); err != nil {
		t.Fatalf("unmarshal deploy response: %v", err)
	}
	if out.ID == "" {
		t.Fatalf("deploy response missing id: %s", string(respBody))
	}
	return out.ID
}

func (e *integrationEnv) processDefinitionID(t *testing.T, deploymentID string) string {
	t.Helper()
	status, body, err := e.jsonRequest(http.MethodGet, "/process-api/repository/process-definitions?deploymentId="+deploymentID, nil)
	if err != nil {
		t.Fatalf("get process definition id: %v", err)
	}
	if status != http.StatusOK {
		t.Fatalf("get process definition status=%d body=%s", status, string(body))
	}
	var out pagedResponse
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("unmarshal process definition response: %v", err)
	}
	if len(out.Data) == 0 {
		t.Fatalf("no process definitions for deployment %s", deploymentID)
	}
	id, _ := out.Data[0]["id"].(string)
	if id == "" {
		t.Fatalf("process definition id missing in response: %s", string(body))
	}
	return id
}

func (e *integrationEnv) caseDefinitionID(t *testing.T, deploymentID string) string {
	t.Helper()
	status, body, err := e.jsonRequest(http.MethodGet, "/cmmn-api/cmmn-repository/case-definitions?deploymentId="+deploymentID, nil)
	if err != nil {
		t.Fatalf("get case definition id: %v", err)
	}
	if status != http.StatusOK {
		t.Fatalf("get case definition status=%d body=%s", status, string(body))
	}
	var out pagedResponse
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("unmarshal case definition response: %v", err)
	}
	if len(out.Data) == 0 {
		t.Fatalf("no case definitions for deployment %s", deploymentID)
	}
	id, _ := out.Data[0]["id"].(string)
	if id == "" {
		t.Fatalf("case definition id missing in response: %s", string(body))
	}
	return id
}

func (e *integrationEnv) startProcess(t *testing.T, processDefinitionID string) string {
	t.Helper()
	status, body, err := e.jsonRequest(http.MethodPost, "/process-api/runtime/process-instances", map[string]string{"processDefinitionId": processDefinitionID})
	if err != nil {
		t.Fatalf("start process: %v", err)
	}
	if status != http.StatusCreated {
		t.Fatalf("start process status=%d body=%s", status, string(body))
	}
	var out deploymentResponse
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("unmarshal start process response: %v", err)
	}
	if out.ID == "" {
		t.Fatalf("start process response missing id: %s", string(body))
	}
	return out.ID
}

func (e *integrationEnv) startCase(t *testing.T, caseDefinitionID string) string {
	t.Helper()
	status, body, err := e.jsonRequest(http.MethodPost, "/cmmn-api/cmmn-runtime/case-instances", map[string]string{"caseDefinitionId": caseDefinitionID})
	if err != nil {
		t.Fatalf("start case: %v", err)
	}
	if status != http.StatusCreated {
		t.Fatalf("start case status=%d body=%s", status, string(body))
	}
	var out deploymentResponse
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("unmarshal start case response: %v", err)
	}
	if out.ID == "" {
		t.Fatalf("start case response missing id: %s", string(body))
	}
	return out.ID
}

func (e *integrationEnv) terminateProcess(t *testing.T, processInstanceID string) {
	t.Helper()
	status, body, err := e.jsonRequest(http.MethodDelete, "/process-api/runtime/process-instances/"+processInstanceID, nil)
	if err != nil {
		t.Fatalf("terminate process: %v", err)
	}
	if status != http.StatusNoContent {
		t.Fatalf("terminate process status=%d body=%s", status, string(body))
	}
}

func (e *integrationEnv) deleteProcessDeployment(t *testing.T, deploymentID string) {
	t.Helper()
	status, body, err := e.jsonRequest(http.MethodDelete, "/process-api/repository/deployments/"+deploymentID, nil)
	if err != nil {
		t.Fatalf("delete process deployment: %v", err)
	}
	if status != http.StatusNoContent {
		t.Fatalf("delete process deployment status=%d body=%s", status, string(body))
	}
}

func (e *integrationEnv) deleteCaseDeployment(t *testing.T, deploymentID string) {
	t.Helper()
	status, body, err := e.jsonRequest(http.MethodDelete, "/cmmn-api/cmmn-repository/deployments/"+deploymentID, nil)
	if err != nil {
		t.Fatalf("delete case deployment: %v", err)
	}
	if status != http.StatusNoContent {
		t.Fatalf("delete case deployment status=%d body=%s", status, string(body))
	}
}

func (e *integrationEnv) acquireJob(t *testing.T, topic, scopeType string) map[string]interface{} {
	t.Helper()
	deadline := time.Now().Add(12 * time.Second)
	for time.Now().Before(deadline) {
		jobs, body, status, err := flowable.Acquire_jobs(flowable.AcquireRequest{
			Topic:           topic,
			LockDuration:    "PT10S",
			NumberOfTasks:   1,
			NumberOfRetries: 3,
			WorkerId:        "test-worker",
			ScopeType:       scopeType,
			URL:             e.baseURL,
		})
		if err != nil {
			t.Fatalf("acquire jobs failed: status=%d body=%s err=%v", status, body, err)
		}
		if status != http.StatusOK {
			t.Fatalf("acquire jobs status=%d body=%s", status, body)
		}
		if len(jobs) == 0 {
			time.Sleep(250 * time.Millisecond)
			continue
		}
		job, ok := jobs[0].(map[string]interface{})
		if !ok {
			t.Fatalf("unexpected acquired job shape: %#v", jobs[0])
		}
		return job
	}
	t.Fatalf("expected at least 1 acquired job for topic %q scopeType %q within timeout", topic, scopeType)
	return nil
}

func jobID(t *testing.T, job map[string]interface{}) string {
	t.Helper()
	id, _ := job["id"].(string)
	if id == "" {
		t.Fatalf("acquired job missing id: %#v", job)
	}
	return id
}

func hasAcquiredVariable(job map[string]interface{}, variableName, expectedValue string) bool {
	vars, ok := job["variables"].([]interface{})
	if !ok {
		return false
	}
	for _, raw := range vars {
		vm, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		name, _ := vm["name"].(string)
		if name != variableName {
			continue
		}
		value := fmt.Sprintf("%v", vm["value"])
		if value == expectedValue {
			return true
		}
	}
	return false
}

func (e *integrationEnv) postJobAction(t *testing.T, action, id string, result *flowable.HandlerResult) {
	t.Helper()
	status, body, err := e.jsonRequest(http.MethodPost, "/external-job-api/acquire/jobs/"+id+"/"+action, result)
	if err != nil {
		t.Fatalf("job %s request failed: %v", action, err)
	}
	if status != http.StatusNoContent {
		t.Fatalf("job %s status=%d body=%s", action, status, string(body))
	}
}

func (e *integrationEnv) historicProcessVariable(t *testing.T, processInstanceID, variableName string) map[string]interface{} {
	t.Helper()
	status, body, err := e.jsonRequest(http.MethodGet, "/process-api/history/historic-variable-instances?processInstanceId="+processInstanceID+"&variableName="+variableName, nil)
	if err != nil {
		t.Fatalf("get process variable: %v", err)
	}
	if status != http.StatusOK {
		t.Fatalf("get process variable status=%d body=%s", status, string(body))
	}
	var out pagedResponse
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("unmarshal process variable response: %v", err)
	}
	if len(out.Data) == 0 {
		return nil
	}
	variable, _ := out.Data[0]["variable"].(map[string]interface{})
	return variable
}

func (e *integrationEnv) historicCaseVariable(t *testing.T, caseInstanceID, variableName string) map[string]interface{} {
	t.Helper()
	status, body, err := e.jsonRequest(http.MethodGet, "/cmmn-api/cmmn-history/historic-variable-instances?caseInstanceId="+caseInstanceID+"&variableName="+variableName, nil)
	if err != nil {
		t.Fatalf("get case variable: %v", err)
	}
	if status != http.StatusOK {
		t.Fatalf("get case variable status=%d body=%s", status, string(body))
	}
	var out pagedResponse
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("unmarshal case variable response: %v", err)
	}
	if len(out.Data) == 0 {
		return nil
	}
	variable, _ := out.Data[0]["variable"].(map[string]interface{})
	return variable
}

func (e *integrationEnv) historicActivityIDs(t *testing.T, processInstanceID string) []string {
	t.Helper()
	status, body, err := e.jsonRequest(http.MethodGet, "/process-api/history/historic-activity-instances?processInstanceId="+processInstanceID, nil)
	if err != nil {
		t.Fatalf("get historic activity instances: %v", err)
	}
	if status != http.StatusOK {
		t.Fatalf("get historic activity status=%d body=%s", status, string(body))
	}
	var out pagedResponse
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("unmarshal activity response: %v", err)
	}
	ids := make([]string, 0, len(out.Data))
	for _, row := range out.Data {
		if v, ok := row["activityId"].(string); ok && v != "" {
			ids = append(ids, v)
		}
	}
	sort.Strings(ids)
	return ids
}

func (e *integrationEnv) jobDetails(t *testing.T, id string) map[string]interface{} {
	t.Helper()
	status, body, err := e.jsonRequest(http.MethodGet, "/external-job-api/jobs/"+id, nil)
	if err != nil {
		t.Fatalf("get job details: %v", err)
	}
	if status != http.StatusOK {
		t.Fatalf("get job details status=%d body=%s", status, string(body))
	}
	var out map[string]interface{}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("unmarshal job details response: %v", err)
	}
	return out
}

func (e *integrationEnv) terminateCase(t *testing.T, caseInstanceID string) {
	t.Helper()
	status, body, err := e.jsonRequest(http.MethodDelete, "/cmmn-api/cmmn-runtime/case-instances/"+caseInstanceID, nil)
	if err != nil {
		t.Fatalf("terminate case: %v", err)
	}
	if status == http.StatusNotFound {
		return
	}
	if status != http.StatusNoContent {
		t.Fatalf("terminate case status=%d body=%s", status, string(body))
	}
}

func waitForVariable(t *testing.T, fn func() map[string]interface{}, timeout time.Duration) map[string]interface{} {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if variable := fn(); variable != nil {
			return variable
		}
		time.Sleep(250 * time.Millisecond)
	}
	return nil
}

func waitForActivities(t *testing.T, fn func() []string, timeout time.Duration) []string {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var latest []string
	for time.Now().Before(deadline) {
		latest = fn()
		if len(latest) > 0 {
			return latest
		}
		time.Sleep(250 * time.Millisecond)
	}
	return latest
}
