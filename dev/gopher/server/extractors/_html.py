"""Minimal static HTML pages shared by the extractors that render their own output.

pdoc and typedoc write their own HTML; go doc, the Python script reader and
the shell reader only produce text, so they wrap it with these. Every page
is self-contained (inline style, relative links only) so a whole tree stays
relocatable under whatever subpath Traefik mounts it at.
"""

import html
import os

#: Shared inline style for every generated page - no static assets to keep
#: relative paths working for, since the whole tree only ever has to stay
#: relocatable under whatever subpath Traefik mounts it at.
STYLE = (
    "body{font-family:system-ui,sans-serif;margin:2rem;color:#1a1a1a}"
    "pre{white-space:pre-wrap;font-family:ui-monospace,monospace}"
    "a{color:#0969da}"
)


def render_doc_page(title: str, doc_text: str) -> str:
    """Wraps one package's `go doc -all` output in a minimal static HTML page.

    Args:
        title: Heading for the page (the package's import path).
        doc_text: Raw stdout from `go doc -all`.

    Returns:
        A complete, self-contained HTML document.
    """
    body = html.escape(doc_text) or "(no doc output)"
    return (
        f"<!doctype html><html><head><meta charset='utf-8'>"
        f"<title>{html.escape(title)}</title><style>{STYLE}</style></head>"
        f"<body><p><a href='index.html'>&larr; index</a></p>"
        f"<h1>{html.escape(title)}</h1><pre>{body}</pre></body></html>"
    )


def render_index_page(title: str, back_href: str | None, links: list[tuple[str, str]]) -> str:
    """Renders a static index page linking to a set of generated pages.

    Args:
        title: Heading for the page.
        back_href: Relative link back to a parent index, or None at the top.
        links: (label, relative href) pairs to list, in order.

    Returns:
        A complete, self-contained HTML document.
    """
    back = f"<p><a href='{html.escape(back_href)}'>&larr; index</a></p>" if back_href else ""
    items = "".join(
        f"<li><a href='{html.escape(href)}'>{html.escape(label)}</a></li>" for label, href in links
    )
    return (
        f"<!doctype html><html><head><meta charset='utf-8'>"
        f"<title>{html.escape(title)}</title><style>{STYLE}</style></head>"
        f"<body>{back}<h1>{html.escape(title)}</h1><ul>{items}</ul></body></html>"
    )


def render_html_page(title: str, body_html: str, back_href: str = "index.html") -> str:
    """Wraps already-escaped HTML in a page with the shared style.

    Args:
        title: Heading for the page.
        body_html: The page body, already HTML-escaped where needed.
        back_href: Relative link back to an index page.

    Returns:
        A complete, self-contained HTML document.
    """
    return (
        f"<!doctype html><html><head><meta charset='utf-8'>"
        f"<title>{html.escape(title)}</title><style>{STYLE}</style></head>"
        f"<body><p><a href='{html.escape(back_href)}'>&larr; index</a></p>"
        f"<h1>{html.escape(title)}</h1>{body_html}</body></html>"
    )


def write(path: str, page: str) -> None:
    """Writes one generated page, creating its parent directories.

    Args:
        path: Where to write it.
        page: The HTML document.
    """
    os.makedirs(os.path.dirname(path), exist_ok=True)
    with open(path, "w") as f:
        f.write(page)
