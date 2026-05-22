package providersmaster

import (
	"crypto/ed25519"
	"testing"

	"mytonprovider-coordinator/internal/models/db"
)

func TestSelectRunChecksEndpoint(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(nil)
	pub := priv.Public().(ed25519.PublicKey)

	provider := db.IPInfo{IP: "1.2.3.4", Port: 39626, PublicKey: pub}
	storage := db.IPInfo{IP: "5.6.7.8", Port: 16015, PublicKey: pub}

	t.Run("provider only", func(t *testing.T) {
		got, ok := selectRunChecksEndpoint(db.ProviderIP{Provider: provider})
		if !ok || got.Port != 39626 {
			t.Fatalf("got %+v ok=%v", got, ok)
		}
	})

	t.Run("storage only", func(t *testing.T) {
		got, ok := selectRunChecksEndpoint(db.ProviderIP{Storage: storage})
		if !ok || got.Port != 16015 {
			t.Fatalf("got %+v ok=%v", got, ok)
		}
	})

	t.Run("prefers provider over storage", func(t *testing.T) {
		got, ok := selectRunChecksEndpoint(db.ProviderIP{Provider: provider, Storage: storage})
		if !ok || got.Port != 39626 {
			t.Fatalf("got %+v ok=%v", got, ok)
		}
	})

	t.Run("both empty", func(t *testing.T) {
		_, ok := selectRunChecksEndpoint(db.ProviderIP{})
		if ok {
			t.Fatal("expected false")
		}
	})

	t.Run("invalid provider falls back to storage", func(t *testing.T) {
		got, ok := selectRunChecksEndpoint(db.ProviderIP{
			Provider: db.IPInfo{IP: "1.2.3.4", Port: 0},
			Storage:  storage,
		})
		if !ok || got.Port != 16015 {
			t.Fatalf("got %+v ok=%v", got, ok)
		}
	})

	// Anchor dead-proof-rates-ok (6f6053…): resolver OK on provider :39626, old RunChecks used storage :16015.
	t.Run("anchor prefers provider port over storage proof port", func(t *testing.T) {
		got, ok := selectRunChecksEndpoint(db.ProviderIP{
			Provider: db.IPInfo{IP: "157.250.201.151", Port: 39626, PublicKey: pub},
			Storage:  db.IPInfo{IP: "157.250.201.151", Port: 16015, PublicKey: pub},
		})
		if !ok || got.Port != 39626 {
			t.Fatalf("got %+v ok=%v, want provider port 39626", got, ok)
		}
	})
}
