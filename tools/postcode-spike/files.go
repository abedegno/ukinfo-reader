package main

import (
	"encoding/json"
	"errors"
	"io"
	"os"
)

func readCases(path string) ([]workloadCase, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, errors.New("private cases unavailable")
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() > 4<<20 || info.Mode().Perm()&0077 != 0 {
		return nil, errors.New("bounded private input required")
	}
	body, err := io.ReadAll(io.LimitReader(f, (4<<20)+1))
	if err != nil || len(body) > 4<<20 {
		return nil, errors.New("bounded private input required")
	}
	var cases []workloadCase
	if json.Unmarshal(body, &cases) != nil {
		return nil, errors.New("invalid private cases")
	}
	if err := validateCases(cases); err != nil {
		return nil, err
	}
	return cases, nil
}

func createReport(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return nil, errors.New("new private output required")
	}
	return f, nil
}
func finishReport(f *os.File, value aggregate) (failure error) {
	defer func() {
		if err := f.Close(); err != nil && failure == nil {
			failure = errors.New("cannot close private aggregate")
		}
	}()
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil || len(body) > 32768 {
		return errors.New("cannot encode bounded aggregate")
	}
	if _, err = f.Write(append(body, '\n')); err != nil {
		return errors.New("cannot save private aggregate")
	}
	if err = f.Sync(); err != nil {
		return errors.New("cannot sync private aggregate")
	}
	return nil
}
func writeReport(path string, value aggregate) error {
	f, err := createReport(path)
	if err != nil {
		return err
	}
	return finishReport(f, value)
}
