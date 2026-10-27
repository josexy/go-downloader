package download

import (
	"encoding/json"
	"os"
)

// partMeta is saved beside a partial file so a resumed download can tell
// whether the remote file is still the one it started.
type partMeta struct {
	URL          string `json:"url"`
	ETag         string `json:"etag,omitempty"`
	LastModified string `json:"lastModified,omitempty"`
	Total        int64  `json:"total"`
	Resumable    bool   `json:"resumable"`
}

func readPartMeta(path string) partMeta {
	var m partMeta
	data, err := os.ReadFile(path)
	if err != nil {
		return m
	}
	_ = json.Unmarshal(data, &m)
	return m
}

func writePartMeta(path string, m partMeta) error {
	data, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
