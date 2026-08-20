package fingerprint_test

import (
	"testing"

	"github.com/MohdFarhanS/sentinelix-backend/pkg/fingerprint"
)

func TestCompute_SameFingerprint_WhenOnlyLineNumberDiffers(t *testing.T) {
	msg := "TypeError: Cannot read property 'id' of undefined"
	// simulasi: file sama, tapi line number geser karena ada refactor
	// (nambah beberapa baris kode di atas titik error)
	stackBeforeRefactor := "at checkout.go:88\nsome caller frame"
	stackAfterRefactor := "at checkout.go:95\nsome caller frame"

	fp1 := fingerprint.Compute(msg, stackBeforeRefactor)
	fp2 := fingerprint.Compute(msg, stackAfterRefactor)

	if fp1 != fp2 {
		t.Errorf("fingerprint must match even if the line number shifts, it should work %s vs %s", fp1, fp2)
	}
}

func TestCompute_DifferentFingerprint_WhenMessageDiffers(t *testing.T) {
	stack := "at checkout.go:88"

	fp1 := fingerprint.Compute("TypeError A", stack)
	fp2 := fingerprint.Compute("TypeError B", stack)

	if fp1 == fp2 {
		t.Errorf("fingerprint must be different for each message")
	}
}

func TestCompute_DifferentFingerprint_WhenTopFrameFileDiffers(t *testing.T) {
	msg := "TypeError: same message"

	fp1 := fingerprint.Compute(msg, "at checkout.go:10")
	fp2 := fingerprint.Compute(msg, "at payment.go:10")

	if fp1 == fp2 {
		t.Errorf("fingerprints must be different for different files/frames")
	}
}

func TestCompute_Deterministic(t *testing.T) {
	msg := "Deterministic test"
	stack := "at foo.go:1"

	if fingerprint.Compute(msg, stack) != fingerprint.Compute(msg, stack) {
		t.Errorf("fingerprint must be deterministic for the same input")
	}
}

func TestCompute_HandlesEmptyStackTrace(t *testing.T) {
	fp := fingerprint.Compute("message tanpa stack trace", "")
	if fp == "" {
		t.Errorf("fingerprint must not be empty, even if the stack trace is empty")
	}
}

func TestCompute_OutputIs64CharHex(t *testing.T) {
	fp := fingerprint.Compute("msg", "at foo.go:1")
	if len(fp) != 64 {
		t.Errorf("expected length 64 characters (sha256 hex), get %d", len(fp))
	}
}