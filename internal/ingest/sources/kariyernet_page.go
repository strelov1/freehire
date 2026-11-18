package sources

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"
)

// A Kariyer.net posting page is a Nuxt 2 server render whose state is one JavaScript function:
//
//	window.__NUXT__=(function(a,b,c,...){return {...,state:{jobDetail:{jobInfo:{...}}}}}(1,"x",null,...));
//
// The returned object names repeated values by their parameter ("title:ai", "companyName:bW"), and
// the values are the arguments after it. jobInfo is the posting exactly as the gateway's /job
// answers it — which refuses the crawl hosts from 2026-11-17 where the page does not — so the
// adapter reads that one object: it finds jobInfo, reads the literal with a small parser that
// resolves each parameter to its argument, and decodes the result as the API's `data`.
//
// The parser knows only what the state is made of (objects, arrays, strings, numbers, the
// minified booleans `!0`/`!1`, null/void 0, parameter names); anything else, a function call
// included, is an error and the posting is deferred — never half-read.

const kariyernetStateMarker = "window.__NUXT__=(function("

// kariyernetPageDetail reads the posting out of a page's server-rendered Nuxt state.
func kariyernetPageDetail(page string) (kariyernetDetail, error) {
	var d kariyernetDetail
	start := strings.Index(page, kariyernetStateMarker)
	if start < 0 {
		return d, fmt.Errorf("kariyernet: page carries no server-rendered state")
	}
	p := &jsParser{s: page, pos: start + len(kariyernetStateMarker)}

	closeParams := strings.Index(page[p.pos:], "){return ")
	if closeParams < 0 {
		return d, fmt.Errorf("kariyernet: state is not the expected function")
	}
	params := strings.Split(page[p.pos:p.pos+closeParams], ",")
	p.pos += closeParams + len("){return ")

	// The returned object is skipped whole to reach the call's arguments, then re-read for jobInfo.
	objStart := p.pos
	if err := p.skipBalanced(); err != nil {
		return d, err
	}
	objEnd := p.pos
	if !strings.HasPrefix(page[p.pos:], "}(") {
		return d, fmt.Errorf("kariyernet: state function is not called with its arguments")
	}
	p.pos += len("}(")
	env := map[string]any{}
	for i := 0; ; i++ {
		p.skipSpace()
		if p.peek() == ')' {
			break
		}
		v, err := p.value()
		if err != nil {
			return d, fmt.Errorf("kariyernet: state argument %d: %w", i, err)
		}
		if i < len(params) {
			env[strings.TrimSpace(params[i])] = v
		}
		p.skipSpace()
		if p.peek() == ',' {
			p.pos++
		}
	}

	ret := page[objStart:objEnd]
	at := strings.Index(ret, "jobDetail:{jobInfo:")
	if at < 0 {
		return d, fmt.Errorf("kariyernet: state carries no jobInfo")
	}
	q := &jsParser{s: ret, pos: at + len("jobDetail:{jobInfo:"), env: env}
	info, err := q.value()
	if err != nil {
		return d, fmt.Errorf("kariyernet: jobInfo: %w", err)
	}
	raw, err := json.Marshal(map[string]any{"data": info})
	if err != nil {
		return d, err
	}
	if err := json.Unmarshal(raw, &d); err != nil {
		return d, fmt.Errorf("kariyernet: jobInfo: %w", err)
	}
	return d, nil
}

// jsParser reads the JavaScript literals Nuxt serialises state with.
type jsParser struct {
	s     string
	pos   int
	env   map[string]any
	depth int
}

const jsMaxDepth = 64

func (p *jsParser) peek() byte {
	if p.pos < len(p.s) {
		return p.s[p.pos]
	}
	return 0
}

func (p *jsParser) skipSpace() {
	for p.pos < len(p.s) && (p.s[p.pos] == ' ' || p.s[p.pos] == '\n' || p.s[p.pos] == '\t' || p.s[p.pos] == '\r') {
		p.pos++
	}
}

// skipBalanced moves past the object or array that opens at pos, string literals included.
func (p *jsParser) skipBalanced() error {
	depth := 0
	for p.pos < len(p.s) {
		switch c := p.s[p.pos]; c {
		case '"', '\'':
			if _, err := p.str(); err != nil {
				return err
			}
			continue
		case '{', '[', '(':
			depth++
		case '}', ']', ')':
			depth--
			if depth == 0 {
				p.pos++
				return nil
			}
		}
		p.pos++
	}
	return fmt.Errorf("unterminated literal")
}

// value reads one literal and resolves a bare name through env.
func (p *jsParser) value() (any, error) {
	p.depth++
	defer func() { p.depth-- }()
	if p.depth > jsMaxDepth {
		return nil, fmt.Errorf("literal nested deeper than %d", jsMaxDepth)
	}
	p.skipSpace()
	switch c := p.peek(); {
	case c == '{':
		return p.object()
	case c == '[':
		return p.array()
	case c == '"' || c == '\'':
		return p.str()
	case c == '!':
		if strings.HasPrefix(p.s[p.pos:], "!0") {
			p.pos += 2
			return true, nil
		}
		if strings.HasPrefix(p.s[p.pos:], "!1") {
			p.pos += 2
			return false, nil
		}
		return nil, fmt.Errorf("unexpected %q at %d", p.s[p.pos:min(p.pos+8, len(p.s))], p.pos)
	case c == '-' || c == '.' || (c >= '0' && c <= '9'):
		return p.number()
	case c == '_' || c == '$' || unicode.IsLetter(rune(c)):
		return p.name()
	}
	return nil, fmt.Errorf("unexpected %q at %d", p.s[p.pos:min(p.pos+8, len(p.s))], p.pos)
}

