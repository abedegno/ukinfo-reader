package main

import (
	"errors"
	"reflect"
	"regexp"
	"strings"
)

type coverIndex struct {
	Generation  string   `json:"generation"`
	Entries     int64    `json:"entries"`
	LastID      int64    `json:"last_id"`
	Version     int      `json:"version"`
	Ready       bool     `json:"ready"`
	Index       string   `json:"index"`
	Table       string   `json:"table"`
	Valid       bool     `json:"valid"`
	IndexReady  bool     `json:"index_ready"`
	Method      string   `json:"method"`
	NKeys       int      `json:"nkeys"`
	NAttrs      int      `json:"natts"`
	Columns     []string `json:"columns"`
	Collations  []string `json:"collations"`
	Options     []int    `json:"options"`
	Partial     bool     `json:"partial"`
	Expressions bool     `json:"expressions"`
	Unique      bool     `json:"unique"`
	Tablespace  int64    `json:"tablespace"`
	Owner       string   `json:"owner"`
	TableOwner  string   `json:"table_owner"`
	Database    string   `json:"database"`
	Bytes       int64    `json:"bytes"`
	Server      int      `json:"server"`
	Search      string   `json:"search"`
	Ordinary    bool     `json:"ordinary"`
}

func validateCoverIndex(x coverIndex, table string) error {
	if !regexp.MustCompile(`^d[1-9][0-9]{0,4}_[a-f0-9]{24}$`).MatchString(table) || !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(x.Generation) || !strings.HasSuffix(table, "_"+x.Generation[:24]) || x.Table != table || x.Index != "p8_pc_"+table+"_cover" || x.Entries < 1 || x.Entries > 2147483647 || x.LastID != x.Entries || x.Version != 1 || !x.Ready || !x.Valid || !x.IndexReady || x.Method != "btree" || x.NKeys != 5 || x.NAttrs != 9 || x.Partial || x.Expressions || x.Unique || x.Tablespace != 0 || x.Owner != "postgres" || x.TableOwner != "postgres" || x.Database != "ukinfo_search" || x.Bytes < 1 || x.Bytes > 16<<30 || x.Server != 180007 || x.Search != "0.26.3" || !x.Ordinary || !reflect.DeepEqual(x.Columns, []string{"kind", "postcode", "name", "first_full", "id", "name_id", "first_id", "postcode_id", "postcode_length"}) || !reflect.DeepEqual(x.Collations, []string{"C", "C", "C"}) || !reflect.DeepEqual(x.Options, []int{0, 0, 0, 0, 0}) {
		return errors.New("exact complete covering index required")
	}
	return nil
}

// Catalog lookup is parameterized; names and generation metadata stay private.
const coverIndexSQL = `SELECT json_build_object(
'generation',e.generation,'entries',e.entries,'last_id',e.last_id,'version',e.version,'ready',e.ready,
'index',c.relname,'table',t.relname,'valid',i.indisvalid,'index_ready',i.indisready,'method',am.amname,
'nkeys',i.indnkeyatts,'natts',i.indnatts,'options',i.indoption::smallint[],'unique',i.indisunique,
'partial',i.indpred IS NOT NULL,'expressions',i.indexprs IS NOT NULL,'tablespace',c.reltablespace::bigint,
'owner',pg_get_userbyid(c.relowner),'table_owner',pg_get_userbyid(t.relowner),'ordinary',t.relkind='r',
'database',current_database(),'server',current_setting('server_version_num')::int,
'search',(SELECT extversion FROM pg_extension WHERE extname='pg_search'),'bytes',pg_relation_size(c.oid),
'columns',(SELECT json_agg(a.attname ORDER BY k.ord) FROM unnest(i.indkey) WITH ORDINALITY k(attnum,ord) JOIN pg_attribute a ON a.attrelid=i.indrelid AND a.attnum=k.attnum),
'collations',(SELECT json_agg(co.collname ORDER BY k.ord) FROM unnest(i.indcollation) WITH ORDINALITY k(coll,ord) JOIN pg_collation co ON co.oid=k.coll))
FROM pg_class c JOIN pg_index i ON i.indexrelid=c.oid JOIN pg_am am ON am.oid=c.relam
JOIN pg_class t ON t.oid=i.indrelid JOIN ukinfo_search.editions e ON e.dataset_id=$3 AND substring(e.generation FROM 1 FOR 24)=split_part(t.relname,'_',2)
WHERE c.oid=to_regclass('ukinfo_search.' || $1) AND t.oid=to_regclass('ukinfo_search.' || $2)`
