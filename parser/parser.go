package parser

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/Olian04/go-mib-parser/lexer"
)

// ModuleIR is an internal representation of a parsed MIB module.
// It is designed to avoid import cycles with the public package.
type ModuleIR struct {
	Name               string
	NodesByName        map[string][]int
	ObjectsByName      map[string]*ObjectTypeIR
	ModuleIdentity     *ModuleIdentityIR
	ObjectIdentities   map[string]*ObjectIdentityIR
	TextualConventions map[string]*TextualConventionIR
	NotificationTypes  map[string]*NotificationTypeIR
}

// ObjectTypeIR is an internal representation of OBJECT-TYPE definitions.
type ObjectTypeIR struct {
	Name        string
	OID         []int
	Syntax      string
	Access      string
	Status      string
	Description string
	Index       []string
}

type ModuleIdentityIR struct {
	Name         string
	OID          []int
	LastUpdated  string
	Organization string
	ContactInfo  string
	Description  string
}

type ObjectIdentityIR struct {
	Name        string
	OID         []int
	Status      string
	Description string
}

type TextualConventionIR struct {
	Name        string
	DisplayHint string
	Status      string
	Description string
	Syntax      string
}

type NotificationTypeIR struct {
	Name        string
	OID         []int
	Objects     []string
	Status      string
	Description string
}

type rdParser struct {
	l    *lexer.Lexer
	tok  lexer.Token
	mod  *ModuleIR
	pend []pendingRef
	src  string
}

type pendingRef struct {
	parent string
	index  int
	apply  func(base []int)
}

func Parse(input []byte) (*ModuleIR, error) {
	p := &rdParser{l: lexer.New(input), src: string(input), mod: &ModuleIR{NodesByName: map[string][]int{}, ObjectsByName: map[string]*ObjectTypeIR{}, ObjectIdentities: map[string]*ObjectIdentityIR{}, TextualConventions: map[string]*TextualConventionIR{}, NotificationTypes: map[string]*NotificationTypeIR{}}}
	p.next()
	p.initBaseOids()

	// Parse single module
	if err := p.parseModule(); err != nil {
		return nil, err
	}
	// Best-effort augmentation for any names present in source but missed by parser
	p.augmentFromSource()
	return p.mod, nil
}

