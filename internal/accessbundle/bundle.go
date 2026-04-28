package accessbundle

import (
	"encoding/json"
	"errors"
	"strings"
)

const (
	DefaultVersion              uint32 = 1
	DefaultServiceDiscoveryMode        = "static"
)

type Bundle struct {
	Version          uint32
	Overlay          Overlay
	ServiceDiscovery ServiceDiscovery
	Roodox           Roodox
}

type Overlay struct {
	Provider   string
	JoinConfig json.RawMessage
}

type ServiceDiscovery struct {
	Mode          string
	Host          string
	Port          uint32
	UseTLS        bool
	TLSServerName string
}

type Roodox struct {
	ServerID     string
	DeviceGroup  string
	SharedSecret string
	DeviceID     string
	DeviceName   string
	DeviceRole   string
}

type fileBundle struct {
	Version           uint32          `json:"version"`
	OverlayProvider   string          `json:"overlayProvider"`
	OverlayJoinConfig json.RawMessage `json:"overlayJoinConfig"`
	ServiceDiscovery  fileDiscovery   `json:"serviceDiscovery"`
	Roodox            fileRoodox      `json:"roodox"`
}

type fileDiscovery struct {
	Mode          string `json:"mode"`
	Host          string `json:"host"`
	Port          uint32 `json:"port"`
	UseTLS        bool   `json:"useTLS"`
	TLSServerName string `json:"tlsServerName"`
}

type fileRoodox struct {
	ServerID     string `json:"serverID"`
	DeviceGroup  string `json:"deviceGroup"`
	SharedSecret string `json:"sharedSecret"`
	DeviceID     string `json:"deviceID,omitempty"`
	DeviceName   string `json:"deviceName,omitempty"`
	DeviceRole   string `json:"deviceRole,omitempty"`
}

func (b Bundle) Normalize() Bundle {
	b.Version = nonZeroUint32(b.Version, DefaultVersion)
	b.Overlay.Provider = normalizeSlug(b.Overlay.Provider)
	if len(b.Overlay.JoinConfig) == 0 {
		b.Overlay.JoinConfig = json.RawMessage([]byte("{}"))
	}
	b.ServiceDiscovery.Mode = normalizeSlugWithDefault(b.ServiceDiscovery.Mode, DefaultServiceDiscoveryMode)
	b.ServiceDiscovery.Host = strings.TrimSpace(b.ServiceDiscovery.Host)
	b.ServiceDiscovery.TLSServerName = strings.TrimSpace(b.ServiceDiscovery.TLSServerName)
	b.Roodox.ServerID = strings.TrimSpace(b.Roodox.ServerID)
	b.Roodox.DeviceGroup = strings.TrimSpace(b.Roodox.DeviceGroup)
	b.Roodox.SharedSecret = strings.TrimSpace(b.Roodox.SharedSecret)
	b.Roodox.DeviceID = strings.TrimSpace(b.Roodox.DeviceID)
	b.Roodox.DeviceName = strings.TrimSpace(b.Roodox.DeviceName)
	b.Roodox.DeviceRole = strings.TrimSpace(b.Roodox.DeviceRole)
	return b
}

func (b Bundle) Validate() error {
	b = b.Normalize()

	if b.Overlay.Provider == "" {
		return errors.New("overlay provider is required")
	}
	if !json.Valid(b.Overlay.JoinConfig) {
		return errors.New("overlay join config must be valid JSON")
	}
	if b.ServiceDiscovery.Mode == "" {
		return errors.New("service discovery mode is required")
	}
	switch b.ServiceDiscovery.Mode {
	case "static":
		if b.ServiceDiscovery.Host == "" {
			return errors.New("service discovery host is required for static mode")
		}
		if b.ServiceDiscovery.Port == 0 {
			return errors.New("service discovery port is required for static mode")
		}
	}
	if b.Roodox.ServerID == "" {
		return errors.New("roodox server id is required")
	}
	if b.Roodox.DeviceGroup == "" {
		return errors.New("roodox device group is required")
	}
	return nil
}

func (b Bundle) MarshalJSONFile() (string, error) {
	b = b.Normalize()
	if err := b.Validate(); err != nil {
		return "", err
	}

	payload := fileBundle{
		Version:           b.Version,
		OverlayProvider:   b.Overlay.Provider,
		OverlayJoinConfig: append(json.RawMessage(nil), b.Overlay.JoinConfig...),
		ServiceDiscovery: fileDiscovery{
			Mode:          b.ServiceDiscovery.Mode,
			Host:          b.ServiceDiscovery.Host,
			Port:          b.ServiceDiscovery.Port,
			UseTLS:        b.ServiceDiscovery.UseTLS,
			TLSServerName: b.ServiceDiscovery.TLSServerName,
		},
		Roodox: fileRoodox{
			ServerID:     b.Roodox.ServerID,
			DeviceGroup:  b.Roodox.DeviceGroup,
			SharedSecret: b.Roodox.SharedSecret,
			DeviceID:     b.Roodox.DeviceID,
			DeviceName:   b.Roodox.DeviceName,
			DeviceRole:   b.Roodox.DeviceRole,
		},
	}

	raw, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func normalizeSlug(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return ""
	}
	var out []rune
	lastDash := false
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			out = append(out, r)
			lastDash = false
		case r >= '0' && r <= '9':
			out = append(out, r)
			lastDash = false
		case !lastDash:
			out = append(out, '-')
			lastDash = true
		}
	}
	return strings.Trim(string(out), "-")
}

func normalizeSlugWithDefault(value, fallback string) string {
	value = normalizeSlug(value)
	if value == "" {
		return normalizeSlug(fallback)
	}
	return value
}

func nonZeroUint32(value, fallback uint32) uint32 {
	if value == 0 {
		return fallback
	}
	return value
}
