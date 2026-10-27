// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package main

import (
	"image"
	"log"
	"runtime"
	"sync"
	"unsafe"

	"fyne.io/fyne/v2"
	"golang.org/x/sys/windows"

	"github.com/barahn/chirp/cmd/chirp/internal/perch"
	"github.com/barahn/chirp/cmd/chirp/internal/ui"
	"github.com/barahn/chirp/cmd/chirp/internal/warm"
)

// The bird by the taskbar on Windows (barahn/chirp#136): a layered window,
// drawn with per-pixel alpha by UpdateLayeredWindow, so only the bird shows
// and clicks on the transparent pixels go to whatever is under them. It is a
// tool window that never activates, kept topmost, standing on the taskbar
// just left of the tray. It runs on a thread of its own, with its own
// message loop; the window's goroutine asks it things by posting messages.

var (
	gdi32                   = windows.NewLazySystemDLL("gdi32.dll")
	procRegisterClassEx     = user32.NewProc("RegisterClassExW")
	procCreateWindowEx      = user32.NewProc("CreateWindowExW")
	procDefWindowProc       = user32.NewProc("DefWindowProcW")
	procGetMessage          = user32.NewProc("GetMessageW")
	procTranslateMessage    = user32.NewProc("TranslateMessage")
	procDispatchMessage     = user32.NewProc("DispatchMessageW")
	procPostMessage         = user32.NewProc("PostMessageW")
	procSetTimer            = user32.NewProc("SetTimer")
	procKillTimer           = user32.NewProc("KillTimer")
	procUpdateLayeredWindow = user32.NewProc("UpdateLayeredWindow")
	procFindWindow          = user32.NewProc("FindWindowW")
	procFindWindowEx        = user32.NewProc("FindWindowExW")
	procGetWindowRect       = user32.NewProc("GetWindowRect")
	procSystemParametersInf = user32.NewProc("SystemParametersInfoW")
	procGetDpiForWindow     = user32.NewProc("GetDpiForWindow")
	procGetDC               = user32.NewProc("GetDC")
	procReleaseDC           = user32.NewProc("ReleaseDC")
	procLoadCursor          = user32.NewProc("LoadCursorW")
	procCreateCompatibleDC  = gdi32.NewProc("CreateCompatibleDC")
	procCreateDIBSection    = gdi32.NewProc("CreateDIBSection")
	procSelectObject        = gdi32.NewProc("SelectObject")
	procDeleteObject        = gdi32.NewProc("DeleteObject")
	procDeleteDC            = gdi32.NewProc("DeleteDC")
)

const (
	wmTimer       = 0x0113
	wmLButtonUp   = 0x0202
	wmMouseActive = 0x0021
	wmApp         = 0x8000
	maNoActivate  = 3
	frameTimer    = 1
	fps           = 30
	// birdWidth is the window's width at 96 DPI: the bird's body, 60 of
	// the art's 164 units, about 40 pixels across, as tall as the taskbar
	// is high.
	birdWidth = 110
)

type winRect struct{ Left, Top, Right, Bottom int32 }

func (r winRect) image() image.Rectangle {
	return image.Rect(int(r.Left), int(r.Top), int(r.Right), int(r.Bottom))
}

type winPoint struct{ X, Y int32 }

type winSize struct{ CX, CY int32 }

type winMsg struct {
	HWnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      winPoint
	Private uint32
}

type wndClassEx struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   windows.Handle
	Icon       windows.Handle
	Cursor     windows.Handle
	Background windows.Handle
	MenuName   *uint16
	ClassName  *uint16
	IconSm     windows.Handle
}

type bitmapInfoHeader struct {
	Size          uint32
	Width         int32
	Height        int32
	Planes        uint16
	BitCount      uint16
	Compression   uint32
	SizeImage     uint32
	XPelsPerMeter int32
	YPelsPerMeter int32
	ClrUsed       uint32
	ClrImportant  uint32
}

// winPerch is ui.Perch on Windows.
type winPerch struct {
	ready chan struct{}
	hwnd  uintptr

	mu     sync.Mutex
	todo   []func() // run on the bird's thread
	tapped func()
	spot   image.Rectangle // where the window stands, in screen pixels

	// The bird's thread only.
	frames  []*image.RGBA
	next    int
	leaving bool
	shown   bool
}

// newPerch starts the bird's thread and window. It returns nil where the
// window cannot be made: then there is no bird, and the balloon is the
// session window.
func newPerch() ui.Perch {
	p := &winPerch{ready: make(chan struct{})}
	go p.loop()
	<-p.ready
	if p.hwnd == 0 {
		return nil
	}
	return p
}