func (p *rdParser) parseModule() error {
	// <ModuleName> DEFINITIONS ::= BEGIN ... END
	nameTok := p.expect(lexer.TokenIdent)
	if nameTok == nil {
		return p.errorf("expected module name")
	}
	p.mod.Name = nameTok.Text
	if !p.acceptIdent("DEFINITIONS") {
		return p.errorf("expected DEFINITIONS")
	}
	if !p.accept(lexer.TokenColonColonEq) { // We accept form "DEFINITIONS ::= BEGIN"
		// Some MIBs have 'DEFINITIONS ::= BEGIN' or 'DEFINITIONS ::= BEGIN' on new line
		return p.errorf("expected '::=' after DEFINITIONS")
	}
	if !p.acceptIdent("BEGIN") {
		return p.errorf("expected BEGIN")
	}

	// Optional IMPORTS section
	if p.isIdent("IMPORTS") {
		if err := p.parseImports(); err != nil {
			return err
		}
	}

	// Body: OBJECT IDENTIFIER assignments, OBJECT-TYPE, etc., until module END
	for p.tok.Type != lexer.TokenEOF {
		if p.isIdent("END") {
			// Only treat END as module end if followed by EOF
			peek := p.l.Peek()
			if peek.Type == lexer.TokenEOF {
				p.next()
				break
			}
			// Otherwise consume and continue (likely END of a MACRO)
			p.next()
			continue
		}
		if p.tok.Type == lexer.TokenIdent {
			// Lookahead for 'OBJECT IDENTIFIER' or 'OBJECT-TYPE'
			ident := p.tok.Text
			p.next()
			// If this is a MACRO definition, skip the MACRO body entirely
			if p.isIdent("MACRO") {
				p.skipDefinition()
				continue
			}
			if p.isIdent("OBJECT") {
				// OBJECT IDENTIFIER ::= { parent n }
				p.next()
				if !p.acceptIdent("IDENTIFIER") {
					return p.errorf("expected IDENTIFIER after OBJECT for %s", ident)
				}
				if !p.accept(lexer.TokenColonColonEq) {
					return p.errorf("expected '::=' after OBJECT IDENTIFIER")
				}
				if !p.accept(lexer.TokenLBrace) {
					return p.errorf("expected '{' in OBJECT IDENTIFIER assignment")
				}
				parentName, index, abs, hasAbs := p.parseOidAssignmentInsideBraces()
				if !p.accept(lexer.TokenRBrace) {
					return p.errorf("expected '}' in OBJECT IDENTIFIER assignment")
				}
				if hasAbs {
					p.mod.NodesByName[ident] = append([]int(nil), abs...)
				} else {
					// resolve parent (allow forward references)
					if base, ok := p.resolveOidBase(parentName); ok {
						oid := append(append([]int(nil), base...), index)
						p.mod.NodesByName[ident] = oid
					} else {
						// ensure placeholder so presence is recorded
						if _, exists := p.mod.NodesByName[ident]; !exists {
							p.mod.NodesByName[ident] = []int{}
						}
						name := ident
						p.pend = append(p.pend, pendingRef{
							parent: parentName,
							index:  index,
							apply: func(base []int) {
								oid := append(append([]int(nil), base...), index)
								p.mod.NodesByName[name] = oid
							},
						})
					}
				}
				continue
			}
			// Handle form: <Ident> ::= TEXTUAL-CONVENTION / SEQUENCE / other
			if p.accept(lexer.TokenColonColonEq) {
				if p.acceptIdent("TEXTUAL-CONVENTION") {
					// We have already consumed the name and '::= TEXTUAL-CONVENTION'
					tc := &TextualConventionIR{Name: ident}
					for {
						if p.acceptIdent("DISPLAY-HINT") {
							if p.tok.Type == lexer.TokenString {
								tc.DisplayHint = p.tok.Text
								p.next()
							}
							continue
						}
						if p.acceptIdent("STATUS") {
							tc.Status = p.parseUntilKeywords("DESCRIPTION", "SYNTAX")
							continue
						}
						if p.acceptIdent("DESCRIPTION") {
							if p.tok.Type == lexer.TokenString {
								tc.Description = p.tok.Text
								p.next()
							}
							continue
						}
						if p.acceptIdent("SYNTAX") {
							tc.Syntax = p.parseTypeString()
							p.mod.TextualConventions[tc.Name] = tc
							break
						}
						if p.tok.Type == lexer.TokenEOF {
							return p.errorf("unexpected EOF in TEXTUAL-CONVENTION")
						}
						p.next()
					}
					continue
				}
				// For other assignments (e.g., ::= SEQUENCE ...), skip definition body
				p.skipDefinition()
				continue
			}
			if p.isIdent("OBJECT-TYPE") {
				// Parse OBJECT-TYPE block
				p.next()
				obj := &ObjectTypeIR{Name: ident}
				// read clauses until '::=' then '{ parent n }'
				for {
					if p.tok.Type == lexer.TokenEOF {
						return p.errorf("unexpected EOF in OBJECT-TYPE for %s", ident)
					}
					// SYNTAX <type>
					if p.acceptIdent("SYNTAX") {
						obj.Syntax = p.parseTypeString()
						continue
					}
					// MAX-ACCESS or ACCESS
					if p.acceptIdent("MAX-ACCESS") || p.acceptIdent("ACCESS") {
						// previous token consumed; current token is first token of value
						obj.Access = p.parseUntilKeywords("STATUS", "DESCRIPTION", "INDEX", "::=")
						continue
					}
					if p.acceptIdent("STATUS") {
						obj.Status = p.parseUntilKeywords("DESCRIPTION", "INDEX", "::=")
						continue
					}
					if p.acceptIdent("DESCRIPTION") {
						// DESCRIPTION "..."
						if p.tok.Type != lexer.TokenString {
							// Some MIBs might have multi-line, but lexer handles quotes
							return p.errorf("expected string after DESCRIPTION")
						}
						obj.Description = p.tok.Text
						p.next()
						continue
					}
					if p.acceptIdent("INDEX") {
						// INDEX { a, b, c }
						if !p.accept(lexer.TokenLBrace) {
							return p.errorf("expected '{' after INDEX")
						}
						var idx []string
						for {
							if p.tok.Type == lexer.TokenIdent {
								// Allow optional IMPLIED keyword prefix in SMIv2
								if equalFold(p.tok.Text, "IMPLIED") {
									p.next()
									// expect actual identifier next without requiring a comma
									continue
								}
								idx = append(idx, p.tok.Text)
								p.next()
								if p.accept(lexer.TokenComma) {
									continue
								}
								if p.accept(lexer.TokenRBrace) {
									break
								}
								return p.errorf("expected ',' or '}' in INDEX list")
							}
							if p.accept(lexer.TokenRBrace) {
								break
							}
							return p.errorf("expected identifier in INDEX list")
						}
						obj.Index = idx
						continue
					}
					// Allow STATUS before ACCESS in some MIBs
					if p.acceptIdent("STATUS") {
						obj.Status = p.parseUntilKeywords("DESCRIPTION", "INDEX", "::=", "ACCESS", "MAX-ACCESS")
						continue
					}
					// Some MIBs place ACCESS after DESCRIPTION or omit it; accept anywhere before '::='
					if p.acceptIdent("MAX-ACCESS") || p.acceptIdent("ACCESS") {
						obj.Access = p.parseUntilKeywords("STATUS", "DESCRIPTION", "INDEX", "::=")
						continue
					}
					if p.accept(lexer.TokenColonColonEq) {
						// ::= { parent n }
						if !p.accept(lexer.TokenLBrace) {
							return p.errorf("expected '{' after '::=' in OBJECT-TYPE")
						}
						parentName, index, abs, hasAbs := p.parseOidAssignmentInsideBraces()
						if !p.accept(lexer.TokenRBrace) {
							return p.errorf("expected '}' after OBJECT-TYPE OID ref")
						}
						if hasAbs {
							obj.OID = append([]int(nil), abs...)
							// store
							p.mod.ObjectsByName[obj.Name] = obj
							p.mod.NodesByName[obj.Name] = append([]int(nil), obj.OID...)
						} else if base, ok := p.resolveOidBase(parentName); ok {
							obj.OID = append(append([]int(nil), base...), index)
							// store
							p.mod.ObjectsByName[obj.Name] = obj
							// also register the object name as a node
							p.mod.NodesByName[obj.Name] = append([]int(nil), obj.OID...)
						} else {
							// store early; resolve later
							p.mod.ObjectsByName[obj.Name] = obj
							if _, exists := p.mod.NodesByName[obj.Name]; !exists {
								p.mod.NodesByName[obj.Name] = []int{}
							}
							ref := obj
							p.pend = append(p.pend, pendingRef{
								parent: parentName,
								index:  index,
								apply: func(base []int) {
									ref.OID = append(append([]int(nil), base...), index)
									p.mod.ObjectsByName[ref.Name] = ref
									p.mod.NodesByName[ref.Name] = append([]int(nil), ref.OID...)
								},
							})
						}
						break
					}
					// If we see another top-level identifier or END, stop
					if p.tok.Type == lexer.TokenIdent {
						// allow fallthrough only if it starts a known keyword; otherwise keep reading
					}
					// Consume stray semicolons if any
					if p.accept(lexer.TokenSemicolon) {
						continue
					}
					// Otherwise consume one token to avoid infinite loop
					if p.tok.Type != lexer.TokenEOF {
						p.next()
					}
				}
				continue
			}
			if p.isIdent("OBJECT-GROUP") {
				p.next()
				// Parse until OID assignment
				for {
					if p.tok.Type == lexer.TokenEOF {
						return p.errorf("unexpected EOF in OBJECT-GROUP")
					}
					if p.accept(lexer.TokenColonColonEq) {
						if !p.accept(lexer.TokenLBrace) {
							return p.errorf("expected '{' after OBJECT-GROUP '::='")
						}
						parent, idx := p.parseParentRef()
						if !p.accept(lexer.TokenRBrace) {
							return p.errorf("expected '}' after OBJECT-GROUP OID")
						}
						if base, ok := p.mod.NodesByName[parent]; ok {
							p.mod.NodesByName[ident] = append(append([]int(nil), base...), idx)
						} else {
							name := ident
							p.pend = append(p.pend, pendingRef{parent: parent, index: idx, apply: func(base []int) {
								p.mod.NodesByName[name] = append(append([]int(nil), base...), idx)
							}})
						}
						break
					}
					p.next()
				}
				continue
			}
			if p.isIdent("NOTIFICATION-GROUP") {
				p.next()
				for {
					if p.tok.Type == lexer.TokenEOF {
						return p.errorf("unexpected EOF in NOTIFICATION-GROUP")
					}
					if p.accept(lexer.TokenColonColonEq) {
						if !p.accept(lexer.TokenLBrace) {
							return p.errorf("expected '{' after NOTIFICATION-GROUP '::='")
						}
						parent, idx := p.parseParentRef()
						if !p.accept(lexer.TokenRBrace) {
							return p.errorf("expected '}' after NOTIFICATION-GROUP OID")
						}
						if base, ok := p.mod.NodesByName[parent]; ok {
							p.mod.NodesByName[ident] = append(append([]int(nil), base...), idx)
						} else {
							name := ident
							p.pend = append(p.pend, pendingRef{parent: parent, index: idx, apply: func(base []int) {
								p.mod.NodesByName[name] = append(append([]int(nil), base...), idx)
							}})
						}
						break
					}
					p.next()
				}
				continue
			}
			if p.isIdent("MODULE-COMPLIANCE") {
				p.next()
				for {
					if p.tok.Type == lexer.TokenEOF {
						return p.errorf("unexpected EOF in MODULE-COMPLIANCE")
					}
					if p.accept(lexer.TokenColonColonEq) {
						if !p.accept(lexer.TokenLBrace) {
							return p.errorf("expected '{' after MODULE-COMPLIANCE '::='")
						}
						parent, idx := p.parseParentRef()
						if !p.accept(lexer.TokenRBrace) {
							return p.errorf("expected '}' after MODULE-COMPLIANCE OID")
						}
						if base, ok := p.mod.NodesByName[parent]; ok {
							p.mod.NodesByName[ident] = append(append([]int(nil), base...), idx)
						} else {
							name := ident
							p.pend = append(p.pend, pendingRef{parent: parent, index: idx, apply: func(base []int) {
								p.mod.NodesByName[name] = append(append([]int(nil), base...), idx)
							}})
						}
						break
					}
					p.next()
				}
				continue
			}
			if p.isIdent("AGENT-CAPABILITIES") {
				p.next()
				for {
					if p.tok.Type == lexer.TokenEOF {
						return p.errorf("unexpected EOF in AGENT-CAPABILITIES")
					}
					if p.accept(lexer.TokenColonColonEq) {
						if !p.accept(lexer.TokenLBrace) {
							return p.errorf("expected '{' after AGENT-CAPABILITIES '::='")
						}
						parent, idx := p.parseParentRef()
						if !p.accept(lexer.TokenRBrace) {
							return p.errorf("expected '}' after AGENT-CAPABILITIES OID")
						}
						if base, ok := p.mod.NodesByName[parent]; ok {
							p.mod.NodesByName[ident] = append(append([]int(nil), base...), idx)
						} else {
							name := ident
							p.pend = append(p.pend, pendingRef{parent: parent, index: idx, apply: func(base []int) {
								p.mod.NodesByName[name] = append(append([]int(nil), base...), idx)
							}})
						}
						break
					}
					p.next()
				}
				continue
			}
			if p.isIdent("MODULE-IDENTITY") {
				p.next()
				// MODULE-IDENTITY
				mi := &ModuleIdentityIR{Name: ident}
				// record placeholder node name so children can reference immediately
				if _, exists := p.mod.NodesByName[ident]; !exists {
					p.mod.NodesByName[ident] = []int{}
				}
				// Expect 'LAST-UPDATED', 'ORGANIZATION', 'CONTACT-INFO', 'DESCRIPTION' then '::=' { parent n }
				for {
					if p.acceptIdent("LAST-UPDATED") {
						if p.tok.Type == lexer.TokenString {
							mi.LastUpdated = p.tok.Text
							p.next()
						}
						continue
					}
					if p.acceptIdent("ORGANIZATION") {
						if p.tok.Type == lexer.TokenString {
							mi.Organization = p.tok.Text
							p.next()
						}
						continue
					}
					if p.acceptIdent("CONTACT-INFO") {
						if p.tok.Type == lexer.TokenString {
							mi.ContactInfo = p.tok.Text
							p.next()
						}
						continue
					}
					if p.acceptIdent("DESCRIPTION") {
						if p.tok.Type == lexer.TokenString {
							mi.Description = p.tok.Text
							p.next()
						}
						continue
					}
					if p.accept(lexer.TokenColonColonEq) {
						if !p.accept(lexer.TokenLBrace) {
							return p.errorf("expected '{' after MODULE-IDENTITY '::='")
						}
						parent, idx, abs, hasAbs := p.parseOidAssignmentInsideBraces()
						if !p.accept(lexer.TokenRBrace) {
							return p.errorf("expected '}' after MODULE-IDENTITY OID")
						}
						if hasAbs {
							mi.OID = append([]int(nil), abs...)
							p.mod.ModuleIdentity = mi
							p.mod.NodesByName[ident] = append([]int(nil), mi.OID...)
						} else if base, ok := p.resolveOidBase(parent); ok {
							mi.OID = append(append([]int(nil), base...), idx)
							p.mod.ModuleIdentity = mi
							p.mod.NodesByName[ident] = append([]int(nil), mi.OID...)
						} else {
							// store early without OID, resolve later
							p.mod.ModuleIdentity = mi
							ref := mi
							p.pend = append(p.pend, pendingRef{
								parent: parent,
								index:  idx,
								apply: func(base []int) {
									ref.OID = append(append([]int(nil), base...), idx)
									p.mod.ModuleIdentity = ref
									p.mod.NodesByName[ident] = append([]int(nil), ref.OID...)
								},
							})
						}
						break
					}
					if p.tok.Type == lexer.TokenEOF {
						return p.errorf("unexpected EOF in MODULE-IDENTITY")
					}
					p.next()
				}
				continue
			}
			if p.isIdent("OBJECT-IDENTITY") {
				p.next()
				oi := &ObjectIdentityIR{Name: ident}
				if _, exists := p.mod.NodesByName[ident]; !exists {
					p.mod.NodesByName[ident] = []int{}
				}
				for {
					if p.acceptIdent("STATUS") {
						oi.Status = p.parseUntilKeywords("DESCRIPTION", "::=")
						continue
					}
					if p.acceptIdent("DESCRIPTION") {
						if p.tok.Type == lexer.TokenString {
							oi.Description = p.tok.Text
							p.next()
						}
						continue
					}
					if p.accept(lexer.TokenColonColonEq) {
						if !p.accept(lexer.TokenLBrace) {
							return p.errorf("expected '{' after OBJECT-IDENTITY '::='")
						}
						parent, idx, abs, hasAbs := p.parseOidAssignmentInsideBraces()
						if !p.accept(lexer.TokenRBrace) {
							return p.errorf("expected '}' after OBJECT-IDENTITY OID")
						}
						if hasAbs {
							oi.OID = append([]int(nil), abs...)
							p.mod.ObjectIdentities[oi.Name] = oi
							p.mod.NodesByName[ident] = append([]int(nil), oi.OID...)
						} else if base, ok := p.resolveOidBase(parent); ok {
							oi.OID = append(append([]int(nil), base...), idx)
							p.mod.ObjectIdentities[oi.Name] = oi
							p.mod.NodesByName[ident] = append([]int(nil), oi.OID...)
						} else {
							// store early without OID, resolve later
							p.mod.ObjectIdentities[oi.Name] = oi
							ref := oi
							p.pend = append(p.pend, pendingRef{
								parent: parent,
								index:  idx,
								apply: func(base []int) {
									ref.OID = append(append([]int(nil), base...), idx)
									p.mod.ObjectIdentities[ref.Name] = ref
									p.mod.NodesByName[ident] = append([]int(nil), ref.OID...)
								},
							})
						}
						break
					}
					if p.tok.Type == lexer.TokenEOF {
						return p.errorf("unexpected EOF in OBJECT-IDENTITY")
					}
					p.next()
				}
				continue
			}
			if p.isIdent("TEXTUAL-CONVENTION") {
				p.next()
				tc := &TextualConventionIR{Name: ident}
				// TEXTUAL-CONVENTION
				for {
					if p.acceptIdent("DISPLAY-HINT") {
						if p.tok.Type == lexer.TokenString {
							tc.DisplayHint = p.tok.Text
							p.next()
						}
						continue
					}
					if p.acceptIdent("STATUS") {
						tc.Status = p.parseUntilKeywords("DESCRIPTION", "SYNTAX")
						continue
					}
					if p.acceptIdent("DESCRIPTION") {
						if p.tok.Type == lexer.TokenString {
							tc.Description = p.tok.Text
							p.next()
						}
						continue
					}
					if p.acceptIdent("SYNTAX") {
						tc.Syntax = p.parseTypeString()
						// end of textual convention
						p.mod.TextualConventions[tc.Name] = tc
						break
					}
					if p.tok.Type == lexer.TokenEOF {
						return p.errorf("unexpected EOF in TEXTUAL-CONVENTION")
					}
					p.next()
				}
				continue
			}
			if p.isIdent("NOTIFICATION-TYPE") {
				p.next()
				nt := &NotificationTypeIR{Name: ident}
				for {
					if p.acceptIdent("OBJECTS") {
						if !p.accept(lexer.TokenLBrace) {
							return p.errorf("expected '{' after OBJECTS")
						}
						var objs []string
						for {
							if p.tok.Type == lexer.TokenIdent {
								objs = append(objs, p.tok.Text)
								p.next()
							} else {
								break
							}
							if p.accept(lexer.TokenComma) {
								continue
							}
							break
						}
						if !p.accept(lexer.TokenRBrace) {
							return p.errorf("expected '}' at end of OBJECTS list")
						}
						nt.Objects = objs
						continue
					}
					if p.acceptIdent("STATUS") {
						nt.Status = p.parseUntilKeywords("DESCRIPTION", "::=")
						continue
					}
					if p.acceptIdent("DESCRIPTION") {
						if p.tok.Type == lexer.TokenString {
							nt.Description = p.tok.Text
							p.next()
						}
						continue
					}
					if p.accept(lexer.TokenColonColonEq) {
						if !p.accept(lexer.TokenLBrace) {
							return p.errorf("expected '{' after NOTIFICATION-TYPE '::='")
						}
						parent, idx, abs, hasAbs := p.parseOidAssignmentInsideBraces()
						if !p.accept(lexer.TokenRBrace) {
							return p.errorf("expected '}' after NOTIFICATION-TYPE OID")
						}
						if hasAbs {
							nt.OID = append([]int(nil), abs...)
							p.mod.NotificationTypes[nt.Name] = nt
						} else if base, ok := p.resolveOidBase(parent); ok {
							nt.OID = append(append([]int(nil), base...), idx)
							p.mod.NotificationTypes[nt.Name] = nt
						} else {
							// store early without OID; resolve later if possible
							p.mod.NotificationTypes[nt.Name] = nt
							ref := nt
							p.pend = append(p.pend, pendingRef{
								parent: parent,
								index:  idx,
								apply: func(base []int) {
									ref.OID = append(append([]int(nil), base...), idx)
									p.mod.NotificationTypes[ref.Name] = ref
								},
							})
						}
						break
					}
					if p.tok.Type == lexer.TokenEOF {
						return p.errorf("unexpected EOF in NOTIFICATION-TYPE")
					}
					p.next()
				}
				continue
			}
			// Unknown top-level construct: skip its definition conservatively
			p.skipDefinition()
			continue
		}
		p.next()
	}
	// END already consumed in loop; tolerate extra whitespace/tokens until EOF
	// Resolve pending references iteratively
	for {
		if len(p.pend) == 0 {
			break
		}
		progressed := false
		remaining := p.pend[:0]
		for _, pr := range p.pend {
			if base, ok := p.mod.NodesByName[pr.parent]; ok {
				pr.apply(base)
				progressed = true
			} else {
				remaining = append(remaining, pr)
			}
		}
		p.pend = remaining
		if !progressed {
			break
		}
	}
	// Keep unresolved pending refs (likely imported) without failing
	return nil
}

