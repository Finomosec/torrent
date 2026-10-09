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

// addV2MultiPiece adds testdata/v2-multi-piece with its data in storage, after letting corrupt
// change it.
func addV2MultiPiece(t *testing.T, corrupt func(data []byte)) (*Client, *Torrent) {
	mi, err := metainfo.LoadFromFile("testdata/v2-multi-piece.torrent")
	qt.Assert(t, qt.IsNil(err))
	dir := t.TempDir()
	data, err := os.ReadFile("testdata/v2-multi-piece/data.bin")
	qt.Assert(t, qt.IsNil(err))
	if corrupt != nil {
		corrupt(data)
	}
	qt.Assert(t, qt.IsNil(os.MkdirAll(filepath.Join(dir, "v2multi"), 0o755)))
	qt.Assert(t, qt.IsNil(os.WriteFile(filepath.Join(dir, "v2multi", "data.bin"), data, 0o644)))

	cfg := TestingConfig(t)
	cfg.DefaultStorage = storage.NewFileOpts(storage.NewFileClientOpts{ClientBaseDir: dir})
	cl, err := NewClient(cfg)
	qt.Assert(t, qt.IsNil(err))
	t.Cleanup(func() { cl.Close() })
	tt, err := cl.AddTorrent(mi)
	qt.Assert(t, qt.IsNil(err))
	<-tt.GotInfo()
	return cl, tt
}

func TestStatsCountPiecesHashed(t *testing.T) {
	cl, tt := addV2MultiPiece(t, func(data []byte) { data[0]++ })
	qt.Assert(t, qt.IsNil(tt.VerifyData()))

	// Checks from adding the torrent may have run as well, so every piece was hashed at least once.
	st := tt.Stats()
	good, bad := st.PiecesHashedGood.Int64(), st.PiecesHashedBad.Int64()
	qt.Check(t, qt.IsTrue(bad >= 1), qt.Commentf("bad: %v", bad))
	qt.Check(t, qt.IsTrue(good >= int64(tt.NumPieces()-1)), qt.Commentf("good: %v of %v pieces", good, tt.NumPieces()))
	cst := cl.Stats()
	qt.Check(t, qt.Equals(cst.PiecesHashedGood.Int64(), good))
	qt.Check(t, qt.Equals(cst.PiecesHashedBad.Int64(), bad))
}

func TestStatsCountPiecesQueuedForHash(t *testing.T) {
	cl, tt := addV2MultiPiece(t, nil)

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
