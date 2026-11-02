package snapshotmulti_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	dbm "github.com/cosmos/cosmos-db"
	"github.com/cosmos/evm/x/vm/store/snapshotmulti"

	"cosmossdk.io/store/cachekv"
	"cosmossdk.io/store/dbadapter"
	storetypes "cosmossdk.io/store/types"
)

func setupStore() (*snapshotmulti.Store, *storetypes.KVStoreKey) {
	key := storetypes.NewKVStoreKey("store")
	kv := cachekv.NewStore(dbadapter.Store{DB: dbm.NewMemDB()})
	stores := map[*storetypes.KVStoreKey]storetypes.CacheWrap{key: kv}
	return snapshotmulti.NewStoreWithKVStores(stores), key
}

func TestSnapshotMultiIndexing(t *testing.T) {
	snapshotStore, _ := setupStore()

	idx0 := snapshotStore.Snapshot()
	require.Equal(t, 0, idx0)

	idx1 := snapshotStore.Snapshot()
	require.Equal(t, 1, idx1)

	idx2 := snapshotStore.Snapshot()
	require.Equal(t, 2, idx2)
}

func TestSnapshotMultiRevertAndWrite(t *testing.T) {
	snapshotStore, key := setupStore()
	kv := snapshotStore.GetKVStore(key)
	kv.Set([]byte("a"), []byte("1"))

	idx0 := snapshotStore.Snapshot()
	snapshotStore.GetKVStore(key).Set([]byte("b"), []byte("2"))

	idx1 := snapshotStore.Snapshot()
	snapshotStore.GetKVStore(key).Set([]byte("c"), []byte("3"))

	snapshotStore.RevertToSnapshot(idx1)
	require.Nil(t, snapshotStore.GetKVStore(key).Get([]byte("c")))
	require.Equal(t, []byte("2"), snapshotStore.GetKVStore(key).Get([]byte("b")))

	snapshotStore.RevertToSnapshot(idx0)
	require.Nil(t, snapshotStore.GetKVStore(key).Get([]byte("b")))
	require.Equal(t, []byte("1"), snapshotStore.GetKVStore(key).Get([]byte("a")))

	snapshotStore.Snapshot()
	snapshotStore.GetKVStore(key).Set([]byte("d"), []byte("4"))
	snapshotStore.Write()

	require.Equal(t, []byte("4"), kv.Get([]byte("d")))
	idx := snapshotStore.Snapshot()
	require.Equal(t, 0, idx)
}

func TestSnapshotMultiRevertOverwriteSameKey(t *testing.T) {
	// Setup a fresh SnapshotMultiStore and a key
	snapshotStore, key := setupStore()
	kv := snapshotStore.GetKVStore(key)

	// Initial write under key "a"
	kv.Set([]byte("a"), []byte("1"))

	// Overwrite "a" with "2"
	idx0 := snapshotStore.Snapshot()
	snapshotStore.GetKVStore(key).Set([]byte("a"), []byte("2"))

	// Overwrite "a" with "3"
	idx1 := snapshotStore.Snapshot()
	snapshotStore.GetKVStore(key).Set([]byte("a"), []byte("3"))

	// Revert to idx1: expect value "2"
	snapshotStore.RevertToSnapshot(idx1)
	require.Equal(t, []byte("2"), snapshotStore.GetKVStore(key).Get([]byte("a")))

	// Revert to idx0: expect value "1"
	snapshotStore.RevertToSnapshot(idx0)
	require.Equal(t, []byte("1"), snapshotStore.GetKVStore(key).Get([]byte("a")))

	// Overwrite "a" with "4"
	idx2 := snapshotStore.Snapshot()
	snapshotStore.GetKVStore(key).Set([]byte("a"), []byte("4"))
	snapshotStore.Write()

	// After write, the base store should have "4"
	require.Equal(t, []byte("4"), kv.Get([]byte("a")))

	// Write clears the snapshot stack, so reverting to idx2 should panic
	expectedErr := fmt.Sprintf("snapshot index %d out of bound [%d..%d)", idx2, 0, 0)
	require.PanicsWithErrorf(t, expectedErr, func() {
		snapshotStore.RevertToSnapshot(idx2)
	}, "RevertToSnapshot should panic when idx out of bounds")
}

func TestSnapshotMultiInvalidIndex(t *testing.T) {
	snapshotStore, _ := setupStore()
	snapshotStore.Snapshot()

	require.PanicsWithError(t, "snapshot index 1 out of bound [0..1)", func() {
		snapshotStore.RevertToSnapshot(1)
	})

	require.PanicsWithError(t, "snapshot index -1 out of bound [0..1)", func() {
		snapshotStore.RevertToSnapshot(-1)
	})
}

func TestSnapshotMultiGetStore(t *testing.T) {
	snapshotStore, key := setupStore()

	s := snapshotStore.GetStore(key)
	require.NotNil(t, s)
	require.Equal(t, snapshotStore.GetKVStore(key), s)

	badKey := storetypes.NewKVStoreKey("bad")
	require.Panics(t, func() { snapshotStore.GetStore(badKey) })
	require.Panics(t, func() { snapshotStore.GetKVStore(badKey) })
}

func TestSnapshotMultiCacheWrap(t *testing.T) {
	snapshotStore, _ := setupStore()

	wrap := snapshotStore.CacheWrap()
	require.IsType(t, &snapshotmulti.Store{}, wrap)
	require.NotSame(t, snapshotStore, wrap)

	// the branch does not push onto the parent's snapshot stack
	idx := snapshotStore.Snapshot()
	require.Equal(t, 0, idx)
}

