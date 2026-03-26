package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/choria-io/fisk"
	"github.com/danielmichaels/cskills"
	"github.com/danielmichaels/cskills/internal"
)

var version = "dev"

func main() {
	app := fisk.New("cskills", "Install Claude Code skills into your repo").
		Version(version).
		WithCheats()

	listCmd := app.Command("list", "List available skills")
	listCmd.Cheat("list", `# List all skills for all languages
cskills list

# List only Go skills
cskills list --lang go

# List only Rust skills
cskills list --lang rust`)
	var listLang string
	listCmd.Flag("lang", "Filter by language: go or rust").
		EnumVar(&listLang, "go", "rust")
	listCmd.Action(func(_ *fisk.ParseContext) error {
		return runList(listLang)
	})

	installCmd := app.Command("install", "Install skills into .claude/skills/")
	installCmd.Cheat("install", `# Install all "always" skills for Go (prompts for custom)
cskills install --lang go

# Install all skills including custom
cskills install --lang go --all

# Install specific skills only
cskills install --lang go --skill tdd,datastar

# Force overwrite existing skills
cskills install --lang rust --all --force`)
	var (
		installLang  string
		installSkill string
		installAll   bool
		installForce bool
	)
	installCmd.Flag("lang", "Language: go or rust").
		Required().
		EnumVar(&installLang, "go", "rust")
	installCmd.Flag("skill", "Comma-separated skill names to install").
		StringVar(&installSkill)
	installCmd.Flag("all", "Install all skills (always + custom)").
		UnNegatableBoolVar(&installAll)
	installCmd.Flag("force", "Overwrite existing skills").
		UnNegatableBoolVar(&installForce)
	installCmd.Action(func(_ *fisk.ParseContext) error {
		return runInstall(installLang, installSkill, installAll, installForce)
	})

	app.MustParseWithUsage(os.Args[1:])
}

func runList(lang string) error {
	langs := []string{"go", "rust"}
	if lang != "" {
		langs = []string{lang}
	}

	for _, l := range langs {
		skills, err := internal.ListSkills(cskills.SkillsFS, l)
		if err != nil {
			return fmt.Errorf("listing %s skills: %w", l, err)
		}
		if len(skills) == 0 {
			continue
		}

		fmt.Printf("[%s]\n", l)
		for _, s := range skills {
			fmt.Printf("  %-20s %-8s %s\n", s.Name, s.Category, s.Description)
		}
		fmt.Println()
	}
	return nil
}

func runInstall(lang, skillFlag string, all, force bool) error {
	skills, err := internal.ListSkills(cskills.SkillsFS, lang)
	if err != nil {
		return fmt.Errorf("listing skills: %w", err)
	}

	if len(skills) == 0 {
		return fmt.Errorf("no skills found for language %q", lang)
	}

	var toInstall []internal.Skill

	switch {
	case skillFlag != "":
		requested := make(map[string]bool)
		for _, name := range strings.Split(skillFlag, ",") {
			requested[strings.TrimSpace(name)] = true
		}
		for _, s := range skills {
			if requested[s.Name] {
				toInstall = append(toInstall, s)
				delete(requested, s.Name)
			}
		}
		for name := range requested {
			fmt.Fprintf(os.Stderr, "warning: skill %q not found for %s\n", name, lang)
		}

	case all:
		toInstall = skills

	default:
		for _, s := range skills {
			if s.Category == "always" {
				toInstall = append(toInstall, s)
			}
		}

		customSkills := filterByCategory(skills, "custom")
		if len(customSkills) > 0 && isTerminal() {
			for _, s := range customSkills {
				if promptYesNo(fmt.Sprintf("Install custom skill %q (%s)?", s.Name, s.Description)) {
					toInstall = append(toInstall, s)
				}
			}
		}
	}

	if len(toInstall) == 0 {
		fmt.Println("no skills to install")
		return nil
	}

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getting working directory: %w", err)
	}

	targetDir := filepath.Join(cwd, ".claude", "skills")

	fmt.Printf("Installing %d skill(s) to %s\n", len(toInstall), targetDir)
	if err := internal.Install(cskills.SkillsFS, toInstall, targetDir, force); err != nil {
		return err
	}
	fmt.Println("done")
	return nil
}

func filterByCategory(skills []internal.Skill, category string) []internal.Skill {
	var result []internal.Skill
	for _, s := range skills {
		if s.Category == category {
			result = append(result, s)
		}
	}
	return result
}

func isTerminal() bool {
	stat, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (stat.Mode() & os.ModeCharDevice) != 0
}

func promptYesNo(question string) bool {
	fmt.Printf("%s [y/N]: ", question)
	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		ans := strings.TrimSpace(strings.ToLower(scanner.Text()))
		return ans == "y" || ans == "yes"
	}
	return false
}
