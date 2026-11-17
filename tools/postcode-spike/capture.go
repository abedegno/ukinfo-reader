package main

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"sync"

	"github.com/jackc/pgx/v5"
)

type workloadCase struct {
	Path  string `json:"path"`
	Query string `json:"query"`
}
type statement struct {
	SQL  string
	Args []any
}
type capture struct {
	mu    sync.Mutex
	found []statement
}

const candidatePrefix = "SELECT id,name_id,first_id,postcode_id,name,first_full FROM ukinfo_search."
const candidateTail = " WHERE kind=$1 AND ((postcode >= $2 AND postcode < $3)) AND (postcode_length=$4 OR postcode=$5) AND postcode @@@ pdb.regex($6) AND id @@@ pdb.all() ORDER BY name,first_full,id LIMIT $7"

var candidateSQL = regexp.MustCompile("^" + regexp.QuoteMeta(candidatePrefix) + `(d[1-9][0-9]*_[a-f0-9]{24})` + regexp.QuoteMeta(" WHERE "))
var regexPredicate = regexp.MustCompile(`postcode @@@ pdb\.regex\(\$([1-9][0-9]*)\)`)

func validateCases(cases []workloadCase) error {
	if len(cases) != 2 {
		return errors.New("exact two-case subset required")
	}
	allowed := map[string]bool{"dataset": true, "kind": true, "postcode": true, "sort": true, "limit": true, "match": true, "first_match": true}
	for _, c := range cases {
		q, err := url.ParseQuery(c.Query)
		if err != nil || c.Path != "/api/search" || q.Get("dataset") == "" || q.Get("dataset") == "all" || q.Get("postcode") == "" || q.Get("sort") != "name" || (q.Get("kind") != "" && q.Get("kind") != "person") {
			return errors.New("unsupported private shape")
		}
		for key, values := range q {
			if !allowed[key] || len(values) != 1 {
				return errors.New("unsupported private parameter")
			}
		}
	}
	return nil
}

func scalarVariant(sql string, args []any) (string, error) {
	bad := func() (string, error) { return "", errors.New("unknown candidate statement") }
	shape := candidateSQL.FindStringSubmatch(sql)
	if len(shape) != 2 || sql != candidatePrefix+shape[1]+candidateTail || len(args) != 7 {
		return bad()
	}
	matches := regexPredicate.FindAllStringSubmatchIndex(sql, -1)
	if len(matches) != 1 {
		return bad()
	}
	match := matches[0]
	parameter, err := strconv.Atoi(sql[match[2]:match[3]])
	if err != nil || parameter != 6 {
		return bad()
	}
	kind, kindOK := args[0].(int16)
	prefix, prefixOK := args[1].(string)
	upper, upperOK := args[2].(string)
	length, lengthOK := args[3].(int)
	partial, partialOK := args[4].(string)
	value, valueOK := args[5].(string)
	limit, limitOK := args[6].(int)
	suffix := length - len(prefix)
	if !kindOK || kind != 0 || !prefixOK || len(prefix) < 2 || len(prefix) > 5 || !regexp.MustCompile(`^[A-Z0-9]+$`).MatchString(prefix) || !upperOK || upper != prefix+"\U0010ffff" || !lengthOK || length < 5 || length > 7 || (suffix != 2 && suffix != 3) || !partialOK || partial != prefix || !valueOK || value != regexp.QuoteMeta(prefix)+fmt.Sprintf("(.{%d})?", suffix) || !limitOK || limit < 1 || limit > 129 {
		return bad()
	}
	// Keep all bound parameters and the authoritative scalar predicates intact.
	// The API generated this non-null regex constant. A parameterized one-time
	// check removes only its redundant index prefilter without renumbering args.
	return sql[:match[0]] + fmt.Sprintf("($%d::text IS NOT NULL)", parameter) + sql[match[1]:], nil
}

func (c *capture) TraceQueryStart(ctx context.Context, _ *pgx.Conn, d pgx.TraceQueryStartData) context.Context {
	if !candidateSQL.MatchString(d.SQL) {
		return ctx
	}
	c.mu.Lock()
	c.found = append(c.found, statement{d.SQL, append([]any(nil), d.Args...)})
	c.mu.Unlock()
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	return cancelled
}
func (c *capture) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}
func (c *capture) only() (statement, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.found) != 1 {
		return statement{}, errors.New("one cancelled target capture required")
	}
	item := c.found[0]
	item.Args = append([]any(nil), item.Args...)
	if _, err := scalarVariant(item.SQL, item.Args); err != nil {
		return statement{}, err
	}
	return item, nil
}
func (c *capture) reset() { c.mu.Lock(); c.found = nil; c.mu.Unlock() }

// A failed capture reports boundary facts only, never SQL, values or errors.
type captureFailure struct {
	Status         int  `json:"http_status"`
	Statements     int  `json:"candidate_statements"`
	KnownSQL       bool `json:"known_sql_template"`
	KnownArguments bool `json:"known_argument_bindings"`
}

func (*captureFailure) Error() string { return "candidate capture boundary failed" }
func (c *capture) failure(status int) *captureFailure {
	c.mu.Lock()
	defer c.mu.Unlock()
	d := &captureFailure{Status: status, Statements: len(c.found)}
	if len(c.found) == 1 {
		s := c.found[0]
		shape := candidateSQL.FindStringSubmatch(s.SQL)
		d.KnownSQL = len(shape) == 2 && s.SQL == candidatePrefix+shape[1]+candidateTail
		_, err := scalarVariant(s.SQL, s.Args)
		d.KnownArguments = d.KnownSQL && err == nil
	}
	return d
}
