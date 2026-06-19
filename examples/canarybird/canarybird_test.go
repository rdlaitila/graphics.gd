package main

import "testing"

func TestItoa(t *testing.T) {
	cases := map[int]string{
		0: "0", 1: "1", 9: "9", 10: "10", 42: "42", 100: "100",
		-1: "-1", -42: "-42", 1234567: "1234567",
	}
	for in, want := range cases {
		if got := itoa(in); got != want {
			t.Fatalf("itoa(%d) = %q, want %q", in, got, want)
		}
	}
}
