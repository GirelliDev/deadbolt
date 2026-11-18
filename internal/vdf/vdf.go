// Package vdf parses Valve's text KeyValues format (appmanifest_*.acf, app_info_print).
package vdf

import (
	"errors"
	"strings"
)

// Node is either a string value or a nested map.
type Node map[string]any

// Parse reads the first top-level "key" { ... } or "key" "value" pairs.
// Text before the first quoted token (steamcmd chatter) is skipped by ParseAt.
func Parse(s string) (Node, error) {
	p := &parser{s: s}
	n, err := p.object(true)
	if err != nil {
		return nil, err
	}
	return n, nil
}

type parser struct {
	s string
	i int
}

func (p *parser) skipSpace() {
	for p.i < len(p.s) {
		c := p.s[p.i]
		if c == ' ' || c == '\t' || c == '\r' || c == '\n' {
			p.i++
		} else if c == '/' && p.i+1 < len(p.s) && p.s[p.i+1] == '/' { // comment
			for p.i < len(p.s) && p.s[p.i] != '\n' {
				p.i++
			}
		} else {
			return
		}
	}
}

func (p *parser) str() (string, error) {
	p.skipSpace()
	if p.i >= len(p.s) || p.s[p.i] != '"' {
		return "", errors.New("vdf: expected quote")
	}
	p.i++
	var b strings.Builder
	for p.i < len(p.s) {
		c := p.s[p.i]
		switch {
		case c == '\\' && p.i+1 < len(p.s):
			p.i++
			switch p.s[p.i] {
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			default:
				b.WriteByte(p.s[p.i])
			}
		case c == '"':
			p.i++
			return b.String(), nil
		default:
			b.WriteByte(c)
		}
		p.i++
	}
	return "", errors.New("vdf: unterminated string")
}

func (p *parser) object(top bool) (Node, error) {
	n := Node{}
	for {
		p.skipSpace()
		if p.i >= len(p.s) {
			if top {
				return n, nil
			}
			return nil, errors.New("vdf: unexpected end")
		}
		if p.s[p.i] == '}' {
			if top {
				return nil, errors.New("vdf: unbalanced }")
			}
			p.i++
			return n, nil
		}
		if top && p.s[p.i] != '"' {
			return n, nil // trailing chatter (e.g. the "steam>" prompt) ends the document
		}
		k, err := p.str()
		if err != nil {
			return nil, err
		}
		p.skipSpace()
		if p.i < len(p.s) && p.s[p.i] == '{' {
			p.i++
			child, err := p.object(false)
			if err != nil {
				return nil, err
			}
			n[k] = child
			continue
		}
		v, err := p.str()
		if err != nil {
			return nil, err
		}
		n[k] = v
	}
}

// Get walks nested keys; ok is false when any step is missing.
func (n Node) Get(path ...string) (any, bool) {
	var cur any = n
	for _, k := range path {
		m, ok := cur.(Node)
		if !ok {
			return nil, false
		}
		if cur, ok = m[k]; !ok {
			return nil, false
		}
	}
	return cur, true
}

func (n Node) String(path ...string) (string, bool) {
	v, ok := n.Get(path...)
	s, isStr := v.(string)
	return s, ok && isStr
}

func (n Node) Map(path ...string) (Node, bool) {
	v, ok := n.Get(path...)
	m, isMap := v.(Node)
	return m, ok && isMap
}
