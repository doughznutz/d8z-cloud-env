import argparse

if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--stdio", action="store_true")
    parser.add_argument("--debug", action="store_true")
    args = parser.parse_args()
    if args.stdio:
        if args.debug: print("DEBUG mode enabled for downstream MCP server.", file=sys.stderr)
        # The tools are now dynamically loaded by the router, this file's main_stdio is no longer used for stdio server operations.
        # The original behavior of this file as a standalone MCP server is being deprecated.
        print("This server.py is deprecated as a standalone MCP server and its tools have been moved.", file=sys.stderr)