func (p *jsParser) number() (any, error) {
	start := p.pos
	for p.pos < len(p.s) && strings.IndexByte("0123456789.eE+-", p.s[p.pos]) >= 0 {
		p.pos++
	}
	f, err := strconv.ParseFloat(p.s[start:p.pos], 64)
	if err != nil {
		return nil, fmt.Errorf("number %q: %w", p.s[start:p.pos], err)
	}
	return f, nil
}

func (p *jsParser) ident() string {
	start := p.pos
	for p.pos < len(p.s) {
		c := p.s[p.pos]
		if c == '_' || c == '$' || (c >= '0' && c <= '9') || unicode.IsLetter(rune(c)) {
			p.pos++
			continue
		}
		break
	}
	return p.s[start:p.pos]
}

// name resolves a keyword or a parameter. A parameter with no argument, or one a call computes,
// is not a value the state can be read with.
func (p *jsParser) name() (any, error) {
	id := p.ident()
	switch id {
	case "null", "undefined":
		return nil, nil
	case "true":
		return true, nil
	case "false":
		return false, nil
	case "void":
		p.skipSpace()
		if p.peek() == '0' {
			p.pos++
		}
		return nil, nil
	}
	if p.peek() == '(' || p.peek() == '.' {
		return nil, fmt.Errorf("%q is a call or a member, not a literal", id)
	}
	if v, ok := p.env[id]; ok {
		return v, nil
	}
	return nil, fmt.Errorf("name %q has no argument", id)
}

func (p *jsParser) object() (any, error) {
	p.pos++ // {
	out := map[string]any{}
	for {
		p.skipSpace()
		if p.peek() == '}' {
			p.pos++
			return out, nil
		}
		var key string
		if c := p.peek(); c == '"' || c == '\'' {
			k, err := p.str()
			if err != nil {
				return nil, err
			}
			key = k.(string)
		} else if key = p.ident(); key == "" {
			return nil, fmt.Errorf("object key expected at %d", p.pos)
		}
		p.skipSpace()
		if p.peek() != ':' {
			return nil, fmt.Errorf("':' expected after %q", key)
		}
		p.pos++
		v, err := p.value()
		if err != nil {
			return nil, fmt.Errorf("%s: %w", key, err)
		}
		out[key] = v
		p.skipSpace()
		if p.peek() == ',' {
			p.pos++
		}
	}
}

func (p *jsParser) array() (any, error) {
	p.pos++ // [
	out := []any{}
	for {
		p.skipSpace()
		if p.peek() == ']' {
			p.pos++
			return out, nil
		}
		v, err := p.value()
		if err != nil {
			return nil, err
		}
		out = append(out, v)
		p.skipSpace()
		if p.peek() == ',' {
			p.pos++
		}
	}
}

// str reads a quoted string with JavaScript's escapes (Nuxt writes "/" as / and "<" as
// <).
func (p *jsParser) str() (any, error) {
	quote := p.s[p.pos]
	p.pos++
	var b strings.Builder
	for p.pos < len(p.s) {
		c := p.s[p.pos]
		switch {
		case c == quote:
			p.pos++
			return b.String(), nil
		case c != '\\':
			r, n := utf8.DecodeRuneInString(p.s[p.pos:])
			b.WriteRune(r)
			p.pos += n
			continue
		}
		p.pos++
		if p.pos >= len(p.s) {
			break
		}
		e := p.s[p.pos]
		p.pos++
		switch e {
		case 'n':
			b.WriteByte('\n')
		case 't':
			b.WriteByte('\t')
		case 'r':
			b.WriteByte('\r')
		case 'b':
			b.WriteByte('\b')
		case 'f':
			b.WriteByte('\f')
		case 'v':
			b.WriteByte('\v')
		case '0':
			b.WriteByte(0)
		case 'u', 'x':
			width := 4
			if e == 'x' {
				width = 2
			}
			if p.pos+width > len(p.s) {
				return nil, fmt.Errorf("short escape at %d", p.pos)
			}
			n, err := strconv.ParseUint(p.s[p.pos:p.pos+width], 16, 16)
			if err != nil {
				return nil, fmt.Errorf("escape at %d: %w", p.pos, err)
			}
			p.pos += width
			r := rune(n)
			// A pair of escapes spells one astral character ("\ud83d\ude00").
			if utf16.IsSurrogate(r) && strings.HasPrefix(p.s[p.pos:], "\\u") && p.pos+6 <= len(p.s) {
				if lo, err := strconv.ParseUint(p.s[p.pos+2:p.pos+6], 16, 16); err == nil {
					if pair := utf16.DecodeRune(r, rune(lo)); pair != unicode.ReplacementChar {
						r = pair
						p.pos += 6
					}
				}
			}
			b.WriteRune(r)
		case '\n':
		default:
			b.WriteByte(e)
		}
	}
	return nil, fmt.Errorf("unterminated string")
}
