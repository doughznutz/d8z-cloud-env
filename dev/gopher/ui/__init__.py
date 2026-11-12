"""Gopher's dashboard, one module per tab, each exposing ``render()``.

Kept to drawing and session state only: everything a tab actually does
lives in the server package, which never imports Streamlit.
"""
