#!/usr/bin/env python3
"""Saves Nanoleaf's API documentation under sdk/aurora-api-specs.

Nanoleaf publishes no machine-readable description of the API, only pages on
a wiki, so sdk/aurora is written by hand against these saved copies and
scripts/apicheck.py checks it against them. Run by `make spec-refresh`; review
the diff afterwards, since a changed page is a changed API.

What is saved:

  wiki/<page>.html    every page of the wiki space, as the wiki renders it
  wiki/<page>.md      the same page as markdown, for reading and grepping
  wiki/attachments/   the pages' images
  mirror/             copies of the older documentation kept by other people
  sources.json        where each file came from, its version and checksum

Standard library only.
"""

import hashlib
import html
import json
import os
import re
import sys
import time
import urllib.error
import urllib.request
from html.parser import HTMLParser

WIKI = "https://nanoleaf.atlassian.net/wiki"
SPACE = "nlapid"
GIST = "ae06bf574748727bcce5127394a8ba43"  # dennishn's copy of the forum documentation
FORUM = "forum.nanoleaf.me/docs/openapi"  # where the documentation used to live

OUT = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "sdk", "aurora-api-specs")
AGENT = "taproot-spec-refresh (+https://github.com/katbyte/nanoleaf-aurora-taproot)"


def fetch(url, tries=3, timeout=60):
    last = None
    for attempt in range(tries):
        try:
            req = urllib.request.Request(url, headers={"User-Agent": AGENT})
            with urllib.request.urlopen(req, timeout=timeout) as res:
                return res.read()
        except (urllib.error.URLError, TimeoutError) as err:
            last = err
            time.sleep(2 * (attempt + 1))
    raise RuntimeError(f"{url}: {last}")


def slug(title):
    return re.sub(r"[^a-z0-9]+", "-", title.lower()).strip("-")


# --- a small html to markdown converter, for what the wiki emits ------------


class Node:
    def __init__(self, tag, attrs=None):
        self.tag, self.attrs, self.children = tag, dict(attrs or []), []


VOID = {"br", "hr", "img", "col", "meta", "link", "input"}


class Tree(HTMLParser):
    def __init__(self):
        super().__init__(convert_charrefs=True)
        self.root = Node("root")
        self.stack = [self.root]

    def handle_starttag(self, tag, attrs):
        node = Node(tag, attrs)
        self.stack[-1].children.append(node)
        if tag not in VOID:
            self.stack.append(node)

    def handle_startendtag(self, tag, attrs):
        self.stack[-1].children.append(Node(tag, attrs))

    def handle_endtag(self, tag):
        for i in range(len(self.stack) - 1, 0, -1):
            if self.stack[i].tag == tag:
                del self.stack[i:]
                return

    def handle_data(self, data):
        self.stack[-1].children.append(data)


BLOCKS = {"p", "div", "table", "ul", "ol", "pre", "h1", "h2", "h3", "h4", "h5", "h6", "hr"}


def text_of(node):
    if isinstance(node, str):
        return node
    return "".join(text_of(c) for c in node.children)


def inline(node, in_table=False):
    """One run of text: a paragraph's, a heading's, a list item's."""
    if isinstance(node, str):
        return re.sub(r"\s+", " ", node)
    tag, inner = node.tag, "".join(inline(c, in_table) for c in node.children)
    if tag == "br":
        return "<br>" if in_table else "  \n"
    if tag == "img":
        return f"![{node.attrs.get('alt', '')}]({node.attrs.get('src', '')})"
    if tag in ("style", "script", "colgroup"):
        return ""
    if not inner.strip():
        return inner
    if tag in ("strong", "b"):
        return f"**{inner.strip()}**"
    # the wiki italicises every table cell, which says nothing; keep emphasis in prose only
    if tag in ("em", "i") and not in_table:
        return f"*{inner.strip()}*"
    if tag == "code":
        return f"`{inner.strip()}`"
    if tag == "a":
        href = node.attrs.get("href", "")
        return inner if not href or href.startswith("#") else f"[{inner.strip()}]({href})"
    return inner


def cell(node):
    """A table cell on one line: paragraphs joined by <br>, code kept as <pre>."""
    parts = []
    for child in node.children:
        if isinstance(child, Node) and child.tag in ("div", "p", "ul", "ol", "pre", "table"):
            if child.tag == "pre":
                code = html.escape(text_of(child).strip("\n"), quote=False)
                parts.append("<pre>" + code.replace("\n", "<br>") + "</pre>")
            elif child.tag in ("ul", "ol"):
                parts.extend("- " + inline(li, True).strip() for li in child.children if isinstance(li, Node))
            else:
                parts.append(cell(child))
        else:
            parts.append(inline(child, True))
    joined = "<br>".join(p.strip() for p in parts if p.strip())
    return joined.replace("|", "\\|")