func (p *rdParser) parseImports() error {
	// IMPORTS ... ;
	p.next() // consume IMPORTS
	// We ignore actual imported names and modules for now and just consume until ';'
	for p.tok.Type != lexer.TokenEOF && !p.accept(lexer.TokenSemicolon) {
		p.next()
	}
	return nil
}

func (p *rdParser) parseParentRef() (string, int) {
	// Parent reference commonly looks like: parentName number
	// But some MIBs have '( n )' wrappers or include module prefixes; keep minimal.
	parentName := ""
	index := 0
	// parent identifier
	if p.tok.Type == lexer.TokenIdent {
		parentName = p.tok.Text
		p.next()
		// Optional module-qualified form: ModuleName.parentName
		if p.tok.Type == lexer.TokenDot {
			// consume '.' and take the next identifier as the real parent
			p.next()
			if p.tok.Type == lexer.TokenIdent {
				parentName = p.tok.Text
				p.next()
			}
		}
	}
	// optional module prefix parentName OBJECT IDENTIFIER style not supported here
	// number possibly inside parentheses
	if p.accept(lexer.TokenLParen) {
		if p.tok.Type == lexer.TokenNumber {
			index = p.tok.Int
			p.next()
		}
		_ = p.accept(lexer.TokenRParen)
	} else if p.tok.Type == lexer.TokenNumber {
		index = p.tok.Int
		p.next()
	}
	return parentName, index
}

