package torrent

import (
	"os"
	"path/filepath"
	"testing"
	"time"

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
	defer cl.Close()
	tt, err := cl.AddTorrent(mi)
	qt.Assert(t, qt.IsNil(err))
	<-tt.GotInfo()

	// Let the checks from adding the torrent finish, then queue every piece and look under the
	// same lock: the hashers started meanwhile have taken their pieces off the queue.
	deadline := time.Now().Add(10 * time.Second)
	for {
		cl.lock()
		if cl.activePieceHashers == 0 || time.Now().After(deadline) {
			break
		}
		cl.unlock()
		time.Sleep(time.Millisecond)
	}
	defer cl.unlock()
	for i := range tt.NumPieces() {
		_, err := tt.queuePieceCheck(i)
		qt.Assert(t, qt.IsNil(err))
	}
	st := tt.statsLocked()
	qt.Check(t, qt.Not(qt.Equals(st.PiecesHashing, 0)))
	qt.Check(t, qt.Equals(st.PiecesQueuedForHash+st.PiecesHashing, tt.NumPieces()))
	cst := cl.statsLocked()
	qt.Check(t, qt.Equals(cst.PiecesHashing, st.PiecesHashing))
	qt.Check(t, qt.Equals(cst.PiecesQueuedForHash, st.PiecesQueuedForHash))
}
