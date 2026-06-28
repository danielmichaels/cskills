package cskills

import "embed"

//go:embed all:go/skills all:rust/skills all:generic/skills
var SkillsFS embed.FS
