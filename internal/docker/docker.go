package docker

import (
	"bytes"
	"context"
	"errors"
	"fmt"
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
		return "macOS: install Docker Desktop from https://docs.docker.com/desktop/setup/install/mac-install/ or run `brew install --cask docker`."
	case "linux":
		return "Linux: install Docker Engine using your distro package manager. Ubuntu/Debian users can follow https://docs.docker.com/engine/install/ubuntu/."
	case "windows":
		return "Windows: install Docker Desktop from https://docs.docker.com/desktop/setup/install/windows-install/."
	default:
		return "Install Docker for your operating system from https://docs.docker.com/get-docker/."
	}
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

func run(ctx context.Context, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

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
