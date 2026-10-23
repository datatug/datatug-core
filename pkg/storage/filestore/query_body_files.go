package filestore

import (
	"fmt"
	"path"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/datatug/datatug-core/pkg/storage"
)

// queryBodyCandidate is one body file name a query may legitimately have;
// the name alone decides the representation (never the content).
type queryBodyCandidate struct {
	ext        string
	structured bool
}

// queryBodyCandidates lists the body files a query of queryType may have.
// A TugQL query has a text body or a structured YAML body; every other type
// has the one "<lowercase type>" body. The type must already be accepted by
// queryTypeFileExtension.
func queryBodyCandidates(queryType datatug.QueryType) []queryBodyCandidate {
	if queryType == datatug.QueryTypeTugQL {
		return []queryBodyCandidate{
			{ext: queryBodyFileExt(datatug.QueryTypeTugQL, false)},
			{ext: tugqlStructuredBodyExt, structured: true},
		}
	}
	return []queryBodyCandidate{{ext: queryBodyFileExt(queryType, false)}}
}

// foundQueryBody is the one body file of a query.
type foundQueryBody struct {
	fileName   string
	ext        string
	structured bool
	data       []byte
}

// readQueryBody reads the single body file of query id under dir, with read
// as the file reader (budgeted or capped). Returns nil when there is none,
// and an error - it never guesses - when more than one body file exists.
func readQueryBody(dir, id string, queryType datatug.QueryType, read func(filePath string) (data []byte, exists bool, err error)) (*foundQueryBody, error) {
	if _, err := queryTypeFileExtension(queryType); err != nil {
		return nil, err
	}
	var found *foundQueryBody
	for _, c := range queryBodyCandidates(queryType) {
		fileName := id + "." + storage.QueryFileSuffix + "." + c.ext
		data, exists, err := read(path.Join(dir, fileName))
		if err != nil {
			return nil, err
		}
		if !exists {
			continue
		}
		if found != nil {
			return nil, fmt.Errorf("query %q has more than one body file (%s and %s); remove one", id, found.fileName, fileName)
		}
		found = &foundQueryBody{fileName: fileName, ext: c.ext, structured: c.structured, data: data}
	}
	return found, nil
}