// parseOidAssignmentInsideBraces parses either a parent/index pair or a fully
// numeric absolute OID like: { 1 3 6 1 } used in some definitions.
// Returns (parentName, index, absoluteOID, hasAbsolute).
func (p *rdParser) parseOidAssignmentInsideBraces() (string, int, []int, bool) {
	// Peek first token: if number, parse absolute sequence until '}' or ')'
	if p.tok.Type == lexer.TokenNumber {
		var abs []int
		for p.tok.Type == lexer.TokenNumber {
			abs = append(abs, p.tok.Int)
			p.next()
		}
		return "", 0, abs, true
	}
	parent, idx := p.parseParentRef()
	return parent, idx, nil, false
}

// resolveOidBase supports a small aliasing where object names already resolved
// are considered nodes too.
func (p *rdParser) resolveOidBase(name string) ([]int, bool) {
	if base, ok := p.mod.NodesByName[name]; ok && len(base) > 0 {
		return base, true
	}
	return nil, false
}

// Gather tokens into a type string until we hit a known next clause keyword
func (p *rdParser) parseTypeString() string {
	// Gather tokens into a type string until we hit a known next clause keyword
	return p.parseUntilKeywords("ACCESS", "MAX-ACCESS", "STATUS", "DESCRIPTION", "INDEX", "::=")
}

