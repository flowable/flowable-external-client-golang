package worker_test

import (
	"encoding/json"
	"testing"

	"github.com/gdharley/flowable-external-client-golang/flowable"
)

func TestGetVar_String(t *testing.T) {
	vars := []flowable.HandlerVariable{{Name: "s", Type: "string", Value: "hello"}}
	if got := flowable.GetVar(vars, "s"); got != "hello" {
		t.Fatalf("expected hello, got %q", got)
	}
}

func TestGetVar_IntegerNumber(t *testing.T) {
	vars := []flowable.HandlerVariable{{Name: "n", Type: "number", Value: float64(5)}}
	if got := flowable.GetVar(vars, "n"); got != "5" {
		t.Fatalf("expected 5, got %q", got)
	}
}

func TestGetVar_FloatNumber(t *testing.T) {
	vars := []flowable.HandlerVariable{{Name: "f", Type: "number", Value: float64(3.14)}}
	if got := flowable.GetVar(vars, "f"); got != "3.14" {
		t.Fatalf("expected 3.14, got %q", got)
	}
}

func TestGetVar_Bool(t *testing.T) {
	vars := []flowable.HandlerVariable{{Name: "b", Type: "boolean", Value: true}}
	if got := flowable.GetVar(vars, "b"); got != "true" {
		t.Fatalf("expected true, got %q", got)
	}
}

func TestGetVar_Object(t *testing.T) {
	obj := map[string]interface{}{"k": "v"}
	vars := []flowable.HandlerVariable{{Name: "o", Type: "json", Value: obj}}
	got := flowable.GetVar(vars, "o")
	// Marshal the expected value to compare
	expBytes, _ := json.Marshal(obj)
	if got != string(expBytes) {
		t.Fatalf("expected %q, got %q", string(expBytes), got)
	}
}

func TestGetVar_Missing(t *testing.T) {
	vars := []flowable.HandlerVariable{{Name: "x", Type: "string", Value: "y"}}
	if got := flowable.GetVar(vars, "nope"); got != "" {
		t.Fatalf("expected empty string for missing var, got %q", got)
	}
}
