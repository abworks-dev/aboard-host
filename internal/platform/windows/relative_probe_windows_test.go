//go:build windows && amd64

package windows

import (
	"os"
	"path/filepath"
	"testing"
	"unsafe"

	win "golang.org/x/sys/windows"
)

// Mandatory negative: exercise NtCreateFile directly after validation's view
// has become stale, rather than only observing the wrapper's early rejection.
func TestDirectoryGateMetadataResolution(t *testing.T) {
	root := syntheticDir(t)
	empty := filepath.Join(root, "empty")
	target := filepath.Join(root, "target")
	for _, p := range []string{empty, target} {
		if err := os.Mkdir(p, 0700); err != nil {
			t.Fatal(err)
		}
	}
	pins, err := pinDirectories(empty)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		for i := len(pins) - 1; i >= 0; i-- {
			_ = pins[i].Close()
		}
	}()
	if err := markJunctionAccess(empty, target, win.FILE_WRITE_ATTRIBUTES); err != nil {
		t.Fatal("metadata mutation stimulus unavailable:", err)
	}
	name, err := win.NewNTUnicodeString("probe.lock")
	if err != nil {
		t.Fatal(err)
	}
	oa := win.OBJECT_ATTRIBUTES{RootDirectory: pins[len(pins)-1].handle, ObjectName: name, Attributes: win.OBJ_CASE_INSENSITIVE | win.OBJ_DONT_REPARSE}
	oa.Length = uint32(unsafe.Sizeof(oa))
	var h win.Handle
	var ios win.IO_STATUS_BLOCK
	err = win.NtCreateFile(&h, win.GENERIC_READ|win.GENERIC_WRITE|win.SYNCHRONIZE, &oa, &ios, nil, win.FILE_ATTRIBUTE_NORMAL, 0, win.FILE_CREATE, win.FILE_NON_DIRECTORY_FILE|win.FILE_OPEN_REPARSE_POINT|win.FILE_SYNCHRONOUS_IO_NONALERT, 0, 0)
	t.Logf("relative CREATE_NEW after root reparse: error=%v information=%d", err, ios.Information)
	if err == nil {
		_ = win.CloseHandle(h)
		t.Fatal("G4 BLOCKED: direct NT relative create accepted an already reparsed parent")
	}
	if _, e := os.Stat(filepath.Join(target, "probe.lock")); !os.IsNotExist(e) {
		t.Fatalf("relative create followed redirected root: %v", e)
	}
	if e := pins[len(pins)-1].Validate(); e == nil {
		t.Fatal("mutated root passed validation")
	}
	nonempty := filepath.Join(root, "nonempty")
	if err := os.Mkdir(nonempty, 0700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(nonempty, "anchor")
	if err := os.WriteFile(file, []byte("synthetic"), 0600); err != nil {
		t.Fatal(err)
	}
	f, err := openPinned(file, win.GENERIC_READ, win.FILE_SHARE_READ|win.FILE_SHARE_WRITE, win.OPEN_EXISTING, false)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := markJunctionAccess(nonempty, target, win.FILE_WRITE_ATTRIBUTES); err == nil {
		t.Fatal("nonempty directory with retained child changed to junction")
	} else {
		t.Logf("nonempty metadata reparse error=%v", err)
	}
}

func TestDirectoryGateMetadataCreateCompetition(t *testing.T) {
	created, reparsed := 0, 0
	for round := 0; round < 40; round++ {
		func() {
			root := syntheticDir(t)
			source := filepath.Join(root, "source")
			target := filepath.Join(root, "target")
			for _, p := range []string{source, target} {
				if e := os.Mkdir(p, 0700); e != nil {
					t.Fatal(e)
				}
			}
			marker := filepath.Join(target, "marker")
			if e := os.WriteFile(marker, []byte("synthetic target unchanged"), 0600); e != nil {
				t.Fatal(e)
			}
			pins, e := pinDirectories(source)
			if e != nil {
				t.Fatal(e)
			}
			defer func() {
				for i := len(pins) - 1; i >= 0; i-- {
					if e := pins[i].Close(); e != nil {
						t.Error(e)
					}
				}
			}()
			type opened struct {
				file *File
				err  error
			}
			createResult := make(chan opened, 1)
			mutateResult := make(chan error, 1)
			start := make(chan struct{})
			go func() {
				<-start
				f, e := openRelativePinned(pins[len(pins)-1], "attempt.lock", win.GENERIC_READ|win.GENERIC_WRITE, 0, win.CREATE_NEW, false)
				createResult <- opened{f, e}
			}()
			go func() { <-start; mutateResult <- markJunctionAccess(source, target, win.FILE_WRITE_ATTRIBUTES) }()
			close(start)
			c, m := <-createResult, <-mutateResult
			if c.file != nil {
				defer func() {
					if e := c.file.Close(); e != nil {
						t.Error(e)
					}
				}()
			}
			// Both native operations have finished; keep the child open until now.
			if c.err == nil && m == nil {
				t.Fatalf("G4 BLOCKED: create and parent reparse both succeeded, round=%d", round)
			}
			if c.err == nil {
				created++
				if e := c.file.Validate(); e != nil {
					t.Fatal(e)
				}
				if _, e := os.Stat(filepath.Join(source, "attempt.lock")); e != nil {
					t.Fatal("create did not land in original source:", e)
				}
			}
			if m == nil {
				reparsed++
				if e := pins[len(pins)-1].Validate(); e == nil {
					t.Fatal("reparsed source still valid")
				}
			}
			entries, e := os.ReadDir(target)
			if e != nil {
				t.Fatal(e)
			}
			b, e := os.ReadFile(marker)
			if e != nil {
				t.Fatal(e)
			}
			if len(entries) != 1 || entries[0].Name() != "marker" || string(b) != "synthetic target unchanged" {
				t.Fatal("redirected target bytes or inventory changed")
			}
			t.Logf("metadata/create round=%d createError=%v mutationError=%v targetUnchanged=true", round, c.err, m)
		}()
	}
	t.Logf("40 actual native competitions complete: created=%d reparsed=%d; ordering coverage also has deterministic direct-NT and retained-child gates", created, reparsed)
}