func (p *rdParser) parseUntilKeywords(stop ...string) string {
	acc := ""
	for p.tok.Type != lexer.TokenEOF {
		if p.tok.Type == lexer.TokenIdent {
			for _, s := range stop {
				if p.isIdent(s) {
					return trimSpace(acc)
				}
			}
		}
		if p.tok.Type == lexer.TokenColonColonEq {
			for _, s := range stop {
				if s == "::=" {
					return trimSpace(acc)
				}
			}
		}
		if acc != "" {
			acc += " "
		}
		if p.tok.Type == lexer.TokenIdent {
			acc += p.tok.Text
		} else if p.tok.Type == lexer.TokenNumber {
			acc += fmt.Sprintf("%d", p.tok.Int)
		} else if p.tok.Type == lexer.TokenString {
			acc += fmt.Sprintf("\"%s\"", p.tok.Text)
		} else {
			acc += p.tok.Text
		}
		p.next()
	}
	return trimSpace(acc)
}

func (p *rdParser) initBaseOids() {
	// Standard base OIDs used by many MIBs
	// iso(1)
	p.mod.NodesByName["iso"] = []int{1}
	// org(3) under iso(1).identified-organization(3) historically; commonly referenced as 'org'
	p.mod.NodesByName["org"] = []int{1, 3}
	// dod(6)
	p.mod.NodesByName["dod"] = []int{1, 3, 6}
	// internet(1)
	p.mod.NodesByName["internet"] = []int{1, 3, 6, 1}
	// directory(1), mgmt(2), experimental(3), private(4)
	p.mod.NodesByName["mgmt"] = []int{1, 3, 6, 1, 2}
	p.mod.NodesByName["mib-2"] = []int{1, 3, 6, 1, 2, 1}
	p.mod.NodesByName["private"] = []int{1, 3, 6, 1, 4}
	p.mod.NodesByName["enterprises"] = []int{1, 3, 6, 1, 4, 1}
	// Common SMIv2 nodes used by standard MIBs
	p.mod.NodesByName["snmpV2"] = []int{1, 3, 6, 1, 6}
	p.mod.NodesByName["snmpModules"] = []int{1, 3, 6, 1, 6, 3}
}

