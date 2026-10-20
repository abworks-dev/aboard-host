//go:build windows && amd64

// Package host composes existing owners; it contains no business SQL, directory
// mutation, alternate authentication, or recovery commands.
package host

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/abworks-dev/aboard-host/internal/protocol"
)

var ErrConfiguration = errors.New("invalid trusted Host configuration")

type Config struct {
	InstallationRoot string   `json:"installationRoot"`
	AllowedRoots     []string `json:"allowedRoots"`
	Listen           string   `json:"listen"`
	PipeName         string   `json:"pipeName"`
}

func localPath(s string) bool {
	return filepath.IsAbs(s) && filepath.Clean(s) == s && len(filepath.VolumeName(s)) == 2 && !strings.HasPrefix(s, `\\`) && !strings.ContainsAny(s, "/\x00")
}
func (c Config) Validate() error {
	if !localPath(c.InstallationRoot) || c.AllowedRoots == nil {
		return ErrConfiguration
	}
	for _, root := range c.AllowedRoots {
		if !localPath(root) {
			return ErrConfiguration
		}
	}
	host, port, err := net.SplitHostPort(c.Listen)
	ip := net.ParseIP(host)
	n, e := strconv.ParseUint(port, 10, 16)
	if err != nil || e != nil || strconv.FormatUint(n, 10) != port || ip == nil || !ip.IsLoopback() {
		return ErrConfiguration
	}
	if !strings.HasPrefix(c.PipeName, "aboard-g5-") || len(c.PipeName) > 100 || strings.ContainsAny(c.PipeName, `\/:`+"\x00") {
		return ErrConfiguration
	}
	return nil
}
func LoadConfig(path string) (Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return Config{}, ErrConfiguration
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, 8193))
	if err != nil || len(b) > 8192 {
		return Config{}, ErrConfiguration
	}
	value, err := protocol.ParseJSON(b)
	if err != nil {
		return Config{}, ErrConfiguration
	}
	object, ok := value.(map[string]any)
	if !ok || len(object) != 4 {
		return Config{}, ErrConfiguration
	}
	for _, key := range []string{"installationRoot", "allowedRoots", "listen", "pipeName"} {
		if _, ok = object[key]; !ok {
			return Config{}, ErrConfiguration
		}
	}
	var c Config
	if json.Unmarshal(b, &c) != nil || c.Validate() != nil {
		return Config{}, ErrConfiguration
	}
	return c, nil
}
