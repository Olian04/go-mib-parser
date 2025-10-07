package mib_parser

import (
	"fmt"
	"strings"
)

type Object interface {
	// OIDString returns the dotted string representation of the object's OID
	// (e.g., "1.3.6.1.2.1").
	OIDString() string
	// OIDSlice returns the numeric OID as a slice of integers.
	OIDSlice() []int
}

// Module represents a parsed SMIv2 MIB module.
// The fields are a minimal but practical subset of SMIv2 that
// are useful for exploring module capabilities and composing SNMP requests.
type Module struct {
	// Name is the ASN.1 module identifier (symbolic name) from the DEFINITIONS header.
	Name string
	// ObjectsByName contains all parsed OBJECT-TYPE definitions in the module,
	// keyed by their symbolic name.
	ObjectsByName map[string]*ObjectType
	// ModuleIdentity is the parsed MODULE-IDENTITY statement (if present),
	// including the module's own OID and metadata such as organization.
	ModuleIdentity *ModuleIdentity
	// ObjectIdentities contains OBJECT-IDENTITY declarations (named OID nodes)
	// keyed by name. These act as anchors within an OID subtree.
	ObjectIdentities map[string]*ObjectIdentity
	// TextualConventions contains parsed TEXTUAL-CONVENTION definitions
	// (named types) keyed by name.
	TextualConventions map[string]*TextualConvention
	// NotificationTypes contains parsed NOTIFICATION-TYPE definitions
	// keyed by name.
	NotificationTypes map[string]*NotificationType
}

// API helpers to explore and construct requests
func (m *Module) GetObjectByName(name string) (*ObjectType, bool) {
	if m == nil || m.ObjectsByName == nil {
		return nil, false
	}
	v, ok := m.ObjectsByName[name]
	return v, ok
}

// GetObjectByOID returns the OBJECT-TYPE whose fully resolved OID matches
// the provided numeric OID slice exactly.
func (m *Module) GetObjectByOID(oid []int) (*ObjectType, bool) {
	if m == nil || m.ObjectsByName == nil {
		return nil, false
	}
	for _, obj := range m.ObjectsByName {
		if oidsEqual(obj.OID, oid) {
			return obj, true
		}
	}
	return nil, false
}

// GetObjectByOIDString returns the OBJECT-TYPE whose OID matches the dotted
// decimal string (e.g., "1.3.6.1.2.1").
func (m *Module) GetObjectByOIDString(oid string) (*ObjectType, bool) {
	if m == nil || m.ObjectsByName == nil {
		return nil, false
	}
	for _, obj := range m.ObjectsByName {
		if oidToString(obj.OID) == oid {
			return obj, true
		}
	}
	return nil, false
}

func oidsEqual(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// ObjectType represents an SMIv2 OBJECT-TYPE definition with its resolved OID.
// It implements the Object interface.
type ObjectType struct {
	// Name is the OBJECT-TYPE's symbolic identifier.
	Name string
	// OID is the fully resolved numeric OID for this object (e.g., 1.3.6.1.2.1.2.2.1.1).
	OID []int
	// Syntax is the declared SYNTAX for the object (e.g., INTEGER, Counter32, Gauge32, OCTET STRING).
	// Any constraints (e.g., SIZE or ranges) are preserved in string form.
	Syntax string
	// Access contains ACCESS or MAX-ACCESS from the definition (e.g., read-only, read-write).
	Access string
	// Status is the object's status (e.g., current, deprecated, obsolete).
	Status string
	// Description is the human-readable DESCRIPTION text.
	Description string
	// Index lists the index objects for tabular objects (INDEX clause).
	// May include SMIv2 IMPLIED semantics; entries are symbolic names as written in the MIB.
	Index []string
}

// ModuleIdentity represents the SMIv2 MODULE-IDENTITY statement.
// It includes the module's OID and administrative metadata.
// It implements the Object interface.
type ModuleIdentity struct {
	// Name is the module identity's symbolic identifier.
	Name string
	// OID is the module identity's numeric OID.
	OID []int
	// LastUpdated is the LAST-UPDATED timestamp string (per RFC 2578 format).
	LastUpdated string
	// Organization is the ORGANIZATION text.
	Organization string
	// ContactInfo is the CONTACT-INFO text.
	ContactInfo string
	// Description is the DESCRIPTION text summarizing the module.
	Description string
}

// ObjectIdentity represents the SMIv2 OBJECT-IDENTITY statement (a named OID).
// It implements the Object interface.
type ObjectIdentity struct {
	// Name is the object's symbolic identifier.
	Name string
	// OID is the numeric OID for this identity node.
	OID []int
	// Status is the identity's status (e.g., current, deprecated, obsolete).
	Status string
	// Description is the human-readable DESCRIPTION text.
	Description string
}

// TextualConvention represents the SMIv2 TEXTUAL-CONVENTION statement
// (a named type alias with semantics and optional display hint).
type TextualConvention struct {
	// Name is the convention's symbolic identifier.
	Name string
	// DisplayHint is the DISPLAY-HINT string, when present.
	DisplayHint string
	// Status is the convention's status (e.g., current, deprecated, obsolete).
	Status string
	// Description is the human-readable DESCRIPTION text.
	Description string
	// Syntax is the underlying base SYNTAX (e.g., OCTET STRING (SIZE(1..32))).
	Syntax string
}

// NotificationType represents the SMIv2 NOTIFICATION-TYPE statement.
// It implements the Object interface.
type NotificationType struct {
	// Name is the notification's symbolic identifier.
	Name string
	// OID is the notification's numeric OID.
	OID []int
	// Objects lists the object names included in the notification payload (OBJECTS clause).
	Objects []string
	// Status is the notification's status (e.g., current, deprecated, obsolete).
	Status string
	// Description is the human-readable DESCRIPTION text.
	Description string
}

// OIDSlice returns the numeric OID for the OBJECT-TYPE.
func (o *ObjectType) OIDSlice() []int {
	return o.OID
}

// OIDString returns the dotted string form of the OBJECT-TYPE's OID.
func (o *ObjectType) OIDString() string {
	return oidToString(o.OID)
}

// OIDSlice returns the numeric OID for the OBJECT-IDENTITY.
func (o *ObjectIdentity) OIDSlice() []int {
	return o.OID
}

// OIDString returns the dotted string form of the OBJECT-IDENTITY's OID.
func (o *ObjectIdentity) OIDString() string {
	return oidToString(o.OID)
}

// OIDSlice returns the numeric OID for the MODULE-IDENTITY.
func (o *ModuleIdentity) OIDSlice() []int {
	return o.OID
}

// OIDString returns the dotted string form of the MODULE-IDENTITY's OID.
func (o *ModuleIdentity) OIDString() string {
	return oidToString(o.OID)
}

// OIDSlice returns the numeric OID for the NOTIFICATION-TYPE.
func (o *NotificationType) OIDSlice() []int {
	return o.OID
}

// OIDString returns the dotted string form of the NOTIFICATION-TYPE's OID.
func (o *NotificationType) OIDString() string {
	return oidToString(o.OID)
}

func oidToString(oid []int) string {
	strs := []string{}
	for _, n := range oid {
		strs = append(strs, fmt.Sprintf("%d", n))
	}
	return strings.Join(strs, ".")
}
