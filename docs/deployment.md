# Deployment

ZVintedWatch is a single executable that contains the monitoring engine, the Telegram notifier and
the web interface. It needs no runtime, no external database and no administrator rights. All its
data lives in one folder (`data` next to the executable by default).

- [Windows](#windows)
- [Linux server](#linux-server)
- [Seedbox or shared host (no root)](#seedbox-or-shared-host-no-root)
- [Keep it at home, send requests from elsewhere](#keep-it-at-home-send-requests-from-elsewhere)
- [Docker](#docker)
- [Accessing the interface from another device](#accessing-the-interface-from-another-device)
- [Updating](#updating) · [Backups](#backups)

## Windows

1. Download `zvintedwatch_<version>_windows_amd64.zip` from the
   [releases](https://github.com/Nikola-Putnik/ZVintedWatch/releases) and extract it into a folder
   of your choice, e.g. `C:\ZVintedWatch`.
2. Double-click `zvintedwatch.exe`. A console window opens (closing it stops ZVintedWatch) and the
   interface opens in your browser at <http://127.0.0.1:8080>.

If SmartScreen shows “Windows protected your PC”, click **More info → Run anyway** (the executable
is not signed).

### Start automatically with Windows

**Simple:** press `Win + R`, type `shell:startup`, and create a shortcut to `zvintedwatch.exe` in
that folder. Edit the shortcut and append ` -open=false` to the target if you don't want the
browser to open at every logon.

**Hidden, at startup (Task Scheduler):**

1. Open *Task Scheduler* → *Create Task…*
2. *General*: name it `ZVintedWatch`, select *Run whether user is logged on or not*.
3. *Triggers*: *At startup* (or *At log on*).
4. *Actions*: *Start a program* → `C:\ZVintedWatch\zvintedwatch.exe`, arguments `run -open=false`,
   *Start in* `C:\ZVintedWatch`.
5. *Settings*: untick *Stop the task if it runs longer than…*

## Linux server

```sh
# Download and extract (use the arm64 archive on ARM machines such as a Raspberry Pi 4/5)
curl -LO https://github.com/Nikola-Putnik/ZVintedWatch/releases/latest/download/zvintedwatch_<version>_linux_amd64.tar.gz
tar -xzf zvintedwatch_*_linux_amd64.tar.gz
./zvintedwatch
```

### As a systemd service

The archive contains `deploy/systemd/zvintedwatch.service`, a hardened service running under a
dedicated user:

```sh
sudo useradd --system --home /opt/zvintedwatch --shell /usr/sbin/nologin zvintedwatch
sudo install -d -o zvintedwatch -g zvintedwatch /opt/zvintedwatch
sudo install -m 755 zvintedwatch /opt/zvintedwatch/zvintedwatch
sudo cp deploy/systemd/zvintedwatch.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now zvintedwatch
journalctl -u zvintedwatch -f
```

This service listens on `0.0.0.0:8080`, so a password is generated on first start and printed in
the logs (`journalctl -u zvintedwatch | grep -A2 password`). Change it in *Settings → Security*.

## Seedbox or shared host (no root)

Seedboxes usually give you a regular user account with SSH access. ZVintedWatch runs there without
any installation:

```sh
mkdir -p ~/zvintedwatch && cd ~/zvintedwatch
curl -LO https://github.com/Nikola-Putnik/ZVintedWatch/releases/latest/download/zvintedwatch_<version>_linux_amd64.tar.gz
tar -xzf zvintedwatch_*_linux_amd64.tar.gz
./zvintedwatch -listen 127.0.0.1:18080      # pick a free port on shared machines
```

Keep it running after you disconnect with **one** of these methods:

- **systemd user service** (if `systemctl --user` works on your host): use
  `deploy/systemd/zvintedwatch-user.service` — instructions are at the top of the file. Ask your
  provider to enable *lingering* if the service stops when you log out.
- **tmux / screen**: `tmux new -s zvw ./zvintedwatch -listen 127.0.0.1:18080`, then detach with
  `Ctrl+B D`.
- **crontab** (restarts it after a reboot):
  ```sh
  crontab -e
  # add:
  @reboot cd $HOME/zvintedwatch && ./zvintedwatch -listen 127.0.0.1:18080 -open=false >> data/stdout.log 2>&1
  ```

To open the interface from your computer, use an **SSH tunnel** — nothing is exposed to the
internet:

```sh
ssh -L 8080:127.0.0.1:18080 user@your-seedbox
# then open http://127.0.0.1:8080 on your computer
```

> **Datacenter IPs:** seedboxes and VPSs use datacenter IP addresses, which marketplaces block more
> easily than home connections. If you see repeated “refused (HTTP 403)” pauses in the activity
> console, run ZVintedWatch at home or add a residential proxy.

## Keep it at home, send requests from elsewhere

ZVintedWatch can route marketplace requests through proxies (*Settings → Network*), each one paced
independently. Any HTTP, HTTPS or SOCKS5 proxy works, including an SSH tunnel to a machine you
control:

```sh
ssh -N -D 1080 user@other-machine
```

Then add `socks5://127.0.0.1:1080` as a proxy for the site. Telegram messages and the web interface
keep using your normal connection.

## Docker

An image for `linux/amd64` and `linux/arm64` is published for every release:

```sh
docker run -d --name zvintedwatch --restart unless-stopped \
  -p 8080:8080 -v "$PWD/data:/data" \
  ghcr.io/nikola-putnik/zvintedwatch:latest
docker logs zvintedwatch     # shows the generated password on first start
```

Or with Compose, using [`deploy/docker-compose.yml`](../deploy/docker-compose.yml):

```sh
docker compose up -d
```

The container runs as a non-root user (uid 65532). If you mount a host folder, make sure it is
writable by that user (`sudo chown 65532:65532 data`).

## Accessing the interface from another device

By default the interface only listens on `127.0.0.1` (this computer). To reach it from your phone
or another computer on your network, start ZVintedWatch with:

```sh
zvintedwatch -listen 0.0.0.0:8080
```

and open `http://<ip-of-the-machine>:8080`. In this mode a **password is mandatory**: one is
generated on first start and printed in the console. You can change it in *Settings → Security*,
and reset it with `zvintedwatch reset-password` if you forget it.

### Behind a reverse proxy (HTTPS)

To expose the interface on the internet, put it behind a reverse proxy with HTTPS. The live
console uses Server-Sent Events, so response buffering must be disabled.

**Caddy**

```
zvw.example.com {
    reverse_proxy 127.0.0.1:8080 {
        flush_interval -1
    }
}
```

**nginx**

```nginx
location / {
    proxy_pass http://127.0.0.1:8080;
    proxy_http_version 1.1;
    proxy_set_header Host $host;
    proxy_buffering off;          # needed by the live console
    proxy_read_timeout 1h;
}
```

## Updating

1. Stop ZVintedWatch.
2. Replace the executable with the new version (keep the `data` folder).
3. Start it again. Database changes are applied automatically.

With Docker: `docker pull ghcr.io/nikola-putnik/zvintedwatch:latest` then recreate the container.

## Backups

Everything is in the data folder:

| File | Content |
|---|---|
| `zvintedwatch.db` (+ `-wal`, `-shm`) | alerts, chats, settings (including the Telegram token and password hash), history |
| `zvintedwatch.log` | log file (rotated at 10 MB) |

Stop ZVintedWatch and copy the folder to back it up or move it to another machine. Treat it as
private: it contains your Telegram bot token.
