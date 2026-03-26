package internal

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

func Install(fsys fs.FS, skills []Skill, targetDir string, force bool) error {
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return fmt.Errorf("creating target directory: %w", err)
	}

	for _, skill := range skills {
		skillTarget := filepath.Join(targetDir, skill.Name)

		if _, err := os.Stat(skillTarget); err == nil {
			if !force {
				fmt.Printf("  skipping %s: already exists (use --force to overwrite)\n", skill.Name)
				continue
			}
			if err := os.RemoveAll(skillTarget); err != nil {
				return fmt.Errorf("removing existing %s: %w", skill.Name, err)
			}
		}

		if err := os.MkdirAll(skillTarget, 0o755); err != nil {
			return fmt.Errorf("creating skill directory %s: %w", skill.Name, err)
		}

		for _, fileName := range skill.Files {
			srcPath := skill.DirPath + "/" + fileName
			data, err := fs.ReadFile(fsys, srcPath)
			if err != nil {
				return fmt.Errorf("reading %s: %w", srcPath, err)
			}

			dstPath := filepath.Join(skillTarget, fileName)
			if err := os.WriteFile(dstPath, data, 0o644); err != nil {
				return fmt.Errorf("writing %s: %w", dstPath, err)
			}
		}

		fmt.Printf("  installed %s\n", skill.Name)
	}

	return nil
}
