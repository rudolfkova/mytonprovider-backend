package utils

import "testing"

func TestSameIP(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		a    string
		b    string
		want bool
	}{
		{name: "equal ipv4", a: "1.2.3.4", b: "1.2.3.4", want: true},
		{name: "trimmed", a: " 1.2.3.4 ", b: "1.2.3.4", want: true},
		{name: "ipv4 mapped", a: "127.0.0.1", b: "::ffff:127.0.0.1", want: true},
		{name: "ipv6 equal", a: "2001:db8::1", b: "2001:db8::1", want: true},
		{name: "ipv6 brackets", a: "[2001:db8::1]", b: "2001:db8::1", want: true},
		{name: "host port", a: "1.2.3.4:8080", b: "1.2.3.4", want: true},
		{name: "mismatch", a: "1.2.3.4", b: "5.6.7.8", want: false},
		{name: "empty", a: "1.2.3.4", b: "", want: false},
		{name: "garbage", a: "not-an-ip", b: "1.2.3.4", want: false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := SameIP(tt.a, tt.b)
			if got != tt.want {
				t.Fatalf("SameIP(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}
