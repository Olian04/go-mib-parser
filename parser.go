package mib_parser

import (
	coreparser "github.com/Olian04/go-mib-parser/parser"
)

// ParseMIB is the public API entrypoint.
// It parses a MIB module and returns a Module with resolved OIDs and objects.
func ParseMIB(mib []byte) (*Module, error) {
	ir, err := coreparser.Parse(mib)
	if err != nil {
		return nil, err
	}
	mod := &Module{
		Name:          ir.Name,
		NodesByName:   map[string]*OidNode{},
		ObjectsByName: map[string]*ObjectType{},
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
	return mod, nil
}