def table(node, out):
    rows = []

    def walk(n):
        for c in n.children:
            if isinstance(c, Node):
                if c.tag == "tr":
                    rows.append([x for x in c.children if isinstance(x, Node) and x.tag in ("td", "th")])
                elif c.tag in ("tbody", "thead", "tfoot"):
                    walk(c)

    walk(node)
    rows = [r for r in rows if r]
    if not rows:
        return
    width = max(len(r) for r in rows)
    lines = [[cell(c) for c in r] + [""] * (width - len(r)) for r in rows]
    # a row of nothing but th cells is a header; the name/value tables have none
    if all(c.tag == "th" for c in rows[0]) and len(rows) > 1:
        head, body = lines[0], lines[1:]
    else:
        head, body = [""] * width, lines
    out.append("| " + " | ".join(head) + " |")
    out.append("|" + "|".join(["---"] * width) + "|")
    out.extend("| " + " | ".join(r) + " |" for r in body)
    out.append("")


def listing(node, out, depth=0):
    n = 0
    for li in node.children:
        if not isinstance(li, Node) or li.tag != "li":
            continue
        n += 1
        mark = f"{n}." if node.tag == "ol" else "-"
        own = "".join(inline(c) for c in li.children if not (isinstance(c, Node) and c.tag in ("ul", "ol")))
        out.append("  " * depth + f"{mark} {own.strip()}")
        for c in li.children:
            if isinstance(c, Node) and c.tag in ("ul", "ol"):
                listing(c, out, depth + 1)
    if depth == 0:
        out.append("")


def blocks(node, out):
    run = []  # loose text between blocks

    def flush():
        line = "".join(run).strip()
        run.clear()
        if line:
            out.extend([line, ""])

    for child in node.children:
        if isinstance(child, str) or child.tag not in BLOCKS:
            run.append(inline(child))
            continue
        flush()
        tag = child.tag
        if tag in ("h1", "h2", "h3", "h4", "h5", "h6"):
            title = inline(child).strip()
            if title:
                out.extend(["#" * int(tag[1]) + " " + title, ""])
        elif tag == "p":
            line = "".join(inline(c) for c in child.children).strip()
            if line:
                out.extend([line, ""])
        elif tag == "pre":
            code = text_of(child).strip("\n")
            lang = "json" if code.lstrip()[:1] in ("{", "[") else ""
            out.extend(["```" + lang, code, "```", ""])
        elif tag in ("ul", "ol"):
            listing(child, out)
        elif tag == "table":
            table(child, out)
        elif tag == "hr":
            out.extend(["---", ""])
        else:
            blocks(child, out)
    flush()


def markdown(body):
    tree = Tree()
    tree.feed(body)
    out = []
    blocks(tree.root, out)
    text = "\n".join(out)
    return re.sub(r"\n{3,}", "\n\n", text).strip() + "\n"


# --- the wiki ----------------------------------------------------------------

PAGE_CSS = """
body { font: 15px/1.5 -apple-system, "Segoe UI", sans-serif; max-width: 62rem; margin: 2rem auto; padding: 0 1rem; }
table { border-collapse: collapse; margin: 1rem 0; }
td, th { border: 1px solid #bbb; padding: .3rem .6rem; vertical-align: top; text-align: left; }
pre { background: #f4f4f4; padding: .6rem; overflow-x: auto; }
img { max-width: 100%; }
.source { color: #555; border-bottom: 1px solid #bbb; padding-bottom: .6rem; }
"""


def clean(body):
    """Drops the ids the wiki stamps on every block: they are noise in a diff."""
    body = re.sub(r'\s(?:data-)?local-id="[^"]*"', "", body)
    return re.sub(r'\sdata-(?:table-width|layout|syntaxhighlighter-params|theme)="[^"]*"', "", body)


def wiki_pages():
    listing_ = json.loads(fetch(f"{WIKI}/rest/api/content?spaceKey={SPACE}&type=page&limit=200"))
    for stub in listing_["results"]:
        expand = "body.export_view,version,ancestors,children.attachment"
        yield json.loads(fetch(f"{WIKI}/rest/api/content/{stub['id']}?expand={expand}"))


