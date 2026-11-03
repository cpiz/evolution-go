package whatsmeow_service

import (
	"fmt"

	"github.com/evolution-foundation/evolution-go/pkg/config"
	"go.mau.fi/whatsmeow/store"
)

// InitializeWhatsAppVersion 必须在启动 HTTP 服务和任何账号连接前调用。
// SetWAVersion 同时更新握手 AppVersion 和配对哈希；DeviceProps.Version
// 只是伴随设备信息，不能替代握手版本。连接运行期间不得修改这些全局状态。
func InitializeWhatsAppVersion(cfg *config.Config) (store.WAVersionContainer, string, error) {
	return initializeWhatsAppVersion(cfg, fetchWhatsAppWebVersion)
}

func initializeWhatsAppVersion(cfg *config.Config, fetch func() (*clientVersion, error)) (store.WAVersionContainer, string, error) {
	version := clientVersion{Major: cfg.WhatsappVersionMajor, Minor: cfg.WhatsappVersionMinor, Patch: cfg.WhatsappVersionPatch}
	source := "environment"
	if version.Major == 0 && version.Minor == 0 && version.Patch == 0 {
		fetched, err := fetch()
		if err != nil {
			return store.WAVersionContainer{}, "", fmt.Errorf("fetch current WhatsApp Web version: %w", err)
		}
		if fetched == nil {
			return store.WAVersionContainer{}, "", fmt.Errorf("WhatsApp Web version is missing")
		}
		version = *fetched
		source = "web"
	}
	for _, part := range []int{version.Major, version.Minor, version.Patch} {
		if part <= 0 || uint64(part) > uint64(^uint32(0)) {
			return store.WAVersionContainer{}, "", fmt.Errorf("invalid WhatsApp version %d.%d.%d: all components must be positive uint32 values", version.Major, version.Minor, version.Patch)
		}
	}
	effective := store.WAVersionContainer{uint32(version.Major), uint32(version.Minor), uint32(version.Patch)}
	store.SetWAVersion(effective)
	return effective, source, nil
}
