package router

import "testing"

func TestShouldExposeSwagger(t *testing.T) {
	tests := []struct {
		env  string
		want bool
	}{
		{env: "dev", want: true},
		{env: "development", want: true},
		{env: "staging", want: true},
		{env: "prod", want: false},
		{env: "production", want: false},
		{env: "", want: false},
	}

	for _, tt := range tests {
		if got := shouldExposeSwagger(tt.env); got != tt.want {
			t.Fatalf("shouldExposeSwagger(%q) = %v, want %v", tt.env, got, tt.want)
		}
	}
}
