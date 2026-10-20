// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

// Command icon draws the app icon (cmd/chirp/internal/warm/bird/icon.svg) as
// the 256-pixel PNG go-winres makes chirp.exe's icon from. go-winres scales
// it down for the smaller sizes. scripts/gen-winres.sh runs it.
//
//	go run ./cmd/chirp/winres/icon out.png
package main

import (
	"bytes"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"os"

	"github.com/fyne-io/oksvg"
	"github.com/srwiley/rasterx"

	"github.com/barahn/chirp/cmd/chirp/internal/warm"
)

const size = 256

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: icon out.png")
		os.Exit(2)
	}
	if err := run(os.Args[1]); err != nil {
		fmt.Fprintln(os.Stderr, "icon:", err)
		os.Exit(1)
	}
}

func run(out string) error {
	icon, err := oksvg.ReadIconStream(bytes.NewReader(warm.AppIcon.Content()))
	if err != nil {
		return err
	}
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	draw.Draw(img, img.Bounds(), image.Transparent, image.Point{}, draw.Src)
	icon.SetTarget(0, 0, size, size)
	icon.Draw(rasterx.NewDasher(size, size, rasterx.NewScannerGV(size, size, img, img.Bounds())), 1)
	var buf bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.BestCompression}
	if err := enc.Encode(&buf, img); err != nil {
		return err
	}
	return os.WriteFile(out, buf.Bytes(), 0o600)
}
