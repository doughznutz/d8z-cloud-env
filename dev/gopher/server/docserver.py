"""Static server for one language's generated doc tree, plus its plain-text view.

Replaces ``python -m http.server`` behind gopher's Traefik (one process per
language, see entrypoint.sh). Pages are served exactly as generated; add
``?format=text`` to any page (or directory) to get the same page as plain
text instead - what a person sees in the browser, minus markup, scripts and
styles. That text view is the *only* way the report agents see the docs
(they curl it from gopher-agents), so there's one source of truth: whatever
the HTML shows, the agents see, with nothing to keep in sync.

Links in the text view are written as paths relative to the docs root
(``typescript/.../types.Gateway.html``), which an agent fetches as
``<docs root>/<path>?format=text`` - short, because link URLs would otherwise
be half of every page (typedoc repeats its whole navigation on each one).

Usage: ``python3 -m server.docserver <port> <directory>``.
"""

import os
import re
import sys
import urllib.parse
from collections.abc import Callable
from functools import partial
from html.parser import HTMLParser
from http.server import SimpleHTTPRequestHandler, ThreadingHTTPServer

#: Elements whose content is never visible text. Only ones with a closing
#: tag - a void element like <input> here would swallow the rest of the page.
_SKIP = {"script", "style", "head", "template", "svg", "noscript", "button", "select"}

#: Elements that start a new line in the text view.
_BLOCK = {
    "p", "div", "section", "article", "header", "footer", "nav", "main", "aside",
    "ul", "ol", "li", "dl", "dt", "dd", "table", "tr", "br", "hr", "pre",
    "h1", "h2", "h3", "h4", "h5", "h6", "details", "summary", "blockquote", "form",
}  # fmt: skip


class _TextRenderer(HTMLParser):
    """Turns one HTML page into readable plain text, keeping links followable."""

    def __init__(self, link: Callable[[str], str]) -> None:
        """Starts an empty rendering.

        Args:
            link: Turns a page's href into the absolute text-view URL to show.
        """
        super().__init__(convert_charrefs=True)
        self._link = link
        self._out: list[str] = []
        self._skip = 0
        self._pre = 0
        self._href: str | None = None
        self._anchor: list[str] = []

    def handle_starttag(self, tag: str, attrs: list[tuple[str, str | None]]) -> None:
        """Opens an element.

        Args:
            tag: Element name.
            attrs: Its attributes.
        """
        a = dict(attrs)
        # Before the skip check: a redirect lives in <head>, which is skipped.
        if tag == "meta" and (a.get("http-equiv") or "").lower() == "refresh":
            target = (a.get("content") or "").partition("url=")[2].strip("'\" ")
            if target:
                self._out.append(f"\n[redirects to]({self._link(target)})\n")
            return
        if tag in _SKIP:
            self._skip += 1
            return
        if self._skip:
            return
        if tag in _BLOCK:
            self._out.append("\n")
        if tag in {"h1", "h2", "h3", "h4", "h5", "h6"}:
            self._out.append("#" * int(tag[1]) + " ")
        elif tag == "li":
            self._out.append("- ")
        elif tag == "pre":
            self._pre += 1
        elif tag == "a" and a.get("href"):
            self._href = a["href"]
            self._anchor = []

    def handle_endtag(self, tag: str) -> None:
        """Closes an element.

        Args:
            tag: Element name.
        """
        if tag in _SKIP:
            self._skip = max(0, self._skip - 1)
            return
        if self._skip:
            return
        if tag == "a" and self._href is not None:
            label = re.sub(r"\s+", " ", "".join(self._anchor)).strip()
            href, self._href = self._href, None
            if label and not href.startswith(("#", "javascript:")):
                self._out.append(f"[{label}]({self._link(href)})")
            elif label:
                self._out.append(label)
            return
        if tag == "pre":
            self._pre = max(0, self._pre - 1)
        if tag in _BLOCK:
            self._out.append("\n")

    def handle_data(self, data: str) -> None:
        """Adds visible text.

        Args:
            data: Text content.
        """
        if self._skip:
            return
        if not self._pre:
            data = re.sub(r"\s+", " ", data)
        if self._href is not None:
            self._anchor.append(data)
        else:
            self._out.append(data)

    def text(self) -> str:
        """Returns the rendering, with blank-line runs and trailing spaces tidied.

        Returns:
            The page as plain text.
        """
        lines = [line.rstrip() for line in "".join(self._out).splitlines()]
        return re.sub(r"\n{3,}", "\n\n", "\n".join(lines)).strip() + "\n"


