# ts-term

ts-term is a browser-based terminal interface which uses ephemeral Tailscale nodes for each session. It allows you to run commands using the pseudo-terminal of the machine it's hosted on.

> [!IMPORTANT]
> ts-term doesn't directly create SSH connections. Once connected to the terminal, you must run the `ssh` command to connect to the machine you want to remote into.
> 
> If you would prefer to directly create an SSH connection, Tailscale's [SSH Console](https://tailscale.com/kb/1216/tailscale-ssh-console) is probably what you're looking for.

ts-term is intended to be hosted from inside a Docker container on a machine in your tailnet. This way, you can access the service through your tailnet and connect to the container's terminal. Then you can run commands available within the container including `ssh`-ing into other nodes on your tailnet.

[Tailscale SSH](https://tailscale.com/kb/1193/tailscale-ssh) and their [SSH Console](https://tailscale.com/kb/1216/tailscale-ssh-console) are great tools for remote access to machines on your tailnet. If they suit your needs, I definitely recommend them.<br>
However, I wanted something I could self-host and which didn't take over my machine's `ssh` command so I made this project.

## Getting Started

Pull and run the container with the command:

```bash
docker run -d -p 4000:3000 -h ts-term --name ts-term -v ts-term-data:/home/appuser/.ssh sammytd/ts-term
```

Then visit your host machine's `<tailscale-domain>:4000` from a browser on a device logged in to your tailnet.

Close the window or type `exit` while not using ssh to end the session.

### Docker run options

| Option | Description |
| --- | --- |
| `-d`, `--detach` | Run the container in the background. |
| `-p`, `--publish` | Publish the container's port to the host. The host port can be whichever available port you want.<br>`<host-port>:<container-port>` |
| `-h`, `--hostname` | The container host name. (optional) |
| `--name` | The container name (optional) |
| `-v`, `--volume` | The volume for persistent data. (recommended) |
| `--env-file` | Read in a file of environment variables. (optional) |

See <https://docs.docker.com/reference/cli/docker/container/run/> for full reference.

### Environment Variables

| Variable | Description | Default |
| --- | --- | --- |
| TS_TERM_ADDR | The address the ts-term server runs on. | `:3000` |
| TS_CONTROL_URL | The coordination server to use. | The default Tailscale server |

## Development

### Run the dev server

ts-term requires a Linux environment to run. If you're on Windows, you can use WSL.

Requirements:

- Node.js
- Go
- bash

Create a `.env` file in the project root if you want to customize ts-term. See [environment variables](#environment-variables).

#### Add Go dependencies

```bash
go get ./...
```

#### Run the dev server

```bash
go run . -dev
```

Then visit `localhost:3000` while logged in to your tailnet.

### Build and run the Docker container

#### Build the image

```bash
docker build --platform=linux/amd64 -t my/ts-term .
```

#### Create and run a container from the image

```bash
docker run -d -p 4000:3000 -h ts-term --name my-ts-term -v ts-term-data:/home/appuser/.ssh my/ts-term
```

Then visit your host machine's `<tailscale-domain>:4000`.
