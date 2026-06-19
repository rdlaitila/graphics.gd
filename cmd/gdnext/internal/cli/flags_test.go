package cli

import (
	"reflect"
	"testing"
)

func TestRewriteShortFlags(t *testing.T) {
	known := map[string]struct{}{
		"goos":   {},
		"goarch": {},
		"cc":     {},
		"port":   {},
	}
	cases := []struct {
		name string
		in   []string
		want []string
	}{
		{
			name: "single-dash multi-char known flag rewritten",
			in:   []string{"gdnext", "build", "-goos", "linux"},
			want: []string{"gdnext", "build", "--goos", "linux"},
		},
		{
			name: "single-dash multi-char known flag with equals rewritten",
			in:   []string{"gdnext", "build", "-goos=linux"},
			want: []string{"gdnext", "build", "--goos=linux"},
		},
		{
			name: "double-dash flag passes through",
			in:   []string{"gdnext", "build", "--goos", "linux"},
			want: []string{"gdnext", "build", "--goos", "linux"},
		},
		{
			name: "single-letter short flag preserved",
			in:   []string{"gdnext", "-h"},
			want: []string{"gdnext", "-h"},
		},
		{
			name: "version short flag preserved",
			in:   []string{"gdnext", "-v"},
			want: []string{"gdnext", "-v"},
		},
		{
			name: "unknown multi-char flag passes through (passthrough to go)",
			in:   []string{"gdnext", "mod", "-mod", "vendor"},
			want: []string{"gdnext", "mod", "-mod", "vendor"},
		},
		{
			name: "double-dash sentinel terminates rewriting",
			in:   []string{"gdnext", "test", "--", "-goos", "linux"},
			want: []string{"gdnext", "test", "--", "-goos", "linux"},
		},
		{
			name: "positional args left alone",
			in:   []string{"gdnext", "doc", "fmt.Println"},
			want: []string{"gdnext", "doc", "fmt.Println"},
		},
		{
			name: "multiple known flags",
			in:   []string{"gdnext", "build", "-goos", "android", "-goarch", "amd64"},
			want: []string{"gdnext", "build", "--goos", "android", "--goarch", "amd64"},
		},
		{
			name: "mixed known and unknown",
			in:   []string{"gdnext", "build", "-goos", "linux", "-tags", "myflag"},
			want: []string{"gdnext", "build", "--goos", "linux", "-tags", "myflag"},
		},
		{
			name: "empty argv",
			in:   nil,
			want: nil,
		},
		{
			name: "argv with only program name",
			in:   []string{"gdnext"},
			want: []string{"gdnext"},
		},
		{
			name: "lone dash kept verbatim",
			in:   []string{"gdnext", "build", "-"},
			want: []string{"gdnext", "build", "-"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := RewriteShortFlags(tc.in, known)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("RewriteShortFlags(%q)\n  got:  %q\n  want: %q", tc.in, got, tc.want)
			}
		})
	}
}
