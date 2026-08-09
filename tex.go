// Copyright (c) the go-tex/tex authors.
// SPDX-License-Identifier: BSD-3-Clause

// Package tex is a pure-Go (CGO=0) processor for a practical subset of LaTeX
// *documents* — not just math. It tokenizes with TeX category codes, expands
// user macros (\def, \newcommand/\renewcommand with mandatory and optional
// arguments), parses document structure (sectioning, font commands, lists,
// inline and display math, paragraphs), and emits semantic HTML with math
// typeset to SVG by go-tex/math. There is no page/line breaking, no package
// ecosystem, and no PDF/DVI back-end: this is the front of the engine (the
// "mouth" and a document builder), a foundation for full LaTeX processing.
package tex

import "strings"

// ── tokens ──────────────────────────────────────────────────────────────────

type ttype uint8

const (
	tChar  ttype = iota // an ordinary character (r)
	tCS                 // a control sequence \name  (s, without backslash)
	tOpen               // {
	tClose              // }
	tMath               // $  (display when s=="$$")
	tPar                // paragraph break (blank line)
	tParam              // #n  (n in r-'0')
	tAmp                // &
	tSpace              // inter-word space (collapsed)
)

type tok struct {
	typ ttype
	s   string
	r   rune
}

// tokenize converts source into TeX-ish tokens: it strips % comments, folds
// runs of spaces, turns blank lines into paragraph breaks, and reads control
// sequences (\word or a single control symbol).
func tokenize(src string) []tok {
	rs := []rune(src)
	var out []tok
	newline := 0 // consecutive newlines
	flushSpace := false
	emit := func(t tok) { out = append(out, t); newline = 0; flushSpace = false }
	for i := 0; i < len(rs); {
		c := rs[i]
		switch {
		case c == '%': // comment to end of line
			for i < len(rs) && rs[i] != '\n' {
				i++
			}
		case c == '\n':
			i++
			newline++
			if newline == 2 {
				emit(tok{typ: tPar})
			} else if newline == 1 {
				flushSpace = true
			}
		case c == ' ' || c == '\t':
			i++
			if newline == 0 {
				flushSpace = true
			}
		case c == '\\':
			if flushSpace {
				out = append(out, tok{typ: tSpace})
			}
			i++
			if i < len(rs) && !isLetter(rs[i]) { // control symbol \{ \% \\ …
				emit(tok{typ: tCS, s: string(rs[i])})
				i++
				continue
			}
			st := i
			for i < len(rs) && isLetter(rs[i]) {
				i++
			}
			emit(tok{typ: tCS, s: string(rs[st:i])})
			// a control word swallows following spaces
			for i < len(rs) && (rs[i] == ' ' || rs[i] == '\t') {
				i++
			}
		case c == '{':
			maybeSpace(&out, flushSpace)
			emit(tok{typ: tOpen})
			i++
		case c == '}':
			maybeSpace(&out, flushSpace)
			emit(tok{typ: tClose})
			i++
		case c == '$':
			maybeSpace(&out, flushSpace)
			i++
			if i < len(rs) && rs[i] == '$' {
				i++
				emit(tok{typ: tMath, s: "$$"})
			} else {
				emit(tok{typ: tMath, s: "$"})
			}
		case c == '#':
			maybeSpace(&out, flushSpace)
			i++
			if i < len(rs) && rs[i] >= '0' && rs[i] <= '9' {
				emit(tok{typ: tParam, r: rs[i]})
				i++
			}
		case c == '&':
			maybeSpace(&out, flushSpace)
			emit(tok{typ: tAmp})
			i++
		case c == '~':
			maybeSpace(&out, flushSpace)
			emit(tok{typ: tChar, r: ' '}) // non-breaking space
			i++
		default:
			maybeSpace(&out, flushSpace)
			emit(tok{typ: tChar, r: c})
			i++
		}
	}
	return out
}

func maybeSpace(out *[]tok, flush bool) {
	if flush && len(*out) > 0 {
		*out = append(*out, tok{typ: tSpace})
	}
}

