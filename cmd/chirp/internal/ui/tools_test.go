// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Fabrintek Engenharia Digital Ltda

package ui

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"fyne.io/fyne/v2"

	"github.com/barahn/chirp/client"
	"github.com/barahn/remotekit/bark"
)

// onFyne runs f on Fyne's goroutine and waits for it.
func onFyne(f func()) { fyne.DoAndWait(f) }

// waitOnFyne is waitUntil with cond read on Fyne's goroutine.
func waitOnFyne(t *testing.T, cond func() bool) {
	t.Helper()
	waitUntil(t, func() bool {
		var ok bool
		onFyne(func() { ok = cond() })
		return ok
	})
}

// inPlace runs the tools' background work in place for the test: Fyne's
// test driver runs fyne.Do on the caller's goroutine, so work left running
// would race the test.
func inPlace(t *testing.T) {
	was := background
	background = func(f func()) { f() }
	t.Cleanup(func() { background = was })
}

// fakeTray records what the window asks of the system tray.
type fakeTray struct {
	shown bool
	menu  *fyne.Menu
	shows int
}

func (f *fakeTray) Show(m *fyne.Menu) { f.shown, f.menu = true, m; f.shows++ }
func (f *fakeTray) Hide()             { f.shown = false }

// alertWindow is a window with its notifications and tray recorded.
func alertWindow(t *testing.T) (*Window, *[]*fyne.Notification, *fakeTray) {
	t.Helper()
	inPlace(t)
	w := newWindow(t)
	var sent []*fyne.Notification
	w.notify = func(n *fyne.Notification) { sent = append(sent, n) }
	tray := &fakeTray{}
	w.Tray = tray
	return w, &sent, tray
}

// Every copy, paste and file the person helping makes is told to the owner,
// naming them as a claim and never showing the text.
func TestEveryControllerActionNotifiesTheOwner(t *testing.T) {
	w, sent, _ := alertWindow(t)
	started(w, "Ana")
	for _, tc := range []struct {
		alert Alert
		want  string
	}{
		{Alert{Kind: AlertPastedIn, Size: 12}, "Text pasted into this computer by “Ana”."},
		{Alert{Kind: AlertCopiedOut, Size: 40}, "Text you copied went to “Ana”."},
		{Alert{Kind: AlertPasswordHeld}, "A password you copied was not shared with “Ana”."},
		{Alert{Kind: AlertFileIn, Name: "report.pdf", Size: 2_200_000, Folder: "/home/o/Downloads/Chirp"}, "Received “report.pdf” (2 MB) from “Ana”. It's in Downloads/Chirp."},
	} {
		n := len(*sent)
		w.Alert("s1", tc.alert)
		if len(*sent) != n+1 {
			t.Fatalf("%v: no notification", tc.alert.Kind)
		}
		if got := (*sent)[n]; got.Content != tc.want || got.Title != "Chirp" {
			t.Errorf("%v: notified %q / %q, want %q", tc.alert.Kind, got.Title, got.Content, tc.want)
		}
	}
	// A session that is not the one in the bar alerts nothing.
	n := len(*sent)
	w.Alert("other", Alert{Kind: AlertPastedIn})
	if len(*sent) != n {
		t.Error("an alert for another session was shown")
	}
}

func TestAlertsWithoutANameStillSayWho(t *testing.T) {
	w, sent, _ := alertWindow(t)
	started(w, "")
	w.Alert("s1", Alert{Kind: AlertPastedIn})
	if got := (*sent)[0].Content; got != "Text pasted into this computer by the person helping you." {
		t.Errorf("unnamed: %q", got)
	}
}

func TestSizesReadAsPeopleSayThem(t *testing.T) {
	for size, want := range map[int64]string{0: "0 bytes", 1: "1 byte", 999: "999 bytes", 1_500: "2 KB", 999_000: "999 KB", 2_200_000: "2 MB", 1_500_000_000: "1500 MB"} {
		if got := sizeText(size); got != want {
			t.Errorf("sizeText(%d) = %q, want %q", size, got, want)
		}
	}
}

