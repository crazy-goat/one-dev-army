package version

import (
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    Version
		wantErr bool
	}{
		{"valid version", "1.2.3", Version{1, 2, 3}, false},
		{"valid with v prefix", "v1.2.3", Version{1, 2, 3}, false},
		{"zero version", "0.0.0", Version{0, 0, 0}, false},
		{"large numbers", "999.888.777", Version{999, 888, 777}, false},
		{"invalid format", "1.2", Version{}, true},
		{"invalid major", "a.2.3", Version{}, true},
		{"invalid minor", "1.b.3", Version{}, true},
		{"invalid patch", "1.2.c", Version{}, true},
		{"empty string", "", Version{}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("Parse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("Parse() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestVersion_BumpMajor(t *testing.T) {
	v := Version{1, 2, 3}
	got := v.BumpMajor()
	want := Version{2, 0, 0}
	if got != want {
		t.Errorf("BumpMajor() = %v, want %v", got, want)
	}
}

func TestVersion_BumpMinor(t *testing.T) {
	v := Version{1, 2, 3}
	got := v.BumpMinor()
	want := Version{1, 3, 0}
	if got != want {
		t.Errorf("BumpMinor() = %v, want %v", got, want)
	}
}

func TestVersion_BumpPatch(t *testing.T) {
	v := Version{1, 2, 3}
	got := v.BumpPatch()
	want := Version{1, 2, 4}
	if got != want {
		t.Errorf("BumpPatch() = %v, want %v", got, want)
	}
}

func TestVersion_String(t *testing.T) {
	v := Version{1, 2, 3}
	got := v.String()
	want := "1.2.3"
	if got != want {
		t.Errorf("String() = %v, want %v", got, want)
	}
}