func isLetter(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}

// ── macros & the expansion machine ──────────────────────────────────────────

type macro struct {
	nargs   int
	optDflt string // default for the (single) optional first arg; "" = no optional
	hasOpt  bool
	body    []tok
}

// machine walks a token slice, expanding user macros on the fly. next() yields
// only non-macro tokens; definitions (\def, \newcommand, …) are executed and
// skipped. A pushback stack holds macro-body expansions.
type machine struct {
	in     []tok
	pos    int
	push   [][]tok // stack of pending token lists (front = LIFO)
	macros map[string]*macro
	nexp   int // total macro expansions (runaway guard)
}

func newMachine(in []tok) *machine {
	return &machine{in: in, macros: map[string]*macro{}}
}

// raw returns the next token without macro expansion (consuming pushback first).
func (m *machine) raw() (tok, bool) {
	for len(m.push) > 0 {
		top := m.push[len(m.push)-1]
		if len(top) == 0 {
			m.push = m.push[:len(m.push)-1]
			continue
		}
		t := top[0]
		m.push[len(m.push)-1] = top[1:]
		return t, true
	}
	if m.pos >= len(m.in) {
		return tok{}, false
	}
	t := m.in[m.pos]
	m.pos++
	return t, true
}

func (m *machine) unraw(t tok) { m.push = append(m.push, []tok{t}) }

// next returns the next token with macros expanded and definitions executed.
func (m *machine) next() (tok, bool) {
	for {
		t, ok := m.raw()
		if !ok {
			return tok{}, false
		}
		if t.typ != tCS {
			return t, true
		}
		switch t.s {
		case "def":
			m.readDef()
			continue
		case "newcommand", "renewcommand", "providecommand":
			m.readNewcommand()
			continue
		case "let":
			m.readLet()
			continue
		}
		if mac, isMacro := m.macros[t.s]; isMacro {
			m.expand(mac)
			continue
		}
		return t, true // a primitive control sequence for the processor
	}
}

// expand grabs a macro's arguments and pushes its substituted body.
func (m *machine) expand(mac *macro) {
	if m.nexp > 100_000 {
		return // runaway-recursion guard: total expansions are hard-capped
	}
	m.nexp++
	args := make([][]tok, mac.nargs)
	i := 0
	if mac.hasOpt {
		args[0] = m.grabOptional(mac.optDflt)
		i = 1
	}
	for ; i < mac.nargs; i++ {
		args[i] = m.grabArg()
	}
	var body []tok
	for _, b := range mac.body {
		if b.typ == tParam {
			n := int(b.r - '1')
			if n >= 0 && n < len(args) {
				body = append(body, args[n]...)
			}
			continue
		}
		body = append(body, b)
	}
	m.push = append(m.push, body)
}

// grabArg reads one argument: a balanced {group} or a single token.
func (m *machine) grabArg() []tok {
	m.skipSpaces()
	t, ok := m.raw()
	if !ok {
		return nil
	}
	if t.typ == tOpen {
		return m.grabGroup()
	}
	return []tok{t}
}

// grabOptional reads an optional [arg] if present, else returns the default.
func (m *machine) grabOptional(dflt string) []tok {
	m.skipSpaces()
	t, ok := m.raw()
	if !ok {
		return tokenize(dflt)
	}
	if t.typ == tChar && t.r == '[' {
		var g []tok
		for {
			u, ok := m.raw()
			if !ok || (u.typ == tChar && u.r == ']') {
				break
			}
			g = append(g, u)
		}
		return g
	}
	m.unraw(t)
	return tokenize(dflt)
}

// grabGroup reads tokens up to the matching close brace (the open is consumed).
func (m *machine) grabGroup() []tok {
	depth := 1
	var g []tok
	for {
		t, ok := m.raw()
		if !ok {
			return g
		}
		switch t.typ {
		case tOpen:
			depth++
		case tClose:
			depth--
			if depth == 0 {
				return g
			}
		}
		g = append(g, t)
	}
}

