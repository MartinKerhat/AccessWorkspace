package keyvault

import (
	"fmt"
	"net/http"
	"testing"
)

func TestIsNotFoundDetectsAzure404(t *testing.T) {
	err := RequestError{StatusCode: http.StatusNotFound, Body: "not found"}
	if !IsNotFound(err) {
		t.Fatalf("expected 404 request error to be treated as not found")
	}
}

func TestIsNotFoundIgnoresOtherErrors(t *testing.T) {
	if IsNotFound(RequestError{StatusCode: http.StatusForbidden, Body: "forbidden"}) {
		t.Fatalf("expected non-404 request error not to be treated as not found")
	}
	if IsNotFound(fmt.Errorf("network failed")) {
		t.Fatalf("expected generic error not to be treated as not found")
	}
}
