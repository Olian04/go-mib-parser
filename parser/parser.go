package parser

import (
	"fmt"

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
}

type pendingRef struct {
	parent string
	index  int
	apply  func(base []int)
}

func Parse(input []byte) (*ModuleIR, error) {
	p := &rdParser{l: lexer.New(input), mod: &ModuleIR{NodesByName: map[string][]int{}, ObjectsByName: map[string]*ObjectTypeIR{}, ObjectIdentities: map[string]*ObjectIdentityIR{}, TextualConventions: map[string]*TextualConventionIR{}, NotificationTypes: map[string]*NotificationTypeIR{}}}
	p.next()
	p.initBaseOids()

	// Parse single module
	if err := p.parseModule(); err != nil {
		return nil, err
	}
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

	// Body: OBJECT IDENTIFIER assignments, OBJECT-TYPE, etc., until END
	for !p.isIdent("END") && p.tok.Type != lexer.TokenEOF {
		if p.tok.Type == lexer.TokenIdent {
			// Lookahead for 'OBJECT IDENTIFIER' or 'OBJECT-TYPE'
			ident := p.tok.Text
			p.next()
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
				parentName, index := p.parseParentRef()
				if !p.accept(lexer.TokenRBrace) {
					return p.errorf("expected '}' in OBJECT IDENTIFIER assignment")
				}
				// resolve parent (allow forward references)
				if parent, ok := p.mod.NodesByName[parentName]; ok {
					oid := append(append([]int(nil), parent...), index)
					p.mod.NodesByName[ident] = oid
				} else {
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
					if p.accept(lexer.TokenColonColonEq) {
						// ::= { parent n }
						if !p.accept(lexer.TokenLBrace) {
							return p.errorf("expected '{' after '::=' in OBJECT-TYPE")
						}
						parentName, index := p.parseParentRef()
						if !p.accept(lexer.TokenRBrace) {
							return p.errorf("expected '}' after OBJECT-TYPE OID ref")
						}
						if parent, ok := p.mod.NodesByName[parentName]; ok {
							obj.OID = append(append([]int(nil), parent...), index)
							// store
							p.mod.ObjectsByName[obj.Name] = obj
						} else {
							ref := obj
							p.pend = append(p.pend, pendingRef{
								parent: parentName,
								index:  index,
								apply: func(base []int) {
									ref.OID = append(append([]int(nil), base...), index)
									p.mod.ObjectsByName[ref.Name] = ref
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
			if p.isIdent("MODULE-IDENTITY") {
				p.next()
				// MODULE-IDENTITY
				mi := &ModuleIdentityIR{Name: ident}
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
						parent, idx := p.parseParentRef()
						if !p.accept(lexer.TokenRBrace) {
							return p.errorf("expected '}' after MODULE-IDENTITY OID")
						}
						if base, ok := p.mod.NodesByName[parent]; ok {
							mi.OID = append(append([]int(nil), base...), idx)
							p.mod.ModuleIdentity = mi
						} else {
							ref := mi
							p.pend = append(p.pend, pendingRef{
								parent: parent,
								index:  idx,
								apply: func(base []int) {
									ref.OID = append(append([]int(nil), base...), idx)
									p.mod.ModuleIdentity = ref
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
						parent, idx := p.parseParentRef()
						if !p.accept(lexer.TokenRBrace) {
							return p.errorf("expected '}' after OBJECT-IDENTITY OID")
						}
						if base, ok := p.mod.NodesByName[parent]; ok {
							oi.OID = append(append([]int(nil), base...), idx)
							p.mod.ObjectIdentities[oi.Name] = oi
						} else {
							ref := oi
							p.pend = append(p.pend, pendingRef{
								parent: parent,
								index:  idx,
								apply: func(base []int) {
									ref.OID = append(append([]int(nil), base...), idx)
									p.mod.ObjectIdentities[ref.Name] = ref
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
						parent, idx := p.parseParentRef()
						if !p.accept(lexer.TokenRBrace) {
							return p.errorf("expected '}' after NOTIFICATION-TYPE OID")
						}
						if base, ok := p.mod.NodesByName[parent]; ok {
							nt.OID = append(append([]int(nil), base...), idx)
							p.mod.NotificationTypes[nt.Name] = nt
						} else {
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
			// If neither OBJECT IDENTIFIER nor OBJECT-TYPE, skip line/construct until next ident; many constructs not yet supported
			// attempt to resync: consume until semicolon or END
			for p.tok.Type != lexer.TokenEOF && !p.isIdent("END") {
				if p.accept(lexer.TokenSemicolon) {
					break
				}
				p.next()
			}
			continue
		}
		p.next()
	}
	if !p.acceptIdent("END") {
		return p.errorf("expected END")
	}
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
