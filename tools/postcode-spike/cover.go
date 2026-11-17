package main

import (
	"context"
	"errors"
	"strings"
)

// Retain every scalar precision/range predicate, argument and stable ordering.
// Remove only the redundant BM25 membership checks, including pdb.all().
func coveringVariant(sql string, args []any) (string, error) {
	scalar, err := scalarVariant(sql, args)
	if err != nil || len(args) != 7 {
		return "", errors.New("district statement required")
	}
	return strings.Replace(scalar, " AND id @@@ pdb.all()", "", 1), nil
}

func districtStatement(statements []statement) (statement, error) {
	if len(statements) != 2 {
		return statement{}, errors.New("original two-case capture required")
	}
	var district statement
	count, full := 0, 0
	for _, s := range statements {
		if _, err := scalarVariant(s.SQL, s.Args); err != nil {
			return statement{}, errors.New("invalid capture")
		}
		if len(s.Args) == 3 {
			full++
			continue
		}
		prefix := s.Args[1].(string)
		if s.Args[3].(int)-len(prefix) != 3 {
			return statement{}, errors.New("district precision required")
		}
		district = s
		count++
	}
	if count != 1 || full != 1 {
		return statement{}, errors.New("one district and one full control required")
	}
	return district, nil
}

func compareCovering(ctx context.Context, statements []statement, run func(context.Context, statement) reading) (aggregate, error) {
	district, err := districtStatement(statements)
	if err != nil {
		return newAggregate(), err
	}
	return compareStrategies(ctx, []statement{district, district}, run, func(s statement) (string, string, error) {
		native, err := coveringVariant(s.SQL, s.Args)
		return s.SQL, native, err
	}, [2]string{"original_bm25", "covering_btree"}, true)
}
