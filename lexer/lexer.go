package lexer

import (
	"unicode"
)

type TokenType int

const (
	TokenEOF TokenType = iota
	TokenIdent
	TokenNumber
	TokenString
	TokenLBrace       // {
	TokenRBrace       // }
	TokenLParen       // (
	TokenRParen       // )
	TokenComma        // ,
	TokenDot          // .
	TokenSemicolon    // ;
	TokenColonColonEq // ::=
	TokenAssignEq     // = (rare in MIBs)
)

type Token struct {
	Type TokenType
	Text string
	Int  int
	Line int
	Col  int
}

type Lexer struct {
	input  []rune
	pos    int
	line   int
	col    int
	peeked *Token
}

func New(input []byte) *Lexer {
	r := []rune(string(input))
	return &Lexer{input: r, pos: 0, line: 1, col: 1}
}

func (l *Lexer) Peek() Token {
	if l.peeked != nil {
		return *l.peeked
	}
	t := l.Next()
	l.peeked = &t
	return t
}

func (l *Lexer) Next() Token {
	if l.peeked != nil {
		t := *l.peeked
		l.peeked = nil
		return t
	}
	l.skipWhitespaceAndComments()
	if l.eof() {
		return l.mk(TokenEOF, "")
	}
	r := l.cur()
	// Identifiers (letters, hyphens allowed inside)
	if isIdentStart(r) {
		startLine, startCol := l.line, l.col
		s := make([]rune, 0, 16)
		s = append(s, r)
		l.advance()
		for !l.eof() {
			r = l.cur()
			if isIdent(r) {
				s = append(s, r)
				l.advance()
				continue
			}
			break
		}
		return Token{Type: TokenIdent, Text: string(s), Line: startLine, Col: startCol}
	}
	// Numbers
	if unicode.IsDigit(r) {
		startLine, startCol := l.line, l.col
		n := 0
		for !l.eof() && unicode.IsDigit(l.cur()) {
			n = n*10 + int(l.cur()-'0')
			l.advance()
		}
		return Token{Type: TokenNumber, Int: n, Text: "", Line: startLine, Col: startCol}
	}
	switch r {
	case '"':
		return l.readString()
	case '{':
		l.advance()
		return l.mk(TokenLBrace, "{")
	case '}':
		l.advance()
		return l.mk(TokenRBrace, "}")
	case '(':
		l.advance()
		return l.mk(TokenLParen, "(")
	case ')':
		l.advance()
		return l.mk(TokenRParen, ")")
	case ',':
		l.advance()
		return l.mk(TokenComma, ",")
	case '.':
		l.advance()
		return l.mk(TokenDot, ".")
	case ';':
		l.advance()
		return l.mk(TokenSemicolon, ";")
	case ':':
		// Expect '::='
		return l.readColonAssign()
	case '=':
		l.advance()
		return l.mk(TokenAssignEq, "=")
	default:
		// Unknown character, skip
		l.advance()
		return l.Next()
	}
}

func (l *Lexer) readString() Token {
	startLine, startCol := l.line, l.col
	// consume opening quote
	l.advance()
	s := make([]rune, 0, 64)
	for !l.eof() {
		r := l.cur()
		if r == '"' {
			l.advance()
			break
		}
		// basic escape of doubled quotes not typical; ASN.1 strings allow quotes by doubling
		if r == '\\' {
			l.advance()
			if l.eof() {
				break
			}
			r = l.cur()
			s = append(s, r)
			l.advance()
			continue
		}
		s = append(s, r)
		l.advance()
	}
	return Token{Type: TokenString, Text: string(s), Line: startLine, Col: startCol}
}

func (l *Lexer) readColonAssign() Token {
	// consume first ':'
	l.advance()
	if l.eof() || l.cur() != ':' {
		return l.mk(TokenAssignEq, ":")
	}
	l.advance()
	if l.eof() || l.cur() != '=' {
		return l.mk(TokenAssignEq, "::")
	}
	l.advance()
	return l.mk(TokenColonColonEq, "::=")
}

func (l *Lexer) mk(t TokenType, s string) Token {
	return Token{Type: t, Text: s, Line: l.line, Col: l.col}
}

func (l *Lexer) skipWhitespaceAndComments() {
	for !l.eof() {
		r := l.cur()
		// whitespace
		if r == ' ' || r == '\t' || r == '\r' || r == '\n' {
			l.advance()
			continue
		}
		// comments: -- to end of line per ASN.1
		if r == '-' && l.peekChar() == '-' {
			// consume until newline
			l.advance() // '-'
			l.advance() // '-'
			for !l.eof() && l.cur() != '\n' {
				l.advance()
			}
			continue
		}
		break
	}
}

func (l *Lexer) cur() rune { return l.input[l.pos] }
func (l *Lexer) eof() bool { return l.pos >= len(l.input) }
func (l *Lexer) peekChar() rune {
	if l.pos+1 >= len(l.input) {
		return 0
	}
	return l.input[l.pos+1]
}

func (l *Lexer) advance() {
	if l.eof() {
		return
	}
	r := l.input[l.pos]
	l.pos++
	if r == '\n' {
		l.line++
		l.col = 1
	} else {
		l.col++
	}
}

func isIdentStart(r rune) bool {
	return unicode.IsLetter(r)
}

func isIdent(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-'
}
