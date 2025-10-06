package mib_parser

// Module represents a parsed MIB module.
// It contains declared OID nodes and OBJECT-TYPE definitions resolved to full OIDs.
type Module struct {
	Name          string
	NodesByName   map[string]*OidNode
	ObjectsByName map[string]*ObjectType
}

// OidNode represents a named node in the OID tree.
// Example: mib-2 => 1.3.6.1.2.1
type OidNode struct {
	Name string
	OID  []int
}

// ObjectType represents an OBJECT-TYPE definition with its resolved OID.
type ObjectType struct {
	Name        string
	OID         []int
	Syntax      string // Kept as string for simplicity (e.g., INTEGER, Counter32, Gauge32, etc.)
	Access      string // MAX-ACCESS/ACCESS
	Status      string
	Description string
	Index       []string // INDEX identifiers if available
}

// API helpers to explore and construct requests
func (m *Module) GetObjectByName(name string) (*ObjectType, bool) {
	if m == nil || m.ObjectsByName == nil {
		return nil, false
	}
	v, ok := m.ObjectsByName[name]
	return v, ok
}

func (m *Module) GetNodeOIDByName(name string) ([]int, bool) {
	if m == nil || m.NodesByName == nil {
		return nil, false
	}
	n, ok := m.NodesByName[name]
	if !ok {
		return nil, false
	}
	return append([]int(nil), n.OID...), true
}

// OIDString converts an OID slice to dotted string form.
func OIDString(oid []int) string {
	if len(oid) == 0 {
		return ""
	}
	// Manual conversion to avoid importing fmt for hot path
	// But clarity is preferred; use fmt for now.
	// The library is not performance critical at this stage.
	s := ""
	for i, n := range oid {
		if i > 0 {
			s += "."
		}
		// safe to use fmt here for correctness and readability
		s += itoa(n)
	}
	return s
}

// itoa is a tiny integer to ascii converter to avoid fmt.Sprintf in tight loops.
// For simplicity and readability, keep it straightforward.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	buf := [20]byte{}
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + (n % 10))
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