// The tray icon is there only while a session is: one the owner allowed, or
// one this machine is helping in.
func TestTheTrayIconExistsOnlyDuringASession(t *testing.T) {
	w, _, tray := alertWindow(t)
	if tray.shown {
		t.Fatal("the tray icon shows with no session")
	}
	started(w, "Ana")
	if !tray.shown || tray.menu == nil {
		t.Fatal("no tray icon during the owner's session")
	}
	w.ApplyOwner(client.OwnerEvent{Ended: true})
	if tray.shown {
		t.Fatal("the tray icon stayed after the session ended")
	}

	r := w.OpenRemote(func(bark.InputEvent) {}, func() {})
	if !tray.shown {
		t.Fatal("no tray icon while helping someone")
	}
	r.Close()
	if tray.shown {
		t.Fatal("the tray icon stayed after the remote window closed")
	}
}

// The tray's menu brings Chirp back and ends the session.
func TestTheTrayMenuOpensChirpAndEnds(t *testing.T) {
	w, _, tray := alertWindow(t)
	ended := 0
	w.OnEndSession = func() { ended++ }
	started(w, "Ana")
	labels := []string{}
	for _, it := range tray.menu.Items {
		labels = append(labels, it.Label)
	}
	if !slices.Equal(labels, []string{"Open Chirp", "End session"}) {
		t.Fatalf("tray menu %v", labels)
	}
	w.win.Hide()
	tray.menu.Items[0].Action()
	tray.menu.Items[1].Action()
	if ended != 1 || w.bar != nil {
		t.Errorf("End session from the tray: ended %d times, bar %v", ended, w.bar)
	}
}

// During a session closing the window only hides it, to the tray; with no
// session it quits Chirp and gives the code up (#50).
func TestClosingTheWindowHidesItDuringASessionAndQuitsOtherwise(t *testing.T) {
	w, _, _ := alertWindow(t)
	quits := 0
	w.quit = func() { quits++ }
	started(w, "Ana")
	w.closeMain()
	if quits != 0 {
		t.Fatal("closing the window during a session quit Chirp")
	}
	w.ApplyOwner(client.OwnerEvent{Ended: true})
	w.closeMain()
	if quits != 1 {
		t.Fatalf("closing the window with no session quit %d times, want once", quits)
	}
}

// When a session is over, the main window is the only one left.
func TestOnlyTheMainWindowIsLeftAfterASession(t *testing.T) {
	w, _, _ := alertWindow(t)
	visible := func() int {
		n := 0
		for _, win := range w.app.Driver().AllWindows() {
			if win.Content() != nil && win.Content().Visible() {
				n++
			}
		}
		return n
	}
	base := visible()
	started(w, "Ana")
	r := w.OpenRemote(func(bark.InputEvent) {}, func() {})
	w.ApplyOwner(client.OwnerEvent{Ended: true})
	r.Close()
	if got := visible(); got != base {
		t.Errorf("%d windows left after the sessions, want %d", got, base)
	}
}

// The owner's bar sends a file to the person helping, when files are allowed,
// and says where those it receives are.
func TestTheBarSendsFilesAndShowsWhatArrived(t *testing.T) {
	w, _, _ := alertWindow(t)
	started(w, "Ana")
	o := w.bar.tools
	if !o.sendFile.Visible() || o.open.Visible() {
		t.Fatal("with files allowed the bar lacks Send a file, or offers Open folder with nothing received")
	}
	w.Alert("s1", Alert{Kind: AlertFileIn, Name: "report\n.pdf", Size: 10, Folder: "/home/ana/Downloads/Chirp"})
	if !o.open.Visible() || !strings.Contains(o.status.Text, "“report .pdf”") {
		t.Errorf("after a file arrived the bar says %q", o.status.Text)
	}

	for _, sw := range w.bar.perms {
		if sw.name == "file_transfer" {
			sw.changed(false)
		}
	}
	if o.sendFile.Visible() {
		t.Error("Send a file stayed after files were taken back")
	}
}

