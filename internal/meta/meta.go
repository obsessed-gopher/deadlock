// Package meta decodes Deadlock match metadata (.meta files from Valve's
// replay CDN) into the generated protobuf types.
package meta

import (
	"fmt"
	"os"

	"google.golang.org/protobuf/proto"

	"deadlock_ideas/internal/compress"
	"deadlock_ideas/internal/pb"
)

// Decode parses decompressed .meta bytes.
func Decode(raw []byte) (*pb.CMsgMatchMetaDataContents_MatchInfo, error) {
	_, mi, err := DecodeWithVersion(raw)
	return mi, err
}

// DecodeWithVersion parses decompressed .meta bytes and also returns the
// container format version.
func DecodeWithVersion(raw []byte) (uint32, *pb.CMsgMatchMetaDataContents_MatchInfo, error) {
	var outer pb.CMsgMatchMetaData
	if err := proto.Unmarshal(raw, &outer); err != nil {
		return 0, nil, fmt.Errorf("meta: outer: %w", err)
	}
	var contents pb.CMsgMatchMetaDataContents
	if err := proto.Unmarshal(outer.GetMatchDetails(), &contents); err != nil {
		return 0, nil, fmt.Errorf("meta: contents: %w", err)
	}
	if contents.GetMatchInfo() == nil {
		return 0, nil, fmt.Errorf("meta: no match_info (version %d)", outer.GetVersion())
	}
	return outer.GetVersion(), contents.GetMatchInfo(), nil
}

// ReadFile opens a (possibly compressed) .meta file and decodes it.
func ReadFile(path string) (*pb.CMsgMatchMetaDataContents_MatchInfo, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	raw, err := compress.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("meta: decompress %s: %w", path, err)
	}
	return Decode(raw)
}
