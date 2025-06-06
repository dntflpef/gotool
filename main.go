package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type DeploymentConfig struct {
	Name   string `json:"name"`
	Branch string `json:"branch"`
	Commit string `json:"commit"`
}

type ReleaseConfig struct {
	Release      string             `json:"release"`
	Project      string             `json:"project"`
	Source       []string           `json:"source"`
	Deploy       []DeploymentConfig `json:"deploy"`
	Repositories []string           `json:"repositories"`
}

type GitService struct {
	TargetRepo     string
	ConfigRepo     string
	SourceBranch   string
	Version        string
	ProjectName    string
	FileNameSuffix string
	DeployConfigs  []DeploymentConfig
	TempDir        string
}

func NewGitService(
	targetRepo string,
	configRepo string,
	sourceBranch string,
	version string,
	projectName string,
	fileNameSuffix string,
) *GitService {
	return &GitService{
		TargetRepo:     targetRepo,
		ConfigRepo:     configRepo,
		SourceBranch:   sourceBranch,
		Version:        version,
		ProjectName:    projectName,
		FileNameSuffix: fileNameSuffix,
		DeployConfigs:  make([]DeploymentConfig, 0),
	}
}

func (g *GitService) Run() error {
	if err := g.setup(); err != nil {
		return fmt.Errorf("setup failed: %w", err)
	}
	defer g.cleanup()

	if err := g.handleTargetRepo(); err != nil {
		return fmt.Errorf("target repo handling failed: %w", err)
	}

	if err := g.handleConfigRepo(); err != nil {
		return fmt.Errorf("config repo handling failed: %w", err)
	}

	return nil
}

func (g *GitService) setup() error {
	g.TempDir = filepath.Join(os.TempDir(), fmt.Sprintf("release-%d", time.Now().Unix()))
	return os.MkdirAll(g.TempDir, 0755)
}

func (g *GitService) cleanup() {
	os.RemoveAll(g.TempDir)
}

func (g *GitService) handleTargetRepo() error {
	targetPath := filepath.Join(g.TempDir, "target")
	newBranch := fmt.Sprintf("%s-v%s", g.SourceBranch, g.Version)

	if err := g.cloneRepo(g.TargetRepo, targetPath); err != nil {
		return err
	}

	if err := g.createBranch(targetPath, g.SourceBranch, newBranch); err != nil {
		return err
	}

	commit, err := g.getLastCommit(targetPath)
	if err != nil {
		return err
	}

	g.DeployConfigs = append(g.DeployConfigs, DeploymentConfig{
		Name:   g.FileNameSuffix,
		Branch: newBranch,
		Commit: commit,
	})

	return nil
}

func (g *GitService) handleConfigRepo() error {
	configPath := filepath.Join(g.TempDir, "config")
	fileName := g.generateConfigFileName()
	configFile := filepath.Join(configPath, fileName)

	if err := g.cloneRepo(g.ConfigRepo, configPath); err != nil {
		return err
	}

	config := ReleaseConfig{
		Release:      "automation-delivery",
		Project:      g.ProjectName,
		Source:       []string{},
		Deploy:       g.DeployConfigs,
		Repositories: []string{},
	}

	if err := g.createConfigFile(configFile, config); err != nil {
		return err
	}

	return g.pushChanges(configPath, fmt.Sprintf("Add release config for %s v%s", g.ProjectName, g.Version))
}

func (g *GitService) generateConfigFileName() string {
	baseName := fmt.Sprintf("%s_%s", g.ProjectName, g.Version)
	if g.FileNameSuffix != "" {
		return fmt.Sprintf("%s_%s.json", baseName, g.FileNameSuffix)
	}
	return fmt.Sprintf("%s.json", baseName)
}

func (g *GitService) cloneRepo(repoURL, path string) error {
	return g.runCommand("git", "clone", repoURL, path)
}

func (g *GitService) createBranch(repoPath, sourceBranch, newBranch string) error {
	commands := [][]string{
		{"checkout", sourceBranch},
		{"checkout", "-b", newBranch},
		{"push", "origin", newBranch},
	}

	for _, cmd := range commands {
		if err := g.runGitCommand(repoPath, cmd...); err != nil {
			return err
		}
	}
	return nil
}

func (g *GitService) getLastCommit(repoPath string) (string, error) {
	commit, err := g.runGitCommandWithOutput(repoPath, "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(commit), nil
}

func (g *GitService) createConfigFile(path string, config ReleaseConfig) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "    ")
	return encoder.Encode(config)
}

func (g *GitService) pushChanges(repoPath, message string) error {
	commands := [][]string{
		{"add", "."},
		{"commit", "-m", message},
		{"push", "origin", "HEAD"},
	}

	for _, cmd := range commands {
		if err := g.runGitCommand(repoPath, cmd...); err != nil {
			return err
		}
	}
	return nil
}

func (g *GitService) runGitCommand(repoPath string, args ...string) error {
	gitArgs := append([]string{"-C", repoPath}, args...)
	return g.runCommand("git", gitArgs...)
}

func (g *GitService) runGitCommandWithOutput(repoPath string, args ...string) (string, error) {
	gitArgs := append([]string{"-C", repoPath}, args...)
	cmd := exec.Command("git", gitArgs...)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("command failed: %w", err)
	}
	return strings.TrimSpace(string(output)), nil
}

func (g *GitService) runCommand(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func main() {
	if len(os.Args) < 7 {
		fmt.Println("Usage: ./release-automation <target-repo> <config-repo> <source-branch> <version> <project-name> <file-suffix>")
		fmt.Println("Example: ./release-automation git@repo.com:target.git git@repo.com:config.git develop 1.0.0 afs production")
		os.Exit(1)
	}

	service := NewGitService(
		os.Args[1],
		os.Args[2],
		os.Args[3],
		os.Args[4],
		os.Args[5],
		os.Args[6],
	)

	if err := service.Run(); err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Release automation completed successfully")
}