// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package main

import (
	"bytes"
	"encoding/binary"
	"encoding/xml"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const iconSVG = "../../cmd/chirp/internal/warm/bird/icon.svg"

func TestBundleVersionComesFromTheTag(t *testing.T) {
	for tag, want := range map[string]string{
		"v1.0.0":        "1.0.0",
		"v0.12.3":       "0.12.3",
		"v1.0.0-rc.1":   "1.0.0", // macOS takes only numbers; the tag says rc
		"v2.1.0-beta.4": "2.1.0",
	} {
		got, err := bundleVersion(tag)
		if err != nil || got != want {
			t.Errorf("bundleVersion(%q) = %q, %v; want %q", tag, got, err, want)
		}
	}
	for _, tag := range []string{"", "1.0.0", "v1.0", "v1.0.0.1", "vx.y.z", "v1.0.0+build", "v01.0.0"} {
		if got, err := bundleVersion(tag); err == nil {
			t.Errorf("bundleVersion(%q) = %q, want an error", tag, got)
		}
	}
}

// The keys macOS reads from the bundle: TCC keys Screen Recording and
// Accessibility to the identifier (barahn/chirp#51), Fyne's language follows
// CFBundleLocalizations (barahn/chirp#152), and the icon is the bird.
func TestInfoPlistDeclaresTheApp(t *testing.T) {
	var buf bytes.Buffer
	if err := writeInfoPlist(&buf, "1.2.3"); err != nil {
		t.Fatal(err)
	}
	d := decodePlist(t, buf.Bytes())
	for key, want := range map[string]string{
		"CFBundleIdentifier":            "app.usechirp.chirp",
		"CFBundleName":                  "Chirp",
		"CFBundleDisplayName":           "Chirp",
		"CFBundleExecutable":            "chirp",
		"CFBundlePackageType":           "APPL",
		"CFBundleInfoDictionaryVersion": "6.0",
		"CFBundleDevelopmentRegion":     "en",
		"CFBundleShortVersionString":    "1.2.3",
		"CFBundleVersion":               "1.2.3",
		"CFBundleIconFile":              "chirp.icns",
		"LSMinimumSystemVersion":        minimumSystemVersion,
		"NSHighResolutionCapable":       "true",
	} {
		if got := d[key]; got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}
	if got := d["CFBundleLocalizations"]; got != "[en pt-BR]" {
		t.Errorf("CFBundleLocalizations = %s, want [en pt-BR]", got)
	}
	if !strings.HasPrefix(buf.String(), `<?xml version="1.0" encoding="UTF-8"?>`+"\n"+`<!DOCTYPE plist`) {
		t.Errorf("not an XML property list:\n%s", buf.String())
	}
}

// Every size Finder, the Dock and the switcher draw, from 16 to 1024 pixels,
// as PNGs in an icns container.
func TestIcnsCarriesEverySize(t *testing.T) {
	svg, err := os.ReadFile(iconSVG)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := writeIcns(&buf, svg); err != nil {
		t.Fatal(err)
	}
	b := buf.Bytes()
	if string(b[:4]) != "icns" || int(binary.BigEndian.Uint32(b[4:8])) != len(b) {
		t.Fatalf("bad icns header % x, length %d", b[:8], len(b))
	}
	want := map[string]int{
		"icp4": 16, "icp5": 32, "ic11": 32, "ic12": 64, "ic07": 128,
		"ic13": 256, "ic08": 256, "ic14": 512, "ic09": 512, "ic10": 1024,
	}
	for rest := b[8:]; len(rest) > 0; {
		typ, n := string(rest[:4]), int(binary.BigEndian.Uint32(rest[4:8]))
		img, err := png.Decode(bytes.NewReader(rest[8:n]))
		if err != nil {
			t.Fatalf("%s: %v", typ, err)
		}
		size, ok := want[typ]
		if !ok {
			t.Fatalf("unexpected chunk %s", typ)
		}
		if r := img.Bounds(); r.Dx() != size || r.Dy() != size {
			t.Errorf("%s is %v, want %dx%d", typ, r, size, size)
		}
		// The tile sits inside macOS's icon grid: a transparent margin, the
		// teal tile in the middle.
		if _, _, _, a := img.At(0, 0).RGBA(); a != 0 {
			t.Errorf("%s: corner not transparent", typ)
		}
		if _, _, _, a := img.At(size/2, size/8+1).RGBA(); a != 0xffff {
			t.Errorf("%s: tile not opaque near the top", typ)
		}
		delete(want, typ)
		rest = rest[n:]
	}
	if len(want) > 0 {
		t.Errorf("missing chunks: %v", want)
	}
}

func TestBundleHasTheMacOSLayout(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "chirp-universal")
	if err := os.WriteFile(bin, []byte("binary"), 0o600); err != nil {
		t.Fatal(err)
	}
	app := filepath.Join(dir, "Chirp.app")
	if err := buildBundle(app, "v1.0.0-rc.2", bin, iconSVG); err != nil {
		t.Fatal(err)
	}
	exe, err := os.Stat(filepath.Join(app, "Contents/MacOS/chirp"))
	if err != nil {
		t.Fatal(err)
	}
	if exe.Mode().Perm() != 0o755 {
		t.Errorf("executable mode %v, want 0755", exe.Mode().Perm())
	}
	plist, err := os.ReadFile(filepath.Join(app, "Contents/Info.plist"))
	if err != nil {
		t.Fatal(err)
	}
	if d := decodePlist(t, plist); d["CFBundleShortVersionString"] != "1.0.0" {
		t.Errorf("version %q, want 1.0.0", d["CFBundleShortVersionString"])
	}
	if _, err := os.Stat(filepath.Join(app, "Contents/Resources/chirp.icns")); err != nil {
		t.Error(err)
	}
	if err := buildBundle(app, "v1.0.0", bin, iconSVG); err == nil {
		t.Error("building over an existing bundle should fail")
	}
}

// decodePlist reads a flat <dict> of strings, booleans and string arrays.
// Arrays come back as fmt's "[a b]", booleans as "true" or "false".
func decodePlist(t *testing.T, b []byte) map[string]string {
	t.Helper()
	dec := xml.NewDecoder(bytes.NewReader(b))
	out := map[string]string{}
	var key string
	var arr []string
	inArray := false
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return out
		}
		if err != nil {
			t.Fatalf("plist: %v", err)
		}
		switch se := tok.(type) {
		case xml.StartElement:
			switch se.Name.Local {
			case "key":
				var s string
				if err := dec.DecodeElement(&s, &se); err != nil {
					t.Fatal(err)
				}
				key = s
			case "string":
				var s string
				if err := dec.DecodeElement(&s, &se); err != nil {
					t.Fatal(err)
				}
				if inArray {
					arr = append(arr, s)
				} else {
					out[key] = s
				}
			case "true", "false":
				out[key] = se.Name.Local
			case "array":
				inArray, arr = true, nil
			}
		case xml.EndElement:
			if se.Name.Local == "array" {
				inArray = false
				out[key] = "[" + strings.Join(arr, " ") + "]"
			}
		}
	}
}
