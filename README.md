# ts-term

ts-term serves a browser-based terminal interface and provides SSH through ephemeral Tailscale nodes.

ts-term is intended to be hosted from inside a [Docker](https://www.docker.com/) container on a machine in your tailnet. This way, you can access the service through your tailnet.<br>Each session runs through ephemeral Tailscale nodes created under the user's identity.

[Tailscale SSH](https://tailscale.com/kb/1193/tailscale-ssh) and their [SSH Console](https://tailscale.com/kb/1216/tailscale-ssh-console) are great tools for remote access to machines on your tailnet. If they suit your needs, I definitely recommend them.<br>
However, I wanted something I could self-host and which didn't take over my machine's `ssh` command so I made this project.

<img width="1607" height="832" alt="Screenshot 2025-08-11 073307" src="https://github.com/user-attachments/assets/9fabd24c-9fd6-4fdb-bb6b-c5594eafb78d" />

## Getting Started

### Running with Docker run

Create a container network:

```bash
docker network create ts-term-net
```

Pull and run the container with the command:

```bash
docker run -d -p 4000:3000 -h ts-term --name ts-term --network=ts-term-net -v ts-term-data:/home/appuser/.ssh sammytd/ts-term
```

Then visit your host machine's `<tailscale-domain>:4000` from a browser on a device logged in to your tailnet.

Close the window or type `exit` while not using SSH to end the session.

#### Docker run options

| Option | Description |
| --- | --- |
| `-d, --detach` | Run the container in the background. |
| `-p, --publish` | Publish the container's port to the host. The host port can be whichever available port you want.<br>`<host-port>:<container-port>` |
| `-h, --hostname` | The container host name. (optional) |
| `--name` | The container name (optional) |
| `--network` | The container network to use. (recommended) |
| `-v, --volume` | The container volume for persistent data. (recommended) |
| `--env-file` | Read in a file of environment variables. (optional)<br>See [environment variables](#environment-variables). |

See <https://docs.docker.com/reference/cli/docker/container/run/> for full reference.

### Running with Docker Compose

```yaml
# compose.yml

name: ts-term

services:
  ts-term:
    image: sammytd/ts-term
    hostname: ts-term
    # env_file: .env
    ports:
      - "4000:3000"
    networks:
      - ts-term-net
    volumes:
      - ts-term-data:/home/appuser/.ssh
    restart: unless-stopped

volumes:
  ts-term-data:

networks:
  ts-term-net:
    name: ts-term-net
```

Pull and run the container with the command:

```bash
docker compose up -d
```

Then visit your host machine's `<tailscale-domain>:4000` from a browser on a device logged in to your tailnet.

Close the window or type `exit` while not using SSH to end the session.

### Environment Variables

| Variable | Description | Default |
| --- | --- | --- |
| TS_TERM_ADDR | The address the ts-term server runs on. | `:3000` |
| TS_CONTROL_URL | The coordination server to use. | The default Tailscale server |
| TS_TERM_KNOWN_HOSTS | The absolute path to the known_hosts file. | `<user-home>/.ssh/known_hosts` |

## Development

### Run the dev server

Requirements:

- [Node.js](https://nodejs.org/)
- [pnpm](https://pnpm.io/) (optional)
- [Go](https://go.dev/)

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
