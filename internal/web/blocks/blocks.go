package blocks

import (
	"embed"
)

//go:embed wrapper.html
var blocksFS embed.FS

func GetWrapper() (string, error) {
	data, err := blocksFS.ReadFile("wrapper.html")
	if err != nil {
		return "", err
	}
	return string(data), nil
}
