package worker_test

import (
	"testing"

	"github.com/gdharley/flowable-external-client-golang/flowable"
	"github.com/gdharley/flowable-external-client-golang/worker"
)

func TestExternalWorkerSuccess(t *testing.T) {
	status, res := worker.ExternalWorker(200, `{"foo":"bar"}`)
	if status != flowable.HandlerSuccess {
		t.Fatalf("expected success status, got %v", status)
	}
	if res == nil {
		t.Fatalf("expected non-nil result")
	}
	if len(res.Variables) == 0 {
		t.Fatalf("expected at least one variable")
	}
}

func TestExternalWorkerFailOnBadStatus(t *testing.T) {
	status, _ := worker.ExternalWorker(500, "")
	if status != flowable.HandlerFail {
		t.Fatalf("expected fail status, got %v", status)
	}
}
