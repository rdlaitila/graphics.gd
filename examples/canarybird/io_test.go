package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"graphics.gd/classdb/Engine"
	"graphics.gd/classdb/FileAccess"
)

// TestGoFileIO_RoundTrip exercises Go's host filesystem path
// (os.WriteFile / os.ReadFile) from inside the engine-hosted test
// runner. Regressions here usually mean the libgodot runtime's fd
// table or musl's syscall wrappers diverged from stock Go, which
// shows up first as silent zero-byte writes in playenv reports.
func TestGoFileIO_RoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "go-roundtrip.bin")
	want := []byte("graphics.gd / canarybird host-side io probe\n")
	if err := os.WriteFile(path, want, 0644); err != nil {
		t.Fatalf("os.WriteFile: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("os.ReadFile: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("round-trip mismatch:\n got=%q\nwant=%q", got, want)
	}
}

// TestGoFileIO_SeekWrite proves partial-write + seek + read works
// under the engine runtime. os.OpenFile/Seek/Write/Read is the
// generic non-cgo io path libgodot's runtime sometimes intercepts;
// a regression manifests as torn writes that look fine via stat()
// but read back garbage.
func TestGoFileIO_SeekWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "seek.bin")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		t.Fatalf("OpenFile: %v", err)
	}
	defer f.Close()
	if _, err := f.Write([]byte("AAAABBBBCCCC")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if _, err := f.Seek(4, 0); err != nil {
		t.Fatalf("Seek: %v", err)
	}
	if _, err := f.Write([]byte("xxxx")); err != nil {
		t.Fatalf("partial Write: %v", err)
	}
	if _, err := f.Seek(0, 0); err != nil {
		t.Fatalf("Seek back: %v", err)
	}
	got := make([]byte, 12)
	if _, err := f.Read(got); err != nil {
		t.Fatalf("Read: %v", err)
	}
	want := []byte("AAAAxxxxCCCC")
	if !bytes.Equal(got, want) {
		t.Fatalf("seek-write mismatch:\n got=%q\nwant=%q", got, want)
	}
}

// TestGodotFileIO_UserRoundTrip writes via FileAccess.Open under
// user:// and reads it back through the same API. user:// resolves
// to OS_*::get_user_data_dir(); the test passes iff Godot's writer
// + reader pair sees the same bytes.
func TestGodotFileIO_UserRoundTrip(t *testing.T) {
	const rel = "user://canarybird-test-roundtrip.bin"
	want := []byte("graphics.gd / canarybird user:// io probe\n")
	runOnMain(t, func(t testing.TB) {
		w := FileAccess.Open(rel, FileAccess.Write)
		if !w.IsOpen() {
			t.Fatalf("FileAccess.Open(Write) failed: %v", FileAccess.GetOpenError())
		}
		if !w.StoreBuffer(want) {
			t.Fatalf("StoreBuffer returned false: %v", FileAccess.GetOpenError())
		}
		w.Flush()
	})
	runOnMain(t, func(t testing.TB) {
		r := FileAccess.Open(rel, FileAccess.Read)
		if !r.IsOpen() {
			t.Fatalf("FileAccess.Open(Read) failed: %v", FileAccess.GetOpenError())
		}
		got := r.GetBuffer(r.GetLength())
		if !bytes.Equal(got, want) {
			t.Fatalf("user:// round-trip mismatch:\n got=%q\nwant=%q", got, want)
		}
	})
}

// TestCrossWriteGoReadGodot writes via Go and reads back via Godot.
// playenv writes go through FileAccess specifically because Go's
// host writer sometimes drops bytes under libgodot; this test pins
// that the *other* direction (Go write → Godot read) still works on
// hosts where it does, so a regression on the green side is loud.
func TestCrossWriteGoReadGodot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "go-write.bin")
	want := []byte("written by Go, read by Godot\n")
	if err := os.WriteFile(path, want, 0644); err != nil {
		t.Fatalf("os.WriteFile: %v", err)
	}
	runOnMain(t, func(t testing.TB) {
		r := FileAccess.Open(path, FileAccess.Read)
		if !r.IsOpen() {
			t.Fatalf("FileAccess.Open(%s) failed: %v", path, FileAccess.GetOpenError())
		}
		got := r.GetBuffer(r.GetLength())
		if !bytes.Equal(got, want) {
			t.Fatalf("cross-read mismatch:\n got=%q\nwant=%q", got, want)
		}
	})
}

