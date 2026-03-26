package internal

import (
	"testing"
	"testing/fstest"
)

func TestParseFrontmatter(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		wantName string
		wantDesc string
		wantCat  string
	}{
		{
			name: "full frontmatter",
			content: `---
name: tdd
description: "Test-driven development"
category: always
---

# TDD Guide`,
			wantName: "tdd",
			wantDesc: "Test-driven development",
			wantCat:  "always",
		},
		{
			name: "missing category defaults to custom",
			content: `---
name: datastar
description: "Datastar framework"
---

# Datastar`,
			wantName: "datastar",
			wantDesc: "Datastar framework",
			wantCat:  "custom",
		},
		{
			name:     "no frontmatter",
			content:  "# Just a heading\n\nSome content.",
			wantName: "",
			wantDesc: "",
			wantCat:  "custom",
		},
		{
			name: "multiline description",
			content: `---
name: datastar
description:
  Long description that spans lines.
category: custom
---`,
			wantName: "datastar",
			wantDesc: "Long description that spans lines.",
			wantCat:  "custom",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name, desc, cat := parseFrontmatter([]byte(tt.content))
			if name != tt.wantName {
				t.Errorf("name = %q, want %q", name, tt.wantName)
			}
			if desc != tt.wantDesc {
				t.Errorf("description = %q, want %q", desc, tt.wantDesc)
			}
			if cat != tt.wantCat {
				t.Errorf("category = %q, want %q", cat, tt.wantCat)
			}
		})
	}
}

func TestListSkills(t *testing.T) {
	fsys := fstest.MapFS{
		"go/skills/tdd/SKILL.md": &fstest.MapFile{
			Data: []byte(`---
name: tdd
description: "TDD practices"
category: always
---

# TDD`),
		},
		"go/skills/tdd/tests.md": &fstest.MapFile{
			Data: []byte("# Tests"),
		},
		"go/skills/datastar/SKILL.md": &fstest.MapFile{
			Data: []byte(`---
name: datastar
description: "Datastar framework"
category: custom
---

# Datastar`),
		},
		"rust/skills/tdd/SKILL.md": &fstest.MapFile{
			Data: []byte(`---
name: tdd
description: "Rust TDD"
category: always
---`),
		},
	}

	t.Run("list go skills", func(t *testing.T) {
		skills, err := ListSkills(fsys, "go")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(skills) != 2 {
			t.Fatalf("got %d skills, want 2", len(skills))
		}

		byName := make(map[string]Skill)
		for _, s := range skills {
			byName[s.Name] = s
		}

		tdd, ok := byName["tdd"]
		if !ok {
			t.Fatal("missing tdd skill")
		}
		if tdd.Category != "always" {
			t.Errorf("tdd category = %q, want always", tdd.Category)
		}
		if tdd.Lang != "go" {
			t.Errorf("tdd lang = %q, want go", tdd.Lang)
		}
		if len(tdd.Files) != 2 {
			t.Errorf("tdd files = %d, want 2", len(tdd.Files))
		}

		ds, ok := byName["datastar"]
		if !ok {
			t.Fatal("missing datastar skill")
		}
		if ds.Category != "custom" {
			t.Errorf("datastar category = %q, want custom", ds.Category)
		}
	})

	t.Run("list rust skills", func(t *testing.T) {
		skills, err := ListSkills(fsys, "rust")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(skills) != 1 {
			t.Fatalf("got %d skills, want 1", len(skills))
		}
		if skills[0].Name != "tdd" {
			t.Errorf("name = %q, want tdd", skills[0].Name)
		}
	})

	t.Run("invalid language", func(t *testing.T) {
		skills, err := ListSkills(fsys, "python")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(skills) != 0 {
			t.Errorf("got %d skills, want 0", len(skills))
		}
	})
}
