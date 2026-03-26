package cskills

import "embed"

//go:embed all:go/skills all:rust/skills
var SkillsFS embed.FS
