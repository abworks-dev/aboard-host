//go:build windows && amd64

package security

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	platform "github.com/abworks-dev/aboard-host/internal/platform/windows"
)

func TestBootstrapGateMaterialUUIDv4AndLease(t *testing.T) {
	root := filepath.Join(gateRoot(t), "install")
	if err := platform.CreatePrivateDirectory(root); err != nil {
		t.Fatal(err)
	}
	lease, err := platform.Acquire(root, platform.Installation, true, false)
	if err != nil {
		t.Fatal(err)
	}
	m, pin, err := InitializeWithLease(root, lease)
	if err != nil {
		t.Fatal(err)
	}
	defer m.Clear()
	for _, id := range []string{m.InstallationID, m.TenantID, m.PrincipalID} {
		if !canonicalUUIDv4(id) {
			t.Fatal("generated ID is not canonical UUIDv4")
		}
	}
	loaded, err := LoadWithLease(root, lease, pin)
	if err != nil {
		t.Fatal(err)
	}
	defer loaded.Clear()
	if loaded != m {
		t.Fatal("lease-aware material reload changed identity/key")
	}
	if err = lease.ValidateInstallation(root); err != nil {
		t.Fatal("load released installation lease", err)
	}
	if _, err = Load(root); err == nil {
		t.Fatal("independent load bypassed long-lived host.lock")
	}
	if _, err = LoadWithLease(filepath.Dir(root), lease, pin); err == nil {
		t.Fatal("wrong root accepted")
	}
	if _, err = LoadWithLease(root, nil, pin); err == nil {
		t.Fatal("nil lease accepted")
	}
	if _, err = LoadWithLease(root, lease, nil); err == nil {
		t.Fatal("nil pin accepted")
	}
	otherRoot := filepath.Join(gateRoot(t), "other-install")
	if err = platform.CreatePrivateDirectory(otherRoot); err != nil {
		t.Fatal(err)
	}
	otherLease, err := platform.Acquire(otherRoot, platform.Installation, true, false)
	if err != nil {
		t.Fatal(err)
	}
	otherMaterial, otherPin, err := InitializeWithLease(otherRoot, otherLease)
	if err != nil {
		t.Fatal(err)
	}
	defer otherMaterial.Clear()
	if _, err = LoadWithLease(root, lease, otherPin); err == nil {
		t.Fatal("foreign lease pin accepted")
	}
	if _, err = LoadWithLease(otherRoot, otherLease, pin); err == nil {
		t.Fatal("foreign installation material accepted")
	}
	if err = otherPin.Close(); err != nil {
		t.Fatal(err)
	}
	if err = otherLease.Close(); err != nil {
		t.Fatal(err)
	}
	workspaceRoot := filepath.Join(gateRoot(t), "workspace")
	if err = os.MkdirAll(filepath.Join(workspaceRoot, ".aboard", "locks"), 0700); err != nil {
		t.Fatal(err)
	}
	workspaceLease, err := platform.Acquire(workspaceRoot, platform.Workspace, true, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = LoadWithLease(workspaceRoot, workspaceLease, pin); err == nil {
		t.Fatal("workspace lease granted installation authority")
	}
	if err = workspaceLease.Close(); err != nil {
		t.Fatal(err)
	}
	if err = pin.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err = LoadWithLease(root, lease, pin); err == nil {
		t.Fatal("closed material pin accepted")
	}
	if err = lease.Close(); err != nil {
		t.Fatal(err)
	}
	if m2, err := Load(root); err != nil {
		t.Fatal(err)
	} else {
		m2.Clear()
	}
}

func TestBootstrapGateRejectNoncanonicalMaterialIdentity(t *testing.T) {
	for _, field := range []string{"installation", "tenant", "principal"} {
		for _, kind := range []string{"legacy-hex", "uppercase", "version", "variant", "short", "missing", "unknown-after-key"} {
			t.Run(field+"/"+kind, func(t *testing.T) {
				root := filepath.Join(gateRoot(t), "install")
				if err := Initialize(root); err != nil {
					t.Fatal(err)
				}
				path := filepath.Join(root, materialName)
				blob, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				plain, err := Unprotect(blob)
				if err != nil {
					t.Fatal(err)
				}
				defer clear(plain)
				var m diskMaterial
				if err = json.Unmarshal(plain, &m); err != nil {
					t.Fatal(err)
				}
				defer clear(m.Key)
				id := &m.InstallationID
				if field == "tenant" {
					id = &m.TenantID
				}
				if field == "principal" {
					id = &m.PrincipalID
				}
				switch kind {
				case "legacy-hex":
					*id = strings.ReplaceAll(*id, "-", "")
				case "uppercase":
					*id = "AAAAAAAA-AAAA-4AAA-8AAA-AAAAAAAAAAAA"
				case "version":
					*id = "aaaaaaaa-aaaa-1aaa-8aaa-aaaaaaaaaaaa"
				case "variant":
					*id = "aaaaaaaa-aaaa-4aaa-7aaa-aaaaaaaaaaaa"
				case "short":
					*id = "a"
				case "missing":
					*id = ""
				}
				altered, err := json.Marshal(m)
				if err != nil {
					t.Fatal(err)
				}
				if kind == "unknown-after-key" {
					altered = append(altered[:len(altered)-1], []byte(`,"Unexpected":true}`)...)
				}
				defer clear(altered)
				encrypted, err := Protect(altered)
				if err != nil {
					t.Fatal(err)
				}
				defer clear(encrypted)
				if err = os.WriteFile(path, encrypted, 0600); err != nil {
					t.Fatal(err)
				}
				if _, err = Load(root); err == nil {
					t.Fatal("noncanonical identity accepted")
				}
				after, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if string(after) != string(encrypted) {
					t.Fatal("invalid identity repaired")
				}
			})
		}
	}
}
