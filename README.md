# Harbor

Harbor is a terminal UI for common Docker workflows. It wraps the Docker CLI with a Bubble Tea interface for checking Docker status, viewing containers, creating starter Dockerfiles, and launching basic containers.

## Requirements

- Go 1.22+
- Docker CLI, unless you only want install guidance from the app

## Run

```sh
go run .
```

## Controls

- `1` status and install guidance
- `2` containers
- `3` create Dockerfile
- `4` run container
- `r` refresh Docker state
- `enter` select or submit
- `esc` back
- `q` quit

## Notes

Harbor shells out to the local `docker` command. It does not run a Docker daemon itself and does not perform privileged installation steps automatically.
