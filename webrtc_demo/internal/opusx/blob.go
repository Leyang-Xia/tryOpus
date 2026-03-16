package opusx

import (
	"fmt"
	"os"
)

func LoadDNNBlob(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read dnn blob failed: %w", err)
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("empty dnn blob: %s", path)
	}
	return data, nil
}
