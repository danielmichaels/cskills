package internal

import (
	"bytes"
	"errors"
	"io/fs"
	"path"
	"strings"
)

type Skill struct {
	Name        string
	Description string
	Category    string
	Lang        string
	DirPath     string
	Files       []string
}

func ListSkills(fsys fs.FS, lang string) ([]Skill, error) {
	skillsDir := path.Join(lang, "skills")
	entries, err := fs.ReadDir(fsys, skillsDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}

	var skills []Skill
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		dirPath := path.Join(skillsDir, entry.Name())
		skillFile := path.Join(dirPath, "SKILL.md")

		data, err := fs.ReadFile(fsys, skillFile)
		if err != nil {
			continue
		}

		name, desc, cat := parseFrontmatter(data)
		if name == "" {
			name = entry.Name()
		}

		files, err := listFiles(fsys, dirPath)
		if err != nil {
			continue
		}

		skills = append(skills, Skill{
			Name:        name,
			Description: desc,
			Category:    cat,
			Lang:        lang,
			DirPath:     dirPath,
			Files:       files,
		})
	}

	return skills, nil
}

func listFiles(fsys fs.FS, dirPath string) ([]string, error) {
	entries, err := fs.ReadDir(fsys, dirPath)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() {
			files = append(files, e.Name())
		}
	}
	return files, nil
}

func parseFrontmatter(content []byte) (name, description, category string) {
	category = "custom"

	if !bytes.HasPrefix(content, []byte("---")) {
		return "", "", category
	}

	end := bytes.Index(content[3:], []byte("\n---"))
	if end == -1 {
		return "", "", category
	}

	frontmatter := string(content[3 : 3+end])
	var descContinuation bool
	var descParts []string

	for _, line := range strings.Split(frontmatter, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		if descContinuation {
			if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
				descParts = append(descParts, strings.Trim(trimmed, "\" "))
				continue
			}
			descContinuation = false
			description = strings.Join(descParts, " ")
			descParts = nil
		}

		parts := strings.SplitN(trimmed, ":", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		val = strings.Trim(val, "\"")

		switch key {
		case "name":
			name = val
		case "description":
			if val == "" || val == "|" || val == ">" {
				descContinuation = true
			} else {
				description = val
			}
		case "category":
			category = val
		}
	}

	if descContinuation {
		description = strings.Join(descParts, " ")
	}

	return name, description, category
}
