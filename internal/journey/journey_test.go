package journey

import (
	"os"
	"testing"
)

func mustNew(t *testing.T, acts ...Action) Journey {
	t.Helper()
	j, err := New("j", "d", acts)
	if err != nil {
		t.Fatal(err)
	}
	return j
}

func TestFlowPreserved(t *testing.T) {
	orig := mustNew(t,
		"Tap LOGIN",
		"Verify PRODUCTS shown",
		"Type \"Dika\" in the First Name field",
		"Type \"QA\" in the Last Name field",
		"Tap CONTINUE",
		"Verify total shown",
	)
	tests := []struct {
		name   string
		healed Journey
		locked int
		ok     bool
	}{
		{"reword+insert+delete after prefix", mustNew(t,
			"Tap LOGIN",
			"Verify PRODUCTS shown",
			"Type \"Dika\" in the Full Name field",
			"Type \"Main St 1\" in the Address Line 1 field",
			"Tap the \"To Payment\" button",
			"Verify total shown",
		), 2, true},
		{"whitespace only", mustNew(t,
			"Tap  LOGIN", "Verify PRODUCTS   shown", "Type \"Dika\" in the First Name field",
			"Type \"QA\" in the Last Name field", "Tap CONTINUE", "Verify total shown",
		), 2, true},
		{"assertion reworded", mustNew(t,
			"Tap LOGIN", "Verify PRODUCTS shown", "Tap CONTINUE", "Verify order total shown",
		), 2, false},
		{"assertion removed", mustNew(t,
			"Tap LOGIN", "Verify PRODUCTS shown", "Tap CONTINUE",
		), 2, false},
		{"assertions reordered", mustNew(t,
			"Tap LOGIN", "Verify total shown", "Tap CONTINUE", "Verify PRODUCTS shown",
		), 0, false},
		{"passed prefix changed", mustNew(t,
			"Tap Login", "Verify PRODUCTS shown", "Tap CONTINUE", "Verify total shown",
		), 2, false},
		{"zero journey", Journey{}, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := FlowPreserved(orig, tt.healed, tt.locked)
			if v.OK != tt.ok {
				t.Fatalf("OK=%v want %v (%s)", v.OK, tt.ok, v.Reason)
			}
		})
	}
}

func TestIndexOf(t *testing.T) {
	j := mustNew(t, "Tap \"Hamburger\" menu", "Tap the \"Sauce Labs Backpack\" product to open its details, then tap \"ADD TO CART\"")
	if got := j.IndexOf("tap \"hamburger\"   menu"); got != 0 {
		t.Fatalf("exact: got %d", got)
	}
	if got := j.IndexOf("Tap the \"Sauce Labs Backpack\" product to open its details"); got != 1 {
		t.Fatalf("prefix: got %d", got)
	}
	if got := j.IndexOf("nope"); got != -1 {
		t.Fatalf("missing: got %d", got)
	}
}

func TestCodecRoundTrip(t *testing.T) {
	for _, f := range []string{"testdata/login.journey.xml", "testdata/checkout.journey.xml"} {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		j, err := Decode(data)
		if err != nil {
			t.Fatal(err)
		}
		j2, err := Decode(Encode(j))
		if err != nil {
			t.Fatal(err)
		}
		if j.Name() != j2.Name() || len(j.Actions()) != len(j2.Actions()) {
			t.Fatalf("%s: round trip mismatch", f)
		}
		for i, a := range j.Actions() {
			if a != j2.Actions()[i] {
				t.Fatalf("%s action %d: %q != %q", f, i, a, j2.Actions()[i])
			}
		}
	}
}

func TestDecodeRejectsEmpty(t *testing.T) {
	if _, err := Decode([]byte(`<journey name="x"><actions></actions></journey>`)); err == nil {
		t.Fatal("want error for no actions")
	}
	if _, err := Decode([]byte(`<journey`)); err == nil {
		t.Fatal("want error for bad xml")
	}
}

func TestEncodeMatchesHandWritten(t *testing.T) {
	data, err := os.ReadFile("testdata/login.journey.xml")
	if err != nil {
		t.Fatal(err)
	}
	j, _ := Decode(data)
	if got := string(Encode(j)); got != string(data) && got != string(data)+"\n" {
		t.Fatalf("canonical encoding differs from hand-written file:\n%s", got)
	}
}
