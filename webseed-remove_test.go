package torrent

import (
	"crypto/rand"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-quicktest/qt"

	"github.com/anacrolix/torrent/internal/testutil"
)

// Web seeds that were removed can be added again, and then still download.
func TestRemovedWebseedsCanBeAddedAgain(t *testing.T) {
	const pieceLen = 2 * defaultChunkSize
	data := make([]byte, 3*pieceLen+5000)
	rand.Read(data)
	tu := testutil.Torrent{
		Name:  "testdata",
		Files: []testutil.File{{Name: "a.bin", Data: string(data)}},
	}
	mi, _ := tu.Generate(int64(pieceLen))

	dir := t.TempDir()
	qt.Assert(t, qt.IsNil(os.MkdirAll(filepath.Join(dir, "testdata"), 0o755)))
	qt.Assert(t, qt.IsNil(os.WriteFile(filepath.Join(dir, "testdata", "a.bin"), data, 0o644)))
	srv := httptest.NewServer(http.FileServer(http.Dir(dir)))
	defer srv.Close()

	cl, err := NewClient(TestingConfig(t))
	qt.Assert(t, qt.IsNil(err))
	defer cl.Close()
	tt, _, err := cl.AddTorrentSpec(&TorrentSpec{
		AddTorrentOpts: AddTorrentOpts{
			InfoHash:  mi.HashInfoBytes(),
			InfoBytes: mi.InfoBytes,
		},
	})
	qt.Assert(t, qt.IsNil(err))
	<-tt.GotInfo()

	urls := []string{srv.URL + "/"}
	tt.AddWebSeeds(urls)
	qt.Assert(t, qt.HasLen(tt.WebseedPeerConns(), 1))
	tt.RemoveWebSeeds()
	qt.Assert(t, qt.HasLen(tt.WebseedPeerConns(), 0))

	tt.AddWebSeeds(urls)
	qt.Assert(t, qt.HasLen(tt.WebseedPeerConns(), 1))
	tt.DownloadAll()
	done := make(chan bool)
	go func() { done <- cl.WaitAll() }()
	select {
	case ok := <-done:
		qt.Assert(t, qt.IsTrue(ok))
	case <-time.After(10 * time.Second):
		t.Fatalf("download stalled at %v of %v bytes", tt.BytesCompleted(), tt.Length())
	}
	r := tt.NewReader()
	defer r.Close()
	got, err := io.ReadAll(r)
	qt.Assert(t, qt.IsNil(err))
	qt.Assert(t, qt.DeepEquals(got, data))

	tt.RemoveWebSeeds()
	qt.Assert(t, qt.HasLen(tt.WebseedPeerConns(), 0))
}

// Removing a web seed while it serves a request cancels that request.
func TestRemovingWebseedsCancelsRequestsInFlight(t *testing.T) {
	const pieceLen = 2 * defaultChunkSize
	data := make([]byte, 4*pieceLen)
	rand.Read(data)
	tu := testutil.Torrent{
		Name:  "testdata",
		Files: []testutil.File{{Name: "a.bin", Data: string(data)}},
	}
	mi, _ := tu.Generate(int64(pieceLen))

	started := make(chan struct{}, 1)
	cancelled := make(chan struct{}, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case started <- struct{}{}:
		default:
		}
		select {
		case <-r.Context().Done():
			select {
			case cancelled <- struct{}{}:
			default:
			}
		case <-time.After(10 * time.Second):
		}
	}))
	defer srv.Close()

	cl, err := NewClient(TestingConfig(t))
	qt.Assert(t, qt.IsNil(err))
	defer cl.Close()
	tt, _, err := cl.AddTorrentSpec(&TorrentSpec{
		AddTorrentOpts: AddTorrentOpts{
			InfoHash:  mi.HashInfoBytes(),
			InfoBytes: mi.InfoBytes,
		},
	})
	qt.Assert(t, qt.IsNil(err))
	<-tt.GotInfo()
	tt.AddWebSeeds([]string{srv.URL + "/"})
	tt.DownloadAll()

	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("the web seed was never asked")
	}
	tt.RemoveWebSeeds()
	select {
	case <-cancelled:
	case <-time.After(5 * time.Second):
		t.Fatal("the request in flight was not cancelled")
	}
	qt.Assert(t, qt.HasLen(tt.WebseedPeerConns(), 0))
}