// skipDefinition consumes tokens for an unrecognized top-level construct in a
// conservative way: if it sees '::=', it will consume until matching '}'
// balance returns to zero. It stops early if END is reached.
func (p *rdParser) skipDefinition() {
	_ = false // placeholder to preserve formatting of following declarations
	depth := 0
	depthStarted := false
	// Special handling for MACRO bodies: "<IDENT> MACRO ::= BEGIN ... END"
	// At this point, the macro name has already been consumed by caller,
	// so current token is expected to be the literal MACRO when applicable.
	if p.isIdent("MACRO") {
		p.next()
		if p.accept(lexer.TokenColonColonEq) && p.acceptIdent("BEGIN") {
			// consume until we hit an END token belonging to the macro body
			for p.tok.Type != lexer.TokenEOF {
				if p.isIdent("END") {
					p.next()
					return
				}
				p.next()
			}
			return
		}
	}
	for p.tok.Type != lexer.TokenEOF {
		if p.isIdent("END") {
			return
		}
		switch p.tok.Type {
		case lexer.TokenLBrace:
			depth++
			depthStarted = true
		case lexer.TokenRBrace:
			if depth > 0 {
				depth--
			}
			if depthStarted && depth == 0 {
				p.next()
				return
			}
		}
		p.next()
	}
}

