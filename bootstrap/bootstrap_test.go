package bootstrap

import "testing"

func TestParse(t *testing.T) {
	for _, tc := range []struct {
		name, in string
		mode     string
		wantErr  bool
	}{
		{name: "empty", in: "", mode: TrustAppend},
		{
			name: "exclusive", in: `{"version":1,"trust":{"mode":"exclusive"}}`,
			mode: TrustExclusive,
		},
		{name: "no mode", in: `{"version":1}`, mode: TrustAppend},
		{name: "unknown key", in: `{"version":1,"env":{"LD_PRELOAD":"x"}}`, wantErr: true},
		{name: "unknown mode", in: `{"version":1,"trust":{"mode":"none"}}`, wantErr: true},
		{name: "future version", in: `{"version":2}`, wantErr: true},
		{name: "trailing", in: `{"version":1} {}`, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, err := Parse([]byte(tc.in))
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if err == nil && c.TrustMode() != tc.mode {
				t.Errorf("mode = %q, want %q", c.TrustMode(), tc.mode)
			}
		})
	}
}

func TestForGuestRoundTrip(t *testing.T) {
	for guest, mode := range map[string]string{
		"": TrustExclusive, "linux": TrustExclusive,
		"windows": TrustAppend, "darwin": TrustAppend,
	} {
		c, err := Parse(ForGuest(guest).Marshal())
		if err != nil {
			t.Fatal(err)
		}
		if c.TrustMode() != mode {
			t.Errorf("%q: mode %q, want %q", guest, c.TrustMode(), mode)
		}
	}
}
