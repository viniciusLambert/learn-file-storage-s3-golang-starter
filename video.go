package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
)

type ffprobeOutput struct {
	Streams []struct {
		Width  int `json:"width,omitempty"`
		Height int `json:"height,omitempty"`
	} `json:"streams"`
}

func getVideoAspectRatio(filePath string) (string, error) {
	cmd := exec.Command(
		"ffprobe",
		"-v", "error",
		"-print_format", "json",
		"-show_streams",
		filePath,
	)
	var buf bytes.Buffer
	cmd.Stdout = &buf

	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("error running ffprobe: %w", err)
	}

	var output ffprobeOutput
	err = json.Unmarshal(buf.Bytes(), &output)
	if err != nil {
		return "", fmt.Errorf("error unmarshiling buffer")
	}

	if len(output.Streams) == 0 {
		return "", fmt.Errorf("output string is empty")
	}
	width := output.Streams[0].Width
	height := output.Streams[0].Height

	if width == 16*height/9 {
		return "16:9", nil
	} else if height == 16*width/9 {
		return "9:16", nil
	}
	return "other", nil
}
