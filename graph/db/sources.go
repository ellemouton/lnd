package graphdb

// Local Source just points to our underlying DB.
type localSource struct {
	DB
}

var _ Source = (*localSource)(nil)