func TestSendingAFileSaysHowItWent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	var got []byte
	var percents []int
	ok := func(name string, f io.ReadSeeker, progress func(sent, total int64)) error {
		got, _ = io.ReadAll(f)
		progress(2, 5)
		progress(2, 5)
		progress(5, 5)
		return nil
	}
	if text := sendOneFile(path, "notes.txt", ok, func(p int) { percents = append(percents, p) }); text != "Sent “notes.txt”." {
		t.Errorf("a sent file says %q", text)
	}
	if string(got) != "hello" || !slices.Equal(percents, []int{40, 100}) {
		t.Errorf("sent %q with progress %v", got, percents)
	}
	failed := func(string, io.ReadSeeker, func(int64, int64)) error { return errors.New("refused") }
	if text := sendOneFile(path, "notes.txt", failed, func(int) {}); !strings.Contains(text, "wasn't sent") {
		t.Errorf("a refused file says %q", text)
	}
	if text := sendOneFile(dir, "folder", ok, func(int) {}); !strings.Contains(text, "is a folder") {
		t.Errorf("a folder says %q", text)
	}
	if text := sendOneFile(filepath.Join(dir, "gone"), "gone", ok, func(int) {}); !strings.Contains(text, "can't be opened") {
		t.Errorf("a missing file says %q", text)
	}
}

// The controller's window takes files only while the owner allows them, and
// says where the ones the owner sent are.
func TestTheRemoteSendsFilesWhileAllowedAndShowsWhatArrived(t *testing.T) {
	inPlace(t)
	w := newWindow(t)
	r := w.OpenRemote(func(bark.InputEvent) {}, func() {})
	defer r.Close()
	if !r.tools.sendFile.Disabled() {
		t.Fatal("Send a file is enabled before anything was allowed")
	}
	r.Granted(allFour)
	waitOnFyne(t, func() bool { return !r.tools.sendFile.Disabled() })
	r.FileReceived("log.txt", "/home/c/Downloads/Chirp")
	waitOnFyne(t, func() bool {
		return r.tools.open.Visible() && strings.Contains(r.tools.status.Text, "“log.txt”")
	})
	r.Granted([]string{"screen_view"})
	waitOnFyne(t, func() bool { return r.tools.sendFile.Disabled() })
}

// Once a session starts, the main window goes to the tray, leaving the
// session's own window -- the owner's bar, or the helper's view of the other
// screen -- and comes back when the session ends.
func TestTheMainWindowGoesToTheTrayDuringASession(t *testing.T) {
	w, _, _ := alertWindow(t)
	if w.mainHidden {
		t.Fatal("the main window is hidden with no session")
	}
	started(w, "Ana")
	if !w.mainHidden {
		t.Fatal("the main window stayed open during the owner's session")
	}
	w.ApplyOwner(client.OwnerEvent{Ended: true})
	if w.mainHidden {
		t.Fatal("the main window did not come back after the session")
	}

	r := w.OpenRemote(func(bark.InputEvent) {}, func() {})
	if !w.mainHidden {
		t.Fatal("the main window stayed open while helping")
	}
	r.Close()
	if w.mainHidden {
		t.Fatal("the main window did not come back after helping")
	}
}

// Without a tray to go to, the main window stays where it is.
func TestWithoutATrayTheMainWindowStays(t *testing.T) {
	inPlace(t)
	w := newWindow(t)
	started(w, "Ana")
	if w.mainHidden {
		t.Fatal("the main window was hidden with no tray to bring it back from")
	}
}

// Open Chirp in the tray brings the main window back during a session.
func TestOpenChirpBringsTheWindowBack(t *testing.T) {
	w, _, tray := alertWindow(t)
	started(w, "Ana")
	tray.menu.Items[0].Action()
	if w.mainHidden {
		t.Fatal("Open Chirp left the main window hidden")
	}
}
