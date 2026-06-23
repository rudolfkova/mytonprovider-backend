package tonclient

import (
	"context"
	"errors"
	"log/slog"
	"testing"
)

// Test example
func Test_GetTransactions(t *testing.T) {
	ctx := context.Background()

	logger := slog.Default()

	client, err := NewClient(ctx, "https://ton-blockchain.github.io/testnet-global.config.json", logger)
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	tx, err := client.GetTransactions(ctx, "UQB3d3d3d3d3d3d3d3d3d3d3d3d3d3d3d3d3d3d3d3d3d0x0", 5)
	if err != nil {
		t.Fatalf("GetTransactions failed: %v", err)
	}

	if len(tx) == 0 {
		t.Fatal("expected non-empty transaction list")
	}
}

func TestIsLiteServerHistoryUnavailableErr(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "state already gcd",
			err:  errors.New("lite server error, code 602: state already gc'd"),
			want: true,
		},
		{
			name: "cannot load state",
			err:  errors.New("failed to run contract method get_providers: cannot load state for account"),
			want: true,
		},
		{
			name: "failed to get account state",
			err:  errors.New("lite server error, code 500: failed to get account state"),
			want: true,
		},
		{
			name: "unrelated error",
			err:  errors.New("context deadline exceeded"),
			want: false,
		},
		{
			name: "nil",
			err:  nil,
			want: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := isLiteServerHistoryUnavailableErr(tt.err)
			if got != tt.want {
				t.Fatalf("isLiteServerHistoryUnavailableErr() = %v, want %v", got, tt.want)
			}
		})
	}
}
