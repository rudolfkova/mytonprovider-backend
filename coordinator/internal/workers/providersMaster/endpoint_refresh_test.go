package providersmaster

import (
	"testing"
	"time"

	"mytonprovider-coordinator/internal/models/db"
)

func TestIsEndpointStateStale(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.May, 23, 12, 0, 0, 0, time.UTC)
	recent := now.Add(-2 * time.Hour)
	old := now.Add(-26 * time.Hour)

	tests := []struct {
		name  string
		state db.ProviderEndpointState
		ttl   time.Duration
		want  bool
	}{
		{
			name: "missing storage endpoint",
			state: db.ProviderEndpointState{
				StorageIP:   "",
				StoragePort: 0,
				UpdatedAt:   &recent,
			},
			ttl:  24 * time.Hour,
			want: true,
		},
		{
			name: "missing updated at",
			state: db.ProviderEndpointState{
				StorageIP:   "1.2.3.4",
				StoragePort: 1000,
				UpdatedAt:   nil,
			},
			ttl:  24 * time.Hour,
			want: true,
		},
		{
			name: "updated within ttl",
			state: db.ProviderEndpointState{
				StorageIP:   "1.2.3.4",
				StoragePort: 1000,
				UpdatedAt:   &recent,
			},
			ttl:  24 * time.Hour,
			want: false,
		},
		{
			name: "updated older than ttl",
			state: db.ProviderEndpointState{
				StorageIP:   "1.2.3.4",
				StoragePort: 1000,
				UpdatedAt:   &old,
			},
			ttl:  24 * time.Hour,
			want: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := isEndpointStateStale(tt.state, tt.ttl, now)
			if got != tt.want {
				t.Fatalf("isEndpointStateStale() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHasValidResolvedStorageEndpoint(t *testing.T) {
	t.Parallel()

	validKey := make([]byte, 32)
	tests := []struct {
		name string
		ip   db.ProviderIP
		want bool
	}{
		{
			name: "valid endpoint",
			ip: db.ProviderIP{
				Storage: db.IPInfo{
					IP:        "1.1.1.1",
					Port:      1000,
					PublicKey: validKey,
				},
			},
			want: true,
		},
		{
			name: "empty storage ip",
			ip: db.ProviderIP{
				Storage: db.IPInfo{
					IP:        "",
					Port:      1000,
					PublicKey: validKey,
				},
			},
			want: false,
		},
		{
			name: "invalid public key length",
			ip: db.ProviderIP{
				Storage: db.IPInfo{
					IP:        "1.1.1.1",
					Port:      1000,
					PublicKey: make([]byte, 10),
				},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := hasValidResolvedStorageEndpoint(tt.ip)
			if got != tt.want {
				t.Fatalf("hasValidResolvedStorageEndpoint() = %v, want %v", got, tt.want)
			}
		})
	}
}
