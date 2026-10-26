package catalog

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

type searchDocument struct {
	addressSource
	Surname      string `json:"surname"`
	First        string `json:"firstName"`
	Organisation string `json:"organisation"`
	Postcode     string `json:"postcode"`
	EntryType    string `json:"entry_type"`
	SIC          string `json:"sic_code"`
	Industry     string `json:"industry"`
}

var searchDocumentColumns = []string{"id", "kind", "name", "first_full", "given", "postcode", "street", "street_2", "locality", "locality_2", "town", "county", "sic", "industry"}

func flattenSearchRecord(id int32, raw []byte) ([]any, error) {
	var r searchDocument
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, errors.New("invalid original record JSON")
	}
	kind := int16(0)
	name, first := Normalize(r.Surname), Normalize(r.First)
	sic, industry := "", ""
	if r.EntryType == "business" {
		kind = 1
		name = Normalize(r.Organisation)
		first = ""
		sic = Normalize(r.SIC)
		industry = Normalize(r.Industry)
	}
	loc, err := r.addressSource.project()
	if err != nil {
		return nil, err
	}
	given, _, _ := strings.Cut(first, " ")
	return []any{id, kind, name, first, given, Postcode(r.Postcode), loc[0], loc[1], loc[2], loc[3], loc[4], loc[5], sic, industry}, nil
}

// searchFieldConditions is shared by the measured adapter and serving adapter.
// Only fixed column names and bound field values enter its SQL.
func searchFieldConditions(q query, target *[]any) []string {
	args := *target
	conditions := []string{}
	kind := int16(0)
	if q.Kind == "business" {
		kind = 1
	}
	args = append(args, kind)
	conditions = append(conditions, fmt.Sprintf("kind=$%d", len(args)))
	add := func(columns []string, key string, prefix bool) {
		if key == "" {
			return
		}
		args = append(args, key)
		lo := len(args)
		hi := 0
		if prefix {
			args = append(args, key+"\U0010ffff")
			hi = len(args)
		}
		parts := []string{}
		for _, col := range columns {
			nonempty := ""
			if col == "street" || col == "street_2" || col == "locality" || col == "locality_2" {
				nonempty = col + "<>'' AND "
			}
			if prefix {
				parts = append(parts, fmt.Sprintf("(%s%s >= $%d AND %s < $%d)", nonempty, col, lo, col, hi))
			} else {
				parts = append(parts, fmt.Sprintf("(%s%s=$%d)", nonempty, col, lo))
			}
		}
		conditions = append(conditions, "("+strings.Join(parts, " OR ")+")")
	}
	name := q.Surname
	if q.Kind == "business" {
		name = q.Organisation
	}
	add([]string{"name"}, strings.TrimSuffix(name, "*"), q.Match == "prefix" || strings.HasSuffix(name, "*"))
	if q.First != "" {
		first := strings.TrimSuffix(q.First, "*")
		mode := q.FirstMatch
		if mode == "" {
			mode = q.Match
		}
		if mode == "given" && !strings.HasSuffix(q.First, "*") && !strings.Contains(first, " ") {
			add([]string{"given"}, first, false)
		} else if mode == "given" && !strings.HasSuffix(q.First, "*") {
			args = append(args, first, first+" ", first+" \U0010ffff")
			conditions = append(conditions, fmt.Sprintf("(first_full=$%d OR (first_full >= $%d AND first_full < $%d))", len(args)-2, len(args)-1, len(args)))
		} else {
			add([]string{"first_full"}, first, mode == "prefix" || mode == "given" || strings.HasSuffix(q.First, "*"))
		}
	}
	if q.Postcode != "" {
		add([]string{"postcode"}, q.Postcode, q.PostcodeLength > 0)
		if q.PostcodeLength > 0 {
			args = append(args, q.PostcodeLength, q.Postcode)
			conditions = append(conditions, fmt.Sprintf("(postcode_length=$%d OR postcode=$%d)", len(args)-1, len(args)))
		}
	}
	for _, f := range []struct {
		v    string
		cols []string
	}{{q.Street, []string{"street", "street_2"}}, {q.Locality, []string{"locality", "locality_2"}}, {q.Town, []string{"town"}}, {q.County, []string{"county"}}, {q.SIC, []string{"sic"}}, {q.Industry, []string{"industry"}}} {
		add(f.cols, strings.TrimSuffix(f.v, "*"), strings.HasSuffix(f.v, "*"))
	}
	if q.SIC != "" {
		conditions = append(conditions, "sic<>''")
	}
	if q.Industry != "" {
		conditions = append(conditions, "industry<>''")
	}
	*target = args
	return conditions
}
