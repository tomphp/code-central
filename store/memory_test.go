package store_test

import (
	"testing"

	"github.com/tomoram/code-central/store"
	"github.com/tomoram/code-central/store/storetest"
)

func TestMemoryStoreContract(t *testing.T) {
	storetest.Run(t, storetest.Factory{
		New: func(t *testing.T) store.Store { return store.NewMemory() },
	})
}
