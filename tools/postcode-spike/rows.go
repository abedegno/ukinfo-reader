package main

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
)

type resultRows interface {
	Next() bool
	Values() ([]any, error)
	Err() error
	Close()
}

func digestRows(rows resultRows) (int, [32]byte, error) {
	defer rows.Close()
	hash := sha256.New()
	count := 0
	for rows.Next() {
		count++
		if count > 129 {
			return count, [32]byte{}, errors.New("SQL result limit exceeded")
		}
		values, err := rows.Values()
		if err != nil {
			return count, [32]byte{}, err
		}
		body, err := json.Marshal(values)
		if err != nil || len(body) > 65536 {
			return count, [32]byte{}, errors.New("bounded SQL row required")
		}
		hash.Write(body)
		hash.Write([]byte{'\n'})
	}
	if err := rows.Err(); err != nil {
		return count, [32]byte{}, err
	}
	var sum [32]byte
	copy(sum[:], hash.Sum(nil))
	return count, sum, nil
}