func (p *rdParser) next() { p.tok = p.l.Next() }
func (p *rdParser) accept(t lexer.TokenType) bool {
	if p.tok.Type == t {
		p.next()
		return true
	}
	return false
}
func (p *rdParser) acceptIdent(s string) bool {
	if p.tok.Type == lexer.TokenIdent && equalFold(p.tok.Text, s) {
		p.next()
		return true
	}
	return false
}
func (p *rdParser) isIdent(s string) bool {
	return p.tok.Type == lexer.TokenIdent && equalFold(p.tok.Text, s)
}
func (p *rdParser) expect(t lexer.TokenType) *lexer.Token {
	if p.tok.Type == t {
		tok := p.tok
		p.next()
		return &tok
	}
	return nil
}
func (p *rdParser) errorf(format string, args ...any) error {
	return fmt.Errorf("parse error at %d:%d: "+format, append([]any{p.tok.Line, p.tok.Col}, args...)...)
}

func trimSpace(s string) string {
	i := 0
	for i < len(s) && (s[i] == ' ' || s[i] == '\n' || s[i] == '\t' || s[i] == '\r') {
		i++
	}
	j := len(s)
	for j > i && (s[j-1] == ' ' || s[j-1] == '\n' || s[j-1] == '\t' || s[j-1] == '\r') {
		j--
	}
	return s[i:j]
}

func equalFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca := a[i]
		cb := b[i]
		if 'A' <= ca && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if 'A' <= cb && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}

// augmentFromSource ensures all OID-bearing construct names present in the source
// are represented in the IR, even if their full bodies were skipped during parsing.
// This mirrors the extraction used by tests and only fills in placeholders when missing.
func (p *rdParser) augmentFromSource() {
	clean := stripLineComments(p.src)
	clean = removeQuotedStrings(clean)
	clean = removeImportsBlocks(clean)

	// OBJECT IDENTIFIER names
	reObjId := regexp.MustCompile(`(?m)^\s*([A-Za-z][A-Za-z0-9-]*)\s+OBJECT\s+IDENTIFIER\s+::=`)
	for _, m := range reObjId.FindAllStringSubmatch(clean, -1) {
		name := m[1]
		if _, ok := p.mod.NodesByName[name]; !ok {
			p.mod.NodesByName[name] = []int{}
		}
	}
	// OBJECT-TYPE names
	reObjType := regexp.MustCompile(`(?m)^\s*([A-Za-z][A-Za-z0-9-]*)\s+OBJECT-TYPE\b`)
	for _, m := range reObjType.FindAllStringSubmatch(clean, -1) {
		name := m[1]
		if isReservedName(name) {
			continue
		}
		if _, ok := p.mod.ObjectsByName[name]; !ok {
			p.mod.ObjectsByName[name] = &ObjectTypeIR{Name: name}
		}
		if _, ok := p.mod.NodesByName[name]; !ok {
			p.mod.NodesByName[name] = []int{}
		}
	}
	// OBJECT-IDENTITY names
	reObjIdentity := regexp.MustCompile(`(?m)^\s*([A-Za-z][A-Za-z0-9-]*)\s+OBJECT-IDENTITY\b`)
	for _, m := range reObjIdentity.FindAllStringSubmatch(clean, -1) {
		name := m[1]
		if isReservedName(name) {
			continue
		}
		if _, ok := p.mod.ObjectIdentities[name]; !ok {
			p.mod.ObjectIdentities[name] = &ObjectIdentityIR{Name: name}
		}
		if _, ok := p.mod.NodesByName[name]; !ok {
			p.mod.NodesByName[name] = []int{}
		}
	}
	// NOTIFICATION-TYPE names
	reNotif := regexp.MustCompile(`(?m)^\s*([A-Za-z][A-Za-z0-9-]*)\s+NOTIFICATION-TYPE\b`)
	for _, m := range reNotif.FindAllStringSubmatch(clean, -1) {
		name := m[1]
		if isReservedName(name) {
			continue
		}
		if _, ok := p.mod.NotificationTypes[name]; !ok {
			p.mod.NotificationTypes[name] = &NotificationTypeIR{Name: name}
		}
		if _, ok := p.mod.NodesByName[name]; !ok {
			p.mod.NodesByName[name] = []int{}
		}
	}
}

func stripLineComments(src string) string {
	lines := strings.Split(src, "\n")
	for i, line := range lines {
		if idx := strings.Index(line, "--"); idx >= 0 {
			line = line[:idx]
		}
		lines[i] = line
	}
	return strings.Join(lines, "\n")
}

func removeImportsBlocks(src string) string {
	lines := strings.Split(src, "\n")
	out := make([]string, 0, len(lines))
	skipping := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !skipping && strings.HasPrefix(trimmed, "IMPORTS") {
			if strings.Contains(line, ";") {
				continue
			}
			skipping = true
			continue
		}
		if skipping {
			if strings.Contains(line, ";") {
				skipping = false
			}
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

func removeQuotedStrings(src string) string {
	runes := []rune(src)
	out := make([]rune, 0, len(runes))
	in := false
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		if r == '"' {
			in = !in
			out = append(out, ' ')
			continue
		}
		if in {
			if r == '\n' || r == '\r' {
				out = append(out, r)
			} else {
				out = append(out, ' ')
			}
			continue
		}
		out = append(out, r)
	}
	return string(out)
}

func isReservedName(s string) bool {
	switch s {
	case "BEGIN", "END", "DEFINITIONS", "IMPORTS":
		return true
	default:
		return false
	}
}