var perchProc = windows.NewCallback(func(hwnd, msg, wParam, lParam uintptr) uintptr {
	if p := thePerch; p != nil {
		switch msg {
		case wmApp:
			p.drain()
			return 0
		case wmTimer:
			p.step()
			return 0
		case wmMouseActive:
			return maNoActivate
		case wmLButtonUp:
			p.mu.Lock()
			tapped := p.tapped
			p.mu.Unlock()
			if tapped != nil {
				fyne.Do(tapped)
			}
			return 0
		}
	}
	r, _, _ := procDefWindowProc.Call(hwnd, msg, wParam, lParam)
	return r
})

// thePerch is the one bird, for the window procedure.
var thePerch *winPerch

func (p *winPerch) loop() {
	runtime.LockOSThread()
	const (
		wsPopup         = 0x80000000
		wsExLayered     = 0x00080000
		wsExToolWindow  = 0x00000080
		wsExTopmost     = 0x00000008
		wsExNoActivate  = 0x08000000
		idcHand         = 32649
		classNameString = "ChirpPerch"
	)
	thePerch = p
	className, _ := windows.UTF16PtrFromString(classNameString)
	cursor, _, _ := procLoadCursor.Call(0, idcHand)
	var instance windows.Handle
	_ = windows.GetModuleHandleEx(0, nil, &instance)
	wc := wndClassEx{WndProc: perchProc, Instance: instance, Cursor: windows.Handle(cursor), ClassName: className}
	wc.Size = uint32(unsafe.Sizeof(wc))
	if r, _, err := procRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
		log.Printf("chirp: the bird's window class: %v", err)
		close(p.ready)
		return
	}
	hwnd, _, err := procCreateWindowEx.Call(
		wsExLayered|wsExToolWindow|wsExTopmost|wsExNoActivate,
		uintptr(unsafe.Pointer(className)), 0, wsPopup,
		0, 0, 1, 1, 0, 0, uintptr(instance), 0)
	if hwnd == 0 {
		log.Printf("chirp: the bird's window: %v", err)
		close(p.ready)
		return
	}
	p.hwnd = hwnd
	close(p.ready)
	var m winMsg
	for {
		r, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			return
		}
		_, _, _ = procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		_, _, _ = procDispatchMessage.Call(uintptr(unsafe.Pointer(&m)))
	}
}

// do runs f on the bird's thread.
func (p *winPerch) do(f func()) {
	p.mu.Lock()
	p.todo = append(p.todo, f)
	p.mu.Unlock()
	_, _, _ = procPostMessage.Call(p.hwnd, wmApp, 0, 0)
}

func (p *winPerch) drain() {
	p.mu.Lock()
	todo := p.todo
	p.todo = nil
	p.mu.Unlock()
	for _, f := range todo {
		f()
	}
}

// Land flies the bird in to its spot by the tray.
func (p *winPerch) Land(tapped func()) {
	// The spot is found here, not on the bird's thread: the balloon is
	// placed against it as soon as Land returns.
	spot := p.findSpot()
	p.mu.Lock()
	p.tapped, p.spot = tapped, spot
	p.mu.Unlock()
	p.do(func() { p.play(warm.Perched, perch.Landing(fps), spot.Dx(), false) })
}

// Pose shows the perched bird in pose, once it has landed.
func (p *winPerch) Pose(name string) {
	pose := warm.Perched
	if name == "chirping" {
		pose = warm.Chirping
	}
	p.do(func() {
		if !p.shown || p.frames != nil {
			return
		}
		p.mu.Lock()
		w := p.spot.Dx()
		p.mu.Unlock()
		if img, err := perch.Frame(pose, perch.Perched, w); err == nil {
			p.blit(img)
		}
	})
}

// FlyOff sends the bird away, the way it came.
func (p *winPerch) FlyOff() {
	p.do(func() {
		if !p.shown {
			return
		}
		p.mu.Lock()
		w := p.spot.Dx()
		p.mu.Unlock()
		p.play(warm.Flying, perch.FlyingOff(fps), w, true)
	})
}

// Place puts the balloon just above the bird's head.
func (p *winPerch) Place(balloon fyne.Window) {
	p.mu.Lock()
	spot := p.spot
	p.mu.Unlock()
	h := hwnd(balloon)
	if h == 0 || spot.Empty() {
		return
	}
	var r winRect
	if ok, _, _ := procGetWindowRect.Call(h, uintptr(unsafe.Pointer(&r))); ok == 0 {
		return
	}
	at := perch.BalloonAt(spot, r.image().Size(), workArea())
	const (
		swpNoSize     = 0x0001
		swpNoZOrder   = 0x0004
		swpNoActivate = 0x0010
	)
	_, _, _ = procSetWindowPos.Call(h, 0, uintptr(at.Min.X), uintptr(at.Min.Y), 0, 0, swpNoSize|swpNoZOrder|swpNoActivate)
}

