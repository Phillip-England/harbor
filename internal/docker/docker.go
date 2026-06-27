package docker

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

type Status struct {
	Installed bool
	Running   bool
	Version   string
	Message   string
}

type Container struct {
	ID      string
	Image   string
	Command string
	Status  string
	Ports   string
	Name    string
}

type RunOptions struct {
	Image  string
	Name   string
	Ports  string
	Detach bool
}

func CheckStatus(ctx context.Context) Status {
	if _, err := exec.LookPath("docker"); err != nil {
		return Status{
			Installed: false,
			Running:   false,
			Message:   "Docker CLI was not found in PATH.",
		}
	}

	version, err := run(ctx, "docker", "version", "--format", "{{.Server.Version}}")
	if err != nil {
		clientVersion, _ := run(ctx, "docker", "version", "--format", "{{.Client.Version}}")
		return Status{
			Installed: true,
			Running:   false,
			Version:   strings.TrimSpace(clientVersion),
			Message:   "Docker CLI is installed, but the Docker daemon is not reachable.",
		}
	}

	return Status{
		Installed: true,
		Running:   true,
		Version:   strings.TrimSpace(version),
		Message:   "Docker is installed and the daemon is reachable.",
	}
}

func InstallHint() string {
	switch runtime.GOOS {
	case "darwin":
		if hasCommand("brew") {
			return "macOS: press i to install Docker Desktop with Homebrew, or install manually from https://docs.docker.com/desktop/setup/install/mac-install/."
		}
		return "macOS: install Homebrew for one-key install support, or install Docker Desktop from https://docs.docker.com/desktop/setup/install/mac-install/."
	case "linux":
		return "Linux: press i to install Docker Engine with Docker's convenience script for development machines, or follow https://docs.docker.com/engine/install/."
	case "windows":
		if hasCommand("winget") {
			return "Windows: press i to install Docker Desktop with winget, or install manually from https://docs.docker.com/desktop/setup/install/windows-install/."
		}
		return "Windows: install Docker Desktop from https://docs.docker.com/desktop/setup/install/windows-install/."
	default:
		return "Install Docker for your operating system from https://docs.docker.com/get-docker/."
	}
}

func InstallDocker(ctx context.Context) (string, error) {
	switch runtime.GOOS {
	case "darwin":
		if !hasCommand("brew") {
			return "", errors.New("Homebrew is required for automatic install on macOS. Install Docker Desktop from https://docs.docker.com/desktop/setup/install/mac-install/")
		}
		if _, err := runLong(ctx, "brew", "install", "--cask", "docker-desktop"); err != nil {
			return "", err
		}
		return "Installed Docker Desktop. Open Docker Desktop once to finish setup and start the daemon.", nil
	case "linux":
		return installLinux(ctx)
	case "windows":
		if !hasCommand("winget") {
			return "", errors.New("winget is required for automatic install on Windows. Install Docker Desktop from https://docs.docker.com/desktop/setup/install/windows-install/")
		}
		if _, err := runLong(ctx, "winget", "install", "-e", "--id", "Docker.DockerDesktop", "--accept-package-agreements", "--accept-source-agreements"); err != nil {
			return "", err
		}
		return "Installed Docker Desktop. Start Docker Desktop once to finish setup and start the daemon.", nil
	default:
		return "", fmt.Errorf("automatic Docker install is not supported on %s; install Docker from https://docs.docker.com/get-docker/", runtime.GOOS)
	}
}

func installLinux(ctx context.Context) (string, error) {
	downloader := ""
	switch {
	case hasCommand("curl"):
		downloader = "curl -fsSL https://get.docker.com"
	case hasCommand("wget"):
		downloader = "wget -qO- https://get.docker.com"
	default:
		return "", errors.New("curl or wget is required to install Docker automatically on Linux")
	}

	installer := downloader + " | sh"
	if os.Geteuid() != 0 {
		if !hasCommand("sudo") {
			return "", errors.New("sudo is required to install Docker automatically on Linux when not running as root")
		}
		installer = downloader + " | sudo -E sh"
	}

	if _, err := runLong(ctx, "sh", "-c", installer); err != nil {
		return "", err
	}
	return "Installed Docker Engine. You may need to log out and back in before running Docker without sudo.", nil
}

func ListContainers(ctx context.Context, all bool) ([]Container, error) {
	args := []string{"ps", "--format", "{{.ID}}\t{{.Image}}\t{{.Command}}\t{{.Status}}\t{{.Ports}}\t{{.Names}}"}
	if all {
		args = []string{"ps", "-a", "--format", "{{.ID}}\t{{.Image}}\t{{.Command}}\t{{.Status}}\t{{.Ports}}\t{{.Names}}"}
	}

	out, err := run(ctx, "docker", args...)
	if err != nil {
		return nil, err
	}

	var containers []Container
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		for len(parts) < 6 {
			parts = append(parts, "")
		}
		containers = append(containers, Container{
			ID:      parts[0],
			Image:   parts[1],
			Command: strings.Trim(parts[2], "\""),
			Status:  parts[3],
			Ports:   parts[4],
			Name:    parts[5],
		})
	}
	return containers, nil
}

func StopContainer(ctx context.Context, id string) error {
	_, err := run(ctx, "docker", "stop", id)
	return err
}

func StartContainer(ctx context.Context, id string) error {
	_, err := run(ctx, "docker", "start", id)
	return err
}

func RemoveContainer(ctx context.Context, id string) error {
	_, err := run(ctx, "docker", "rm", "-f", id)
	return err
}

func Logs(ctx context.Context, id string) (string, error) {
	return run(ctx, "docker", "logs", "--tail", "80", id)
}

func RunContainer(ctx context.Context, opts RunOptions) (string, error) {
	if strings.TrimSpace(opts.Image) == "" {
		return "", errors.New("image is required")
	}

	args := []string{"run"}
	if opts.Detach {
		args = append(args, "-d")
	}
	if strings.TrimSpace(opts.Name) != "" {
		args = append(args, "--name", strings.TrimSpace(opts.Name))
	}
	if strings.TrimSpace(opts.Ports) != "" {
		for _, mapping := range strings.Split(opts.Ports, ",") {
			mapping = strings.TrimSpace(mapping)
			if mapping != "" {
				args = append(args, "-p", mapping)
			}
		}
	}
	args = append(args, strings.TrimSpace(opts.Image))

	return run(ctx, "docker", args...)
}

func hasCommand(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func run(ctx context.Context, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	return runCommand(ctx, name, args...)
}

func runLong(ctx context.Context, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Minute)
	defer cancel()

	return runCommand(ctx, name, args...)
}

func runCommand(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = strings.TrimSpace(stdout.String())
		}
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("%s %s: %s", name, strings.Join(args, " "), msg)
	}
	return stdout.String(), nil
}