func (m *machine) skipSpaces() {
	for {
		t, ok := m.raw()
		if !ok {
			return
		}
		if t.typ != tSpace {
			m.unraw(t)
			return
		}
	}
}

// readDef handles \def\name<params>{body} (simple #1#2… parameter text only).
func (m *machine) readDef() {
	t, ok := m.raw()
	if !ok || t.typ != tCS {
		return
	}
	name := t.s
	nargs := 0
	for {
		u, ok := m.raw()
		if !ok {
			return
		}
		if u.typ == tParam {
			nargs++
			continue
		}
		if u.typ == tOpen {
			body := m.grabGroup()
			m.macros[name] = &macro{nargs: nargs, body: body}
			return
		}
		// ignore other delimiter tokens in the param text
	}
}

// readNewcommand handles \newcommand{\name}[n][opt]{body} (braces or bare cs).
func (m *machine) readNewcommand() {
	m.skipSpaces()
	name := m.readCommandName()
	if name == "" {
		return
	}
	mac := &macro{}
	m.skipSpaces()
	if t, ok := m.raw(); ok {
		if t.typ == tChar && t.r == '[' {
			mac.nargs = m.readBracketInt()
			m.skipSpaces()
			if u, ok := m.raw(); ok {
				if u.typ == tChar && u.r == '[' {
					mac.hasOpt = true
					mac.optDflt = tokToString(m.readBracketRaw())
				} else {
					m.unraw(u)
				}
			}
		} else {
			m.unraw(t)
		}
	}
	m.skipSpaces()
	if t, ok := m.raw(); ok && t.typ == tOpen {
		mac.body = m.grabGroup()
	}
	m.macros[name] = mac
}

// readLet handles a minimal \let\a=\b or \let\a\b (alias).
func (m *machine) readLet() {
	a := m.readCommandName()
	m.skipSpaces()
	if t, ok := m.raw(); ok && !(t.typ == tChar && t.r == '=') {
		m.unraw(t)
	}
	m.skipSpaces()
	b, ok := m.raw()
	if !ok || a == "" {
		return
	}
	if b.typ == tCS {
		if mb, isMac := m.macros[b.s]; isMac {
			m.macros[a] = mb
			return
		}
		m.macros[a] = &macro{body: []tok{b}}
	}
}

// readCommandName reads a \name, optionally wrapped in {…}.
func (m *machine) readCommandName() string {
	t, ok := m.raw()
	if !ok {
		return ""
	}
	if t.typ == tOpen {
		u, ok := m.raw()
		if ok && u.typ == tCS {
			m.skipToClose()
			return u.s
		}
		return ""
	}
	if t.typ == tCS {
		return t.s
	}
	return ""
}

func (m *machine) skipToClose() {
	for {
		t, ok := m.raw()
		if !ok || t.typ == tClose {
			return
		}
	}
}

// readBracketInt reads digits up to ']' as an int (the '[' already consumed).
func (m *machine) readBracketInt() int {
	n := 0
	for {
		t, ok := m.raw()
		if !ok || (t.typ == tChar && t.r == ']') {
			return n
		}
		if t.typ == tChar && t.r >= '0' && t.r <= '9' {
			n = n*10 + int(t.r-'0')
		}
	}
}

// readBracketRaw reads tokens up to ']' (the '[' already consumed).
func (m *machine) readBracketRaw() []tok {
	var g []tok
	for {
		t, ok := m.raw()
		if !ok || (t.typ == tChar && t.r == ']') {
			return g
		}
		g = append(g, t)
	}
}

// tokToString flattens tokens to their source-ish text (for defaults).
func tokToString(ts []tok) string {
	var b strings.Builder
	for _, t := range ts {
		switch t.typ {
		case tChar:
			b.WriteRune(t.r)
		case tCS:
			b.WriteString("\\" + t.s)
		case tSpace:
			b.WriteByte(' ')
		}
	}
	return b.String()
}
