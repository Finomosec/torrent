package torrent

import (
	"os"
	"path/filepath"
	"testing"

	qt "github.com/go-quicktest/qt"

	"github.com/anacrolix/torrent/metainfo"
	"github.com/anacrolix/torrent/storage"
)

func TestStatsCountPiecesQueuedForHash(t *testing.T) {
	mi, err := metainfo.LoadFromFile("testdata/v2-multi-piece.torrent")
	qt.Assert(t, qt.IsNil(err))
	dir := t.TempDir()
	data, err := os.ReadFile("testdata/v2-multi-piece/data.bin")
	qt.Assert(t, qt.IsNil(err))
	qt.Assert(t, qt.IsNil(os.MkdirAll(filepath.Join(dir, "v2multi"), 0o755)))
	qt.Assert(t, qt.IsNil(os.WriteFile(filepath.Join(dir, "v2multi", "data.bin"), data, 0o644)))

	cfg := TestingConfig(t)
	cfg.DefaultStorage = storage.NewFileOpts(storage.NewFileClientOpts{ClientBaseDir: dir})
	cl, err := NewClient(cfg)
	qt.Assert(t, qt.IsNil(err))
	t.Cleanup(func() { cl.Close() })
	setMaxActivePieceHashers(t, cl, 0)
	tt, err := cl.AddTorrent(mi)
	qt.Assert(t, qt.IsNil(err))
	<-tt.GotInfo()

	// No hashers may run, so every piece queued for a check waits.
	cl.lock()
	for i := range tt.NumPieces() {
		_, err := tt.queuePieceCheck(i)
		qt.Assert(t, qt.IsNil(err))
	}
	cl.unlock()
	st := tt.Stats()
	qt.Check(t, qt.Equals(st.PiecesQueuedForHash, tt.NumPieces()))
	qt.Check(t, qt.Equals(st.PiecesHashing, 0))
	qt.Check(t, qt.Equals(cl.Stats().PiecesQueuedForHash, tt.NumPieces()))

	cl.lock()
	maxActivePieceHashers = 1
	cl.startPieceHashers()
	st = tt.statsLocked()
	cl.unlock()
	qt.Check(t, qt.Equals(st.PiecesHashing, 1))
	qt.Check(t, qt.Equals(st.PiecesQueuedForHash, tt.NumPieces()-1))
}
