package discover

// ParseAutoconfig reads a Mozilla autoconfig document (config-v1.1.xml) for
// an address. Task T-0049 implements it; the stub finds nothing.
func ParseAutoconfig(_ []byte, _ string) (Settings, error) { return Settings{}, ErrNothing }
