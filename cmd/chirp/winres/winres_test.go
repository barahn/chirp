// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

// Package winres holds what chirp.exe carries besides code: its version
// information, its icon and its application manifest. scripts/gen-winres.sh
// compiles winres.json and icon.png into ../rsrc_windows_amd64.syso, which
// go build links into the Windows binary and ignores everywhere else.
//
// The tests here read those resources back out of a Windows executable, the
// way Explorer's Properties → Details and an antivirus scanner do.
package winres

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tc-hib/winres"
	"github.com/tc-hib/winres/version"
)

// devVersion is what the committed .syso says. A release stamps its own.
const devVersion = "0.0.0-dev"

// TestCommittedResources links the committed .syso into an empty Windows
// program -- cmd/chirp itself needs cgo, which does not cross-compile -- and
// reads the resources back.
func TestCommittedResources(t *testing.T) {
	dir := t.TempDir()
	syso, err := os.ReadFile(filepath.Join("..", "rsrc_windows_amd64.syso"))
	if err != nil {
		t.Fatalf("the committed .syso: %v (run scripts/gen-winres.sh)", err)
	}
	for name, data := range map[string][]byte{
		"go.mod":                  []byte("module stub\n\ngo 1.26\n"),
		"main.go":                 []byte("package main\n\nfunc main() {}\n"),
		"rsrc_windows_amd64.syso": syso,
	} {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	exe := filepath.Join(dir, "chirp.exe")
	cmd := exec.Command("go", "build", "-o", exe, ".")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOOS=windows", "GOARCH=amd64", "CGO_ENABLED=0", "GOWORK=off", "GOFLAGS=")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build the stub: %v\n%s", err, out)
	}
	checkExe(t, exe, devVersion)
}

// TestShippedExe checks a real chirp.exe when CI points it at one, with the
// version it was stamped with: CHIRP_EXE=path CHIRP_VERSION=1.2.3-rc.4.
func TestShippedExe(t *testing.T) {
	exe := os.Getenv("CHIRP_EXE")
	if exe == "" {
		t.Skip("CHIRP_EXE is not set")
	}
	want := os.Getenv("CHIRP_VERSION")
	if want == "" {
		t.Fatal("CHIRP_EXE is set without CHIRP_VERSION")
	}
	checkExe(t, exe, want)
}

func checkExe(t *testing.T, exe, ver string) {
	t.Helper()
	f, err := os.Open(exe) // #nosec G304 -- a path the test built or CI named
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	rs, err := winres.LoadFromEXE(f)
	if err != nil {
		t.Fatalf("read the resources of %s: %v", exe, err)
	}
	t.Run("version", func(t *testing.T) { checkVersion(t, rs, ver) })
	t.Run("icon", func(t *testing.T) { checkIcon(t, rs) })
	t.Run("manifest", func(t *testing.T) { checkManifest(t, rs) })
}

func checkVersion(t *testing.T, rs *winres.ResourceSet, ver string) {
	var data []byte
	rs.WalkType(winres.RT_VERSION, func(_ winres.Identifier, _ uint16, d []byte) bool {
		data = d
		return false
	})
	if data == nil {
		t.Fatal("no VERSIONINFO")
	}
	vi, err := version.FromBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := vi.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Fixed struct {
			FileVersion    string `json:"file_version"`
			ProductVersion string `json:"product_version"`
		} `json:"fixed"`
		Info map[string]map[string]string `json:"info"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	strs, ok := got.Info["0409"]
	if !ok {
		t.Fatalf("no en-US string table in %s", raw)
	}
	for key, want := range map[string]string{
		version.CompanyName:      "Fabrintek Engenharia Digital Ltda",
		version.ProductName:      "Chirp",
		version.FileDescription:  "Chirp remote support",
		version.InternalName:     "chirp",
		version.OriginalFilename: "chirp.exe",
		version.LegalCopyright:   "Copyright (C) 2026 Fabrintek Engenharia Digital Ltda. AGPL-3.0-only.",
		version.ProductVersion:   ver,
		version.FileVersion:      ver,
	} {
		if strs[key] != want {
			t.Errorf("%s = %q, want %q", key, strs[key], want)
		}
	}
	// The fixed part is four numbers: 1.2.3-rc.4 becomes 1.2.3.0.
	fixed := strings.SplitN(ver, "-", 2)[0] + ".0"
	if got.Fixed.FileVersion == "" {
		got.Fixed.FileVersion = "0.0.0.0" // MarshalJSON leaves out a zero version
	}
	if got.Fixed.ProductVersion == "" {
		got.Fixed.ProductVersion = "0.0.0.0"
	}
	if got.Fixed.FileVersion != fixed {
		t.Errorf("fixed file version = %q, want %q", got.Fixed.FileVersion, fixed)
	}
	if got.Fixed.ProductVersion != got.Fixed.FileVersion {
		t.Errorf("fixed product version = %q, file version %q", got.Fixed.ProductVersion, got.Fixed.FileVersion)
	}
}

// checkIcon wants the bird as Explorer's first icon, up to 256 pixels.
func checkIcon(t *testing.T, rs *winres.ResourceSet) {
	icon, err := rs.GetIcon(winres.Name("APP"))
	if err != nil {
		t.Fatalf("no APP icon: %v", err)
	}
	var ico bytes.Buffer
	if err := icon.SaveICO(&ico); err != nil {
		t.Fatal(err)
	}
	b := ico.Bytes()
	if len(b) < 6 {
		t.Fatal("empty icon")
	}
	sizes := map[int]bool{}
	for i := range int(binary.LittleEndian.Uint16(b[4:6])) {
		w := int(b[6+16*i])
		if w == 0 {
			w = 256 // an ICO entry writes 256 as 0
		}
		sizes[w] = true
	}
	for _, want := range []int{16, 32, 48, 256} {
		if !sizes[want] {
			t.Errorf("no %dpx image in the icon (has %v)", want, sizes)
		}
	}
}

// checkManifest wants a program that never asks for elevation, scales per
// monitor the way GLFW (under Fyne) sets itself at start-up, and declares
// Windows 10 and 11, so the OS does not shim it as a legacy app.
func checkManifest(t *testing.T, rs *winres.ResourceSet) {
	data := rs.Get(winres.RT_MANIFEST, winres.ID(1), winres.LCIDDefault)
	if data == nil {
		t.Fatal("no manifest")
	}
	m, err := winres.AppManifestFromXML(data)
	if err != nil {
		t.Fatal(err)
	}
	if m.ExecutionLevel != winres.AsInvoker || m.UIAccess {
		t.Errorf("execution level %v (uiAccess %v), want asInvoker", m.ExecutionLevel, m.UIAccess)
	}
	if m.DPIAwareness != winres.DPIPerMonitorV2 {
		t.Errorf("DPI awareness %v, want per monitor v2", m.DPIAwareness)
	}
	if m.Compatibility != winres.Win10AndAbove {
		t.Errorf("supported OS %v, want Windows 10 and above", m.Compatibility)
	}
	if !m.UseCommonControlsV6 {
		t.Error("common controls v6 not requested: message boxes would draw in the Windows 95 style")
	}
}
