package services

// StorageUsage is what Settings → Storage shows: the local caches and the
// database, in bytes.
type StorageUsage struct {
	ArtworkBytes      int64 `json:"artworkBytes"`
	ArtworkLimitBytes int64 `json:"artworkLimitBytes"` // settings storage.artCacheMiB
	CatalogBytes      int64 `json:"catalogBytes"`      // cached catalogue responses
	CatalogEntries    int64 `json:"catalogEntries"`
	// DatabaseBytes is the database on disk (with its WAL); the catalogue
	// cache is part of it. DatabaseFreeBytes is what Compact gives back.
	DatabaseBytes     int64 `json:"databaseBytes"`
	DatabaseFreeBytes int64 `json:"databaseFreeBytes"`
	// LastMaintenance is when the database was last optimised this session
	// (Unix ms; 0 when not yet).
	LastMaintenance int64 `json:"lastMaintenance"`
}

// ClearResult reports what Clear removed.
type ClearResult struct {
	ArtworkBytes   int64 `json:"artworkBytes"`
	CatalogEntries int64 `json:"catalogEntries"`
}
