// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

// Command macapp lays out Chirp.app around a built cmd/chirp binary: the
// Info.plist and the icon. release.yml runs it on the macOS runner, then
// signs the bundle (docs/release.md, Signing).
//
//	go run ./scripts/macapp -tag v1.0.0 -bin dist/chirp-universal -out dist/Chirp.app
package main

import (
	"bytes"
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"io"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"text/template"

	"github.com/fyne-io/oksvg"
	"github.com/srwiley/rasterx"
)

const (
	// bundleID is the app's identity. Screen Recording and Accessibility
	// are granted to it, together with the signing certificate, so it never
	// changes (barahn/chirp#51).
	bundleID = "app.usechirp.chirp"

	// minimumSystemVersion is what remotekit's macOS backend needs:
	// ScreenCaptureKit's SCStream and SCShareableContent (barahn/remotekit#140).
	// Go 1.26 itself needs macOS 12.
	minimumSystemVersion = "12.3"
)

func main() {
	tag := flag.String("tag", "", "release tag, vMAJOR.MINOR.PATCH[-pre]")
	bin := flag.String("bin", "", "the cmd/chirp binary to put in the bundle")
	icon := flag.String("icon", "cmd/chirp/internal/warm/bird/icon.svg", "the app icon, as SVG")
	out := flag.String("out", "Chirp.app", "the bundle to create")
	flag.Parse()
	if err := buildBundle(*out, *tag, *bin, *icon); err != nil {
		log.Fatal(err)
	}
}

// buildBundle creates app, which must not exist yet.
func buildBundle(app, tag, bin, iconSVG string) error {
	version, err := bundleVersion(tag)
	if err != nil {
		return err
	}
	if _, err := os.Stat(app); err == nil {
		return fmt.Errorf("%s already exists", app)
	}
	exe, err := os.ReadFile(bin) // #nosec G304 -- the caller's own build output
	if err != nil {
		return err
	}
	svg, err := os.ReadFile(iconSVG) // #nosec G304 -- the caller's
	if err != nil {
		return err
	}
	var plist, icns bytes.Buffer
	if err := writeInfoPlist(&plist, version); err != nil {
		return err
	}
	if err := writeIcns(&icns, svg); err != nil {
		return err
	}
	// Everything in a bundle is public, and the executable must run.
	files := []struct {
		path string
		data []byte
		mode os.FileMode
	}{
		{"Contents/MacOS/chirp", exe, 0o755},
		{"Contents/Info.plist", plist.Bytes(), 0o644},
		{"Contents/Resources/chirp.icns", icns.Bytes(), 0o644},
	}
	for _, f := range files {
		if err := put(filepath.Join(app, f.path), f.data, f.mode); err != nil {
			return err
		}
	}
	return nil
}

// put writes a file with exactly mode, whatever the umask.
func put(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), mode|0o111); err != nil {
		return err
	}
	if err := os.WriteFile(path, data, mode); err != nil {
		return err
	}
	return os.Chmod(path, mode)
}

var tagPattern = regexp.MustCompile(`^v((?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*))(?:-[0-9A-Za-z.]+)?$`)

// bundleVersion is the tag's MAJOR.MINOR.PATCH. macOS reads versions as
// numbers only, so a pre-release (-rc.1) shows as the version it leads to.
func bundleVersion(tag string) (string, error) {
	m := tagPattern.FindStringSubmatch(tag)
	if m == nil {
		return "", fmt.Errorf("tag %q is not vMAJOR.MINOR.PATCH[-pre]", tag)
	}
	return m[1], nil
}

var infoPlist = template.Must(template.New("Info.plist").Parse(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleDevelopmentRegion</key>
	<string>en</string>
	<key>CFBundleDisplayName</key>
	<string>Chirp</string>
	<key>CFBundleExecutable</key>
	<string>chirp</string>
	<key>CFBundleIconFile</key>
	<string>chirp.icns</string>
	<key>CFBundleIdentifier</key>
	<string>{{.ID}}</string>
	<key>CFBundleInfoDictionaryVersion</key>
	<string>6.0</string>
	<key>CFBundleLocalizations</key>
	<array>
		<string>en</string>
		<string>pt-BR</string>
	</array>
	<key>CFBundleName</key>
	<string>Chirp</string>
	<key>CFBundlePackageType</key>
	<string>APPL</string>
	<key>CFBundleShortVersionString</key>
	<string>{{.Version}}</string>
	<key>CFBundleVersion</key>
	<string>{{.Version}}</string>
	<key>LSApplicationCategoryType</key>
	<string>public.app-category.utilities</string>
	<key>LSMinimumSystemVersion</key>
	<string>{{.Min}}</string>
	<key>NSHighResolutionCapable</key>
	<true/>
	<key>NSHumanReadableCopyright</key>
	<string>Copyright (C) 2026 Fabrintek Engenharia Digital Ltda. AGPL-3.0-only.</string>
</dict>
</plist>
`))

// writeInfoPlist writes the bundle's Info.plist. The version is only digits
// and dots, and the rest is fixed, so nothing needs escaping.
func writeInfoPlist(w io.Writer, version string) error {
	return infoPlist.Execute(w, struct{ ID, Version, Min string }{bundleID, version, minimumSystemVersion})
}

// icnsSizes are the icns chunk types macOS 10.7 and later read as PNG, and
// the pixel size of each.
var icnsSizes = []struct {
	typ  string
	size int
}{
	{"icp4", 16}, {"icp5", 32}, {"ic11", 32}, {"ic12", 64}, {"ic07", 128},
	{"ic13", 256}, {"ic08", 256}, {"ic14", 512}, {"ic09", 512}, {"ic10", 1024},
}

// writeIcns draws the SVG at every size and writes them as an icns file.
// The tile takes 824 of 1024 pixels, centred, like every macOS icon since
// Big Sur, so Chirp's sits the same size as its neighbours in the Dock.
func writeIcns(w io.Writer, svg []byte) error {
	var body bytes.Buffer
	for _, s := range icnsSizes {
		img, err := drawIcon(svg, s.size)
		if err != nil {
			return err
		}
		var p bytes.Buffer
		if err := png.Encode(&p, img); err != nil {
			return err
		}
		body.WriteString(s.typ)
		_ = binary.Write(&body, binary.BigEndian, uint32(8+p.Len())) // #nosec G115 -- a few KB
		body.Write(p.Bytes())
	}
	var head bytes.Buffer
	head.WriteString("icns")
	_ = binary.Write(&head, binary.BigEndian, uint32(8+body.Len())) // #nosec G115 -- well under 4 GB
	if _, err := w.Write(head.Bytes()); err != nil {
		return err
	}
	_, err := w.Write(body.Bytes())
	return err
}

func drawIcon(svg []byte, size int) (image.Image, error) {
	icon, err := oksvg.ReadIconStream(bytes.NewReader(svg))
	if err != nil {
		return nil, err
	}
	if size <= 0 {
		return nil, errors.New("icon size must be positive")
	}
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	draw.Draw(img, img.Bounds(), image.Transparent, image.Point{}, draw.Src)
	tile := float64(size) * 824 / 1024
	off := (float64(size) - tile) / 2
	icon.SetTarget(off, off, tile, tile)
	icon.Draw(rasterx.NewDasher(size, size, rasterx.NewScannerGV(size, size, img, img.Bounds())), 1)
	return img, nil
}
