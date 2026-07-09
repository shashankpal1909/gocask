package options

// Options holds configuration settings for the Bitcask database engine.
type Options struct {
	DirPath     string // Directory path where data files and logs will be stored
	MaxFileSize int64  // Maximum size in bytes of an active log file before rotating
	SyncOnPut   bool   // If true, forces fsync after every write operation
}

// DefaultOptions returns standard configuration defaults for local storage.
func DefaultOptions() Options {
	return Options{
		DirPath:     "/tmp/gocask",
		MaxFileSize: 64 * 1024 * 1024, // 64 MB default rotation size
		SyncOnPut:   false,
	}
}
