package gobale

// Version returns the library version
func Version() string {
    return "v1.0.0"
}

// Hello returns a greeting message
func Hello(name string) string {
    return "Hello " + name + " from GoBale!"
}

// NewBot creates a new bot instance
func New_Bot(name string) string {
    return "Bot " + name + " created!"
}
