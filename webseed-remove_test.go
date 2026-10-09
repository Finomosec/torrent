package torrent

import (
	"crypto/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-quicktest/qt"

	"github.com/anacrolix/torrent/bencode"
	"github.com/anacrolix/torrent/metainfo"
)

func TestRemovedWebseedsCanBeAddedAgain(t *testing.T) {
	const pieceLen = 4 * defaultChunkSize
	data := make([]byte, 3*pieceLen+5000)
	rand.Read(data)
	dir := t.TempDir()
	qt.Assert(t, qt.IsNil(os.MkdirAll(filepath.Join(dir, "ws"), 0o755)))
	qt.Assert(t, qt.IsNil(os.WriteFile(filepath.Join(dir, "ws", "a.bin"), data, 0o644)))
	root, layer := v2FileHashes(data, pieceLen)
	infoBytes, err := bencode.Marshal(&metainfo.Info{
		Name:        "ws",
		PieceLength: pieceLen,
		MetaVersion: 2,
		FileTree: metainfo.FileTree{Dir: map[string]metainfo.FileTree{
			"a.bin": {File: metainfo.FileTreeFile{Length: int64(len(data)), PiecesRoot: string(root[:])}},
		}},
	})
	qt.Assert(t, qt.IsNil(err))
	srv := httptest.NewServer(http.FileServer(http.Dir(dir)))
	defer srv.Close()

	cl, err := NewClient(TestingConfig(t))
	qt.Assert(t, qt.IsNil(err))
	defer cl.Close()
	tt, err := cl.AddTorrent(&metainfo.MetaInfo{
		InfoBytes:   infoBytes,
		PieceLayers: map[string]string{string(root[:]): string(layer)},
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
	tt.RemoveWebSeeds()
	qt.Assert(t, qt.HasLen(tt.WebseedPeerConns(), 0))
}
