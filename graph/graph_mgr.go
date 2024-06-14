package graph

type Config struct {
	Graph Graph
}

// Manager maintains a view of the Lightning Network graph.
type Manager struct {
	cfg *Config
}

func NewManager(cfg *Config) *Manager {
	return &Manager{
		cfg: cfg,
	}
}
