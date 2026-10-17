package catalog

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/klauspost/compress/zstd"
)

func TestRecordBlockIntegrity(t *testing.T) {
	encoder, e := zstd.NewWriter(nil, zstd.WithEncoderConcurrency(1), zstd.WithEncoderCRC(true))
	if e != nil {
		t.Fatal(e)
	}
	defer encoder.Close()
	decoder, e := zstd.NewReader(nil, zstd.WithDecoderConcurrency(1), zstd.WithDecoderMaxMemory(16<<20))
	if e != nil {
		t.Fatal(e)
	}
	defer decoder.Close()
	original := []storedRecord{{"base:123:0", json.RawMessage(`{"integer":9007199254740993,"null":null,"text":"\\u00df"}`)}, {"business:update:9", json.RawMessage(`{"organisation":"Invented & Sons"}`)}}
	compressed, e := packBlock(encoder, original)
	if e != nil {
		t.Fatal(e)
	}
	raw, e := unpackBlock(decoder, compressed)
	if e != nil {
		t.Fatal(e)
	}
	for i, want := range original {
		got, e := recordAt(raw, i)
		if e != nil || got.SourceID != want.SourceID || string(got.Record) != string(want.Record) {
			t.Fatalf("byte preservation: %+v %v", got, e)
		}
	}
	corrupt := append([]byte(nil), compressed...)
	corrupt[len(corrupt)-1] ^= 1
	if _, e = unpackBlock(decoder, corrupt); e == nil {
		t.Fatal("checksum corruption accepted")
	}
	if _, e = unpackBlock(decoder, compressed[:len(compressed)/2]); e == nil {
		t.Fatal("truncation accepted")
	}
	if _, e = recordAt(raw, len(original)); e == nil {
		t.Fatal("out-of-range record accepted")
	}
}
func TestExternalIdentityValidation(t *testing.T) {
	for _, values := range [][]string{{"z", "a", "m", "b", "c"}, {"a", "b", "a"}, {"a", "a"}} {
		s, e := newIdentities(2)
		if e != nil {
			t.Fatal(e)
		}
		duplicate := values[len(values)-1] == "a"
		var failed bool
		for _, v := range values {
			if e = s.add(v); e != nil {
				failed = true
				break
			}
		}
		if !failed {
			failed = s.verify(context.Background()) != nil
		}
		if failed != duplicate {
			t.Fatal("wrong duplicate result", values, failed)
		}
		directory := s.directory
		s.close()
		if _, e = os.Stat(directory); !os.IsNotExist(e) {
			t.Fatal("identity runs were not removed")
		}
	}
	s, e := newIdentities(2)
	if e != nil {
		t.Fatal(e)
	}
	defer s.close()
	_ = s.add("a")
	_ = s.add("b")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if e = s.verify(ctx); e == nil {
		t.Fatal("cancellation ignored")
	}
	if files, _ := filepath.Glob(filepath.Join(s.directory, "*")); len(files) == 0 {
		t.Fatal("test did not exercise external runs")
	}
}
