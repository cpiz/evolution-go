package whatsmeow_service

import (
	"errors"
	"testing"

	"github.com/evolution-foundation/evolution-go/pkg/config"
	"go.mau.fi/whatsmeow/store"
)

func TestInitializeWhatsAppHandshakeVersion(t *testing.T) {
	original := store.GetWAVersion()
	t.Cleanup(func() { store.SetWAVersion(original) })
	for _, tc := range []struct {
		name      string
		cfg       config.Config
		fetched   *clientVersion
		fetchErr  error
		want      store.WAVersionContainer
		source    string
		wantFetch bool
		wantErr   bool
	}{
		{name: "web version updates actual handshake", fetched: &clientVersion{2, 3000, 1052769028}, want: store.WAVersionContainer{2, 3000, 1052769028}, source: "web", wantFetch: true},
		{name: "explicit version skips network", cfg: config.Config{WhatsappVersionMajor: 2, WhatsappVersionMinor: 3000, WhatsappVersionPatch: 1052556849}, want: store.WAVersionContainer{2, 3000, 1052556849}, source: "environment"},
		{name: "fetch failure does not silently use old version", fetchErr: errors.New("network unavailable"), wantFetch: true, wantErr: true},
		{name: "partial configuration rejected", cfg: config.Config{WhatsappVersionMajor: 2}, wantErr: true},
		{name: "negative configuration rejected", cfg: config.Config{WhatsappVersionMajor: 2, WhatsappVersionMinor: 3000, WhatsappVersionPatch: -1}, wantErr: true},
		{name: "invalid fetched version rejected", fetched: &clientVersion{2, 3000, 0}, wantFetch: true, wantErr: true},
		{name: "missing fetched version rejected", wantFetch: true, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := store.GetWAVersion()
			called := false
			got, source, err := initializeWhatsAppVersion(&tc.cfg, func() (*clientVersion, error) {
				called = true
				return tc.fetched, tc.fetchErr
			})
			if called != tc.wantFetch {
				t.Fatalf("fetch called=%v, want %v", called, tc.wantFetch)
			}
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected initialization error")
				}
				if store.GetWAVersion() != before {
					t.Fatal("failed initialization changed handshake version")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want || source != tc.source {
				t.Fatalf("got %v/%s, want %v/%s", got, source, tc.want, tc.source)
			}
			payload := store.BaseClientPayload.GetUserAgent().GetAppVersion()
			if store.GetWAVersion() != tc.want || payload.GetPrimary() != tc.want[0] || payload.GetSecondary() != tc.want[1] || payload.GetTertiary() != tc.want[2] {
				t.Fatalf("actual handshake payload not updated: %v", payload)
			}
		})
	}
}
