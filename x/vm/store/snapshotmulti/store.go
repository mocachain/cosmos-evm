package snapshotmulti

import (
	"fmt"
	"io"
	"sort"

	"github.com/cosmos/evm/x/vm/store/snapshotkv"
	"github.com/cosmos/evm/x/vm/store/types"
	vmtypes "github.com/cosmos/evm/x/vm/types"

	storetypes "cosmossdk.io/store/types"
)

type Store struct {
	stores    map[storetypes.StoreKey]types.SnapshotKVStore
	storeKeys []*storetypes.KVStoreKey // ordered keys
	head      int
}

var _ types.SnapshotMultiStore = (*Store)(nil)

// NewStore creates a new Store objectwith CacheMultiStore and KVStoreKeys
func NewStore(cms storetypes.CacheMultiStore, keys map[string]*storetypes.KVStoreKey) *Store {
	s := newStore(vmtypes.SortedKVStoreKeys(keys))

	for _, key := range s.storeKeys {
		store := cms.GetKVStore(key).(storetypes.CacheKVStore)
		s.stores[key] = snapshotkv.NewStore(store)
	}

	return s
}

func newStore(keys []*storetypes.KVStoreKey) *Store {
	return &Store{
		stores:    make(map[storetypes.StoreKey]types.SnapshotKVStore, len(keys)),
		storeKeys: keys,
		head:      types.InitialHead,
	}
}

// NewStore creates a new Store object with KVStores
func NewStoreWithKVStores(stores map[*storetypes.KVStoreKey]storetypes.CacheWrap) *Store {
	s := &Store{
		stores: make(map[storetypes.StoreKey]types.SnapshotKVStore),
		head:   types.InitialHead,
	}

	for key, store := range stores {
		s.stores[key] = snapshotkv.NewStore(store.(storetypes.CacheKVStore))
		s.storeKeys = append(s.storeKeys, key)
	}

	sort.Slice(s.storeKeys, func(i, j int) bool {
		return s.storeKeys[i].Name() < s.storeKeys[j].Name()
	})

	return s
}

// Snapshot pushes a new cached context to the stack,
// and returns the index of it.
func (s *Store) Snapshot() int {
	for _, key := range s.storeKeys {
		s.stores[key].Snapshot()
	}
	s.head++

	// latest snapshot is just before head
	return s.head - 1
}

// RevertToSnapshot pops all the cached stores
// whose index is greator than or equal to target.
// The target should be snapshot index returned by `Snapshot`.
// This function panics if the index is out of bounds.
func (s *Store) RevertToSnapshot(target int) {
	for _, key := range s.storeKeys {
		s.stores[key].RevertToSnapshot(target)
	}
	s.head = target
}

// GetStoreType returns the type of the store.
func (s *Store) GetStoreType() storetypes.StoreType {
	return storetypes.StoreTypeMulti
}

// Implements CacheWrapper.
func (s *Store) CacheWrap() storetypes.CacheWrap {
	return s.CacheMultiStore().(storetypes.CacheWrap)
}

// CacheWrapWithTrace implements the CacheWrapper interface.
//
// NOTE: CacheWrapWithTrace is a method that enables a Store to satisfy the CacheWrapper interface.
// Although it accepts an io.Writer and tracingContext as inputs, these are not used in the implementation.
// It returns the same branch as CacheWrap.
func (s *Store) CacheWrapWithTrace(_ io.Writer, _ storetypes.TraceContext) storetypes.CacheWrap {
	return s.CacheWrap()
}

// CacheMultiStore returns a new Store branched off the current top of the
// snapshot stack, which is what sdk.Context.CacheContext() hands to code running
// inside a precompile. Writing the branch merges its changes into the current
// layer only, and dropping it discards them. The snapshot stack of s is left
// untouched, so a snapshot taken before the branch can still be reverted to.
func (s *Store) CacheMultiStore() storetypes.CacheMultiStore {
	branch := newStore(s.storeKeys)
	for _, key := range s.storeKeys {
		branch.stores[key] = s.stores[key].Branch()
	}
	return branch
}

// CacheMultiStoreWithVersion load stores at a snapshot version.
//
// NOTE: CacheMultiStoreWithVersion is no-op function.
func (s *Store) CacheMultiStoreWithVersion(_ int64) (storetypes.CacheMultiStore, error) {
	return s, nil
}

// GetStore returns an underlying Store by key.
func (s *Store) GetStore(key storetypes.StoreKey) storetypes.Store {
	store := s.stores[key]
	if key == nil || store == nil {
		panic(fmt.Sprintf("kv store with key %v has not been registered in stores", key))
	}
	return store.CurrentStore()
}

// GetKVStore returns an underlying KVStore by key.
func (s *Store) GetKVStore(key storetypes.StoreKey) storetypes.KVStore {
	store := s.stores[key]
	if key == nil || store == nil {
		panic(fmt.Sprintf("kv store with key %v has not been registered in stores", key))
	}
	return store.CurrentStore()
}

// TracingEnabled returns if tracing is enabled for the MultiStore.
func (s *Store) TracingEnabled() bool {
	return false
}

// SetTracer sets the tracer for the MultiStore that the underlying
// stores will utilize to trace operations. A MultiStore is returned.
//
// NOTE: SetTracer no-op function.
func (s *Store) SetTracer(_ io.Writer) storetypes.MultiStore {
	return s
}

// SetTracingContext updates the tracing context for the MultiStore by merging
// the given context with the existing context by key. Any existing keys will
// be overwritten. It is implied that the caller should update the context when
// necessary between tracing operations. It returns a modified MultiStore.
//
// NOTE: SetTracingContext no-op function
func (s *Store) SetTracingContext(_ storetypes.TraceContext) storetypes.MultiStore {
	return s
}

// LatestVersion returns the branch version of the store
func (s *Store) LatestVersion() int64 {
	return int64(s.head)
}

// Write calls Write on each underlying store.
func (s *Store) Write() {
	for _, key := range s.storeKeys {
		s.stores[key].Commit()
	}
	s.head = types.InitialHead
}
