# AntennaDeck

**Station antenna control for contesting.** Current version: **1.1.0** (see [Releases](../../releases)).

A compact Windows app that controls a **4O3A Rotator Genius**, a **SteppIR SDA100**
(through a TCP-to-RS232 converter) and a **4O3A Antenna Genius** from one window,
and takes beam headings and radio frequencies from **N1MM Logger+** during a contest.

Written in Go with [Wails](https://wails.io); the window is plain HTML/CSS/JS in
`frontend/dist`.

![Horizontal layout](docs/horizontal.png)

<img src="docs/vertical.png" alt="Vertical layout" width="260" align="right">

The **⇆** button switches to a narrow vertical layout that sits beside N1MM's windows.

<br clear="right">

## Running it

The program is built to:

```
%LOCALAPPDATA%\Programs\AntennaDeck\AntennaDeck.exe
```

Pin it to the taskbar or make a shortcut. Click **⚙** to enter the IP address and port
of each device, then **Save**. Settings and a log file are kept in
`%APPDATA%\AntennaDeck\` (`config.json`, `antennadeck.log`).

Toolbar buttons: **⇆** switches between the horizontal and vertical layout, **📌** keeps
the window on top of N1MM, **⚙** opens settings and the log.

## What each panel does

**Rotor**: click anywhere on the compass to turn there, type a heading and press GO or Enter,
or use the preset buttons (edit them in settings). **SP** and **LP** show the last heading N1MM
asked for, along with its long-path reciprocal; click either one to turn there.

**SteppIR**: N / 180° / Bi (plus ¾λ for verticals if enabled), band buttons, step up/down,
Retract, Calibrate, and the controller's Autotrack on/off. **Tracking** makes the SteppIR follow
the N1MM radio frequency. It retunes when the radio moves more than the selected kHz
or changes band.
**Elements never move while the controlling radio is transmitting.** A frequency change is
held and sent as soon as transmit ends.

**Antenna Genius**: one row per antenna with an **A** and a **B** button for the two radios (SO2R).
Green shows radio A's antenna, blue shows radio B's, and red means that radio is transmitting.
Antennas not allowed on a port's current band are greyed out.
If this PC is on a different subnet from the AG, the AG treats it as a remote connection and
asks for its **remote access code**. Enter that code in Settings (it is set in the AG's own network setup). The AG keeps its own
band-based automatic selection. A manual pick holds until the next band change.

## N1MM Logger+ setup

In N1MM: **Config ▸ Configure Ports, Mode Control… ▸ Broadcast Data**

* tick **Radio** and send to `127.0.0.1:12060`
* send rotor commands to `127.0.0.1:12040`

If those ports are already used by something else (e.g. the old Node-RED flow listened on
12041), change the ports in AntennaDeck's settings to match. The Log tab shows an error if a port
is already in use.

**Follow radio** (SteppIR settings) picks which N1MM radio the SteppIR tracks: radio 1, radio 2,
the active (TX) radio, or *Radio with SteppIR on AG*, which follows whichever AG port
currently has the SteppIR antenna selected. That last option suits SO2R; set
*SteppIR antenna on AG* to the right AG antenna.

Also add `127.0.0.1:12060` to the **Radio** line if N1MM already sends radio data elsewhere.
Addresses on one line are separated by a space, and N1MM sends to all of them.

## Stream Deck and remote control

AntennaDeck can take commands as simple web addresses (HTTP GET), so a Stream Deck button,
a browser bookmark or a script can turn the beam, change SteppIR band or switch antennas.
This works from another PC on the LAN or over the VPN you use for remote operating.

1. In AntennaDeck: **⚙ ▸ Stream Deck / remote control ▸ Enable web commands**, then **Save**.
   The default port is 8090. The first time, Windows asks to allow AntennaDeck through the firewall.
   Allow it on **private** networks.
2. Click **Show all command URLs**. A page opens listing every command for *your* station,
   including your presets, bands and AG antenna names. Copy URLs from there.
3. On the PC with the Stream Deck, install a plugin from the Elgato Marketplace that sends an
   **HTTP GET / web request** (search "API" or "web request"). Paste one URL per button.

Example commands (replace `shack-pc` with the shack PC's IP address):

| URL | Does |
|---|---|
| `http://shack-pc:8090/api/rotor/220` | Turn to 220° |
| `http://shack-pc:8090/api/rotor/preset/EU` | Turn to the EU preset |
| `http://shack-pc:8090/api/rotor/cw/10` · `/ccw/10` | Nudge 10° |
| `http://shack-pc:8090/api/rotor/sp` · `/lp` | Short / long path to the last N1MM heading |
| `http://shack-pc:8090/api/rotor/stop` | Stop |
| `http://shack-pc:8090/api/steppir/band/20m` | SteppIR to 20 m |
| `http://shack-pc:8090/api/steppir/dir/180` | SteppIR 180° (`normal`, `180`, `bi`) |
| `http://shack-pc:8090/api/steppir/up` · `/down` | Step frequency |
| `http://shack-pc:8090/api/steppir/tracking/toggle` | N1MM tracking on/off |
| `http://shack-pc:8090/api/steppir/retract` | Retract elements (no confirmation!) |
| `http://shack-pc:8090/api/ag/A/3` | Antenna Genius: radio A to antenna 3 |
| `http://shack-pc:8090/api/ag/B/name/40M V` | Radio B to the antenna named "40M V" |
| `http://shack-pc:8090/api/status` | Everything's current state as JSON |

Each command answers `{"ok":true}`, or `{"ok":false,"error":"..."}`, and appears in the Log tab.
Set a **Key** in settings to require `?key=<key>` on every URL, e.g. if the shack network is
shared. Don't forward this port to the internet. Use your VPN.

## Building

You need [Go](https://go.dev/dl/) 1.22 or newer and the WebView2 runtime (built into Windows 11).

Double-click **`build.bat`**. It installs the Wails build tool the first time, runs the tests,
builds the app, and opens the folder containing the new `AntennaDeck.exe`.

It builds in a copy under `%TEMP%`, so it also works if the source folder is protected by
Windows Defender **Controlled folder access** (e.g. inside Documents).

## Adding your own features with Claude Code

This app was written with [Claude Code](https://claude.com/claude-code), Anthropic's AI coding
assistant, by a ham who doesn't consider himself a programmer. You can extend it the same way.
Describe what you want in plain English, and Claude Code reads the source, makes the change,
builds it and runs the tests.

1. **Fork** this repository on GitHub (the *Fork* button, top right) so you have your own copy.
2. **Clone** your fork to your PC, e.g. `git clone https://github.com/<your-call>/AntennaDeck`.
   Keep it out of `Documents` if Windows Controlled folder access is on.
3. **Install Claude Code** (desktop app or `npm install -g @anthropic-ai/claude-code`) and open the
   cloned folder in it.
4. **Ask for what you want.** For example:
   * "Add a button that turns the beam to long path of the last N1MM heading and holds it there."
   * "Support a second Rotator Genius for my 40 m beam, as a second compass."
   * "Add a Stream Deck command that turns the beam and selects the matching AG antenna in one press."
   * "My controller is a SteppIR SDA2000. Check the protocol differences and adapt steppir.go."
   * "Make the band buttons bigger and add 60 m."
5. Run **`build.bat`** to build and test, then try it on the air.
6. Commit and push to your fork. If the feature would help others, open a **pull request**
   back to this repository.

Tips:

* Tell Claude Code your station details: device models, IP addresses, firmware versions, and
  SO2R or single radio. Paste lines from the app's **Log** tab when something doesn't work.
  Most fixes in this project came from a pasted log or a captured raw device reply.
* Ask it to test with read-only commands or fakes, and say so if it must **not** turn the
  rotator or move SteppIR elements while testing.
* A working example helps it a lot, e.g. a Node-RED flow or a capture from another program
  that already talks to your device.

## Protocol notes

* Rotator Genius (TCP 9006): `|h` status, `|A1<az>` turn rotor 1, `|S` stop. Reply is
  `|h2` + 0x00 then a 34-byte block per rotor (heading, moving flag, target, name).
* SteppIR SDA100: 11-byte `@A` packets (frequency in 10 Hz units, direction byte, command
  byte), status with `?A`. Command bytes: 0x00 set, `R`/`U` autotrack on/off, `S` home, `V` calibrate.
* Antenna Genius (TCP 9007): 4O3A Genius API: `port set <1|2> rxant=<n>`, `sub port all`,
  `antenna list`, `band list`. Commands must end in LF. Firmware 4.1.16 ignores the CR shown
  in 4O3A's docs. Connections from another subnet must first send `auth code=<code>`.
* N1MM: `<N1MMRotor>` (goazi / stop) and `<RadioInfo>` (Freq in 10 Hz units, IsTransmitting).

## Version history

* **1.1.0** (2026-10-01): Stream Deck / remote control over HTTP, with a page listing every
  command URL for your station. Fixed: a fresh install (no saved settings) crashed at startup.
* **1.0.0** (2026-10-01): first release. Rotator Genius, SteppIR SDA100 and Antenna Genius
  (SO2R) control, plus N1MM rotor commands and frequency tracking. Formerly called "Beam Controller";
  settings are carried over automatically.

## Planned

* Full AntennaDeck window in a web browser, for remote operating without remote desktop.