func TestSnapshotMultiCacheWrapWithTrace(t *testing.T) {
	snapshotStore, _ := setupStore()

	// NOTES: CacheWrapWithTrace of snapshotmulti.Store is same with regualr CacheWrap,
	// and arguments are not actually used.
	wrap := snapshotStore.CacheWrapWithTrace(nil, nil)
	require.IsType(t, &snapshotmulti.Store{}, wrap)
	require.NotSame(t, snapshotStore, wrap)

	idx := snapshotStore.Snapshot()
	require.Equal(t, 0, idx)
}

func TestSnapshotMultiCacheMultiStore(t *testing.T) {
	snapshotStore, _ := setupStore()

	m := snapshotStore.CacheMultiStore()
	require.IsType(t, &snapshotmulti.Store{}, m)
	require.NotSame(t, snapshotStore, m)

	idx := snapshotStore.Snapshot()
	require.Equal(t, 0, idx)
}

// Writing a CacheMultiStore branch, as sdk.Context.CacheContext's write function
// does, merges into the current layer only and keeps earlier snapshots revertible.
func TestSnapshotMultiCacheMultiStoreWriteKeepsSnapshots(t *testing.T) {
	snapshotStore, key := setupStore()
	base := snapshotStore.GetKVStore(key)
	base.Set([]byte("a"), []byte("1"))

	idx0 := snapshotStore.Snapshot()
	snapshotStore.GetKVStore(key).Set([]byte("b"), []byte("2"))

	branch := snapshotStore.CacheMultiStore()
	require.Equal(t, []byte("2"), branch.GetKVStore(key).Get([]byte("b")))
	branch.GetKVStore(key).Set([]byte("c"), []byte("3"))
	require.Nil(t, snapshotStore.GetKVStore(key).Get([]byte("c")))

	branch.Write()
	require.Equal(t, []byte("3"), snapshotStore.GetKVStore(key).Get([]byte("c")))
	require.Nil(t, base.Get([]byte("c")))
	require.Nil(t, base.Get([]byte("b")))

	require.NotPanics(t, func() { snapshotStore.RevertToSnapshot(idx0) })
	require.Nil(t, snapshotStore.GetKVStore(key).Get([]byte("b")))
	require.Nil(t, snapshotStore.GetKVStore(key).Get([]byte("c")))
	require.Equal(t, []byte("1"), snapshotStore.GetKVStore(key).Get([]byte("a")))
}

// A branch that is dropped without Write leaves the parent unchanged.
func TestSnapshotMultiCacheMultiStoreDiscard(t *testing.T) {
	snapshotStore, key := setupStore()

	idx0 := snapshotStore.Snapshot()
	branch := snapshotStore.CacheMultiStore()
	branch.GetKVStore(key).Set([]byte("x"), []byte("1"))

	require.Nil(t, snapshotStore.GetKVStore(key).Get([]byte("x")))
	require.Equal(t, idx0+1, snapshotStore.Snapshot())
}

// Snapshots taken and reverted inside a branch stay inside it; writing the
// branch then lands in the parent's current layer.
func TestSnapshotMultiCacheMultiStoreNested(t *testing.T) {
	snapshotStore, key := setupStore()

	idx0 := snapshotStore.Snapshot()
	branch := snapshotStore.CacheMultiStore().(*snapshotmulti.Store)
	inner := branch.Snapshot()
	branch.GetKVStore(key).Set([]byte("dropped"), []byte("1"))
	branch.RevertToSnapshot(inner)
	branch.Snapshot()
	branch.GetKVStore(key).Set([]byte("kept"), []byte("1"))
	branch.Write()

	require.Nil(t, snapshotStore.GetKVStore(key).Get([]byte("dropped")))
	require.Equal(t, []byte("1"), snapshotStore.GetKVStore(key).Get([]byte("kept")))

	snapshotStore.RevertToSnapshot(idx0)
	require.Nil(t, snapshotStore.GetKVStore(key).Get([]byte("kept")))
}

func TestSnapshotMultiCacheMultiStoreWithVersion(t *testing.T) {
	snapshotStore, _ := setupStore()

	m, _ := snapshotStore.CacheMultiStoreWithVersion(1)
	require.Equal(t, snapshotStore, m)
}

func TestSnapshotMultiMetadata(t *testing.T) {
	snapshotStore, _ := setupStore()

	require.Equal(t, storetypes.StoreTypeMulti, snapshotStore.GetStoreType())
	require.False(t, snapshotStore.TracingEnabled())
	require.Equal(t, snapshotStore, snapshotStore.SetTracer(nil))
	require.Equal(t, snapshotStore, snapshotStore.SetTracingContext(nil))
}

func TestSnapshotMultiLatestVersion(t *testing.T) {
	snapshotStore, _ := setupStore()

	initialVersion := int64(0)
	ver0 := snapshotStore.LatestVersion()
	require.Equal(t, ver0, initialVersion)

	idx0 := snapshotStore.Snapshot()
	ver1 := snapshotStore.LatestVersion()
	require.Equal(t, ver1, int64(idx0+1))

	idx1 := snapshotStore.Snapshot()
	ver2 := snapshotStore.LatestVersion()
	require.Equal(t, ver2, int64(idx1+1))
}
