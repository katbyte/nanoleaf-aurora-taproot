# Security Policy

## Supported Versions

Only the [latest release](https://github.com/katbyte/nanoleaf-aurora-taproot/releases/latest) is supported — please update before reporting an issue.

## Reporting a Vulnerability

Please **do not** open a public issue for security vulnerabilities.

Instead, report privately via [GitHub's private vulnerability reporting](https://github.com/katbyte/nanoleaf-aurora-taproot/security/advisories/new).

I will do my best to acknowledge reports within 2 weeks and aim to release a fix or mitigation within 6 weeks for confirmed issues; timelines are best-effort.

## Scope

`taproot` holds a token for each Nanoleaf controller it is connected to, in `controllers.json` under its configuration directory. A token never expires and gives full control of its controller, and the controller's API carries it in the path of every request, over plain http, on the local network. Issues involving a token leaking are particularly relevant: appearing in output, logs, error messages, a `--dry-run`, a `--json` document, a backup, a test recording, or anything `taproot serve` sends to a browser.

`taproot serve` is a page with no login that changes real things. It refuses a request sent to a name it does not expect, and one that changes anything unless it comes from the page itself; a way for another site to get a request past those checks is in scope. That anyone who can reach the port can use the page is by design: it is meant for a home network, with a login in front of it anywhere else.
