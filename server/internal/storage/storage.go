package storage

// AppRef points to one stored app version.
type AppRef struct {
	Slug    string
	Version string
	Path    string
}

// Provider abstracts file storage.
type Provider interface {
	Save(slug, version, filename string, data []byte) (string, error)
	Open(slug, version, filename string) ([]byte, error)
	List() ([]AppRef, error)
}
