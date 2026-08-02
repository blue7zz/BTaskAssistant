package reasonix

import (
	"testing"

	"reasonix/bridge"
)

func TestKernelBridge(t *testing.T) {
	home, err := bridge.KernelInfo()
	if err != nil {
		t.Fatalf("bridge.KernelInfo: %v", err)
	}
	if home == "" {
		t.Fatal("ReasonixHomeDir empty")
	}
}