// TestCrossWriteGodotReadGo is the mirror of TestCrossWriteGoReadGodot.
// It's the path playenv now relies on (Godot writes the report, the
// driver pulls it back via the host filesystem). Failing here means
// FileAccess wrote to a path the host's open() can't see, which is
// the entire reason the playenv switch was made.
func TestCrossWriteGodotReadGo(t *testing.T) {
	path := filepath.Join(t.TempDir(), "godot-write.bin")
	want := []byte("written by Godot, read by Go\n")
	runOnMain(t, func(t testing.TB) {
		w := FileAccess.Open(path, FileAccess.Write)
		if !w.IsOpen() {
			t.Fatalf("FileAccess.Open(%s) failed: %v", path, FileAccess.GetOpenError())
		}
		if !w.StoreBuffer(want) {
			t.Fatalf("StoreBuffer returned false: %v", FileAccess.GetOpenError())
		}
		w.Flush()
	})
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("os.ReadFile: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("cross-write mismatch:\n got=%q\nwant=%q", got, want)
	}
}

// TestGoStdoutCapture redirects os.Stdout to a pipe, prints, drains,
// and asserts the bytes round-trip. Catches a regression where the
// libgodot runtime replaces os.Stdout with a non-writable file (or
// silently nulls it), which would have masked the playenv switch
// because writePlayReport used to fmt.Println its base64 payload.
func TestGoStdoutCapture(t *testing.T) {
	withCapturedStdFD(t, &os.Stdout, func() {
		if _, err := os.Stdout.WriteString("graphics.gd / canarybird stdout probe\n"); err != nil {
			t.Fatalf("stdout WriteString: %v", err)
		}
	}, "graphics.gd / canarybird stdout probe\n")
}

// TestGoStderrCapture mirrors TestGoStdoutCapture for os.Stderr.
// gdnext + the Godot host hand it back to the spawning shell on
// most platforms; the engine on android routes it to logcat. Either
// way the file descriptor has to accept a write.
func TestGoStderrCapture(t *testing.T) {
	withCapturedStdFD(t, &os.Stderr, func() {
		if _, err := os.Stderr.WriteString("graphics.gd / canarybird stderr probe\n"); err != nil {
			t.Fatalf("stderr WriteString: %v", err)
		}
	}, "graphics.gd / canarybird stderr probe\n")
}

// TestGodotPrint covers Engine.Print, Println, Log, and PrintRich.
// Godot's logger doesn't expose a capture API, so this is a smoke
// test: each call must complete without a panic or a non-recoverable
// engine error. Regressions in the variant marshaller or the print
// function pointer table show up as panics or hangs here.
func TestGodotPrint(t *testing.T) {
	runOnMain(t, func(t testing.TB) {
		Engine.Print("graphics.gd Engine.Print probe", 1, 2, 3)
		Engine.Println("graphics.gd Engine.Println probe")
		Engine.Log("graphics.gd Engine.Log probe")
		Engine.PrintRich("[b]graphics.gd[/b] Engine.PrintRich probe")
	})
}

// withCapturedStdFD swaps an *os.File (typically os.Stdout / os.Stderr)
// for a pipe, calls fn, restores the original, and asserts the pipe
// received want bytes. Reading happens on a goroutine so writes
// bigger than the pipe buffer don't deadlock.
func withCapturedStdFD(t *testing.T, target **os.File, fn func(), want string) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	saved := *target
	*target = w
	done := make(chan []byte, 1)
	go func() {
		buf, _ := io.ReadAll(r)
		done <- buf
	}()
	fn()
	*target = saved
	_ = w.Close()
	got := string(<-done)
	_ = r.Close()
	if got != want {
		t.Fatalf("capture mismatch:\n got=%q\nwant=%q", got, want)
	}
}