def _text_url(tree: str, page_path: str, href: str) -> str:
    """Resolves a page's link to a path relative to the docs root (external links as-is).

    Args:
        tree: This tree's name under the docs root (e.g. "typescript").
        page_path: The current page's path within the tree.
        href: The link as written in the page.

    Returns:
        e.g. "typescript/dev/dood/console/interfaces/types.Gateway.html#gw".
    """
    parts = urllib.parse.urlsplit(href)
    if parts.scheme or parts.netloc:
        return href
    path = urllib.parse.urljoin(page_path, parts.path) if parts.path else page_path
    return f"{tree}{path}" + (f"#{parts.fragment}" if parts.fragment else "")


def render_text(page: str, tree: str, page_path: str) -> str:
    """Renders one HTML page as the plain text the agents read.

    Args:
        page: The page's HTML.
        tree: This tree's name under the docs root (e.g. "python").
        page_path: The page's path within the tree.

    Returns:
        The page as text.
    """
    r = _TextRenderer(partial(_text_url, tree, page_path))
    r.feed(page)
    r.close()
    return r.text()


class _Handler(SimpleHTTPRequestHandler):
    """Static files as usual; ``?format=text`` returns the text view instead."""

    def do_GET(self) -> None:  # noqa: N802 - SimpleHTTPRequestHandler's naming
        """Serves the text view when asked for, else the file itself."""
        url = urllib.parse.urlsplit(self.path)
        if urllib.parse.parse_qs(url.query).get("format") != ["text"]:
            super().do_GET()
            return
        # Traefik strips ".../docs/<tree>" before passing the request on, and
        # says what it stripped; its last segment is this tree's name.
        tree = self.headers.get("X-Forwarded-Prefix", "").rstrip("/").rsplit("/", 1)[-1]
        local = self.translate_path(url.path)
        page_path = url.path
        if os.path.isdir(local):
            if not page_path.endswith("/"):
                page_path += "/"
            index = os.path.join(local, "index.html")
            if os.path.isfile(index):
                local, page_path = index, page_path + "index.html"
            else:
                self._send_text(self._listing(local, tree, page_path))
                return
        if not os.path.isfile(local) or not local.endswith(".html"):
            self.send_error(404, "no text view for this path")
            return
        with open(local, encoding="utf-8", errors="replace") as f:
            self._send_text(render_text(f.read(), tree, page_path))

    def _listing(self, local: str, tree: str, page_path: str) -> str:
        """Renders a directory without an index.html as a list of links.

        Args:
            local: The directory on disk.
            tree: This tree's name under the docs root.
            page_path: The directory's path within the tree.

        Returns:
            The listing as text.
        """
        lines = [f"# Index of {tree}{page_path}", ""]
        for name in sorted(os.listdir(local)):
            full = os.path.join(local, name)
            if os.path.isdir(full):
                lines.append(f"- [{name}/]({_text_url(tree, page_path, name + '/')})")
            elif name.endswith(".html"):
                lines.append(f"- [{name}]({_text_url(tree, page_path, name)})")
        return "\n".join(lines) + "\n"

    def _send_text(self, body: str) -> None:
        """Sends a text/plain response.

        Args:
            body: The text.
        """
        data = body.encode()
        self.send_response(200)
        self.send_header("Content-Type", "text/plain; charset=utf-8")
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)


def main() -> None:
    """Serves ``argv[2]`` on loopback port ``argv[1]`` until killed."""
    port, directory = int(sys.argv[1]), sys.argv[2]
    handler = partial(_Handler, directory=directory)
    ThreadingHTTPServer(("127.0.0.1", port), handler).serve_forever()


if __name__ == "__main__":
    main()
