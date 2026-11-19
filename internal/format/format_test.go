package format

import "testing"

func TestParseCopyMode(t *testing.T) {
	cases := []struct {
		in      string
		want    CopyMode
		wantErr bool
	}{
		{in: "", want: CopyModeAuto},
		{in: "  AUTO ", want: CopyModeAuto},
		{in: "copy", want: CopyModeCopy},
		{in: "HardLink", want: CopyModeHardlink},
		{in: "symlink", wantErr: true},
	}

	for _, tc := range cases {
		got, err := ParseCopyMode(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Fatalf("ParseCopyMode(%q) expected error", tc.in)
			}
			continue
		}
		if err != nil {
			t.Fatalf("ParseCopyMode(%q) unexpected error: %v", tc.in, err)
		}
		if got != tc.want {
			t.Fatalf("ParseCopyMode(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
