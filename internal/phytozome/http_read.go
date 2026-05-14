package phytozome

import (
	"fmt"
	"io"
)

const (
	maxPhytozomeJSONBytes  = 64 << 20
	maxPhytozomeHTMLBytes  = 16 << 20
	maxPhytozomeBundleSize = 64 << 20
)

func readLimited(body io.Reader, limit int64, description string) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("%s exceeds %d bytes", description, limit)
	}
	return data, nil
}

func readShortErrorBody(body io.Reader) string {
	data, _ := io.ReadAll(io.LimitReader(body, 4096))
	return string(data)
}
