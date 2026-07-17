package whatsmeow_service

import (
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/evolution-foundation/evolution-go/pkg/config"
)

func TestGetAuthContainerRetriesFailureAndReusesSuccess(t *testing.T) {
	sharedAuthContainerMu.Lock()
	sharedAuthContainer = nil
	sharedAuthContainerMu.Unlock()

	t.Cleanup(func() {
		sharedAuthContainerMu.Lock()
		if sharedAuthContainer != nil {
			_ = sharedAuthContainer.Close()
		}
		sharedAuthContainer = nil
		sharedAuthContainerMu.Unlock()
	})

	badService := whatsmeowService{
		config: &config.Config{},
		exPath: filepath.Join(t.TempDir(), "does-not-exist"),
	}
	if _, err := badService.getAuthContainer(); err == nil {
		t.Fatal("expected auth container creation to fail for a missing directory")
	}

	sharedAuthContainerMu.Lock()
	failedContainer := sharedAuthContainer
	sharedAuthContainerMu.Unlock()
	if failedContainer != nil {
		t.Fatalf("failed container should not be cached: %#v", failedContainer)
	}

	validPath := t.TempDir()
	if err := os.MkdirAll(filepath.Join(validPath, "dbdata"), 0o755); err != nil {
		t.Fatalf("create SQLite data directory: %v", err)
	}
	validService := whatsmeowService{
		config: &config.Config{},
		exPath: validPath,
	}

	first, err := validService.getAuthContainer()
	if err != nil {
		t.Fatalf("retry after failure should succeed: %v", err)
	}
	if first == nil {
		t.Fatal("auth container is nil after successful initialization")
	}

	second, err := validService.getAuthContainer()
	if err != nil {
		t.Fatalf("reuse shared auth container: %v", err)
	}
	if first != second {
		t.Fatal("auth container was recreated instead of reused")
	}
}
