# Harbor

Harbor is a terminal UI for common Docker workflows. It wraps the Docker CLI with a Bubble Tea interface for checking Docker status, viewing containers, creating starter Dockerfiles, and launching basic containers.

## Requirements

- Go 1.22+
- Docker CLI, or use Harbor's status screen to install Docker when it is missing

## Run

```sh
go run .
```

## Controls

- `1` status and install guidance
- `i` install Docker when the status screen reports the CLI is missing
- `2` containers
- `3` create Dockerfile
- `4` run container
- `r` refresh Docker state
- `enter` select or submit
- `esc` back
- `q` quit

## Notes

Harbor shells out to the local `docker` command. When Docker is missing, Harbor can start an OS-specific install:

- macOS: Docker Desktop through Homebrew
- Linux: Docker Engine through Docker's official convenience script
- Windows: Docker Desktop through winget

Docker Desktop still needs to be opened once after installation so it can finish setup and start the daemon. Linux users may need to log out and back in before running Docker without `sudo`.
