package mib_parser

import (
	"github.com/Olian04/go-mib-parser/parser"
)

// ParseMIB is the public API entrypoint.
// It parses a MIB module and returns a Module with resolved OIDs and objects.
func ParseMIB(mib []byte) (*Module, error) {
	ir, err := parser.Parse(mib)
	if err != nil {
		return nil, err
	}
	mod := &Module{
		Name:               ir.Name,
		NodesByName:        map[string]*OidNode{},
		ObjectsByName:      map[string]*ObjectType{},
		ObjectIdentities:   map[string]*ObjectIdentity{},
		TextualConventions: map[string]*TextualConvention{},
		NotificationTypes:  map[string]*NotificationType{},
	}
	for name, oid := range ir.NodesByName {
		mod.NodesByName[name] = &OidNode{Name: name, OID: append([]int(nil), oid...)}
	}
	for name, obj := range ir.ObjectsByName {
		mod.ObjectsByName[name] = &ObjectType{
			Name:        obj.Name,
			OID:         append([]int(nil), obj.OID...),
			Syntax:      obj.Syntax,
			Access:      obj.Access,
			Status:      obj.Status,
			Description: obj.Description,
			Index:       append([]string(nil), obj.Index...),
		}
	}
	if ir.ModuleIdentity != nil {
		mod.ModuleIdentity = &ModuleIdentity{
			Name:         ir.ModuleIdentity.Name,
			OID:          append([]int(nil), ir.ModuleIdentity.OID...),
			LastUpdated:  ir.ModuleIdentity.LastUpdated,
			Organization: ir.ModuleIdentity.Organization,
			ContactInfo:  ir.ModuleIdentity.ContactInfo,
			Description:  ir.ModuleIdentity.Description,
		}
	}
	for name, oi := range ir.ObjectIdentities {
		mod.ObjectIdentities[name] = &ObjectIdentity{
			Name:        oi.Name,
			OID:         append([]int(nil), oi.OID...),
			Status:      oi.Status,
			Description: oi.Description,
		}
	}
	for name, tc := range ir.TextualConventions {
		mod.TextualConventions[name] = &TextualConvention{
			Name:        tc.Name,
			DisplayHint: tc.DisplayHint,
			Status:      tc.Status,
			Description: tc.Description,
			Syntax:      tc.Syntax,
		}
	}
	for name, nt := range ir.NotificationTypes {
		mod.NotificationTypes[name] = &NotificationType{
			Name:        nt.Name,
			OID:         append([]int(nil), nt.OID...),
			Objects:     append([]string(nil), nt.Objects...),
			Status:      nt.Status,
			Description: nt.Description,
		}
	}
	return mod, nil
}
