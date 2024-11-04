package ffmpeg

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
)

func MovieToImages(input string) ([]string, error) {
	probe, err := FFProbe(input)
	if err != nil {
		return nil, fmt.Errorf("failed to probe video: %w", err)
	}

	// Create tmp directory
	dir, err := os.MkdirTemp("", "moview")
	if err != nil {
		return nil, fmt.Errorf("failed to create tmp directory: %w", err)
	}

	// Execute ffmpeg
	cmd := exec.Command("ffmpeg", "-i", input, "-vf", fmt.Sprintf("fps=fps=%f", probe.FrameRate), filepath.Join(dir, "%d.jpg"))
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to execute ffmpeg: %w", err)
	}

	// Get list of generated files
	files, err := filepath.Glob(filepath.Join(dir, "*.jpg"))
	if err != nil {
		return nil, fmt.Errorf("failed to list generated files: %w", err)
	}

	sort.Slice(files, func(i, j int) bool {
		if len(files[i]) == len(files[j]) {
			return files[i] < files[j]
		}
		return len(files[i]) < len(files[j])
	})

	return files, nil
}
