package mib_parser

import (
	"fmt"
	"strings"
)

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

// String converts an OidNode to dotted string form.
func (oid OidNode) String() string {
	strs := []string{}
	for _, n := range oid.OID {
		strs = append(strs, fmt.Sprintf("%d", n))
	}
	return strings.Join(strs, ".")
}