def save_wiki(sources):
    os.makedirs(os.path.join(OUT, "wiki", "attachments"), exist_ok=True)
    for page in sorted(wiki_pages(), key=lambda p: p["title"]):
        name, pid = slug(page["title"]), page["id"]
        url = f"{WIKI}/spaces/{SPACE}/pages/{pid}"
        body = clean(page["body"]["export_view"]["value"])
        files = []

        for att in page["children"]["attachment"]["results"]:
            data = fetch(WIKI + att["_links"]["download"])
            rel = f"wiki/attachments/{pid}-{att['title']}"
            write(rel, data, files)
            # the page links its images on the wiki; point them at the saved copies
            body = re.sub(
                r'src="[^"]*/download/attachments/' + pid + "/" + re.escape(att["title"]) + r'[^"]*"',
                f'src="attachments/{pid}-{att["title"]}"',
                body,
            )

        version, when = page["version"]["number"], page["version"]["when"][:10]
        parents = " / ".join(a["title"] for a in page.get("ancestors", []))
        note = f'Saved from <a href="{url}">{url}</a>, version {version} of {when}.'
        doc = (
            f'<!doctype html>\n<html lang="en">\n<head>\n<meta charset="utf-8">\n'
            f"<title>{html.escape(page['title'])}</title>\n<style>{PAGE_CSS}</style>\n</head>\n<body>\n"
            f'<p class="source">{note}</p>\n<h1>{html.escape(page["title"])}</h1>\n{body}\n</body>\n</html>\n'
        )
        write(f"wiki/{name}.html", doc.encode(), files)
        head = f"<!-- Saved from {url}, version {version} of {when}. The .html beside this is the original. -->\n\n"
        write(f"wiki/{name}.md", (head + f"# {page['title']}\n\n" + markdown(body)).encode(), files)

        sources.append(
            {"title": page["title"], "under": parents, "url": url, "version": version, "edited": when, "files": files}
        )
        print(f"  wiki  v{version:<3} {when}  {page['title']}")


# --- the copies other people kept ---------------------------------------------


def save_gist(sources):
    meta = json.loads(fetch(f"https://api.github.com/gists/{GIST}"))
    files = []
    for name, entry in meta["files"].items():
        write(f"mirror/gist-{slug(os.path.splitext(name)[0])}.md", fetch(entry["raw_url"]), files)
    sources.append(
        {
            "title": "Nanoleaf Docs (a copy of the forum documentation, kept as a gist by dennishn)",
            "url": meta["html_url"],
            "version": meta["history"][0]["version"][:12],
            "edited": meta["updated_at"][:10],
            "files": files,
        }
    )
    print(f"  gist  {meta['updated_at'][:10]}  {meta['html_url']}")


def save_forum(sources):
    """The forum page itself is gone; keep whatever the Internet Archive has of it."""
    entry = {"title": "The forum documentation, as the Internet Archive last saw it", "url": "https://" + FORUM}
    try:
        found = json.loads(fetch(f"https://archive.org/wayback/available?url={FORUM}", tries=2, timeout=30))
        snap = found.get("archived_snapshots", {}).get("closest")
        if not snap:
            raise RuntimeError("the archive holds no copy")
        stamp = snap["timestamp"]
        data = fetch(f"https://web.archive.org/web/{stamp}id_/https://{FORUM}", tries=2)
        files = []
        write(f"mirror/forum-openapi-archived-{stamp[:8]}.html", data, files)
        entry.update({"version": stamp, "edited": f"{stamp[:4]}-{stamp[4:6]}-{stamp[6:8]}", "files": files})
        print(f"  forum {entry['edited']}  archived copy")
    except (RuntimeError, ValueError, KeyError) as err:
        # keep the copy already saved, if any, rather than lose it to an archive outage
        kept = [s for s in previous() if s.get("url") == entry["url"] and s.get("files")]
        if kept:
            entry = kept[0]
            print(f"  forum kept the copy already saved ({err})")
        else:
            entry["not_saved"] = str(err)
            print(f"  forum NOT SAVED: {err}")
    sources.append(entry)


def previous():
    try:
        with open(os.path.join(OUT, "sources.json"), encoding="utf-8") as fh:
            return json.load(fh)["sources"]
    except (OSError, ValueError, KeyError):
        return []


def write(rel, data, files):
    path = os.path.join(OUT, rel)
    os.makedirs(os.path.dirname(path), exist_ok=True)
    with open(path, "wb") as fh:
        fh.write(data)
    files.append({"file": rel, "bytes": len(data), "sha256": hashlib.sha256(data).hexdigest()})


def main():
    print("==> saving Nanoleaf's documentation into sdk/aurora-api-specs...")
    sources = []
    save_wiki(sources)
    save_gist(sources)
    save_forum(sources)
    with open(os.path.join(OUT, "sources.json"), "w", encoding="utf-8") as fh:
        json.dump({"fetched": time.strftime("%Y-%m-%d"), "sources": sources}, fh, indent=2)
        fh.write("\n")
    missing = [s["title"] for s in sources if s.get("not_saved")]
    if missing:
        print("not saved this time (run again later):", "; ".join(missing))
    return 0


if __name__ == "__main__":
    sys.exit(main())
