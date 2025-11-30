# router.py
import argparse
import time
import logging
from core.mcp_server import MCPServer
from core.logger import setup_logging

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--host", default="0.0.0.0")
    parser.add_argument("--port", type=int, default=3456)
    parser.add_argument("--debug", action="store_true", help="Enable debug mode (sets log level to DEBUG).")
    parser.add_argument("--log-level", default="INFO", help="Set the log level (e.g., DEBUG, INFO, WARNING).")
    parser.add_argument("--log-file", help="Path to a file to write logs to.")
    args = parser.parse_args()

    # Configure logging
    setup_logging(debug=args.debug, log_level=args.log_level, log_file=args.log_file)

    # MCPServer now handles registry, downstream manager, and tool loading
    server = MCPServer(host=args.host, port=args.port)
    server.start()

    try:
        while True:
            time.sleep(1)
    except KeyboardInterrupt:
        logging.info("Shutting down.")

if __name__ == "__main__":
    main()