// play renders pose through motions and shows them, one a frame; leaving
// hides the window at the end.
func (p *winPerch) play(pose warm.Pose, motions []perch.Motion, width int, leaving bool) {
	frames := make([]*image.RGBA, 0, len(motions))
	for _, m := range motions {
		img, err := perch.Frame(pose, m, width)
		if err != nil {
			return
		}
		frames = append(frames, img)
	}
	p.frames, p.next, p.leaving = frames, 0, leaving
	p.step()
	_, _, _ = procSetTimer.Call(p.hwnd, frameTimer, 1000/fps, 0)
}

// step shows the next frame, and stops at the last.
func (p *winPerch) step() {
	if p.frames == nil {
		_, _, _ = procKillTimer.Call(p.hwnd, frameTimer)
		return
	}
	p.blit(p.frames[p.next])
	p.next++
	if p.next < len(p.frames) {
		return
	}
	_, _, _ = procKillTimer.Call(p.hwnd, frameTimer)
	p.frames = nil
	if p.leaving {
		_, _, _ = procShowWindow.Call(p.hwnd, 0) // SW_HIDE
		p.shown = false
	}
}

// blit shows img in the window at its spot, with per-pixel alpha.
func (p *winPerch) blit(img *image.RGBA) {
	p.mu.Lock()
	spot := p.spot
	p.mu.Unlock()
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	screen, _, _ := procGetDC.Call(0)
	defer procReleaseDC.Call(0, screen)
	mem, _, _ := procCreateCompatibleDC.Call(screen)
	defer procDeleteDC.Call(mem)
	bi := bitmapInfoHeader{Width: int32(w), Height: -int32(h), Planes: 1, BitCount: 32}
	bi.Size = uint32(unsafe.Sizeof(bi))
	var bits unsafe.Pointer
	dib, _, _ := procCreateDIBSection.Call(mem, uintptr(unsafe.Pointer(&bi)), 0, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if dib == 0 || bits == nil {
		return
	}
	defer procDeleteObject.Call(dib)
	old, _, _ := procSelectObject.Call(mem, dib)
	defer procSelectObject.Call(mem, old)
	// image.RGBA is premultiplied, as UpdateLayeredWindow wants; Windows
	// wants the bytes as BGRA.
	dst := unsafe.Slice((*byte)(bits), w*h*4)
	for i := 0; i < len(dst); i += 4 {
		dst[i], dst[i+1], dst[i+2], dst[i+3] = img.Pix[i+2], img.Pix[i+1], img.Pix[i], img.Pix[i+3]
	}
	pos := winPoint{int32(spot.Min.X), int32(spot.Min.Y)}
	size := winSize{int32(w), int32(h)}
	var src winPoint
	blend := [4]byte{0, 0, 255, 1} // AC_SRC_OVER, 0, opaque, AC_SRC_ALPHA
	const ulwAlpha = 2
	_, _, _ = procUpdateLayeredWindow.Call(p.hwnd, screen, uintptr(unsafe.Pointer(&pos)), uintptr(unsafe.Pointer(&size)),
		mem, uintptr(unsafe.Pointer(&src)), 0, uintptr(unsafe.Pointer(&blend)), ulwAlpha)
	if !p.shown {
		_, _, _ = procShowWindow.Call(p.hwnd, 4) // SW_SHOWNOACTIVATE
		p.shown = true
	}
}

// findSpot is where the bird stands now: on the taskbar, by the tray, sized
// for the taskbar's DPI.
func (p *winPerch) findSpot() image.Rectangle {
	trayClass, _ := windows.UTF16PtrFromString("Shell_TrayWnd")
	notifyClass, _ := windows.UTF16PtrFromString("TrayNotifyWnd")
	var taskbar, tray image.Rectangle
	dpi := uintptr(96)
	if bar, _, _ := procFindWindow.Call(uintptr(unsafe.Pointer(trayClass)), 0); bar != 0 {
		var r winRect
		if ok, _, _ := procGetWindowRect.Call(bar, uintptr(unsafe.Pointer(&r))); ok != 0 {
			taskbar = r.image()
		}
		if notify, _, _ := procFindWindowEx.Call(bar, 0, uintptr(unsafe.Pointer(notifyClass)), 0); notify != 0 {
			if ok, _, _ := procGetWindowRect.Call(notify, uintptr(unsafe.Pointer(&r))); ok != 0 {
				tray = r.image()
			}
		}
		if d, _, _ := procGetDpiForWindow.Call(bar); d != 0 {
			dpi = d
		}
	}
	w := int(birdWidth * dpi / 96)
	return perch.Spot(taskbar, tray, workArea(), w, perch.Height(w))
}

// workArea is the primary monitor's area not covered by the taskbar.
func workArea() image.Rectangle {
	const spiGetWorkArea = 0x0030
	var r winRect
	if ok, _, _ := procSystemParametersInf.Call(spiGetWorkArea, 0, uintptr(unsafe.Pointer(&r)), 0); ok == 0 {
		return image.Rectangle{}
	}
	return r.image()
}
