package ffmpeg

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
)

func MovieToImages(input string, out string) ([]string, error) {
	probe, err := FFProbe(input)
	if err != nil {
		return nil, err
	}

	// Execute ffmpeg
	cmd := exec.Command("ffmpeg", "-i", input, "-vf", fmt.Sprintf("fps=fps=%f", probe.FrameRate), filepath.Join(out, "%d.jpg"))
	if err := cmd.Run(); err != nil {
		return nil, err
	}

	// Get list of generated files
	files, err := filepath.Glob(filepath.Join(out, "*.jpg"))
	if err != nil {
		return nil, err
	}

	sort.Slice(files, func(i, j int) bool {
		if len(files[i]) == len(files[j]) {
			return files[i] < files[j]
		}
		return len(files[i]) < len(files[j])
	})

	return files, nil
}
