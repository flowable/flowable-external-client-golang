package worker_test

import (
	"testing"

	"github.com/flowable/flowable-external-client-golang/flowable"
)

func TestExtractVariablesFromBody_MapFormat(t *testing.T) {
	body := `{"variables":{"foo":{"value":"bar","type":"string"},"num":{"value":42,"type":"number"}}}`
	vars, err := flowable.ExtractVariablesFromBody(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(vars) != 2 {
		t.Fatalf("expected 2 variables, got %d", len(vars))
	}
	m := make(map[string]flowable.HandlerVariable)
	for _, v := range vars {
		m[v.Name] = v
	}
	if v, ok := m["foo"]; !ok || v.Value != "bar" {
		t.Fatalf("missing or wrong foo variable: %#v", v)
	}
	if v, ok := m["num"]; !ok {
		t.Fatalf("missing num variable")
	} else {
		// numeric values are unmarshaled as float64
		if _, ok := v.Value.(float64); !ok {
			t.Fatalf("expected num to be numeric (float64), got %T", v.Value)
		}
	}
}

func TestExtractVariablesFromBody_ArrayFormat(t *testing.T) {
	body := `{"variables":[{"name":"a","type":"string","value":"x"},{"name":"b","type":"number","value":5}]}`
	vars, err := flowable.ExtractVariablesFromBody(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(vars) != 2 {
		t.Fatalf("expected 2 variables, got %d", len(vars))
	}
	m := make(map[string]flowable.HandlerVariable)
	for _, v := range vars {
		m[v.Name] = v
	}
	if v, ok := m["a"]; !ok || v.Value != "x" {
		t.Fatalf("missing or wrong a variable: %#v", v)
	}
	if v, ok := m["b"]; !ok {
		t.Fatalf("missing b variable")
	} else {
		if _, ok := v.Value.(float64); !ok {
			t.Fatalf("expected b to be numeric (float64), got %T", v.Value)
		}
	}
}

func TestExtractVariablesFromBody_NoVariables(t *testing.T) {
	body := `{"foo":"bar"}`
	vars, err := flowable.ExtractVariablesFromBody(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(vars) != 0 {
		t.Fatalf("expected 0 variables, got %d", len(vars))
	}
}

func TestExtractVariablesFromBody_InvalidJSON(t *testing.T) {
	body := `{"variables":`
	_, err := flowable.ExtractVariablesFromBody(body)
	if err == nil {
		t.Fatalf("expected error for invalid JSON, got nil")
	}
}
