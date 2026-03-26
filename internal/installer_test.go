package internal

import (
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
)

func testFS() fstest.MapFS {
	return fstest.MapFS{
		"go/skills/tdd/SKILL.md": &fstest.MapFile{
			Data: []byte(`---
name: tdd
description: "TDD practices"
category: always
---

# TDD Guide`),
		},
		"go/skills/tdd/tests.md": &fstest.MapFile{
			Data: []byte("# Testing patterns"),
		},
		"go/skills/datastar/SKILL.md": &fstest.MapFile{
			Data: []byte(`---
name: datastar
description: "Datastar framework"
category: custom
---

# Datastar`),
		},
	}
}

func TestInstall(t *testing.T) {
	fsys := testFS()
	skills, err := ListSkills(fsys, "go")
	if err != nil {
		t.Fatalf("list skills: %v", err)
	}

	t.Run("installs skill files to target directory", func(t *testing.T) {
		targetDir := t.TempDir()

		if err := Install(fsys, skills, targetDir, false); err != nil {
			t.Fatalf("install: %v", err)
		}

		skillMd := filepath.Join(targetDir, "tdd", "SKILL.md")
		if _, err := os.Stat(skillMd); err != nil {
			t.Errorf("expected %s to exist", skillMd)
		}

		testsMd := filepath.Join(targetDir, "tdd", "tests.md")
		if _, err := os.Stat(testsMd); err != nil {
			t.Errorf("expected %s to exist", testsMd)
		}

		dsSKILL := filepath.Join(targetDir, "datastar", "SKILL.md")
		if _, err := os.Stat(dsSKILL); err != nil {
			t.Errorf("expected %s to exist", dsSKILL)
		}
	})

	t.Run("preserves file content", func(t *testing.T) {
		targetDir := t.TempDir()

		if err := Install(fsys, skills, targetDir, false); err != nil {
			t.Fatalf("install: %v", err)
		}

		data, err := os.ReadFile(filepath.Join(targetDir, "tdd", "tests.md"))
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		if string(data) != "# Testing patterns" {
			t.Errorf("content = %q, want %q", string(data), "# Testing patterns")
		}
	})

	t.Run("skips existing without force", func(t *testing.T) {
		targetDir := t.TempDir()

		tddDir := filepath.Join(targetDir, "tdd")
		os.MkdirAll(tddDir, 0o755)
		os.WriteFile(filepath.Join(tddDir, "SKILL.md"), []byte("original"), 0o644)

		if err := Install(fsys, skills, targetDir, false); err != nil {
			t.Fatalf("install: %v", err)
		}

		data, err := os.ReadFile(filepath.Join(tddDir, "SKILL.md"))
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		if string(data) != "original" {
			t.Errorf("file was overwritten, got %q", string(data))
		}
	})

	t.Run("overwrites existing with force", func(t *testing.T) {
		targetDir := t.TempDir()

		tddDir := filepath.Join(targetDir, "tdd")
		os.MkdirAll(tddDir, 0o755)
		os.WriteFile(filepath.Join(tddDir, "SKILL.md"), []byte("original"), 0o644)

		if err := Install(fsys, skills, targetDir, true); err != nil {
			t.Fatalf("install: %v", err)
		}

		data, err := os.ReadFile(filepath.Join(tddDir, "SKILL.md"))
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		if string(data) == "original" {
			t.Error("file was not overwritten with --force")
		}
	})

	t.Run("creates target directory if needed", func(t *testing.T) {
		targetDir := filepath.Join(t.TempDir(), "nested", "skills")

		if err := Install(fsys, skills, targetDir, false); err != nil {
			t.Fatalf("install: %v", err)
		}

		if _, err := os.Stat(targetDir); err != nil {
			t.Errorf("target dir was not created")
		}
	})
}
