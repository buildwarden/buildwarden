package hyperv

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/buildwarden/buildwarden/driver"
)

func TestSlotCount(t *testing.T) {
	for n, ok := range map[int]bool{1: true, DefaultSlots: true, MaxSlots: true,
		0: false, -1: false, MaxSlots + 1: false} {
		err := ValidateSlotCount(n)
		var fe *driver.FixableError
		if ok != (err == nil) || (err != nil && (!errors.As(err, &fe) ||
			!strings.Contains(fe.Fix, "pass --slots between 1 and 64"))) {
			t.Errorf("ValidateSlotCount(%d) = %v", n, err)
		}
	}
	o := SetupOptions{}
	o.applyDefaults()
	if o.Slots != 4 {
		t.Errorf("default slots = %d, want 4", o.Slots)
	}
}

// TestSetupRefusesSlotCount checks an out-of-range count is refused before
// setup touches anything (it runs on any OS, before the platform check).
func TestSetupRefusesSlotCount(t *testing.T) {
	err := RunSetup(context.Background(), io.Discard, SetupOptions{Slots: MaxSlots + 1})
	if err == nil || !strings.Contains(err.Error(), "--slots 65 is out of range") {
		t.Fatalf("err = %v", err)
	}
}
