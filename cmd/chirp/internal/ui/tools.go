// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package ui

import (
	"io"
	"net/url"
	"os"
	"path/filepath"
	"slices"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// The clipboard and files during a session (barahn/chirp#134, docs/ux.md,
// During a session).
//
// Once the owner allows the clipboard, it syncs both ways on its own; what a
// password manager marks as secret never leaves (barahn/remotekit#130). The
// owner is told of everything that moves -- each paste into their computer,
// each copy that leaves it, each file -- in a system notification, and files
// in the session bar too, with Open folder. Files go both ways, into a Chirp
// folder in Downloads at either end.

// AlertKind is what happened, for Alert.
type AlertKind int

const (
	// AlertPastedIn is text the person helping put on this computer's
	// clipboard.
	AlertPastedIn AlertKind = iota + 1
	// AlertCopiedOut is text copied on this computer that went to them.
	AlertCopiedOut
	// AlertPasswordHeld is a copy a password manager marked as secret,
	// which was not sent.
	AlertPasswordHeld
	// AlertFileIn is a file they sent, saved in Folder.
	AlertFileIn
)

// Alert is one thing the owner is told about. Name, Size and Folder are the
// file's, for the file kinds; Size is the text's for the clipboard ones.
type Alert struct {
	Kind   AlertKind
	Name   string
	Size   int64
	Folder string
}

// Alert tells the owner what happened in the session sessionID, in a
// notification, and in the session bar for a file that arrived. It is
// called on Fyne's goroutine.
func (w *Window) Alert(sessionID string, a Alert) {
	b := w.bar
	if b == nil || b.session.ID != sessionID || b.done {
		return
	}
	if a.Kind == AlertFileIn {
		b.tools.received(a.Name, a.Folder)
	}
	text := alertText(b.session.Name, a)
	if text == "" || w.alertsOff {
		return
	}
	w.notify(fyne.NewNotification("Chirp", text))
}

// alertText is what the owner is told, naming the person helping as a claim.
func alertText(name string, a Alert) string {
	who := tr("the person helping you")
	if c := claim(name); c != "" {
		who = "“" + c + "”"
	}
	switch a.Kind {
	case AlertPastedIn:
		return trf("Text pasted into this computer by %s.", who)
	case AlertCopiedOut:
		return trf("Text you copied went to %s.", who)
	case AlertPasswordHeld:
		return trf("A password you copied was not shared with %s.", who)
	case AlertFileIn:
		return trf("Received “%s” (%s) from %s. It's in %s.", claim(a.Name), sizeText(a.Size), who, shortFolder(a.Folder))
	}
	return ""
}

// sizeText is a size as people say it, in whole units.
func sizeText(n int64) string {
	switch {
	case n == 1:
		return tr("1 byte")
	case n < 1000:
		return trf("%d bytes", n)
	case n < 1_000_000:
		return trf("%d KB", (n+500)/1000)
	}
	return trf("%d MB", (n+500_000)/1_000_000)
}

// shortFolder is the last two parts of a folder, as Downloads/Chirp.
func shortFolder(dir string) string {
	if dir == "" {
		return ""
	}
	return filepath.Join(filepath.Base(filepath.Dir(dir)), filepath.Base(dir))
}

// Tray is the system tray icon, shown only while a session is (#134).
// cmd/chirp provides it; without one there is no icon.
type Tray interface {
	// Show puts the icon in the tray with menu, or updates its menu.
	Show(menu *fyne.Menu)
	// Hide takes the icon out of the tray.
	Hide()
}

// inSession reports whether a session is on: one the owner allowed, or one
// this machine is helping in.
func (w *Window) inSession() bool { return w.bar != nil || w.remote != nil }

// updateTray shows the tray icon while a session is on, and hides it after.
// The main window goes to the tray when a session starts, leaving the
// session's own window, and comes back when it ends.
func (w *Window) updateTray() {
	if w.Tray == nil {
		return
	}
	if !w.inSession() {
		w.Tray.Hide()
		if w.mainHidden {
			w.mainHidden = false
			w.win.Show()
		}
		return
	}
	alerts := tr("Turn alerts off")
	if w.alertsOff {
		alerts = tr("Turn alerts on")
	}
	w.Tray.Show(fyne.NewMenu("Chirp",
		fyne.NewMenuItem(tr("Open Chirp"), func() {
			w.mainHidden = false
			w.win.Show()
			w.win.RequestFocus()
		}),
		fyne.NewMenuItem(alerts, func() {
			w.alertsOff = !w.alertsOff
			w.updateTray()
		}),
		fyne.NewMenuItem(tr("End session"), w.endFromTray),
	))
	if !w.mainHidden {
		w.mainHidden = true
		w.win.Hide()
	}
}

// endFromTray ends whatever session is on, as End does where it is.
func (w *Window) endFromTray() {
	if w.bar != nil {
		w.bar.endNow()
	}
	if r := w.remote; r != nil {
		r.end()
	}
}

// closeMain is the main window's close button. During a session it hides
// the window, to the tray; otherwise it quits, giving the code and link up
// at once (#50).
func (w *Window) closeMain() {
	if w.inSession() && w.Tray != nil {
		w.mainHidden = true
		w.win.Hide()
		return
	}
	w.quit()
}

// remoteTools is the controller's toolbar: Send a file, usable while the
// other person allows files, and a line for how things went.
type remoteTools struct {
	r        *Remote
	sendFile *widget.Button
	open     *widget.Button
	status   *widget.Label
	files    bool
	sending  string // the name of the file being sent, if one is
	folder   string
}

func newRemoteTools(r *Remote) *remoteTools {
	t := &remoteTools{r: r}
	t.sendFile = widget.NewButtonWithIcon(tr("Send a file"), theme.UploadIcon(), t.chooseFile)
	t.open = widget.NewButtonWithIcon(tr("Open folder"), theme.FolderOpenIcon(), func() { openFolder(r.app, t.folder) })
	t.open.Hide()
	t.status = widget.NewLabel("")
	t.status.Wrapping = fyne.TextWrapWord
	t.status.Hide()
	t.update()
	return t
}

// buttons is the toolbar's row, before End.
func (t *remoteTools) buttons() fyne.CanvasObject {
	return container.NewHBox(t.sendFile, t.open, layout.NewSpacer())
}

// granted takes what the other person allows now.
func (t *remoteTools) granted(permissions []string) {
	t.files = slices.Contains(permissions, "file_transfer")
	t.update()
}

// update enables what may be used now.
func (t *remoteTools) update() {
	if t.files && t.sending == "" {
		t.sendFile.Enable()
	} else {
		t.sendFile.Disable()
	}
}

// say shows how the last thing went.
func (t *remoteTools) say(text string) {
	if t.r.closed {
		return
	}
	t.status.SetText(text)
	t.status.Show()
	t.r.win.Content().Refresh()
}

// received says a file from the other person arrived, and where.
func (t *remoteTools) received(name, folder string) {
	t.folder = folder
	t.open.Show()
	t.say(trf("They sent you “%s”. It's in %s.", claim(name), shortFolder(folder)))
}

// chooseFile asks which file to send.
func (t *remoteTools) chooseFile() {
	chooseFile(t.r.win, func(uri fyne.URI) { t.send([]fyne.URI{uri}) })
}

// dropped sends the files dropped on the window.
func (t *remoteTools) dropped(_ fyne.Position, uris []fyne.URI) {
	if len(uris) == 0 {
		return
	}
	if !t.files {
		t.say(tr(filesNotAllowed))
		return
	}
	t.send(uris)
}

// send sends files one after another, saying how each goes. One batch at a
// time: a drop while one is going is turned away.
func (t *remoteTools) send(uris []fyne.URI) {
	if t.sending != "" {
		t.say(trf("Wait for “%s” to finish first.", claim(t.sending)))
		return
	}
	sendFile := t.r.SendFile
	paths := localPaths(uris)
	if sendFile == nil || len(paths) == 0 {
		return
	}
	t.sending = filepath.Base(paths[0])
	t.update()
	background(func() {
		for _, p := range paths {
			name := filepath.Base(p)
			fyne.Do(func() { t.sending = name; t.say(trf("Sending “%s”…", claim(name))) })
			text := sendOneFile(p, name, sendFile, func(percent int) {
				fyne.Do(func() { t.say(trf("Sending “%s”: %d%%", claim(name), percent)) })
			})
			fyne.Do(func() { t.say(text) })
		}
		fyne.Do(func() {
			t.sending = ""
			t.update()
		})
	})
}

// localPaths keeps the files among uris.
func localPaths(uris []fyne.URI) []string {
	paths := make([]string, 0, len(uris))
	for _, u := range uris {
		if u.Scheme() == "file" {
			paths = append(paths, u.Path())
		}
	}
	return paths
}

// chooseFile opens the file picker over win, and hands the file chosen to
// picked.
func chooseFile(win fyne.Window, picked func(fyne.URI)) {
	dialog.NewFileOpen(func(f fyne.URIReadCloser, err error) {
		if err != nil || f == nil {
			return
		}
		uri := f.URI()
		_ = f.Close()
		picked(uri)
	}, win).Show()
}

// sendOneFile sends the file at path and says how it went. progress is
// called as the percentage the other end has grows.
func sendOneFile(path, name string, send func(string, io.ReadSeeker, func(sent, total int64)) error, progress func(percent int)) string {
	info, err := os.Stat(path)
	if err == nil && info.IsDir() {
		return trf("“%s” is a folder. Send the files in it instead.", claim(name))
	}
	f, err := os.Open(path) // #nosec G304 -- the file the user picked to send
	if err != nil {
		return trf("“%s” can't be opened here.", claim(name))
	}
	defer func() { _ = f.Close() }()
	last := -1
	err = send(name, f, func(sent, total int64) {
		percent := 100
		if total > 0 {
			percent = int(sent * 100 / total)
		}
		if percent != last {
			last = percent
			progress(percent)
		}
	})
	if err != nil {
		return trf("“%s” wasn't sent. They may have taken files back, or the connection dropped.", claim(name))
	}
	return trf("Sent “%s”.", claim(name))
}

// ownerTools is what the owner's session bar adds for files: Send a file
// while files are allowed, and a line for what arrived, with Open folder.
type ownerTools struct {
	b        *sessionBar
	sendFile *widget.Button
	status   *widget.Label
	open     *widget.Button
	folder   string
	sending  bool
}

func newOwnerTools(b *sessionBar) *ownerTools {
	o := &ownerTools{b: b}
	o.sendFile = widget.NewButtonWithIcon(tr("Send a file"), theme.UploadIcon(), o.chooseFile)
	o.status = widget.NewLabel("")
	o.status.Wrapping = fyne.TextWrapWord
	o.status.Hide()
	o.open = widget.NewButtonWithIcon(tr("Open folder"), theme.FolderOpenIcon(), func() { openFolder(b.w.app, o.folder) })
	o.open.Hide()
	o.granted(b.granted)
	return o
}

func (o *ownerTools) content() fyne.CanvasObject {
	return container.NewVBox(container.NewHBox(o.sendFile, o.open), o.status)
}

// granted shows Send a file only while files are allowed.
func (o *ownerTools) granted(permissions []string) {
	if slices.Contains(permissions, "file_transfer") {
		o.sendFile.Show()
	} else {
		o.sendFile.Hide()
	}
}

func (o *ownerTools) say(text string) {
	if o.b.done {
		return
	}
	o.status.SetText(text)
	o.status.Show()
	o.b.win.Content().Refresh()
}

// chooseFile asks which file to send to the person helping.
func (o *ownerTools) chooseFile() {
	if o.sending {
		return
	}
	chooseFile(o.b.win, func(uri fyne.URI) {
		paths := localPaths([]fyne.URI{uri})
		send, id := o.b.w.OnSendFile, o.b.session.ID
		if len(paths) == 0 || send == nil {
			return
		}
		path, name := paths[0], filepath.Base(paths[0])
		o.sending = true
		o.say(trf("Sending “%s”…", claim(name)))
		background(func() {
			text := sendOneFile(path, name, func(n string, f io.ReadSeeker, progress func(int64, int64)) error {
				return send(id, n, f, progress)
			}, func(percent int) {
				fyne.Do(func() { o.say(trf("Sending “%s”: %d%%", claim(name), percent)) })
			})
			fyne.Do(func() {
				o.sending = false
				o.say(text)
			})
		})
	})
}

// received says a file arrived, and where.
func (o *ownerTools) received(name, folder string) {
	o.folder = folder
	o.open.Show()
	o.say(trf("They sent you “%s”. It's in %s.", claim(name), shortFolder(folder)))
}

// openFolder shows folder in the system's file manager.
func openFolder(a fyne.App, folder string) {
	if folder == "" {
		return
	}
	if u, err := url.Parse(storage.NewFileURI(folder).String()); err == nil {
		_ = a.OpenURL(u)
	}
}

// background runs what waits on the other computer off Fyne's goroutine.
// Tests run it in place: Fyne's test driver runs fyne.Do on the caller's
// goroutine, so work left running would race the test.
var background = func(f func()) { go f() }

const filesNotAllowed = "They haven't allowed sending them files."
