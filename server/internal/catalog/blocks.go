package catalog

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"

	"github.com/klauspost/compress/zstd"
)

const blockRows = 128
const maxRecordBytes = 64 << 10
const maxSourceIDBytes = 512
const maxBlockBytes = blockRows*(maxRecordBytes+maxSourceIDBytes+6) + 7
const blockMagic = "UKIB1"

type storedRecord struct {
	SourceID string
	Record   json.RawMessage
}

// The envelope is independently versioned from SQL. Length-prefixed records let
// point searches skip unwanted JSON without decoding every occurrence in a block.
func packBlock(encoder *zstd.Encoder, records []storedRecord) ([]byte, error) {
	if len(records) == 0 || len(records) > blockRows {
		return nil, recordIntegrity("invalid block row count")
	}
	var raw bytes.Buffer
	raw.WriteString(blockMagic)
	_ = binary.Write(&raw, binary.LittleEndian, uint16(len(records)))
	for _, r := range records {
		if len(r.SourceID) == 0 || len(r.SourceID) > maxSourceIDBytes || len(r.Record) > maxRecordBytes || !json.Valid(r.Record) {
			return nil, recordIntegrity("invalid record block entry")
		}
		_ = binary.Write(&raw, binary.LittleEndian, uint16(len(r.SourceID)))
		_ = binary.Write(&raw, binary.LittleEndian, uint32(len(r.Record)))
		raw.WriteString(r.SourceID)
		raw.Write(r.Record)
	}
	return encoder.EncodeAll(raw.Bytes(), nil), nil
}

func unpackBlock(decoder *zstd.Decoder, compressed []byte) ([]byte, error) {
	raw, err := decoder.DecodeAll(compressed, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: record block checksum/encoding: %v", errRecordIntegrity, err)
	}
	if len(raw) < 7 || len(raw) > maxBlockBytes || string(raw[:5]) != blockMagic {
		return nil, recordIntegrity("unsupported or corrupt record block")
	}
	count := int(binary.LittleEndian.Uint16(raw[5:7]))
	if count < 1 || count > blockRows {
		return nil, recordIntegrity("invalid record block count")
	}
	pos := 7
	for i := 0; i < count; i++ {
		if pos+6 > len(raw) {
			return nil, recordIntegrity("truncated record block")
		}
		sourceLen := int(binary.LittleEndian.Uint16(raw[pos:]))
		recordLen := int(binary.LittleEndian.Uint32(raw[pos+2:]))
		pos += 6
		if sourceLen < 1 || sourceLen > maxSourceIDBytes || recordLen < 2 || recordLen > maxRecordBytes || sourceLen+recordLen > len(raw)-pos {
			return nil, recordIntegrity("invalid record block lengths")
		}
		pos += sourceLen + recordLen
	}
	if pos != len(raw) {
		return nil, recordIntegrity("trailing bytes in record block")
	}
	return raw, nil
}
func recordAt(raw []byte, offset int) (storedRecord, error) {
	count := int(binary.LittleEndian.Uint16(raw[5:7]))
	if offset < 0 || offset >= count {
		return storedRecord{}, recordIntegrity("record outside block")
	}
	pos := 7
	for i := 0; i < count; i++ {
		a := int(binary.LittleEndian.Uint16(raw[pos:]))
		b := int(binary.LittleEndian.Uint32(raw[pos+2:]))
		pos += 6
		if i == offset {
			return storedRecord{string(raw[pos : pos+a]), append(json.RawMessage(nil), raw[pos+a:pos+a+b]...)}, nil
		}
		pos += a + b
	}
	return storedRecord{}, recordIntegrity("record not found in block")
}
