"""The plain-text view of the docs (server/docserver.py) - the agents' only way in.

Agents see exactly what this renders, so these pin down that it shows a
page's visible text, hides markup, and keeps every link followable.
"""

import threading
import urllib.error
import urllib.request
from collections.abc import Iterator
from functools import partial
from http.server import ThreadingHTTPServer
from pathlib import Path

import pytest

from server.docserver import _Handler, render_text


def test_visible_text_only() -> None:
    """Scripts, styles and <head> never reach the text view; visible text does."""
    page = (
        "<html><head><title>T</title><style>.x{color:red}</style></head>"
        "<body><script>var secret = 1;</script><h1>Title</h1><p>Body text.</p></body></html>"
    )
    text = render_text(page, "python", "/index.html")
    assert "# Title" in text and "Body text." in text
    assert "secret" not in text and "color:red" not in text


def test_void_elements_dont_swallow_the_page() -> None:
    """An <input> (no closing tag) mustn't hide everything after it."""
    text = render_text('<input type="text"><p>after the input</p>', "python", "/x.html")
    assert "after the input" in text


def test_head_redirect_becomes_a_link() -> None:
    """A pdoc package index.html, only a <meta refresh> inside <head>, links on."""
    page = '<html><head><meta http-equiv="refresh" content="0; url=./server.html"/></head></html>'
    text = render_text(page, "python", "/dev/carol/server/index.html")
    assert "[redirects to](python/dev/carol/server/server.html)" in text


def test_links_are_paths_under_the_docs_root() -> None:
    """Relative links resolve to docs-root paths; fragments stay; external links stay."""
    page = (
        '<a href="../classes/Foo.html#bar">Foo</a>'
        '<a href="https://example.com/x">ext</a>'
        '<a href="#local">here</a>'
    )
    text = render_text(page, "typescript", "/proj/modules/m.html")
    assert "[Foo](typescript/proj/classes/Foo.html#bar)" in text
    assert "[ext](https://example.com/x)" in text
    assert "](#local)" not in text and "here" in text


def test_pre_keeps_its_layout() -> None:
    """Preformatted text (go doc output, signatures) keeps line breaks and indentation."""
    text = render_text("<pre>func A()\n    does a thing\n</pre>", "go", "/g.html")
    assert "func A()\n    does a thing" in text


@pytest.fixture
def served(tmp_path: Path) -> Iterator[str]:
    """Serves a small doc tree with the real handler, on a free loopback port.

    Args:
        tmp_path: pytest's per-test temporary directory.

    Yields:
        The server's base URL.
    """
    (tmp_path / "index.html").write_text("<h1>Python</h1><a href='pkg/a.html'>a</a>")
    (tmp_path / "pkg").mkdir()
    (tmp_path / "pkg" / "a.html").write_text("<h1>A</h1>")
    (tmp_path / "noindex").mkdir()
    (tmp_path / "noindex" / "b.html").write_text("<h1>B</h1>")
    (tmp_path / "style.css").write_text("body{}")
    server = ThreadingHTTPServer(("127.0.0.1", 0), partial(_Handler, directory=str(tmp_path)))
    threading.Thread(target=server.serve_forever, daemon=True).start()
    try:
        yield f"http://127.0.0.1:{server.server_address[1]}"
    finally:
        server.shutdown()


def _get(url: str) -> tuple[str, str]:
    """Fetches a URL as Traefik would forward it (with the stripped prefix).

    Args:
        url: The URL.

    Returns:
        (content type, body).
    """
    req = urllib.request.Request(url, headers={"X-Forwarded-Prefix": "/carol/docs/python"})
    with urllib.request.urlopen(req, timeout=5) as r:
        return r.headers.get("Content-Type", ""), r.read().decode()


def test_html_is_served_unchanged(served: str) -> None:
    """Without ?format=text the page is the generated HTML, exactly."""
    ctype, body = _get(served + "/index.html")
    assert ctype.startswith("text/html") and body.startswith("<h1>Python</h1>")


def test_text_view_of_page_and_directory(served: str) -> None:
    """?format=text works on pages and on directories with an index.html."""
    for url in (served + "/index.html?format=text", served + "/?format=text"):
        ctype, body = _get(url)
        assert ctype.startswith("text/plain")
        assert "# Python" in body and "[a](python/pkg/a.html)" in body


def test_text_listing_for_directory_without_index(served: str) -> None:
    """A directory with no index.html is listed, as links."""
    _, body = _get(served + "/noindex/?format=text")
    assert "[b.html](python/noindex/b.html)" in body


def test_no_text_view_for_non_html(served: str) -> None:
    """Assets have no text view (404), so agents can't pull css/js through it."""
    with pytest.raises(urllib.error.HTTPError) as e:
        _get(served + "/style.css?format=text")
    assert e.value.code == 404
