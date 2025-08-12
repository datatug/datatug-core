package fbauth

// Store is an interface for persisting service accounts list and state.
// It allows different backends (file, memory, etc.).
// Keeping a small surface helps testing and makes UI independent from storage.

type Store interface {
	// Load returns the list of service accounts and optional active index/name if needed later.
	Load() ([]ServiceAccountDbo, error)
	// Save persists the list of service accounts.
	Save([]ServiceAccountDbo) error
}
